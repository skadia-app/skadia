package enrichment

import (
	"context"
	"log/slog"
	"math"
	"time"

	suncalc "github.com/sixdouglas/suncalc"

	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/event"
	"github.com/skadia-app/skadia/services/api/internal/model"
	"github.com/skadia-app/skadia/services/api/internal/pmc"
)

// RunStage1 executes synchronous enrichment for an activity after upload.
// This includes time-of-day, intensity, round-trip detection, auto-naming, and autotags.
// If a NATS publisher is provided, a stage-2 event is emitted after successful completion.
func RunStage1(ctx context.Context, db *database.DB, userID, activityID string, data *model.ActivityData, pub ...*event.Publisher) {
	fields := make(map[string]any)

	// Load user profile for HR-based calculations
	profile, profileErr := db.GetUserProfile(ctx, userID)
	if profileErr != nil {
		slog.Warn("stage1: could not load user profile", "user_id", userID, "error", profileErr)
		profile = &model.UserProfile{}
	}

	// Time of day (requires GPS + start/end time)
	if data.StartLat != 0 && data.StartLng != 0 && !data.StartedAt.IsZero() && !data.EndedAt.IsZero() {
		if periods := TimeOfDayPeriods(data.StartLat, data.StartLng, data.StartedAt, data.EndedAt); len(periods) > 0 {
			fields["time_of_day"] = periods
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

	// Round trip detection (requires GPS start+end)
	if data.StartLat != 0 && data.EndLat != 0 {
		isRT := IsRoundTrip(data.StartLat, data.StartLng, data.EndLat, data.EndLng, 500)
		fields["is_round_trip"] = isRT
	}

	// TRIMP calculation (requires HR stream)
	if len(data.Streams) > 0 {
		var hrStream []int
		for _, s := range data.Streams {
			if s.HeartRate != nil && *s.HeartRate > 0 {
				hrStream = append(hrStream, int(*s.HeartRate))
			}
		}
		if len(hrStream) > 30 {
			maxHR := profile.EffectiveMaxHR()
			restingHR := profile.EffectiveRestingHR()
			trimp := pmc.CalculateTRIMP(hrStream, restingHR, maxHR)
			if trimp > 0 {
				fields["trimp"] = trimp // activities.trimp (migration 000003)
				data.TRIMP = trimp   // make available for intensity tag fallback
			}
		}
	}

	// Variability Index
	if data.NormalizedPowerW > 0 && data.AvgPowerW > 0 {
		vi := data.NormalizedPowerW / data.AvgPowerW
		fields["variability_index"] = vi
	}

	// Efficiency Factor
	if data.AvgPowerW > 0 && data.AvgHeartRate > 0 {
		ef := data.AvgPowerW / data.AvgHeartRate
		fields["efficiency_factor"] = ef
	}

	// Aerobic Decoupling: compare EF of first vs second half of streams
	if data.AvgPowerW > 0 && data.AvgHeartRate > 0 && len(data.Streams) > 60 {
		half := len(data.Streams) / 2
		var p1Sum, hr1Sum, p2Sum, hr2Sum float64
		var p1Count, hr1Count, p2Count, hr2Count int
		for i, s := range data.Streams {
			if s.PowerW != nil && *s.PowerW > 0 {
				if i < half {
					p1Sum += float64(*s.PowerW)
					p1Count++
				} else {
					p2Sum += float64(*s.PowerW)
					p2Count++
				}
			}
			if s.HeartRate != nil && *s.HeartRate > 0 {
				if i < half {
					hr1Sum += float64(*s.HeartRate)
					hr1Count++
				} else {
					hr2Sum += float64(*s.HeartRate)
					hr2Count++
				}
			}
		}
		if p1Count > 0 && hr1Count > 0 && p2Count > 0 && hr2Count > 0 {
			ef1 := (p1Sum / float64(p1Count)) / (hr1Sum / float64(hr1Count))
			ef2 := (p2Sum / float64(p2Count)) / (hr2Sum / float64(hr2Count))
			if ef1 > 0 {
				decoupling := (ef1 - ef2) / ef1 * 100
				fields["aerobic_decoupling"] = decoupling
			}
		}
	}

	// Intensity tag
	var intensityTag string
	if data.NormalizedPowerW > 0 && profile.FtpW != nil && *profile.FtpW > 0 {
		iff := data.NormalizedPowerW / float64(*profile.FtpW)
		switch {
		case iff < 0.65:
			intensityTag = "locker"
		case iff < 0.80:
			intensityTag = "aerob"
		case iff < 0.95:
			intensityTag = "schwellenbereich"
		default:
			intensityTag = "hochintensiv"
		}
		// intensity_factor (numeric IF) is derivable from NP/FTP in the frontend — not stored separately
	} else if data.TSS > 0 && data.MovingTimeS > 0 {
		tssPerHour := data.TSS / (float64(data.MovingTimeS) / 3600.0)
		switch {
		case tssPerHour < 60:
			intensityTag = "locker"
		case tssPerHour < 100:
			intensityTag = "moderat"
		default:
			intensityTag = "hochintensiv"
		}
	} else if data.TRIMP > 0 && data.MovingTimeS > 0 {
		trimpPerHour := data.TRIMP / (float64(data.MovingTimeS) / 3600.0)
		switch {
		case trimpPerHour < 60:
			intensityTag = "locker"
		case trimpPerHour < 100:
			intensityTag = "moderat"
		default:
			intensityTag = "hochintensiv"
		}
	}
	if intensityTag != "" {
		fields["intensity"] = intensityTag
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

	// Weather (historical, from Open-Meteo)
	if data.StartLat != 0 && !data.StartedAt.IsZero() {
		if weather, err := FetchWeather(data.StartLat, data.StartLng, data.StartedAt); err == nil {
			fields["weather_temp_c"] = weather.TempC
			fields["weather_condition"] = weather.Condition
			fields["weather_humidity"] = weather.Humidity
			fields["weather_wind_speed_ms"] = weather.WindSpeedMS
			if len(weather.Tags) > 0 {
				fields["weather_tags"] = weather.Tags
			}
		} else {
			slog.Warn("stage1: weather fetch failed (non-fatal)", "activity_id", activityID, "error", err)
		}
	}

	// Auto title
	startCity, _ := fields["start_city"].(string)
	if title := GenerateAutoTitle(data.SportType, startCity); title != "" {
		fields["title_auto"] = title
	}

	// Sub-type to automatic tag conversion:
	// - cycling + commuting sub_type  → tag "Pendeln", clear sub_type
	// - strength_training/hiit + amrap → tag "AMRAP"
	// - strength_training/hiit + emom  → tag "EMOM"
	if data.SubType == "commuting" && data.SportType == "cycling" {
		tagBase := data.Tags
		tagBase = appendTagIfMissing(tagBase, "Pendeln")
		fields["tags"] = tagBase
		fields["sub_type"] = ""
		data.SubType = "" // prevent downstream use
	}
	if (data.SportType == "strength_training" || data.SportType == "hiit") && data.SubType == "amrap" {
		tagBase, _ := fields["tags"].([]string)
		if tagBase == nil {
			tagBase = data.Tags
		}
		fields["tags"] = appendTagIfMissing(tagBase, "AMRAP")
	}
	if (data.SportType == "strength_training" || data.SportType == "hiit") && data.SubType == "emom" {
		tagBase, _ := fields["tags"].([]string)
		if tagBase == nil {
			tagBase = data.Tags
		}
		fields["tags"] = appendTagIfMissing(tagBase, "EMOM")
	}

	// Autotag evaluation: collect enrichment context and evaluate all autotag rules
	enrichCtx := &EnrichmentContext{
		StartCity:    startCity,
		StartCountry: "",
		IsRoundTrip:  false,
	}
	if sc, ok := fields["start_country"].(string); ok {
		enrichCtx.StartCountry = sc
	}
	if tod, ok := fields["time_of_day"].([]string); ok {
		enrichCtx.TimeOfDay = tod
	}
	if rt, ok := fields["is_round_trip"].(bool); ok {
		enrichCtx.IsRoundTrip = rt
	}

	if autoTags := EvaluateAutoTags(ctx, db, userID, data, enrichCtx); len(autoTags) > 0 {
		// Start from already-accumulated tags (e.g. from sub_type conversions above),
		// falling back to the original activity tags.
		base, _ := fields["tags"].([]string)
		if base == nil {
			base = data.Tags
		}
		for _, tag := range autoTags {
			base = appendTagIfMissing(base, tag)
		}
		fields["tags"] = base
	}

	if len(fields) == 0 {
		return
	}

	if err := db.UpdateEnrichmentFields(ctx, activityID, fields); err != nil {
		slog.Error("stage1 enrichment failed", "activity_id", activityID, "error", err)
		return
	}

	// Emit stage-2 event (graceful degradation: log on failure, never panic)
	if len(pub) > 0 && pub[0] != nil {
		if err := pub[0].PublishStage2Ready(event.Stage2ReadyEvent{
			ActivityID: activityID,
			UserID:     userID,
		}); err != nil {
			slog.Warn("stage1: failed to publish stage2 event", "activity_id", activityID, "error", err)
		}
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

// hasFrostTemperature returns true if any stream point recorded a temperature
// below 0°C, or if the average temperature is negative.
func hasFrostTemperature(streams []model.StreamPoint, avgTempC float64) bool {
	for i := range streams {
		if streams[i].TemperatureC != nil && *streams[i].TemperatureC < 0 {
			return true
		}
	}
	// Fallback: average temperature negative means frost conditions existed
	return avgTempC < 0
}

func abs64(v int64) int64 {
	if v < 0 {
		return -v
	}
	return v
}
