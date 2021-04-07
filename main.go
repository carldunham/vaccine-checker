package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/hashicorp/go-multierror"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

const (
	defaultsearchURLPattern    = "https://www.vaccinespotter.org/api/v0/states/%s.json"
	defaultSearchMethod        = "GET"
	defaultSearchTimeout       = 20 * time.Second
	defaultNotificationURL     = "https://api.virtualbuttons.com/v1"
	defaultNotificationMethod  = "GET"
	defaultNotificationTimeout = 10 * time.Second
	defaultCheckInterval       = 30 * time.Second
	defaultDistanceKilometers  = 10
)

var (
	errInvalidLocation        = errors.New("missing or invalid location, should be latitude,longitude")
	errMissingNotificationURL = errors.New("missing --notification-url")
	errMissingLatitude        = errors.New("missing --latitude")
	errMissingLongitude       = errors.New("missing --longitude")
)

func main() {
	flags := pflag.NewFlagSet(os.Args[0], pflag.ContinueOnError)

	flags.String("search-url-pattern", defaultsearchURLPattern, "Sprintf pattern for URL to search for appointments")
	flags.String("search-method", defaultSearchMethod, "HTTP method to hit search-url with")
	flags.StringSlice("search-params", nil, "query params (or body params for POST) to send with search")
	flags.Duration("search-timeout", defaultSearchTimeout, "request timeout for searches")
	flags.Float64("latitude", 0, "latitude of location to check around")
	flags.Float64("longitude", 0, "longitude of location to check around")
	flags.Int32("distance", defaultDistanceKilometers, "kilometers from location to check")
	flags.Bool("include-second-dose-only", false, "If given, include sites that are only giving second doses")
	flags.String("notification-url", defaultNotificationURL, "URL to hit when appointments are found")
	flags.String("notification-method", defaultNotificationMethod, "HTTP method to hit notification-url with")
	flags.StringSlice("notification-params", nil, "query params (or body params for POST) to send with notification")
	flags.Duration("notification-timeout", defaultNotificationTimeout, "request timeout for notifications")
	flags.Duration("check-interval", defaultCheckInterval, "how often to check")
	flags.Bool("silent", false, "skip notification")

	if err := flags.Parse(os.Args[1:]); err != nil {
		if errors.Is(err, pflag.ErrHelp) {
			exitFunc(0)
		}
		fmt.Fprintln(os.Stderr, err)
		fmt.Fprintf(os.Stderr, "Usage of %s:\n", os.Args[0])
		flags.PrintDefaults()
		exitFunc(1)
	}
	viper.BindPFlags(flags)

	viper.SetEnvPrefix("VC")
	viper.AutomaticEnv()
	viper.SetEnvKeyReplacer(strings.NewReplacer("-", "_"))

	viper.SetConfigName("config")
	viper.AddConfigPath(".")
	if err := viper.ReadInConfig(); err != nil {
		if !errors.Is(err, &viper.ConfigFileNotFoundError{}) {
			fmt.Fprintln(os.Stderr, "Fatal error reading config file:", err)
			exitFunc(2)
		}
		// else ignore file not found
	}

	if err := validateParams(); err != nil {
		fmt.Fprintln(os.Stderr, "invalid params:", err)
		exitFunc(3)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, os.Kill)

	checker := NewChecker()

	quitter := func() {
		fmt.Println("\nterminating...")
		stop()
		fmt.Println("done.")
		exitFunc(0)
	}

	ticker := time.NewTicker(viper.GetDuration("check-interval"))
	defer ticker.Stop()

	for {
		if err := checker.Check(ctx); err != nil {
			fmt.Fprintf(os.Stderr, "error checking sites, moving on: %v", err)
		}

		select {
		case <-ctx.Done():
			quitter()
		case <-ticker.C:
			continue
		}
	}
}

func validateParams() error {
	var ret *multierror.Error

	if !viper.IsSet("latitude") {
		ret = multierror.Append(ret, errMissingLatitude)
	}

	if !viper.IsSet("longitude") {
		ret = multierror.Append(ret, errMissingLongitude)
	}

	if !viper.GetBool("silent") && viper.GetString("notification-url") == "" {
		ret = multierror.Append(ret, errMissingNotificationURL)
	}

	return ret.ErrorOrNil()
}

// for mocking
var (
	exitFunc = os.Exit
)
