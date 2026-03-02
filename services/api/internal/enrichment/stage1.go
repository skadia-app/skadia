package enrichment

import (
	"context"
	"log/slog"
	"math"
	"time"

	suncalc "github.com/sixdouglas/suncalc"

	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/model"
)

// RunStage1 executes synchronous enrichment for an activity after upload.
// This includes time-of-day, intensity, round-trip detection, and auto-naming.
func RunStage1(ctx context.Context, db *database.DB, activityID string, data *model.ActivityData) {
	fields := make(map[string]any)

	// Time of day (requires GPS)
	if data.StartLat != 0 && data.StartLng != 0 && !data.StartedAt.IsZero() {
		if tod := TimeOfDay(data.StartLat, data.StartLng, data.StartedAt); tod != "" {
			fields["time_of_day"] = tod
		}
	}

	// Intensity (requires heart rate)
	if data.AvgHeartRate > 0 && data.MaxHeartRate > 0 {
		if intensity := Intensity(data.AvgHeartRate, float64(data.MaxHeartRate)); intensity != "" {
			fields["intensity"] = intensity
		}
	}

	// Round trip detection (requires GPS start+end)
	if data.StartLat != 0 && data.EndLat != 0 {
		isRT := IsRoundTrip(data.StartLat, data.StartLng, data.EndLat, data.EndLng, 500)
		fields["is_round_trip"] = isRT
	}

	// Reverse geocoding (PostGIS)
	if data.StartLat != 0 && data.StartLng != 0 {
		if geo, err := ReverseGeocode(ctx, db.Pool, data.StartLat, data.StartLng); err == nil && geo != nil {
			if geo.City != "" {
				fields["start_city"] = geo.City
			}
			if geo.State != "" {
				fields["start_state"] = geo.State
			}
			if geo.Country != "" {
				fields["start_country"] = geo.Country
			}
			if geo.CountryCode != "" {
				fields["start_country_code"] = geo.CountryCode
			}
		}
	}
	if data.EndLat != 0 && data.EndLng != 0 {
		if geo, err := ReverseGeocode(ctx, db.Pool, data.EndLat, data.EndLng); err == nil && geo != nil {
			if geo.City != "" {
				fields["end_city"] = geo.City
			}
		}
	}

	// Auto title
	startCity, _ := fields["start_city"].(string)
	if title := GenerateAutoTitle(data.SportType, startCity); title != "" {
		fields["title_auto"] = title
	}

	if len(fields) == 0 {
		return
	}

	if err := db.UpdateEnrichmentFields(ctx, activityID, fields); err != nil {
		slog.Error("stage1 enrichment failed", "activity_id", activityID, "error", err)
	}
}

// TimeOfDay returns the time-of-day category based on sun position.
// Categories: vor-sonnenaufgang, goldene-stunde, tagestour, nach-sonnenuntergang, nacht
func TimeOfDay(lat, lng float64, t time.Time) string {
	times := suncalc.GetTimes(t, lat, lng)

	dawn := times[suncalc.Dawn].Value
	sunrise := times[suncalc.Sunrise].Value
	goldenHourEnd := times[suncalc.GoldenHourEnd].Value
	goldenHourStart := times[suncalc.GoldenHour].Value
	sunset := times[suncalc.Sunset].Value
	dusk := times[suncalc.Dusk].Value

	switch {
	case t.Before(dawn):
		return "nacht"
	case t.Before(sunrise):
		return "vor-sonnenaufgang"
	case t.Before(goldenHourEnd):
		return "goldene-stunde"
	case t.Before(goldenHourStart):
		return "tagestour"
	case t.Before(sunset):
		return "goldene-stunde"
	case t.Before(dusk):
		return "nach-sonnenuntergang"
	default:
		return "nacht"
	}
}

// Intensity returns intensity category based on heart rate zones.
// Returns: locker (< 60% max), moderat (60-80% max), intensiv (> 80% max)
func Intensity(avgHR, maxHR float64) string {
	if maxHR <= 0 || avgHR <= 0 {
		return ""
	}
	ratio := avgHR / maxHR
	switch {
	case ratio < 0.6:
		return "locker"
	case ratio < 0.8:
		return "moderat"
	default:
		return "intensiv"
	}
}

// IsRoundTrip checks if start and end points are within threshold meters.
func IsRoundTrip(startLat, startLng, endLat, endLng, thresholdM float64) bool {
	dist := haversine(startLat, startLng, endLat, endLng)
	return dist <= thresholdM
}

// haversine returns distance in meters between two lat/lng points.
func haversine(lat1, lng1, lat2, lng2 float64) float64 {
	const earthRadiusM = 6371000.0
	dLat := (lat2 - lat1) * math.Pi / 180.0
	dLng := (lng2 - lng1) * math.Pi / 180.0
	lat1Rad := lat1 * math.Pi / 180.0
	lat2Rad := lat2 * math.Pi / 180.0

	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(lat1Rad)*math.Cos(lat2Rad)*
			math.Sin(dLng/2)*math.Sin(dLng/2)
	c := 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
	return earthRadiusM * c
}
