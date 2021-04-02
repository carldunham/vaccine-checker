package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"strings"
	"time"

	"github.com/paulmach/orb"
	"github.com/paulmach/orb/geo"
	"github.com/paulmach/orb/geojson"
	"github.com/spf13/viper"
)

var (
	errInvalidStatusReturned = errors.New("unexpected status returned")
)

func check(ctx context.Context, location orb.Point, distance float64) error {
	req, err := http.NewRequest(viper.GetString("search-method"), searchURL(), body())
	if err != nil {
		return fmt.Errorf("error creating request: %w", err)
	}
	fmt.Printf("\n*** Checking at %s ***\n\n", time.Now().Format(time.RFC1123))

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("error fetching appointments: %v", err)
	}
	defer resp.Body.Close()

	var fc geojson.FeatureCollection

	if err := json.NewDecoder(resp.Body).Decode(&fc); err != nil {
		return err
	}
	return handle(ctx, location, distance, &fc)
}

func handle(ctx context.Context, location orb.Point, distance float64, fc *geojson.FeatureCollection) error {
	var (
		available uint64
		found     []*geojson.Feature
	)

	for _, f := range fc.Features {
		if !f.Properties.MustBool("appointments_available", false) {
			continue
		}

		if !viper.GetBool("include-second-dose-only") && f.Properties.MustBool("appointments_available_2nd_dose_only", false) {
			continue
		}
		available++

		if geo.Distance(f.Geometry.(orb.Point), location) <= distance {
			printFeature(f, location)
			found = append(found, f)
		}
	}
	fmt.Printf("found %d nearby, out of %d available from %d locations.\n", len(found), available, len(fc.Features))

	if len(found) > 0 {
		notify(found)
	}

	return nil
}

func printFeature(f *geojson.Feature, location orb.Point) {
	fmt.Printf(
		"%s - %s, %s, %s - %.2f km\n",
		f.Properties.MustString("provider_brand_name", "(unknown name)"),
		f.Properties.MustString("address", "(unknown address)"),
		f.Properties.MustString("city", "(unknown city)"),
		f.Properties.MustString("state", "(unknown state)"),
		geo.Distance(f.Geometry.(orb.Point), location)/1000.0,
	)
	if prop, ok := f.Properties["appointments"]; ok {
		if appts, ok := prop.([]interface{}); ok {
			for _, appt := range appts {
				if fields, ok := appt.(map[string]interface{}); ok {
					fmt.Printf(
						"  %v: %v\n",
						mapString(fields, "time", "(unknown time)"),
						mapString(fields, "type", "(unknown type)"),
					)
					// for k, v := range fields {
					// 	fmt.Printf("  %s: %v\n", k, v)
					// }
				}
			}
		}
	}
	fmt.Println()
}

func mapString(m map[string]interface{}, key string, fallback interface{}) interface{} {
	if value, ok := m[key]; ok {
		return value
	}
	return fallback
}

func notify(found []*geojson.Feature) error {
	if viper.GetBool("silent") {
		return nil
	}
	req, err := http.NewRequest(viper.GetString("notification-method"), notificationURL(), body())
	if err != nil {
		return fmt.Errorf("error creating request: %w", err)
	}
	fmt.Printf("notifying at %s\n", time.Now().Format(time.RFC1123))

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("error notifying: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%w: %s", errInvalidStatusReturned, resp.Status)
	}

	if b, err := ioutil.ReadAll(resp.Body); err == nil {
		fmt.Println(string(b))
	}
	return nil
}

func searchURL() string {
	var params []interface{}

	for _, s := range viper.GetStringSlice("search-params") {
		params = append(params, s)
	}
	return fmt.Sprintf(viper.GetString("search-url-pattern"), params...)
}

func notificationURL() string {
	ret := viper.GetString("notification-url")

	if params := viper.GetStringSlice("notification-params"); len(params) > 0 {
		ret += "?" + strings.Join(params, "&")
	}
	return ret
}

func body() io.Reader {
	// TODO: construct body from viper.GetString("notification-params")
	return nil
}
