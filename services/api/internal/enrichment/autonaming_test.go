package enrichment

import (
	"testing"

	"github.com/skadia-app/skadia/services/api/internal/model"
)

func f64(v float64) *float64 { return &v }

func TestResolveAutoTitlePointToPoint(t *testing.T) {
	in := AutoTitleInput{
		SportType: "cycling",
		SubType:   "road",
		StartCity: "Beispielstadt West",
		EndCity:   "Muenchen",
		StartLat:  f64(48.3705), StartLng: f64(10.8978),
		EndLat: f64(48.1372), EndLng: f64(11.5756),
		MinElevationM: f64(480), MaxElevationM: f64(520),
		WeatherPrefs: model.WeatherPrefs{MountainThresholdM: 500},
	}

	got := ResolveAutoTitle(in)
	if got.Title != "Beispielstadt West nach Muenchen" {
		t.Fatalf("Title = %q, want Augsburg nach Muenchen", got.Title)
	}
	if got.Strategy != AutoTitleStrategyPointToPoint {
		t.Fatalf("Strategy = %q, want %q", got.Strategy, AutoTitleStrategyPointToPoint)
	}
}

func TestResolveAutoTitleNoLoopNameWhenStartEndClose(t *testing.T) {
	in := AutoTitleInput{
		SportType: "running",
		SubType:   "trail",
		StartCity: "Tegernsee",
		EndCity:   "Tegernsee",
		StartLat:  f64(47.7080), StartLng: f64(11.7580),
		EndLat: f64(47.7090), EndLng: f64(11.7590),
		MinElevationM: f64(740), MaxElevationM: f64(900),
		WeatherPrefs: model.WeatherPrefs{MountainThresholdM: 500},
	}

	got := ResolveAutoTitle(in)
	if got.Title != "Trailrun ab Tegernsee" {
		t.Fatalf("Title = %q, want Trailrun ab Tegernsee", got.Title)
	}
	if got.Strategy != AutoTitleStrategySportPlace {
		t.Fatalf("Strategy = %q, want %q", got.Strategy, AutoTitleStrategySportPlace)
	}
}

func TestResolveAutoTitleSummitRequiresWeatherMountainGate(t *testing.T) {
	in := AutoTitleInput{
		SportType: "hiking",
		StartCity: "Marquartstein",
		StartLat:  f64(47.7580), StartLng: f64(12.4620),
		EndLat: f64(47.7590), EndLng: f64(12.4630),
		MinElevationM: f64(600), MaxElevationM: f64(900),
		WeatherPrefs: model.WeatherPrefs{MountainThresholdM: 500},
		GeoTags: []model.GeoTag{{
			Category: "summit",
			Name:     "Hochplatte",
			Lat:      f64(47.7430),
			Lng:      f64(12.3980),
			EleM:     f64(1586),
			DistM:    f64(20),
		}},
	}

	got := ResolveAutoTitle(in)
	if got.Title != "Wanderung ab Marquartstein" {
		t.Fatalf("Title = %q, want Wanderung ab Marquartstein", got.Title)
	}
	if got.Strategy != AutoTitleStrategySportPlace {
		t.Fatalf("Strategy = %q, want %q", got.Strategy, AutoTitleStrategySportPlace)
	}
}

func TestResolveAutoTitleSummitWithEndOnSummitSuppressesNachTarget(t *testing.T) {
	in := AutoTitleInput{
		SportType: "hiking",
		StartCity: "Marquartstein",
		EndCity:   "Hochplatte",
		StartLat:  f64(47.7580), StartLng: f64(12.4620), StartElevationM: f64(600),
		EndLat: f64(47.7431), EndLng: f64(12.3981), EndElevationM: f64(1570),
		MinElevationM: f64(600), MaxElevationM: f64(1586),
		WeatherPrefs: model.WeatherPrefs{MountainThresholdM: 500},
		GeoTags: []model.GeoTag{{
			Category: "summit",
			Name:     "Hochplatte",
			Lat:      f64(47.7430),
			Lng:      f64(12.3980),
			EleM:     f64(1586),
			DistM:    f64(20),
		}},
	}

	got := ResolveAutoTitle(in)
	if got.Title != "Hochplatte ab Marquartstein" {
		t.Fatalf("Title = %q, want Hochplatte ab Marquartstein", got.Title)
	}
}

func TestResolveAutoTitleSummitTraversalAddsTargetWhenTargetIsNotSummit(t *testing.T) {
	in := AutoTitleInput{
		SportType: "hiking",
		StartCity: "Marquartstein",
		EndCity:   "Schleching",
		StartLat:  f64(47.7580), StartLng: f64(12.4620), StartElevationM: f64(600),
		EndLat: f64(47.7200), EndLng: f64(12.3940), EndElevationM: f64(580),
		MinElevationM: f64(580), MaxElevationM: f64(1586),
		WeatherPrefs: model.WeatherPrefs{MountainThresholdM: 500},
		GeoTags: []model.GeoTag{{
			Category: "summit",
			Name:     "Hochplatte",
			Lat:      f64(47.7430),
			Lng:      f64(12.3980),
			EleM:     f64(1586),
			DistM:    f64(20),
		}},
	}

	got := ResolveAutoTitle(in)
	if got.Title != "Hochplatte ab Marquartstein nach Schleching" {
		t.Fatalf("Title = %q, want Hochplatte ab Marquartstein nach Schleching", got.Title)
	}
}

func TestResolveAutoTitleSummitStartOnSummit(t *testing.T) {
	in := AutoTitleInput{
		SportType: "hiking",
		StartCity: "Hochplatte",
		EndCity:   "Marquartstein",
		StartLat:  f64(47.7431), StartLng: f64(12.3981), StartElevationM: f64(1570),
		EndLat: f64(47.7580), EndLng: f64(12.4620), EndElevationM: f64(600),
		MinElevationM: f64(600), MaxElevationM: f64(1586),
		WeatherPrefs: model.WeatherPrefs{MountainThresholdM: 500},
		GeoTags: []model.GeoTag{{
			Category: "summit",
			Name:     "Hochplatte",
			Lat:      f64(47.7430),
			Lng:      f64(12.3980),
			EleM:     f64(1586),
			DistM:    f64(20),
		}},
	}

	got := ResolveAutoTitle(in)
	if got.Title != "Hochplatte nach Marquartstein" {
		t.Fatalf("Title = %q, want Hochplatte nach Marquartstein", got.Title)
	}
}
