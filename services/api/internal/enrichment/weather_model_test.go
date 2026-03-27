package enrichment

import (
	"testing"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/model"
)

func TestSelectWeatherModel(t *testing.T) {
	recent := time.Now().UTC().Add(-7 * 24 * time.Hour) // 7 days ago — within window
	old := time.Now().UTC().Add(-365 * 24 * time.Hour)  // 1 year ago — outside window

	tests := []struct {
		name            string
		lat, lng        float64
		date            time.Time
		wantModel       string
		wantEndpointKey string // "archive" or "forecast"
	}{
		{"Munich ICON D2", 48.1, 11.6, recent, "dwd_icon_d2", "forecast"},
		{"Vienna ICON D2", 48.2, 16.4, recent, "dwd_icon_d2", "forecast"},
		{"Zurich ICON D2", 47.4, 8.5, recent, "dwd_icon_d2", "forecast"},
		{"Paris AROME", 48.85, 2.35, recent, "meteofrance_arome_france_hd", "forecast"},
		{"Amsterdam ICON EU", 52.37, 4.90, recent, "dwd_icon_eu", "forecast"},
		{"Tokyo IFS", 35.68, 139.69, recent, "ecmwf_ifs_hres", "forecast"},
		{"Old Munich", 48.1, 11.6, old, "era5_land", "archive"},
		{"Old Paris", 48.85, 2.35, old, "era5_land", "archive"},
		{"Old Tokyo", 35.68, 139.69, old, "era5_land", "archive"},
		{"Very old 2015", 48.1, 11.6, time.Date(2015, 6, 1, 0, 0, 0, 0, time.UTC), "era5_land", "archive"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			endpoint, m := selectWeatherModel(tt.lat, tt.lng, tt.date)
			if m != tt.wantModel {
				t.Errorf("model: got %q, want %q", m, tt.wantModel)
			}
			if tt.wantEndpointKey == "archive" && endpoint != archiveEndpoint {
				t.Errorf("expected archive endpoint, got %q", endpoint)
			}
			if tt.wantEndpointKey == "forecast" && endpoint != forecastEndpoint {
				t.Errorf("expected forecast endpoint, got %q", endpoint)
			}
		})
	}
}

func TestDetermineWeatherCase(t *testing.T) {
	defaultPrefs := model.WeatherPrefs{ReadingMode: "auto", MountainThresholdM: 500, LongDurationS: 14400}

	tests := []struct {
		name           string
		elapsedSec     int
		minAlt, maxAlt float64
		prefs          model.WeatherPrefs
		want           WeatherCase
	}{
		{"short flat", 3600, 500, 600, defaultPrefs, CaseSingle},
		{"long flat", 18000, 500, 600, defaultPrefs, CaseLong},
		{"mountain short", 3600, 500, 1100, defaultPrefs, CaseMountain},
		{"mountain long", 18000, 900, 2300, defaultPrefs, CaseMountain},
		{"exactly 500m", 3600, 500, 1000, defaultPrefs, CaseMountain},
		{"499m delta", 3600, 500, 999, defaultPrefs, CaseSingle},
		{"exactly 4h", 14400, 500, 600, defaultPrefs, CaseLong},
		{"no elevation", 3600, 0, 0, defaultPrefs, CaseSingle},
		{"custom mountain 300m", 3600, 500, 800,
			model.WeatherPrefs{ReadingMode: "auto", MountainThresholdM: 300, LongDurationS: 7200},
			CaseMountain},
		{"custom long 2h", 7200, 500, 600,
			model.WeatherPrefs{ReadingMode: "auto", MountainThresholdM: 500, LongDurationS: 7200},
			CaseLong},
		{"always_one mountain", 3600, 500, 2000,
			model.WeatherPrefs{ReadingMode: "always_one", MountainThresholdM: 500, LongDurationS: 14400},
			CaseSingle},
		{"always_two short flat", 3600, 500, 600,
			model.WeatherPrefs{ReadingMode: "always_two", MountainThresholdM: 500, LongDurationS: 14400},
			CaseLong},
		{"always_two mountain", 3600, 500, 1100,
			model.WeatherPrefs{ReadingMode: "always_two", MountainThresholdM: 500, LongDurationS: 14400},
			CaseMountain},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DetermineWeatherCase(tt.elapsedSec, tt.minAlt, tt.maxAlt, tt.prefs)
			if got != tt.want {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}
