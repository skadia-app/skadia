package enrichment

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"slices"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/database"
)

const (
	stopFallbackSpeedMS   = 0.3     // m/s for speed-based fallback (stricter than old 0.5)
	stopFallbackMinDurS   = 180     // 3 min for unnamed stops (was 60s)
	stopFallbackConfinM   = 20.0    // max spatial spread for fallback stops
	stopGastronomyMinDurS = 600     // 10 min for gastronomy geo-tag
	stopPOISearchRadiusM  = 100.0   // outer SQL search radius (covers all categories)
	degreesToMetersLat    = 111320.0 // meters per degree latitude
)

// poiStopConfig defines per-category thresholds for POI-first stop detection.
type poiStopConfig struct {
	SearchRadiusM float64 // GPS points within this radius are "near" the POI
	ConfinementM  float64 // max spatial spread of points to confirm a stop
	GapToleranceS float64 // merge temporal windows if gap < this
	MinDwellS     float64 // minimum dwell time to confirm a stop
	SpeedRejectMS float64 // median speed above this -> fast-reject (passing through)
}

// poiThresholds maps OSM POI categories to their stop detection thresholds.
var poiThresholds = map[string]poiStopConfig{
	"gastronomy":    {40, 25, 60, 300, 3.0},
	"accommodation": {50, 30, 120, 600, 2.0},
	"summit":        {80, 40, 30, 60, 5.0},
	"pass":          {80, 40, 30, 60, 5.0},
	"viewpoint":     {50, 30, 30, 30, 4.0},
	"shelter":       {30, 20, 60, 120, 2.0},
	"transport":     {30, 20, 30, 60, 2.0},
	"water_supply":  {20, 15, 15, 30, 3.0},
	"camping":       {50, 30, 300, 1800, 1.5},
	"landmark":      {50, 30, 30, 30, 4.0},
	"shop":          {30, 20, 60, 120, 2.0},
}

// poiCandidate is a POI found near the activity track.
type poiCandidate struct {
	OsmID    int64
	Name     string
	Category string
	Subtype  string
	Lat, Lng float64
	Ele      *float64
	Config   poiStopConfig
}

// confirmedStop is a stop confirmed by the spatial confinement algorithm.
type confirmedStop struct {
	POI       *poiCandidate // nil for speed-fallback stops
	StartedAt time.Time
	EndedAt   time.Time
	DurationS int
	CenterLat float64
	CenterLng float64
}

// temporalWindow represents a contiguous time window of GPS points near a POI.
type temporalWindow struct {
	startIdx int
	endIdx   int
}

// streamPoint holds a single row from activity_streams.
type streamPoint struct {
	T       time.Time
	Lat     float64
	Lng     float64
	SpeedMS float64
}

// equirectDist computes approximate distance in meters between two lat/lng points.
// Uses equirectangular projection — accurate within 0.5% for distances < 1km
// and ~50ns per call (vs. ~200ns for haversine). Suitable for POI radius checks.
func equirectDist(lat1, lng1, lat2, lng2 float64) float64 {
	dLat := (lat2 - lat1) * degreesToMetersLat
	dLng := (lng2 - lng1) * math.Cos(lat1*math.Pi/180) * degreesToMetersLat
	return math.Sqrt(dLat*dLat + dLng*dLng)
}

// fetchPOICandidates queries all stop-relevant POIs within 100m of the track.
// Uses the simplified track + partial GIST indexes per category for performance.
func fetchPOICandidates(ctx context.Context, db *database.DB, activityID string) ([]poiCandidate, error) {
	rows, err := db.Pool.Query(ctx, `
		WITH route AS (
			SELECT ST_Simplify(track_full, 0.0001) AS geom
			FROM activity_tracks WHERE activity_id = $1
		)
		SELECT p.osm_id, p.name, p.category, p.subtype, p.ele,
		       ST_Y(ST_Centroid(p.geom)) AS lat,
		       ST_X(ST_Centroid(p.geom)) AS lng
		FROM osm_pois p, route
		WHERE ST_DWithin(route.geom::geography, p.geom::geography, $2)
		  AND p.category IN (
		      'gastronomy', 'accommodation', 'shelter', 'summit', 'pass',
		      'viewpoint', 'transport', 'water_supply', 'camping',
		      'landmark', 'shop'
		  )
		ORDER BY p.category, p.name
	`, activityID, stopPOISearchRadiusM)
	if err != nil {
		return nil, fmt.Errorf("stops: poi query: %w", err)
	}
	defer rows.Close()

	var pois []poiCandidate
	for rows.Next() {
		var p poiCandidate
		if err := rows.Scan(&p.OsmID, &p.Name, &p.Category, &p.Subtype,
			&p.Ele, &p.Lat, &p.Lng); err != nil {
			return nil, fmt.Errorf("stops: poi scan: %w", err)
		}
		cfg, ok := poiThresholds[p.Category]
		if !ok {
			continue // unknown category, skip
		}
		p.Config = cfg
		pois = append(pois, p)
	}
	return pois, rows.Err()
}

