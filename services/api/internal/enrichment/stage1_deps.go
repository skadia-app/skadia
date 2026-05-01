package enrichment

import (
	"context"

	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/model"
)

// WeatherFetcher abstracts the Open-Meteo weather lookup performed by
// FetchWeatherReadings. It exists so RunStage1 can be unit-tested without
// hitting the network.
type WeatherFetcher interface {
	FetchReadings(ctx context.Context, reqs []WeatherRequest) ([]model.WeatherReading, error)
}

// ReverseGeocoder abstracts the PostGIS-backed reverse-geocoding lookup
// performed by ReverseGeocode. It exists so RunStage1 can be unit-tested
// without a populated osm_boundaries table.
type ReverseGeocoder interface {
	Reverse(ctx context.Context, db *database.DB, userID string, lat, lng float64) (*GeoResult, error)
}

// Stage1Deps bundles the external dependencies of RunStage1. A nil pointer or
// nil fields fall back to the production implementations (defaultWeatherFetcher
// / defaultReverseGeocoder), so existing call-sites keep their behaviour by
// passing nil. Tests construct a Stage1Deps with fakes for unit coverage.
type Stage1Deps struct {
	Weather  WeatherFetcher
	Geocoder ReverseGeocoder
}

// defaultWeatherFetcher delegates to the package-level FetchWeatherReadings
// function, preserving the production HTTP path.
type defaultWeatherFetcher struct{}

func (defaultWeatherFetcher) FetchReadings(ctx context.Context, reqs []WeatherRequest) ([]model.WeatherReading, error) {
	return FetchWeatherReadings(ctx, reqs)
}

// defaultReverseGeocoder delegates to the package-level ReverseGeocode
// function, preserving the production PostGIS + external-fallback path.
type defaultReverseGeocoder struct{}

func (defaultReverseGeocoder) Reverse(ctx context.Context, db *database.DB, userID string, lat, lng float64) (*GeoResult, error) {
	return ReverseGeocode(ctx, db, userID, lat, lng)
}

// resolveStage1Deps returns concrete WeatherFetcher / ReverseGeocoder values,
// substituting defaults for nil fields. Always returns non-nil interfaces.
func resolveStage1Deps(deps *Stage1Deps) (WeatherFetcher, ReverseGeocoder) {
	var weather WeatherFetcher = defaultWeatherFetcher{}
	var geocoder ReverseGeocoder = defaultReverseGeocoder{}
	if deps != nil {
		if deps.Weather != nil {
			weather = deps.Weather
		}
		if deps.Geocoder != nil {
			geocoder = deps.Geocoder
		}
	}
	return weather, geocoder
}
