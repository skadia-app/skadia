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

// forecastModelWindow is the empirical retention window for all models on
// historical-forecast-api.open-meteo.com. All four models (ICON D2, ICON EU,
// AROME France HD, IFS HRES) return HTTP 500 for dates older than this,
// regardless of their documented start dates. Empirically measured 2026-03-18:
// cutoff is 2025-11-11 = 128 days. 120 days used as conservative safety margin.
const forecastModelWindow = 120 * 24 * time.Hour

// selectWeatherModel returns the best Open-Meteo endpoint and model for the
// given location and date, prioritising highest spatial resolution.
//
// Within the ~120-day rolling window of historical-forecast-api:
//
//	ICON D2 (~2 km, D/A/CH) > AROME France HD (~1.5 km, France) >
//	ICON EU (~7 km, Europe) > IFS HRES (~9 km, global)
//
// Beyond that window all forecast models return HTTP 500. ERA5-Land
// (archive-api, ~11 km) is the only reliable option and covers data back to 1940.
func selectWeatherModel(lat, lng float64, date time.Time) (endpoint, modelID string) {
	if time.Since(date) > forecastModelWindow {
		return archiveEndpoint, "era5_land"
	}
	// Within the rolling window, use the highest-resolution model for the region.
	// ICON D2 is checked before AROME because it is calibrated for D/A/CH
	// (including Swiss and Austrian Alps) and covers the overlap zone lng 5–10.
	if isIconD2Domain(lat, lng) {
		return forecastEndpoint, "dwd_icon_d2"
	}
	if isFranceDomain(lat, lng) {
		return forecastEndpoint, "meteofrance_arome_france_hd"
	}
	if isIconEUDomain(lat, lng) {
		return forecastEndpoint, "dwd_icon_eu"
	}
	return forecastEndpoint, "ecmwf_ifs_hres"
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