// detectPOIStops is the core spatial confinement algorithm.
// For each POI candidate, it finds GPS points within the POI's search radius,
// applies speed-based fast rejection, builds temporal windows with gap merging,
// computes the spatial spread (stddev of positions in meters), and confirms
// a stop if the spread is below the confinement threshold and duration exceeds
// the minimum dwell time.
//
// Algorithm based on Li/Zheng Stay-Point Detection (Microsoft Research 2008),
// adapted for POI-first approach.
func detectPOIStops(points []streamPoint, pois []poiCandidate) []confirmedStop {
	var results []confirmedStop

	for pi := range pois {
		poi := &pois[pi]

		// Step 1: Find all points within POI search radius
		var nearIndices []int
		for i := range points {
			if equirectDist(points[i].Lat, points[i].Lng, poi.Lat, poi.Lng) <= poi.Config.SearchRadiusM {
				nearIndices = append(nearIndices, i)
			}
		}
		if len(nearIndices) < 3 {
			continue
		}

		// Step 2: Speed fast-reject — median speed of near points
		speeds := make([]float64, len(nearIndices))
		for j, idx := range nearIndices {
			speeds[j] = points[idx].SpeedMS
		}
		slices.Sort(speeds)
		medianSpeed := speeds[len(speeds)/2]
		if medianSpeed > poi.Config.SpeedRejectMS {
			continue // clearly just passing through
		}

		// Step 3: Build temporal windows with gap merging
		windows := mergeTemporalWindows(points, nearIndices, poi.Config.GapToleranceS)

		// Step 4: For each window, check confinement + duration
		for _, w := range windows {
			startT := points[w.startIdx].T
			endT := points[w.endIdx].T
			durationS := int(endT.Sub(startT).Seconds())
			if durationS < int(poi.Config.MinDwellS) {
				continue
			}

			// Compute spatial spread (stddev of positions in meters).
			// Note: spread includes all points between start/end, including brief
			// excursions outside the POI radius (tolerated by gap merging). This may
			// slightly inflate the spread, but gap tolerance is conservative enough
			// that the effect is negligible in practice.
			spread, meanLat, meanLng := spatialSpread(points, w.startIdx, w.endIdx)
			if spread > poi.Config.ConfinementM {
				continue // not confined — likely walking/cycling through
			}

			results = append(results, confirmedStop{
				POI:       poi,
				StartedAt: startT,
				EndedAt:   endT,
				DurationS: durationS,
				CenterLat: meanLat,
				CenterLng: meanLng,
			})
		}
	}

	// Step 5: Deduplicate overlapping stops
	return deduplicateStops(results)
}

// mergeTemporalWindows groups consecutive point indices into temporal windows,
// merging gaps shorter than gapToleranceS.
func mergeTemporalWindows(points []streamPoint, indices []int, gapToleranceS float64) []temporalWindow {
	if len(indices) == 0 {
		return nil
	}
	var windows []temporalWindow
	wStart := indices[0]
	wEnd := indices[0]

	for i := 1; i < len(indices); i++ {
		prev, cur := indices[i-1], indices[i]
		gap := points[cur].T.Sub(points[prev].T).Seconds()
		if gap > gapToleranceS || cur-prev > 100 {
			// Gap too large — close current window, start new one
			windows = append(windows, temporalWindow{startIdx: wStart, endIdx: wEnd})
			wStart = cur
		}
		wEnd = cur
	}
	windows = append(windows, temporalWindow{startIdx: wStart, endIdx: wEnd})
	return windows
}

