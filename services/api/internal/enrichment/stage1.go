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

	// Time of day (requires GPS + start/end time)
	if data.StartLat != 0 && data.StartLng != 0 && !data.StartedAt.IsZero() && !data.EndedAt.IsZero() {
		if periods := TimeOfDayPeriods(data.StartLat, data.StartLng, data.StartedAt, data.EndedAt); len(periods) > 0 {
			fields["time_of_day"] = periods
		}
	}

	// Auto-tag "Nacht" when activity reaches astronomical night
	if todPeriods, ok := fields["time_of_day"].([]string); ok {
		for _, p := range todPeriods {
			if p == "nacht" {
				fields["tags"] = appendTagIfMissing(data.Tags, "Nacht")
				break
			}
		}
	}

	// Sunrise/Sunset (requires GPS + start/end time)
	if data.StartLat != 0 && !data.StartedAt.IsZero() && !data.EndedAt.IsZero() {
		sun := CalculateSunTimes(
			data.StartLat, data.StartLng, data.EndLat, data.EndLng,
			data.StartedAt, data.EndedAt, data.Streams,
		)
		if sun.SunriseAt != nil {
			fields["sunrise_at"] = *sun.SunriseAt
		}
		if sun.SunsetAt != nil {
			fields["sunset_at"] = *sun.SunsetAt
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

// dayPeriod defines a named time-of-day period with its boundaries.
type dayPeriod struct {
	name  string
	start time.Time // inclusive
	end   time.Time // exclusive
}

// TimeOfDayPeriods returns all time-of-day periods an activity passes through,
// based on astronomical sun position at the given coordinates.
//
// Periods (chronological order for a day):
//   - nacht:               astronomical night (sun below -18°)
//   - astronomische-daemmerung: astronomical twilight (-18° to -12°)
//   - nautische-daemmerung:     nautical twilight (-12° to -6°)
//   - buergerliche-daemmerung:  civil twilight (-6° to sunrise/sunset)
//   - goldene-stunde:           golden hour (sun 0° to 6°)
//   - tag:                      full daylight (sun above 6°)
//
// Morning and evening periods of the same type are merged (e.g. only one
// "goldene-stunde" even if both morning and evening golden hour are touched).
func TimeOfDayPeriods(lat, lng float64, start, end time.Time) []string {
	times := suncalc.GetTimes(start, lat, lng)

	// Build ordered periods for the day. Some times can be zero (polar regions);
	// we skip those and let the surrounding periods absorb the gap.
	nightEnd := times[suncalc.NightEnd].Value       // astro dawn
	nauticalDawn := times[suncalc.NauticalDawn].Value
	dawn := times[suncalc.Dawn].Value                // civil dawn
	sunrise := times[suncalc.Sunrise].Value
	goldenHourEnd := times[suncalc.GoldenHourEnd].Value
	goldenHourStart := times[suncalc.GoldenHour].Value
	sunset := times[suncalc.Sunset].Value
	dusk := times[suncalc.Dusk].Value                // civil dusk
	nauticalDusk := times[suncalc.NauticalDusk].Value
	night := times[suncalc.Night].Value              // astro night

	// sentinel far-future time for the last period's end
	farFuture := start.Add(48 * time.Hour)

	// Build the period list; skip periods where boundaries are zero (polar)
	var periods []dayPeriod

	addPeriod := func(name string, s, e time.Time) {
		if s.IsZero() || e.IsZero() {
			return
		}
		if !e.After(s) {
			return
		}
		periods = append(periods, dayPeriod{name, s, e})
	}

	// Morning half
	morningNightEnd := nightEnd
	if morningNightEnd.IsZero() {
		morningNightEnd = start.Truncate(24 * time.Hour) // midnight
	}
	addPeriod("nacht", start.Add(-24*time.Hour), morningNightEnd)
	addPeriod("astronomische-daemmerung", nightEnd, nauticalDawn)
	addPeriod("nautische-daemmerung", nauticalDawn, dawn)
	addPeriod("buergerliche-daemmerung", dawn, sunrise)
	addPeriod("goldene-stunde", sunrise, goldenHourEnd)

	// Daytime
	addPeriod("tag", goldenHourEnd, goldenHourStart)

	// Evening half
	addPeriod("goldene-stunde", goldenHourStart, sunset)
	addPeriod("buergerliche-daemmerung", sunset, dusk)
	addPeriod("nautische-daemmerung", dusk, nauticalDusk)
	addPeriod("astronomische-daemmerung", nauticalDusk, night)
	addPeriod("nacht", night, farFuture)

	// Find which periods the activity [start, end] overlaps
	seen := make(map[string]bool)
	var result []string
	for _, p := range periods {
		// overlap: activity [start, end] ∩ period [p.start, p.end)
		if start.Before(p.end) && end.After(p.start) {
			if !seen[p.name] {
				seen[p.name] = true
				result = append(result, p.name)
			}
		}
	}

	return result
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
		return "" // needs better criteria: power zones, TSS, training type — see roadmap
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

// SunResult holds the calculated sunrise and sunset times for an activity.
type SunResult struct {
	SunriseAt *time.Time
	SunsetAt  *time.Time
}

// CalculateSunTimes computes sunrise/sunset times for an activity, using
// the GPS coordinates at the time of each event for accuracy on long tracks.
//
// Algorithm:
//  1. Compute preliminary times at the start point
//  2. If sunrise falls WITHIN [startedAt, endedAt]: find the stream point
//     closest to that time and recompute with its coordinates
//  3. If sunset falls WITHIN [startedAt, endedAt]: same refinement
//  4. If sunrise is BEFORE the activity: use start point coordinates
//  5. If sunset is AFTER the activity: use end point coordinates
//  6. If an event is completely irrelevant (e.g. next day): return nil
func CalculateSunTimes(
	startLat, startLng, endLat, endLng float64,
	startedAt, endedAt time.Time,
	streams []model.StreamPoint,
) SunResult {
	var result SunResult

	// Preliminary calculation at start point
	times := suncalc.GetTimes(startedAt, startLat, startLng)
	prelimSunrise := times[suncalc.Sunrise].Value
	prelimSunset := times[suncalc.Sunset].Value

	// Handle sunrise
	if !prelimSunrise.IsZero() {
		if prelimSunrise.After(startedAt) && prelimSunrise.Before(endedAt) {
			// Sunrise during activity — refine with track coordinates
			lat, lng := findCoordsAtTime(prelimSunrise, startLat, startLng, streams)
			refined := suncalc.GetTimes(startedAt, lat, lng)
			t := refined[suncalc.Sunrise].Value
			if !t.IsZero() {
				result.SunriseAt = &t
			}
		} else if !prelimSunrise.After(startedAt) {
			// Sunrise before activity — use start point value
			result.SunriseAt = &prelimSunrise
		}
		// else: sunrise after activity end → irrelevant (nil)
	}

	// Handle sunset
	if !prelimSunset.IsZero() {
		if prelimSunset.After(startedAt) && prelimSunset.Before(endedAt) {
			// Sunset during activity — refine with track coordinates
			lat, lng := findCoordsAtTime(prelimSunset, startLat, startLng, streams)
			refined := suncalc.GetTimes(startedAt, lat, lng)
			t := refined[suncalc.Sunset].Value
			if !t.IsZero() {
				result.SunsetAt = &t
			}
		} else if !prelimSunset.Before(endedAt) {
			// Sunset after activity — use end point (or start if no end GPS)
			if endLat != 0 {
				t := suncalc.GetTimes(startedAt, endLat, endLng)[suncalc.Sunset].Value
				if !t.IsZero() {
					result.SunsetAt = &t
				}
			} else {
				result.SunsetAt = &prelimSunset
			}
		}
		// else: sunset before activity start → irrelevant (nil)
	}

	return result
}

// findCoordsAtTime returns the GPS coordinates of the stream point closest to
// the given time. Uses binary search since streams are chronologically sorted.
// Falls back to the provided default coordinates if no streams have GPS data.
func findCoordsAtTime(t time.Time, defaultLat, defaultLng float64, streams []model.StreamPoint) (float64, float64) {
	if len(streams) == 0 {
		return defaultLat, defaultLng
	}

	target := t.UnixMilli()

	// Binary search for the closest timestamp
	lo, hi := 0, len(streams)-1
	for lo < hi {
		mid := (lo + hi) / 2
		if streams[mid].Time.UnixMilli() < target {
			lo = mid + 1
		} else {
			hi = mid
		}
	}

	// Check lo and lo-1 to find the closest
	bestIdx := lo
	if lo > 0 {
		diffLo := abs64(streams[lo].Time.UnixMilli() - target)
		diffPrev := abs64(streams[lo-1].Time.UnixMilli() - target)
		if diffPrev < diffLo {
			bestIdx = lo - 1
		}
	}

	// Walk outward from bestIdx to find a point with GPS data
	for d := 0; d < len(streams); d++ {
		for _, idx := range []int{bestIdx + d, bestIdx - d} {
			if idx < 0 || idx >= len(streams) {
				continue
			}
			if streams[idx].Lat != nil && streams[idx].Lng != nil {
				return *streams[idx].Lat, *streams[idx].Lng
			}
		}
	}

	return defaultLat, defaultLng
}

// appendTagIfMissing appends a tag to the slice if it's not already present.
func appendTagIfMissing(tags []string, tag string) []string {
	for _, t := range tags {
		if t == tag {
			return tags
		}
	}
	return append(tags, tag)
}

func abs64(v int64) int64 {
	if v < 0 {
		return -v
	}
	return v
}
