package enrichment

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/enrichment/lifts"
)

// enrichStreamLifts is the lift-detection adapter (Phase 4c — three-stage
// snap pipeline). It is registered as a stream-enrichment module in
// stream_enrich.go and replaces the legacy independent Method A + Method B
// pair from services/ml/enrichment/lifts.py.
//
// Pipeline (must run in this order — see snap.go header):
//   1. DetectHeuristic     — rides from the in-memory stream
//   2. SnapToOSMLifts      — Stage 1, conservative snap
//   3. MergeMultiRideForOSM — Stage 2, cable-hold recovery
//   4. MethodACoverageGap  — Stage 3, OSM-only safety net
//   5. RecomputeLiftStats  — recount activities.lift_* with the dedup query
func enrichStreamLifts(
	ctx context.Context,
	db *database.DB,
	userID, activityID string,
	stream []EnrichmentStreamPoint,
) error {
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

	// Idempotent delete — guard user_id so only the activity's owner is
	// affected. Both candidate rows AND lift-typed segment_efforts must go,
	// or a re-run would accumulate duplicates.
	if _, err := db.Pool.Exec(ctx, `
		DELETE FROM activity_lift_candidates alc
		USING activities a
		WHERE alc.activity_id = $1 AND a.id = alc.activity_id AND a.user_id = $2
	`, activityID, userID); err != nil {
		return fmt.Errorf("enrichStreamLifts: delete candidates: %w", err)
	}
	if _, err := db.Pool.Exec(ctx, `
		DELETE FROM segment_efforts se
		USING activities a, segments s
		WHERE se.activity_id = $1
		  AND a.id = se.activity_id
		  AND a.user_id = $2
		  AND s.id = se.segment_id
		  AND s.segment_type = 'lift'
	`, activityID, userID); err != nil {
		return fmt.Errorf("enrichStreamLifts: delete lift segment_efforts: %w", err)
	}

	// 1. Heuristic detection (in-process) — index-bearing variant so Stage 2
	//    can correlate ride boundaries against the underlying samples for
	//    cable-hold signal evaluation.
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
	rides := lifts.DetectHeuristicWithIndices(streamPts, lifts.DefaultDetectorConfig(act.SportType))

	// 2. Pre-fetch OSM-lift candidates in the activity neighbourhood with
	//    their resolved segment_id + segment_version_id, length and bearing.
	osmCandidates, err := loadOSMLiftCandidates(ctx, db, activityID, userID)
	if err != nil {
		return fmt.Errorf("enrichStreamLifts: load osm candidates: %w", err)
	}

	// 3. For each ride × osm-candidate pair, compute (hausdorff,
	//    intersection_len). Pure PostGIS, one round-trip per ride.
	overlaps, err := computeRideOSMOverlaps(ctx, db, streamPts, rides, osmCandidates)
	if err != nil {
		return fmt.Errorf("enrichStreamLifts: compute overlaps: %w", err)
	}

	// 4. Pipeline.
	stage1 := lifts.SnapToOSMLifts(rides, osmCandidates, overlaps, streamPts, nil)
	stage2 := lifts.MergeMultiRideForOSM(stage1, rides, osmCandidates, overlaps, streamPts)
	stage3 := lifts.MethodACoverageGap(stage2, osmCandidates)

	// 5. Persist Stage-1 + Stage-2 results.
	osmMatches := 0
	for _, res := range stage2 {
		ride := rides[res.RideIdx]
		switch res.Action {
		case lifts.SnapActionSnap:
			osm := osmCandidates[res.OSMCandidateIdx]
			if res.SuppressedByMerge {
				// Audit-only candidate referencing the canonical merge's
				// segment_effort_id. The canonical row creates the
				// segment_effort itself; we look it up by the merged
				// (activity, started_at) key.
				canonicalEffortID, err := lookupCanonicalSegmentEffortID(ctx, db, activityID, rides[res.MergedRideIndices[0]].Ride.StartedAt)
				if err != nil {
					return fmt.Errorf("enrichStreamLifts: lookup canonical merge id: %w", err)
				}
				if err := insertAuditCandidate(ctx, db, userID, activityID, ride.Ride, canonicalEffortID); err != nil {
					return fmt.Errorf("enrichStreamLifts: insert suppressed audit candidate: %w", err)
				}
				continue
			}
			// Canonical Stage-1 snap or canonical Stage-2 merge.
			startedAt, durationS, ascentM, distanceM := canonicalRideMetrics(rides, res, osm)
			effortID, err := insertSegmentEffort(ctx, db, userID, activityID, osm,
				startedAt, durationS, distanceM, ascentM, res.IsReverse,
				true /*isLiftSegmentMatch*/, false /*isMethodAOnly*/, res.IsCableHoldMerge)
			if err != nil {
				return fmt.Errorf("enrichStreamLifts: insert segment_effort: %w", err)
			}
			if err := insertAuditCandidate(ctx, db, userID, activityID, ride.Ride, effortID); err != nil {
				return fmt.Errorf("enrichStreamLifts: insert audit candidate: %w", err)
			}
			osmMatches++

		case lifts.SnapActionPassthrough:
			if err := insertHeuristicCandidate(ctx, db, userID, activityID, ride.Ride); err != nil {
				return fmt.Errorf("enrichStreamLifts: insert heuristic candidate: %w", err)
			}

		case lifts.SnapActionMultiRideGroup:
			// Stage 2 should have rewritten this; if anything slips through
			// treat as heuristic-only (defensive).
			if err := insertHeuristicCandidate(ctx, db, userID, activityID, ride.Ride); err != nil {
				return fmt.Errorf("enrichStreamLifts: insert heuristic candidate (defensive): %w", err)
			}
		}
	}

	// 6. Stage 3 — for OSM lifts no Stage-1/2 ride covered, run the legacy
	//    per-lift time-window SQL (extract samples within 75m of the lift,
	//    derive duration/ascent/bearing) and emit one segment_efforts plus
	//    one audit-only candidate (source='osm', confirmed=true).
	for _, plan := range stage3 {
		osm := osmCandidates[plan.OSMCandidateIdx]
		ok, err := insertStage3SegmentEffort(ctx, db, userID, activityID, osm)
		if err != nil {
			return fmt.Errorf("enrichStreamLifts: stage 3 insert: %w", err)
		}
		if ok {
			osmMatches++
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

// loadOSMLiftCandidates fetches every OSM lift in PostGIS proximity of the
// activity track, resolves the latest segment_version per lift (by version
// number), and returns a slice in candidate order. Lifts without a
// corresponding `segments` row (no segment_versions match) are dropped —
// they cannot be snapped onto.
func loadOSMLiftCandidates(ctx context.Context, db *database.DB, activityID, userID string) ([]lifts.OSMLiftCandidate, error) {
	const query = `
		SELECT
			l.osm_id,
			COALESCE(l.name, '') AS name,
			ST_Length(l.geom::geography) AS length_m,
			degrees(ST_Azimuth(ST_StartPoint(l.geom), ST_EndPoint(l.geom))) AS bearing_deg,
			s.id AS segment_id,
			sv.id AS segment_version_id
		FROM osm_lifts l
		JOIN segments s ON s.osm_lift_id = l.osm_id
		JOIN LATERAL (
			SELECT id FROM segment_versions sv
			WHERE sv.segment_id = s.id
			ORDER BY sv.version_number DESC
			LIMIT 1
		) sv ON true,
		(
			SELECT at.track_full AS geom
			FROM activity_tracks at
			JOIN activities a ON a.id = at.activity_id
			WHERE at.activity_id = $1 AND a.user_id = $2
		) route
		WHERE ST_DWithin(route.geom::geography, l.geom::geography, $3)
		ORDER BY l.osm_id
	`
	rows, err := db.Pool.Query(ctx, query, activityID, userID, float64(lifts.ProximityM))
	if err != nil {
		return nil, fmt.Errorf("query: %w", err)
	}
	defer rows.Close()
	out := []lifts.OSMLiftCandidate{}
	for rows.Next() {
		var c lifts.OSMLiftCandidate
		if err := rows.Scan(&c.OSMID, &c.Name, &c.LengthM, &c.BearingDeg, &c.SegmentID, &c.SegmentVersionID); err != nil {
			return nil, fmt.Errorf("scan: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// computeRideOSMOverlaps issues one PostGIS query per ride that returns
// the hausdorff-distance and intersection-length against every osm
// candidate. Each ride is shipped as a WKT LineString built from the
// in-memory stream points between StartSampleIdx and EndSampleIdx. When the
// ride has fewer than 2 GPS-bearing samples the overlap query is skipped
// (the ride cannot be matched against any OSM geometry without a track).
func computeRideOSMOverlaps(
	ctx context.Context,
	db *database.DB,
	stream []lifts.StreamPoint,
	rides []lifts.RideWithIndices,
	candidates []lifts.OSMLiftCandidate,
) ([]lifts.RideOSMOverlap, error) {
	if len(rides) == 0 || len(candidates) == 0 {
		return nil, nil
	}
	osmIDs := make([]int64, len(candidates))
	for i, c := range candidates {
		osmIDs[i] = c.OSMID
	}
	osmIdxByID := make(map[int64]int, len(candidates))
	for i, c := range candidates {
		osmIdxByID[c.OSMID] = i
	}
	var out []lifts.RideOSMOverlap
	for ri, ride := range rides {
		wkt := rideLineStringWKT(stream, ride)
		if wkt == "" {
			continue
		}
		const query = `
			WITH route AS (
				SELECT ST_GeomFromText($1, 4326) AS geom
			)
			SELECT
				l.osm_id,
				ST_HausdorffDistance(
					ST_Intersection(
						ST_Buffer(route.geom::geography, 30)::geometry,
						l.geom
					),
					l.geom, 0.1
				) AS hausdorff_m,
				ST_Length(
					ST_Intersection(
						ST_Buffer(route.geom::geography, 30)::geometry,
						l.geom
					)::geography
				) AS intersection_len_m
			FROM osm_lifts l, route
			WHERE l.osm_id = ANY($2)
		`
		rows, err := db.Pool.Query(ctx, query, wkt, osmIDs)
		if err != nil {
			return nil, fmt.Errorf("ride %d overlap query: %w", ri, err)
		}
		for rows.Next() {
			var osmID int64
			var hausdorff, isectLen *float64
			if err := rows.Scan(&osmID, &hausdorff, &isectLen); err != nil {
				rows.Close()
				return nil, fmt.Errorf("scan overlap: %w", err)
			}
			if hausdorff == nil || isectLen == nil {
				continue
			}
			osmIdx, ok := osmIdxByID[osmID]
			if !ok {
				continue
			}
			out = append(out, lifts.RideOSMOverlap{
				RideIdx:         ri,
				OSMIdx:          osmIdx,
				HausdorffM:      *hausdorff,
				IntersectionLen: *isectLen,
			})
		}
		rowsErr := rows.Err()
		rows.Close()
		if rowsErr != nil {
			return nil, fmt.Errorf("rows err: %w", rowsErr)
		}
	}
	return out, nil
}

// rideLineStringWKT builds a 2D LineString WKT from the stream samples
// covered by the ride. Returns "" when fewer than 2 samples have GPS data.
func rideLineStringWKT(stream []lifts.StreamPoint, ride lifts.RideWithIndices) string {
	if ride.StartSampleIdx < 0 || ride.EndSampleIdx >= len(stream) {
		return ""
	}
	parts := make([]string, 0, ride.EndSampleIdx-ride.StartSampleIdx+1)
	for k := ride.StartSampleIdx; k <= ride.EndSampleIdx; k++ {
		s := stream[k]
		if s.Lat == nil || s.Lng == nil {
			continue
		}
		parts = append(parts, fmt.Sprintf("%.7f %.7f", *s.Lng, *s.Lat))
	}
	if len(parts) < 2 {
		return ""
	}
	return "LINESTRING(" + strings.Join(parts, ",") + ")"
}

// canonicalRideMetrics derives started_at + duration_s + ascent_m for a
// canonical Stage-1 snap or Stage-2 merge. For Stage 2 the canonical row
// spans from the earliest ride's StartedAt to the latest ride's EndedAt
// (which encloses the cable-hold pause); ascent sums per-ride. distance_m
// always comes from the OSM lift length (OSM is ground-truth for distance,
// the heuristic ride may have boundary drift).
func canonicalRideMetrics(rides []lifts.RideWithIndices, res lifts.SnapResult, osm lifts.OSMLiftCandidate) (time.Time, int, float64, float64) {
	if !res.IsCableHoldMerge || len(res.MergedRideIndices) <= 1 {
		r := rides[res.RideIdx].Ride
		return r.StartedAt, r.DurationS, ascentForDirection(r, res.IsReverse), osm.LengthM
	}
	first := rides[res.MergedRideIndices[0]].Ride
	last := rides[res.MergedRideIndices[len(res.MergedRideIndices)-1]].Ride
	durationS := int(last.EndedAt.Sub(first.StartedAt).Seconds())
	ascent := 0.0
	for _, idx := range res.MergedRideIndices {
		ascent += ascentForDirection(rides[idx].Ride, res.IsReverse)
	}
	return first.StartedAt, durationS, ascent, osm.LengthM
}

func ascentForDirection(r lifts.LiftRide, isReverse bool) float64 {
	if isReverse {
		return 0
	}
	return r.AscentM
}

// insertSegmentEffort writes one Stage-1 / Stage-2 / Stage-3 segment_efforts
// row with the Phase 4c audit flags. Returns the inserted id (UUID); the
// composite-PK hypertable design means we generate the id client-side and
// thread it back to the audit-candidate insert.
func insertSegmentEffort(
	ctx context.Context,
	db *database.DB,
	userID, activityID string,
	osm lifts.OSMLiftCandidate,
	startedAt time.Time,
	durationS int,
	distanceM, ascentM float64,
	isReverse, isLiftSegmentMatch, isMethodAOnly, isCableHoldMerge bool,
) (string, error) {
	effortID := uuid.NewString()
	_, err := db.Pool.Exec(ctx, `
		INSERT INTO segment_efforts
			(id, activity_id, segment_id, segment_version_id, user_id,
			 started_at, elapsed_time_s, distance_m, elevation_gain_m,
			 is_reverse, sport_type, sub_type, in_leaderboard,
			 is_lift_segment_match, is_method_a_only, is_cable_hold_merge)
		SELECT $1, $2, $3, $4, a.user_id, $5, $6, $7, $8, $9,
			   a.sport_type, COALESCE(NULLIF(a.sub_type, ''), 'generic'), false,
			   $10, $11, $12
		FROM activities a
		WHERE a.id = $2 AND a.user_id = $13
		ON CONFLICT DO NOTHING
	`, effortID, activityID, osm.SegmentID, osm.SegmentVersionID,
		startedAt, durationS, distanceM, ascentM, isReverse,
		isLiftSegmentMatch, isMethodAOnly, isCableHoldMerge, userID)
	if err != nil {
		return "", fmt.Errorf("insert: %w", err)
	}
	return effortID, nil
}

// insertAuditCandidate writes the activity_lift_candidates row that mirrors
// a Stage-1/2/3 segment_effort. The candidate carries source='osm',
// confirmed=true and segment_effort_id pointing at the effort. This row
// is purely for the read-side unified API; it is excluded from the
// recomputeLiftStats heuristic-only count by the partial-index dedup.
func insertAuditCandidate(
	ctx context.Context,
	db *database.DB,
	userID, activityID string,
	ride lifts.LiftRide,
	segmentEffortID string,
) error {
	_, err := db.Pool.Exec(ctx, `
		INSERT INTO activity_lift_candidates
			(activity_id, started_at, ended_at, duration_s, ascent_m, descent_m, distance_m,
			 confidence, confirmed, source, segment_effort_id)
		SELECT $1, $2, $3, $4, $5, $6, $7, $8, true, 'osm', $9
		WHERE EXISTS (SELECT 1 FROM activities a WHERE a.id = $1 AND a.user_id = $10)
		ON CONFLICT (activity_id, started_at) DO UPDATE
		SET source = EXCLUDED.source,
		    segment_effort_id = EXCLUDED.segment_effort_id,
		    confirmed = EXCLUDED.confirmed
	`, activityID, ride.StartedAt, ride.EndedAt, ride.DurationS,
		ride.AscentM, ride.DescentM, ride.DistanceM, ride.Confidence,
		segmentEffortID, userID)
	if err != nil {
		return fmt.Errorf("insert: %w", err)
	}
	return nil
}

// insertHeuristicCandidate writes a heuristic-only candidate (source='heuristic',
// segment_effort_id=NULL, confirmed=false) for a ride that did not snap to any
// OSM lift. User must confirm before it counts in lift stats.
func insertHeuristicCandidate(
	ctx context.Context,
	db *database.DB,
	userID, activityID string,
	ride lifts.LiftRide,
) error {
	_, err := db.Pool.Exec(ctx, `
		INSERT INTO activity_lift_candidates
			(activity_id, started_at, ended_at, duration_s, ascent_m, descent_m, distance_m,
			 confidence, confirmed, source)
		SELECT $1, $2, $3, $4, $5, $6, $7, $8, false, 'heuristic'
		WHERE EXISTS (SELECT 1 FROM activities a WHERE a.id = $1 AND a.user_id = $9)
		ON CONFLICT (activity_id, started_at) DO NOTHING
	`, activityID, ride.StartedAt, ride.EndedAt, ride.DurationS,
		ride.AscentM, ride.DescentM, ride.DistanceM, ride.Confidence, userID)
	if err != nil {
		return fmt.Errorf("insert: %w", err)
	}
	return nil
}

// lookupCanonicalSegmentEffortID returns the id of the segment_effort that
// the canonical Stage-2 ride just inserted (matched by activity_id +
// started_at). Returns the empty string when not found — caller treats that
// as "no canonical row" and skips the audit insert.
func lookupCanonicalSegmentEffortID(ctx context.Context, db *database.DB, activityID string, startedAt time.Time) (string, error) {
	var id string
	err := db.Pool.QueryRow(ctx, `
		SELECT id FROM segment_efforts
		WHERE activity_id = $1 AND started_at = $2 AND is_cable_hold_merge = true
		ORDER BY created_at DESC LIMIT 1
	`, activityID, startedAt).Scan(&id)
	if err != nil {
		return "", fmt.Errorf("lookup: %w", err)
	}
	return id, nil
}

// insertStage3SegmentEffort runs the legacy Method-A per-lift time-window
// SQL: extracts samples within 75m of the OSM lift, derives
// duration/ascent/bearing, then INSERTs one segment_efforts row
// (is_method_a_only=true) plus the audit-only candidate. Returns true when
// a row was successfully inserted (false for "below 30s duration floor" or
// "fewer than 2 samples in window" — both legitimate skip cases).
func insertStage3SegmentEffort(
	ctx context.Context,
	db *database.DB,
	userID, activityID string,
	osm lifts.OSMLiftCandidate,
) (bool, error) {
	type windowRow struct {
		t         time.Time
		altitudeM *float64
		lat       *float64
		lng       *float64
	}
	rows, err := db.Pool.Query(ctx, `
		SELECT ast.time, ast.altitude_m, ast.lat, ast.lng
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
	`, activityID, osm.OSMID, userID)
	if err != nil {
		return false, fmt.Errorf("time-window query: %w", err)
	}
	var pts []windowRow
	for rows.Next() {
		var r windowRow
		if err := rows.Scan(&r.t, &r.altitudeM, &r.lat, &r.lng); err != nil {
			rows.Close()
			return false, fmt.Errorf("scan: %w", err)
		}
		pts = append(pts, r)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return false, fmt.Errorf("rows: %w", err)
	}
	if len(pts) < 2 {
		return false, nil
	}
	startedAt := pts[0].t
	endedAt := pts[len(pts)-1].t
	durationS := int(endedAt.Sub(startedAt).Seconds())
	if durationS < 30 {
		return false, nil
	}
	var ascentM float64
	if first, last := pts[0].altitudeM, pts[len(pts)-1].altitudeM; first != nil && last != nil && *last > *first {
		ascentM = *last - *first
	}
	var trackBearing float64
	first, last := pts[0], pts[len(pts)-1]
	if first.lat != nil && last.lat != nil && first.lng != nil && last.lng != nil {
		trackBearing = lifts.BearingDeg(*first.lat, *first.lng, *last.lat, *last.lng)
	}
	isReverse := lifts.IsReverseHeading(trackBearing, osm.BearingDeg)

	effortID, err := insertSegmentEffort(ctx, db, userID, activityID, osm,
		startedAt, durationS, osm.LengthM, ascentM, isReverse,
		true /*isLiftSegmentMatch*/, true /*isMethodAOnly*/, false /*isCableHoldMerge*/)
	if err != nil {
		return false, err
	}
	// Audit-only candidate row mirrors the Stage-3 segment_effort.
	auditRide := lifts.LiftRide{
		StartedAt:  startedAt,
		EndedAt:    endedAt,
		DurationS:  durationS,
		AscentM:    ascentM,
		DescentM:   0,
		DistanceM:  osm.LengthM,
		Confidence: "high",
	}
	if err := insertAuditCandidate(ctx, db, userID, activityID, auditRide, effortID); err != nil {
		return false, fmt.Errorf("insert audit candidate: %w", err)
	}
	return true, nil
}