// spatialSpread computes the RMS distance of points from their centroid, in meters.
// Returns (spread_m, mean_lat, mean_lng).
func spatialSpread(points []streamPoint, startIdx, endIdx int) (float64, float64, float64) {
	n := endIdx - startIdx + 1
	if n < 2 {
		return 0, points[startIdx].Lat, points[startIdx].Lng
	}

	// Compute centroid
	var sumLat, sumLng float64
	for i := startIdx; i <= endIdx; i++ {
		sumLat += points[i].Lat
		sumLng += points[i].Lng
	}
	meanLat := sumLat / float64(n)
	meanLng := sumLng / float64(n)

	// Compute RMS distance from centroid in meters
	cosLat := math.Cos(meanLat * math.Pi / 180)
	var sumSqDist float64
	for i := startIdx; i <= endIdx; i++ {
		dLat := (points[i].Lat - meanLat) * degreesToMetersLat
		dLng := (points[i].Lng - meanLng) * cosLat * degreesToMetersLat
		sumSqDist += dLat*dLat + dLng*dLng
	}
	spread := math.Sqrt(sumSqDist / float64(n))
	return spread, meanLat, meanLng
}

// deduplicateStops removes overlapping stops. When two stops overlap in time,
// the one whose centroid is closer to its POI wins.
func deduplicateStops(stops []confirmedStop) []confirmedStop {
	if len(stops) < 2 {
		return stops
	}

	// Sort by start time
	slices.SortFunc(stops, func(a, b confirmedStop) int {
		return a.StartedAt.Compare(b.StartedAt)
	})

	var result []confirmedStop
	for _, s := range stops {
		overlapping := false
		for j := range result {
			// Check overlap: result[j].EndedAt > s.StartedAt
			if result[j].EndedAt.After(s.StartedAt) {
				overlapping = true
				// Keep the one closer to its POI
				if s.POI != nil && result[j].POI != nil {
					distJ := equirectDist(result[j].CenterLat, result[j].CenterLng,
						result[j].POI.Lat, result[j].POI.Lng)
					distI := equirectDist(s.CenterLat, s.CenterLng, s.POI.Lat, s.POI.Lng)
					if distI < distJ {
						result[j] = s // replace with closer match
					}
				}
				break
			}
		}
		if !overlapping {
			result = append(result, s)
		}
	}
	return result
}

// detectSpeedFallbackStops detects unnamed stops where no POI was found.
// Uses stricter thresholds than the old algorithm: speed < 0.3 m/s (was 0.5),
// duration >= 180s (was 60s), and spatial confinement <= 20m.
func detectSpeedFallbackStops(points []streamPoint) []confirmedStop {
	var stops []confirmedStop
	inStop := false
	startIdx := 0

	flush := func(endIdx int) {
		cluster := points[startIdx : endIdx+1]
		durationS := int(cluster[len(cluster)-1].T.Sub(cluster[0].T).Seconds())
		if durationS < stopFallbackMinDurS {
			return
		}
		// Check spatial confinement
		spread, meanLat, meanLng := spatialSpread(points, startIdx, endIdx)
		if spread > stopFallbackConfinM {
			return
		}
		stops = append(stops, confirmedStop{
			POI:       nil, // unnamed stop
			StartedAt: cluster[0].T,
			EndedAt:   cluster[len(cluster)-1].T,
			DurationS: durationS,
			CenterLat: meanLat,
			CenterLng: meanLng,
		})
	}

	for i, p := range points {
		if p.SpeedMS < stopFallbackSpeedMS {
			if !inStop {
				inStop = true
				startIdx = i
			}
		} else {
			if inStop {
				flush(i - 1)
				inStop = false
			}
		}
	}
	if inStop {
		flush(len(points) - 1)
	}
	return stops
}

