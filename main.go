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
	"github.com/paulmach/orb"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

const (
	metersPerKilometer = 1000.0

	defaultsearchURLPattern   = "https://www.vaccinespotter.org/api/v0/states/%s.json"
	defaultSearchMethod       = "GET"
	defaultNotificationURL    = "https://api.virtualbuttons.com/v1"
	defaultNotificationMethod = "GET"
	defaultCheckInterval      = 30 * time.Second
	defaultTickInterval       = 5 * time.Minute
	defaultDistanceKilometers = 10
)

var (
	errInvalidLocation        = errors.New("missing or invalid location, should be latitude,longitude")
	errMissingNotificationURL = errors.New("missing --notification-url")
	errMissingLatitude        = errors.New("missing --latitude")
	errMissingLongitude       = errors.New("missing --longitude")
)

func main() {
	pflag.String("search-url-pattern", defaultsearchURLPattern, "Sprintf pattern for URL to search for appointments")
	pflag.String("search-method", defaultSearchMethod, "HTTP method to hit search-url with")
	pflag.StringSlice("search-params", nil, "query params (or body params for POST) to send with search")
	pflag.Float64("latitude", 0, "latitude of location to check around")
	pflag.Float64("longitude", 0, "longitude of location to check around")
	pflag.Int32("distance", defaultDistanceKilometers, "kilometers from location to check")
	pflag.Bool("include-second-dose-only", false, "If given, include sites that are only giving second doses")
	pflag.String("notification-url", defaultNotificationURL, "URL to hit when appointments are found")
	pflag.String("notification-method", defaultNotificationMethod, "HTTP method to hit notification-url with")
	pflag.StringSlice("notification-params", nil, "query params (or body params for POST) to send with notification")
	pflag.Duration("check-interval", defaultCheckInterval, "how often to check")
	pflag.Duration("tick-interval", defaultTickInterval, "how often to just give an alive message")
	pflag.Bool("silent", false, "skip notification")

	pflag.Parse()
	viper.BindPFlags(pflag.CommandLine)

	viper.SetEnvPrefix("VC")
	viper.AutomaticEnv()
	viper.SetEnvKeyReplacer(strings.NewReplacer("-", "_"))

	viper.SetConfigName("config")
	viper.AddConfigPath(".")
	if err := viper.ReadInConfig(); err != nil {
		if !errors.As(err, &viper.ConfigFileNotFoundError{}) {
			panic(fmt.Errorf("Fatal error config file: %s \n", err))
		}
		// else ignore file not found
	}

	if err := validateParams(); err != nil {
		panic(fmt.Sprintf("invalid params: %v", err))
	}
	location := orb.Point{viper.GetFloat64("longitude"), viper.GetFloat64("latitude")}
	distance := viper.GetFloat64("distance") * metersPerKilometer

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, os.Kill)

	if err := check(ctx, location, distance); err != nil {
		fmt.Fprintf(os.Stderr, "error checking sites, moving on: %v\n", err)
	}

	for {
		select {
		case <-ctx.Done():
			fmt.Println("terminating...")
			stop()
			fmt.Println("done.")
			exitFunc(0)
		case <-time.After(viper.GetDuration("check-interval")):
			if err := check(ctx, location, distance); err != nil {
				fmt.Fprintf(os.Stderr, "error checking sites, moving on: %v", err)
			}
		case <-time.After(viper.GetDuration("tick-interval")):
			fmt.Println("tick")
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

	if viper.GetString("notification-url") == "" {
		ret = multierror.Append(ret, errMissingNotificationURL)
	}

	return ret.ErrorOrNil()
}

// for mocking
var (
	exitFunc = os.Exit
)
