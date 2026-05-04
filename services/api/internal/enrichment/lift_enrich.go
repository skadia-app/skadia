package enrichment

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/enrichment/lifts"
)

// enrichStreamLifts is the Method-A + Method-B lift-detection adapter.
// It is registered as a stream-enrichment module in stream_enrich.go.
// Ported from services/ml/enrichment/lifts.py:enrich_lifts.
func enrichStreamLifts(
	ctx context.Context,
	db *database.DB,
	userID, activityID string,
	stream []EnrichmentStreamPoint,
) error {
	// Load activity to check relevance.
	act, err := db.GetActivity(ctx, activityID)
	if err != nil {
		return fmt.Errorf("enrichStreamLifts: get activity: %w", err)
	}
	if act == nil {
		return nil
	}
	subType := ""
	if act.SubType != nil {
		subType = *act.SubType
	}
	if !lifts.IsLiftRelevant(act.SportType, subType, act.IsIndoor, act.IsElectric, act.IsVirtual) {
		return nil
	}

	// Skip if no OSM lift data is loaded at all.
	var hasOSM bool
	if err := db.Pool.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM osm_lifts LIMIT 1)").Scan(&hasOSM); err != nil {
		return fmt.Errorf("enrichStreamLifts: check osm_lifts: %w", err)
	}
	if !hasOSM {
		return nil
	}

	// Verify track exists (user_id guard prevents cross-user reads).
	var trackExists bool
	if err := db.Pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM activity_tracks at
			JOIN activities a ON a.id = at.activity_id
			WHERE at.activity_id = $1 AND a.user_id = $2
		)`, activityID, userID).Scan(&trackExists); err != nil {
		return fmt.Errorf("enrichStreamLifts: check track: %w", err)
	}
	if !trackExists {
		return nil
	}

	// Idempotent delete — guards user_id so only the activity's owner is affected.
	if _, err := db.Pool.Exec(ctx, `
		DELETE FROM activity_lift_candidates alc
		USING activities a
		WHERE alc.activity_id = $1 AND a.id = alc.activity_id AND a.user_id = $2
	`, activityID, userID); err != nil {
		return fmt.Errorf("enrichStreamLifts: delete candidates: %w", err)
	}

	// Method A: OSM-based matching.
	// Port of _osm_lift_candidates_sql from lifts.py:62-90.
	// $1=activity_id, $2=buffer_m (30), $3=proximity_m, $4=user_id
	const osmCandidatesSQL = `
		SELECT
			l.id AS id, l.osm_id, l.name,
			ST_Length(l.geom::geography) AS lift_length_m,
			ST_HausdorffDistance(
				ST_Intersection(
					ST_Buffer(route.geom::geography, $2)::geometry,
					l.geom
				),
				l.geom, 0.1
			) AS hausdorff_m,
			ST_Length(
				ST_Intersection(
					ST_Buffer(route.geom::geography, $2)::geometry,
					l.geom
				)::geography
			) / NULLIF(ST_Length(l.geom::geography), 0) AS coverage_ratio,
			degrees(ST_Azimuth(ST_StartPoint(l.geom), ST_EndPoint(l.geom))) AS lift_bearing
		FROM osm_lifts l,
			 (
				 SELECT at.track_full AS geom
				 FROM activity_tracks at
				 JOIN activities a ON a.id = at.activity_id
				 WHERE at.activity_id = $1 AND a.user_id = $4
			 ) route
		WHERE ST_DWithin(route.geom::geography, l.geom::geography, $3)
		ORDER BY hausdorff_m
	`

	rows, err := db.Pool.Query(ctx, osmCandidatesSQL, activityID, 30.0, float64(lifts.ProximityM), userID)
	if err != nil {
		return fmt.Errorf("enrichStreamLifts: osm candidates query: %w", err)
	}
	defer rows.Close()

	type candidate struct {
		id            string
		osmID         int64
		name          *string
		liftLengthM   float64
		hausdorffM    *float64
		coverageRatio *float64
		liftBearing   float64
	}
	var candidates []candidate
	for rows.Next() {
		var c candidate
		if err := rows.Scan(&c.id, &c.osmID, &c.name, &c.liftLengthM, &c.hausdorffM, &c.coverageRatio, &c.liftBearing); err != nil {
			return fmt.Errorf("enrichStreamLifts: scan candidate: %w", err)
		}
		candidates = append(candidates, c)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("enrichStreamLifts: candidates rows: %w", err)
	}

	osmMatches := 0
	for _, c := range candidates {
		if c.hausdorffM == nil || *c.hausdorffM > lifts.HausdorffMaxM {
			continue
		}
		if c.coverageRatio == nil || *c.coverageRatio < lifts.MinCoverage {
			continue
		}

		// Find the latest segment version for this OSM lift.
		var segmentID, segmentVersionID string
		err := db.Pool.QueryRow(ctx, `
			SELECT s.id, sv.id AS version_id
			FROM segments s
			JOIN segment_versions sv ON sv.segment_id = s.id
			WHERE s.osm_lift_id = $1
			ORDER BY sv.version_number DESC LIMIT 1
		`, c.osmID).Scan(&segmentID, &segmentVersionID)
		if err != nil {
			// No segment yet for this OSM lift — skip.
			continue
		}

		// Extract the time-window from activity_streams within proximity of the lift.
		// Port of _extract_ride_timewindow from lifts.py:195-228.
		type streamRow struct {
			t         time.Time
			altitudeM *float64
			speedMs   *float64
			lat       *float64
			lng       *float64
		}
		windowRows, err := db.Pool.Query(ctx, `
			SELECT ast.time, ast.altitude_m, ast.speed_ms, ast.lat, ast.lng
			FROM activity_streams ast
			JOIN activities a ON a.id = ast.activity_id
			WHERE ast.activity_id = $1
			  AND a.user_id = $3
			  AND ast.lat IS NOT NULL
			  AND ST_DWithin(
				ST_SetSRID(ST_MakePoint(ast.lng, ast.lat), 4326)::geography,
				(SELECT geom::geography FROM osm_lifts WHERE osm_id = $2),
				75
			  )
			ORDER BY ast.time
		`, activityID, c.osmID, userID)
		if err != nil {
			return fmt.Errorf("enrichStreamLifts: time-window query: %w", err)
		}

		var windowPts []streamRow
		for windowRows.Next() {
			var r streamRow
			if err := windowRows.Scan(&r.t, &r.altitudeM, &r.speedMs, &r.lat, &r.lng); err != nil {
				windowRows.Close()
				return fmt.Errorf("enrichStreamLifts: time-window scan: %w", err)
			}
			windowPts = append(windowPts, r)
		}
		windowRows.Close()
		if err := windowRows.Err(); err != nil {
			return fmt.Errorf("enrichStreamLifts: time-window rows: %w", err)
		}

		if len(windowPts) < 2 {
			continue
		}
		startedAt := windowPts[0].t
		endedAt := windowPts[len(windowPts)-1].t
		durationS := int(endedAt.Sub(startedAt).Seconds())
		if durationS < 30 {
			continue
		}

		var ascentM float64
		firstAlt := windowPts[0].altitudeM
		lastAlt := windowPts[len(windowPts)-1].altitudeM
		if firstAlt != nil && lastAlt != nil && *lastAlt > *firstAlt {
			ascentM = *lastAlt - *firstAlt
		}

		var trackBearing float64
		first := windowPts[0]
		last := windowPts[len(windowPts)-1]
		if first.lat != nil && last.lat != nil {
			trackBearing = lifts.BearingDeg(*first.lat, *first.lng, *last.lat, *last.lng)
		}
		isReverse := lifts.IsReverseHeading(trackBearing, c.liftBearing)

		// Insert segment_effort — port of _segment_effort_insert_sql from lifts.py:47-59.
		// $1=activity_id, $2=segment_id, $3=segment_version_id, $4=started_at,
		// $5=elapsed_time_s, $6=elevation_gain_m, $7=is_reverse, $8=user_id
		_, err = db.Pool.Exec(ctx, `
			INSERT INTO segment_efforts
				(activity_id, segment_id, segment_version_id, user_id,
				 started_at, elapsed_time_s, distance_m, elevation_gain_m,
				 is_reverse, sport_type, sub_type, in_leaderboard)
			SELECT $1, $2, $3, a.user_id, $4, $5,
				   ST_Length(sv.track::geometry::geography),
				   $6, $7, a.sport_type, COALESCE(NULLIF(a.sub_type, ''), 'generic'), false
			FROM segment_versions sv, activities a
			WHERE sv.id = $3 AND a.id = $1 AND a.user_id = $8
			ON CONFLICT DO NOTHING
		`, activityID, segmentID, segmentVersionID, startedAt, durationS, ascentM, isReverse, userID)
		if err != nil {
			return fmt.Errorf("enrichStreamLifts: insert segment_effort: %w", err)
		}
		osmMatches++
	}

	// Method B: heuristic detection (stub in Phase 2 — DetectHeuristic returns nil).
	streamPts := make([]lifts.StreamPoint, 0, len(stream))
	for _, sp := range stream {
		streamPts = append(streamPts, lifts.StreamPoint{
			Time:      sp.Time,
			Lat:       sp.Lat,
			Lng:       sp.Lng,
			AltitudeM: sp.AltitudeM,
			SpeedMs:   sp.SpeedMs,
			DistanceM: sp.DistanceM,
		})
	}
	rides := lifts.DetectHeuristic(streamPts, lifts.DefaultDetectorConfig(act.SportType))

	// Port of enrich_lifts:175-188 in lifts.py.
	for _, ride := range rides {
		_, err := db.Pool.Exec(ctx, `
			INSERT INTO activity_lift_candidates
				(activity_id, started_at, ended_at, duration_s, ascent_m, descent_m, distance_m, confidence, confirmed)
			SELECT $1, $2, $3, $4, $5, $6, $7, $8, false
			WHERE EXISTS (
				SELECT 1 FROM activities a WHERE a.id = $1 AND a.user_id = $9
			)
			ON CONFLICT (activity_id, started_at) DO NOTHING
		`, activityID, ride.StartedAt, ride.EndedAt,
			ride.DurationS, ride.AscentM, ride.DescentM,
			ride.DistanceM, ride.Confidence, userID)
		if err != nil {
			return fmt.Errorf("enrichStreamLifts: insert lift candidate: %w", err)
		}
	}

	if err := db.RecomputeLiftStats(ctx, activityID); err != nil {
		return fmt.Errorf("enrichStreamLifts: recompute lift stats: %w", err)
	}

	slog.Debug("enrichStreamLifts: done",
		"activity_id", activityID,
		"osm_matches", osmMatches,
		"heuristic_rides", len(rides),
	)
	return nil
}