// enrichStreamStops is the Go port of services/ml/enrichment/stops.py.
//
// Algorithm (POI-first redesign):
//  1. Filter GPS+speed points from the shared stream (lat/lng must be non-nil).
//  2. Fetch POI candidates along the activity track (11 categories).
//  3. POI-first detection via spatial confinement (Li/Zheng adapted).
//  4. Speed-based fallback for unnamed stops (stricter thresholds).
//  5. DELETE existing activity_stops + gastronomy geo tags within TX, then INSERT fresh results.
//  6. If duration >= 600 s AND poi category is gastronomy AND poi_name is set,
//     also INSERT an activity_geo_tags row with category='gastronomy'.
func enrichStreamStops(ctx context.Context, db *database.DB, _ string, activityID string, stream []EnrichmentStreamPoint) error {
	// 1. Filter to GPS points
	var points []streamPoint
	for _, sp := range stream {
		if sp.Lat == nil || sp.Lng == nil {
			continue
		}
		speedMS := 0.0
		if sp.SpeedMs != nil {
			speedMS = *sp.SpeedMs
		}
		points = append(points, streamPoint{
			T: sp.Time, Lat: *sp.Lat, Lng: *sp.Lng, SpeedMS: speedMS,
		})
	}
	if len(points) == 0 {
		return nil
	}

	// 2. Fetch POI candidates along route
	pois, err := fetchPOICandidates(ctx, db, activityID)
	if err != nil {
		slog.Warn("stops: poi fetch failed, falling back to speed-only",
			"activity_id", activityID, "error", err)
		pois = nil
	}

	// 3. POI-first detection (spatial confinement)
	var allStops []confirmedStop
	if len(pois) > 0 {
		allStops = detectPOIStops(points, pois)
	}

	// 4. Speed-based fallback for unnamed stops (not overlapping with POI stops)
	fallbackStops := detectSpeedFallbackStops(points)
	for _, fs := range fallbackStops {
		overlaps := false
		for _, ps := range allStops {
			if ps.EndedAt.After(fs.StartedAt) && ps.StartedAt.Before(fs.EndedAt) {
				overlaps = true
				break
			}
		}
		if !overlaps {
			allStops = append(allStops, fs)
		}
	}

	// 5. Persist in transaction (DELETE + INSERT atomar — bei Abbruch bleiben alte Daten erhalten)
	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("stops: begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	// DELETE innerhalb der TX — nicht vorher! Sonst Datenverlust bei Abbruch.
	if _, err := tx.Exec(ctx,
		"DELETE FROM activity_stops WHERE activity_id = $1",
		activityID,
	); err != nil {
		return fmt.Errorf("stops: delete existing stops: %w", err)
	}
	if _, err := tx.Exec(ctx,
		"DELETE FROM activity_geo_tags WHERE activity_id = $1 AND category = 'gastronomy'",
		activityID,
	); err != nil {
		return fmt.Errorf("stops: delete existing gastronomy tags: %w", err)
	}

	if len(allStops) == 0 {
		// No new stops — commit the deletes
		return tx.Commit(ctx)
	}

	gastronomyCount := 0
	for _, stop := range allStops {
		var poiType, poiName, poiCategory, confidence *string
		var poiOsmID *int64

		if stop.POI != nil {
			poiType = &stop.POI.Subtype
			poiName = &stop.POI.Name
			poiCategory = &stop.POI.Category
			poiOsmID = &stop.POI.OsmID
			conf := "poi_matched"
			confidence = &conf
		} else {
			conf := "speed_fallback"
			confidence = &conf
		}

		if _, err := tx.Exec(ctx, `
			INSERT INTO activity_stops
			    (activity_id, started_at, ended_at, duration_s,
			     location, poi_type, poi_name, poi_osm_id,
			     poi_category, confidence)
			VALUES ($1, $2, $3, $4,
			        ST_SetSRID(ST_MakePoint($6, $5), 4326),
			        $7, $8, $9, $10, $11)
		`, activityID, stop.StartedAt, stop.EndedAt, stop.DurationS,
			stop.CenterLat, stop.CenterLng,
			poiType, poiName, poiOsmID,
			poiCategory, confidence,
		); err != nil {
			slog.Warn("stops: insert stop failed",
				"activity_id", activityID,
				"started_at", stop.StartedAt,
				"error", err,
			)
		}

		// Gastronomy geo-tag (like before, >= 600s at a gastronomy POI)
		if stop.POI != nil && stop.POI.Category == "gastronomy" &&
			stop.DurationS >= stopGastronomyMinDurS && stop.POI.Name != "" {
			metaBytes, _ := json.Marshal(map[string]string{"poi_type": stop.POI.Subtype})
			durationMin := stop.DurationS / 60
			if _, err := tx.Exec(ctx, `
				INSERT INTO activity_geo_tags
				    (activity_id, category, name, osm_id, map_type,
				     map_point_lat, map_point_lon, duration_min, meta)
				VALUES ($1, 'gastronomy', $2, $3, 'point', $4, $5, $6, $7)
				ON CONFLICT DO NOTHING
			`, activityID, stop.POI.Name, poiOsmID,
				stop.CenterLat, stop.CenterLng, durationMin, metaBytes,
			); err != nil {
				slog.Warn("stops: insert gastronomy tag failed",
					"activity_id", activityID,
					"poi_name", stop.POI.Name,
					"error", err,
				)
			} else {
				gastronomyCount++
			}
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("stops: commit: %w", err)
	}

	slog.Info("stops: enrichment complete",
		"activity_id", activityID,
		"total_stops", len(allStops),
		"gastronomy_tags", gastronomyCount,
	)
	return nil
}
