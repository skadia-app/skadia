package enrichment

import (
	"time"

	"github.com/skadia-app/skadia/services/api/internal/model"
)

const (
	archiveEndpoint  = "https://archive-api.open-meteo.com/v1/archive"
	forecastEndpoint = "https://historical-forecast-api.open-meteo.com/v1/forecast"
)

// WeatherCase is the fetch strategy for an activity.
type WeatherCase int

const (
	CaseSingle   WeatherCase = iota // <4h AND Δalt<500m → 1 reading at midpoint
	CaseMountain                    // Δalt≥500m → 2 readings: start + summit
	CaseLong                        // ≥4h AND Δalt<500m → 2 readings: start + midpoint
)

// selectWeatherModel returns the best Open-Meteo endpoint and model for the given location and date.
func selectWeatherModel(lat, lng float64, date time.Time) (endpoint, modelID string) {
	iconD2Start := time.Date(2022, 11, 24, 0, 0, 0, 0, time.UTC)
	aromeStart  := time.Date(2022, 11, 13, 0, 0, 0, 0, time.UTC)
	ifsStart    := time.Date(2017, 1,  1,  0, 0, 0, 0, time.UTC)

	if !date.Before(iconD2Start) && isIconD2Domain(lat, lng) {
		return forecastEndpoint, "dwd_icon_d2"
	}
	if !date.Before(aromeStart) && isFranceDomain(lat, lng) {
		return forecastEndpoint, "meteofrance_arome_france_hd"
	}
	if !date.Before(iconD2Start) && isIconEUDomain(lat, lng) {
		return forecastEndpoint, "dwd_icon_eu"
	}
	if !date.Before(ifsStart) {
		return forecastEndpoint, "ecmwf_ifs_hres"
	}
	return archiveEndpoint, "era5_land"
}

// isIconD2Domain covers Germany, Austria, Switzerland and adjacent Alpine regions.
func isIconD2Domain(lat, lng float64) bool {
	return lat >= 43.2 && lat <= 57.4 && lng >= 5.0 && lng <= 20.0
}

// isFranceDomain covers metropolitan France.
func isFranceDomain(lat, lng float64) bool {
	return lat >= 41.0 && lat <= 51.5 && lng >= -5.5 && lng <= 10.0
}

// isIconEUDomain covers Europe broadly (ICON-EU domain).
func isIconEUDomain(lat, lng float64) bool {
	return lat >= 29.0 && lat <= 70.5 && lng >= -23.5 && lng <= 62.5
}

// DetermineWeatherCase returns the fetch strategy for an activity, respecting user prefs.
func DetermineWeatherCase(elapsedSec int, minAlt, maxAlt float64, prefs model.WeatherPrefs) WeatherCase {
	mountainThreshold := float64(prefs.MountainThresholdM)
	if mountainThreshold <= 0 {
		mountainThreshold = 500
	}
	longDurationS := float64(prefs.LongDurationS)
	if longDurationS <= 0 {
		longDurationS = 14400
	}

	switch prefs.ReadingMode {
	case "always_one":
		return CaseSingle
	case "always_two":
		if maxAlt-minAlt >= mountainThreshold {
			return CaseMountain
		}
		return CaseLong
	default: // "auto"
		if maxAlt-minAlt >= mountainThreshold {
			return CaseMountain
		}
		if float64(elapsedSec) >= longDurationS {
			return CaseLong
		}
		return CaseSingle
	}
}
