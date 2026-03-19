package enrichment

import (
	"context"
	"log/slog"
	"slices"
	"time"

	suncalc "github.com/sixdouglas/suncalc"

	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/event"
	"github.com/skadia-app/skadia/services/api/internal/model"
	"github.com/skadia-app/skadia/services/api/internal/pmc"
)

// enableDEMEnrichment controls whether Open-Meteo elevation correction is applied.
// Disabled until a local DEM solution (e.g. SRTM tiles) is available.
// The Open-Meteo elevation API rate-limits aggressively under any sustained load.
const enableDEMEnrichment = false

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
				data.TRIMP = trimp      // make available for intensity tag fallback
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

	// DEM-Höhenkorrektur via Open-Meteo (best-effort, vor Reverse-Geocoding)
	if enableDEMEnrichment && data.HasGPS && len(data.Streams) > 0 {
		// Koordinaten aus Streams extrahieren
		demLats := make([]float64, 0, len(data.Streams))
		demLngs := make([]float64, 0, len(data.Streams))
		demIndices := make([]int, 0, len(data.Streams))
		for i, s := range data.Streams {
			if s.Lat != nil && s.Lng != nil {
				demLats = append(demLats, *s.Lat)
				demLngs = append(demLngs, *s.Lng)
				demIndices = append(demIndices, i)
			}
		}
		if len(demLats) > 0 {
			if demElevs, err := FetchDEMElevations(ctx, demLats, demLngs); err == nil {
				// Altitude-Pointer-Slice für applyDEMCorrectionToStreams aufbauen
				altPtrs := make([]*float64, len(data.Streams))
				for i := range data.Streams {
					altPtrs[i] = data.Streams[i].AltitudeM
				}
				applyDEMCorrectionToStreams(altPtrs, demElevs, demIndices)
				// Korrigierte Werte zurückschreiben
				for i := range data.Streams {
					data.Streams[i].AltitudeM = altPtrs[i]
				}
				fields["elevation_source"] = "dem"
			} else {
				slog.Warn("DEM elevation fetch failed (non-fatal)", "activity_id", activityID, "err", err)
				fields["elevation_source"] = "device"
			}
		}
	}

	// Setze elevation_source default falls DEM deaktiviert oder nicht applicable.
	if _, ok := fields["elevation_source"]; !ok {
		fields["elevation_source"] = "device"
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

	// Weather enrichment — case-based, 1–2 readings stored in activity_weather.
	// Streams are not yet available here, so summit/midpoint GPS is unknown.
	// Stream-Refinement (summit/midpoint GPS) runs in service.go after stream insert.
	if data.StartLat != 0 && data.StartLng != 0 && !data.StartedAt.IsZero() {
		var minElevPtr, maxElevPtr *float64
		if data.MinElevationM != 0 {
			v := data.MinElevationM
			minElevPtr = &v
		}
		if data.MaxElevationM != 0 {
			v := data.MaxElevationM
			maxElevPtr = &v
		}
		weatherReqs := BuildWeatherRequests(
			data.StartLat, data.StartLng, data.StartedAt,
			data.ElapsedTimeS,
			minElevPtr, maxElevPtr,
			nil, nil, 0,  // summit point not yet known
			0, 0, 0,      // midpoint GPS not yet known
			nil, nil,
			model.DefaultWeatherPrefs(),
		)
		if len(weatherReqs) > 0 {
			if readings, err := FetchWeatherReadings(ctx, weatherReqs); err == nil {
				if err := db.InsertWeatherReadings(ctx, activityID, readings); err != nil {
					slog.Warn("stage1: weather insert failed (non-fatal)", "activity_id", activityID, "error", err)
				} else {
					// Sync weather tags to activity_tags junction table
					prefs, _ := db.GetWeatherPrefs(ctx, userID)
					if prefs.AutoTags {
						var allWeatherTags []string
						for _, r := range readings {
							allWeatherTags = append(allWeatherTags, r.Tags...)
						}
						if err := db.SyncWeatherTagsToActivityTags(ctx, activityID, userID, allWeatherTags); err != nil {
							slog.Warn("stage1: sync weather tags failed (non-fatal)", "activity_id", activityID, "error", err)
						}
					}
				}
			} else {
				slog.Warn("stage1: weather fetch failed (non-fatal)", "activity_id", activityID, "error", err)
			}
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
	// - sail_race / ultra sub_type     → tag "Wettkampf"
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
	// Race sub-sports (FIT SubSport) → tag "Wettkampf"
	raceSubSports := map[string]bool{
		"sail_race": true,
		"ultra":     true,
	}
	if raceSubSports[data.SubType] {
		tagBase, _ := fields["tags"].([]string)
		if tagBase == nil {
			tagBase = data.Tags
		}
		fields["tags"] = appendTagIfMissing(tagBase, "Wettkampf")
		// Ensure the "Wettkampf" tag exists in the tags table and is linked in activity_tags
		// so that analytics filters (which join activity_tags) can find this activity.
		// Stage2 has no race module, so we do the full upsert here.
		var raceTagID string
		row := db.Pool.QueryRow(ctx, `
			INSERT INTO tags (user_id, name, color, text_color, is_auto, category)
			VALUES ($1, 'Wettkampf', '#DC2626', '#FFFFFF', true, 'activity')
			ON CONFLICT (user_id, name) DO UPDATE SET name = tags.name
			RETURNING id
		`, userID)
		if err := row.Scan(&raceTagID); err == nil {
			db.LinkActivityTag(ctx, activityID, raceTagID, "auto")
		}
	}

	// E-Bike tag: if is_ebike is true, ensure "E-Bike" tag is in tags array
	// and link in activity_tags junction table (same pattern as "Wettkampf" above).
	if data.IsEbike {
		tagBase, _ := fields["tags"].([]string)
		if tagBase == nil {
			tagBase = data.Tags
		}
		fields["tags"] = appendTagIfMissing(tagBase, "E-Bike")

		// Upsert the "E-Bike" system tag and link it to this activity.
		var ebikeTagID string
		row := db.Pool.QueryRow(ctx, `
			INSERT INTO tags (user_id, name, color, text_color, is_system, is_auto, description)
			VALUES ($1, 'E-Bike', '#16A34A', '#FFFFFF', true, false, 'Automatisch bei E-Bike-Aktivitäten (Sync mit is_ebike)')
			ON CONFLICT (user_id, name) DO UPDATE SET name = tags.name
			RETURNING id
		`, userID)
		if err := row.Scan(&ebikeTagID); err == nil {
			_ = db.LinkActivityTag(ctx, activityID, ebikeTagID, "auto")
		}
	}

	if len(fields) == 0 {
		return
	}

	if err := db.UpdateEnrichmentFields(ctx, activityID, fields); err != nil {
		slog.Error("stage1 enrichment failed", "activity_id", activityID, "error", err)
		return
	}

	// Load enrichment settings once — used both for geo-enrichment gating and
	// for the Stage-2 NATS event (avoids a second DB round-trip).
	var enrichSettings *model.EnrichmentSettings
	if s, err := db.GetEnrichmentSettings(ctx, userID); err != nil {
		slog.Warn("stage1: could not load enrichment settings, running all geo modules",
			"user_id", userID, "error", err)
	} else {
		enrichSettings = s
	}

	geoSettings := allGeoEnabled()
	if enrichSettings != nil {
		geoSettings = geoSettingsFromModel(enrichSettings)
	}

	// Geo enrichment (peaks, settlements, forests, water, borders, lifts, surfaces) — Go SQL, no NATS.
	// Best-effort: failures are logged but do not block the Stage-2 event.
	if err := RunGeoEnrichment(ctx, db, activityID, geoSettings); err != nil {
		slog.Warn("stage1: geo enrichment failed (non-fatal)", "activity_id", activityID, "error", err)
	}

	streamSettings := allStreamEnabled()
	if enrichSettings != nil {
		streamSettings = streamSettingsFromModel(enrichSettings)
	}
	if err := RunStreamEnrichment(ctx, db, userID, activityID, streamSettings); err != nil {
		slog.Warn("stage1: stream enrichment failed (non-fatal)", "activity_id", activityID, "error", err)
	}

	// Emit stage-2 event (graceful degradation: log on failure, never panic)
	if len(pub) > 0 && pub[0] != nil {
		var enabledModules []string
		if enrichSettings != nil {
			enabledModules = enrichSettings.EnabledModules()
		}

		// These modules have been migrated to Go SQL (geo_enrich.go / surfaces.go) and no
		// longer need to run in the Python ML service.
		migratedToGo := map[string]bool{
			"peaks": true, "settlements": true, "forests": true,
			"water": true, "borders": true, "surfaces": true,
			"stops": true, "ascents": true, "intervals": true,
		}
		if enabledModules == nil {
			// nil means "all enabled" — expand to full list minus migrated ones
			allModules := []string{
				"lifts", "commute", "embeddings",
			}
			enabledModules = allModules
		} else {
			filtered := enabledModules[:0]
			for _, m := range enabledModules {
				if !migratedToGo[m] {
					filtered = append(filtered, m)
				}
			}
			enabledModules = filtered
		}

		if err := pub[0].PublishStage2Ready(event.Stage2ReadyEvent{
			ActivityID:     activityID,
			UserID:         userID,
			EnabledModules: enabledModules,
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
	nightEnd := times[suncalc.NightEnd].Value // astro dawn
	nauticalDawn := times[suncalc.NauticalDawn].Value
	dawn := times[suncalc.Dawn].Value // civil dawn
	sunrise := times[suncalc.Sunrise].Value
	goldenHourEnd := times[suncalc.GoldenHourEnd].Value
	goldenHourStart := times[suncalc.GoldenHour].Value
	sunset := times[suncalc.Sunset].Value
	dusk := times[suncalc.Dusk].Value // civil dusk
	nauticalDusk := times[suncalc.NauticalDusk].Value
	night := times[suncalc.Night].Value // astro night

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
	for d := range streams {
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
	if slices.Contains(tags, tag) {
		return tags
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
