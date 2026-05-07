package enrichment

import (
	"context"
	"fmt"
	"hash/fnv"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/enrichment/lifts"
)

// liftEnrichAdvisoryLockNs scopes the per-activity advisory-lock key so it
// cannot collide with other features that hash activity_id into an int64
// lock space. The 32-bit FNV-1a hash of the activity UUID occupies the low
// half; this namespace tag occupies the high half.
//
//	key = (liftEnrichAdvisoryLockNs << 32) | fnv32a(activityID)
//
// Mnemonic value 0x4C494654 spells "LIFT" in ASCII.
const liftEnrichAdvisoryLockNs int64 = 0x4C494654

// liftEnrichLockKey derives the advisory-lock key for one activity_id. It
// combines the namespace tag with a 32-bit FNV-1a hash of the activity UUID
// to yield a stable int64 that pg_advisory_lock accepts directly.
func liftEnrichLockKey(activityID string) int64 {
	h := fnv.New32a()
	_, _ = h.Write([]byte(activityID))
	return (liftEnrichAdvisoryLockNs << 32) | int64(h.Sum32())
}

// enrichStreamLifts is the lift-detection adapter (Phase 4c — three-stage
// snap pipeline). It is registered as a stream-enrichment module in
// stream_enrich.go and replaces the legacy independent Method A + Method B
// pair from services/ml/enrichment/lifts.py.
//
// Pipeline (must run in this order — see snap.go header):
//  1. DetectHeuristic     — rides from the in-memory stream
//  2. SnapToOSMLifts      — Stage 1, conservative snap
//  3. MergeMultiRideForOSM — Stage 2, cable-hold recovery
//  4. MethodACoverageGap  — Stage 3, OSM-only safety net
//  5. RecomputeLiftStats  — recount activities.lift_* with the dedup query
func enrichStreamLifts(
	ctx context.Context,
	db *database.DB,
	userID, activityID string,
	stream []EnrichmentStreamPoint,
) error {
	// Critic F18 — serialize concurrent re-enrichments of the same activity.
	//
	// Without this lock, two callers (e.g. cron retry + manual retry, or two
	// stream-enrichment pipeline runs) can race on the DELETE → INSERT
	// sequence below. That race produces duplicated rows on the audit-row
	// path (lookupCanonicalSegmentEffortIDTx's `ORDER BY created_at DESC LIMIT
	// 1` becomes non-deterministic) and orphaned segment_efforts.
	//
	// The lock is session-scoped (pg_try_advisory_lock) on a dedicated
	// connection that is held until the function returns, mirroring the
	// established sidecar pattern in handler/activity_edit_ops.go.
	// pg_advisory_xact_lock is unsuitable here: this function does not run
	// inside a single transaction (RecomputeLiftStats and the gear-refresh
	// helper open their own internal transactions and would deadlock or
	// require signature changes in helpers outside this file's scope).
	//
	// pg_try_advisory_lock + early return is preferred over the blocking
	// pg_advisory_lock variant: another worker is already producing the
	// same result, so doubling up wastes CPU without changing the outcome.
	lockConn, err := db.Pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("enrichStreamLifts: acquire conn for advisory lock: %w", err)
	}
	defer lockConn.Release()
	lockKey := liftEnrichLockKey(activityID)
	var locked bool
	if err := lockConn.QueryRow(ctx, "SELECT pg_try_advisory_lock($1)", lockKey).Scan(&locked); err != nil {
		return fmt.Errorf("enrichStreamLifts: try advisory lock: %w", err)
	}
	if !locked {
		slog.Info("enrichStreamLifts: another worker is enriching this activity, skipping",
			"activity_id", activityID)
		return nil
	}
	defer func() {
		// Use a fresh context bounded by ctx — even on cancellation we
		// want to release the lock. pgx returns the connection on Release;
		// the lock itself is bound to the underlying server-side session,
		// so explicit unlock is required (Release alone does not free
		// session-scoped advisory locks until the connection truly closes).
		releaseCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if _, unlockErr := lockConn.Exec(releaseCtx, "SELECT pg_advisory_unlock($1)", lockKey); unlockErr != nil {
			slog.Warn("enrichStreamLifts: advisory unlock failed",
				"activity_id", activityID,
				"error", unlockErr)
		}
	}()

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

	// Phase 4c.5.1: when osm_lifts is empty the snap stages cannot run, but
	// the heuristic detector still does — its rides land as unsegmented
	// heuristic candidates (the user-confirm flow takes over from there).
	var hasOSM bool
	if err := db.Pool.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM osm_lifts LIMIT 1)").Scan(&hasOSM); err != nil {
		return fmt.Errorf("enrichStreamLifts: check osm_lifts: %w", err)
	}
	if !hasOSM {
		slog.Warn("enrichStreamLifts: osm_lifts is empty — running heuristic detector without snap pipeline",
			"activity_id", activityID)
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
	//    When osm_lifts is empty (no OSM data globally) the pipeline
	//    short-circuits to "all rides are heuristic-only".
	//
	//    Both reads run BEFORE the destructive cleanup Tx below — they are
	//    pure reads of pre-existing data (osm_lifts + activity_tracks +
	//    segments) that the Tx neither modifies nor depends on. Running them
	//    on db.Pool keeps the Tx window short.
	var osmCandidates []lifts.OSMLiftCandidate
	var overlaps []lifts.RideOSMOverlap
	if hasOSM {
		osmCandidates, err = loadOSMLiftCandidates(ctx, db, activityID, userID)
		if err != nil {
			return fmt.Errorf("enrichStreamLifts: load osm candidates: %w", err)
		}
		overlaps, err = computeRideOSMOverlaps(ctx, db, streamPts, rides, osmCandidates)
		if err != nil {
			return fmt.Errorf("enrichStreamLifts: compute overlaps: %w", err)
		}
	}

	// 3. Pipeline. With no OSM candidates Stage 1 + 2 emit Passthrough
	//    for every ride, Stage 3 plans nothing — every ride lands as a
	//    heuristic-only candidate, which is the Phase 4c.5.1 contract.
	//    Pure in-memory; no DB access.
	stage1 := lifts.SnapToOSMLifts(rides, osmCandidates, overlaps, streamPts, nil)
	stage2 := lifts.MergeMultiRideForOSM(stage1, rides, osmCandidates, overlaps, streamPts)
	stage3 := lifts.MethodACoverageGap(stage2, osmCandidates)

	// 4. Atomic destructive cleanup + rebuild — Codex N5a (Option B).
	//
	// Pre-fix the cleanup (DELETE candidates, NULL-out rejected pointers,
	// DELETE segment_efforts) and the rebuild (INSERT segment_efforts +
	// audit candidates) ran on db.Pool as auto-commit statements. If the
	// rebuild failed mid-way after the cleanup committed, the activity was
	// left in an empty/inconsistent lift state with no recovery hook — the
	// advisory lock made the next caller skip silently, so the bad state
	// could persist indefinitely.
	//
	// The fix wraps cleanup + rebuild in a single transaction: on any
	// failure both phases roll back together and the original rows
	// (including rejected_by_user audit rows with their original
	// segment_effort_id pointers) are restored. RecomputeLiftStats stays
	// outside the Tx — it has its own internal Tx for the activities.lift_*
	// UPDATE + gear-stats refresh, and a recompute failure leaves the
	// committed lift candidates / segment_efforts intact (re-running
	// enrichment recovers stale activities.lift_* values).
	//
	// All write helpers here have *Tx variants that route via tx.Exec.
	// Read helpers that touch only pre-existing rejected rows
	// (IsRejectedAnySourceInWindow) keep using db.Pool: those rows are
	// preserved by the Tx's `NOT alc.rejected_by_user` DELETE filter, so a
	// separate-connection read returns correct results regardless of
	// READ COMMITTED visibility.
	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("enrichStreamLifts: begin tx: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck // Rollback after Commit is a no-op

	// Idempotent delete — guard user_id so only the activity's owner is
	// affected. Rows with rejected_by_user=true are intentionally preserved:
	// they are user-authored audit traces that must survive re-enrichment
	// (Phase 4d). The re-insert path uses insertHeuristicCandidateTx (and
	// insertAuditCandidateTx) to suppress re-detection of overlapping
	// rejected candidates.
	if _, err := tx.Exec(ctx, `
		DELETE FROM activity_lift_candidates alc
		USING activities a
		WHERE alc.activity_id = $1 AND a.id = alc.activity_id AND a.user_id = $2
		  AND NOT alc.rejected_by_user
	`, activityID, userID); err != nil {
		return fmt.Errorf("enrichStreamLifts: delete candidates: %w", err)
	}
	// Critic F5 — break the orphan FK on soft-rejected audit rows. Those
	// rows are intentionally preserved across re-enrichment so the user's
	// reject decision survives, but their segment_effort_id pointer would
	// dangle once the segment_efforts DELETE below removes the referenced
	// row. There is no DB-level FK from activity_lift_candidates →
	// segment_efforts (the hypertable's composite PK is not referenceable),
	// so an explicit NULL update is the only way to keep the audit row
	// orphan-free. The column is nullable since migration 000349. NULL is
	// the correct soft-reject state (matches the heuristic-rejected case
	// where the candidate never had a segment_effort).
	//
	// N5c trade-off note: with cleanup + rebuild now atomic in one Tx, the
	// NULL-update is no longer a defensive measure against rebuild failure
	// (Tx-rollback would restore both the segment_efforts rows and the
	// original pointers). It IS still required on the success path: the
	// segment_efforts DELETE that follows would otherwise leave the
	// preserved rejected rows pointing at deleted IDs after Commit. Keeping
	// the NULL-update preserves that contract regardless of Tx outcome.
	if _, err := tx.Exec(ctx, `
		UPDATE activity_lift_candidates alc
		SET segment_effort_id = NULL
		FROM activities a
		WHERE alc.activity_id = $1
		  AND a.id = alc.activity_id
		  AND a.user_id = $2
		  AND alc.rejected_by_user = true
		  AND alc.segment_effort_id IS NOT NULL
	`, activityID, userID); err != nil {
		return fmt.Errorf("enrichStreamLifts: null-out rejected audit segment_effort_id: %w", err)
	}
	if _, err := tx.Exec(ctx, `
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

	// 5. Persist Stage-1 + Stage-2 results inside the Tx.
	//
	// Phase-4 critic F2 (Variante B) — reject persistence. Before each
	// SnapAction-Snap insert (canonical or suppressed-merge) we ask the DB
	// whether a soft-rejected audit-row (any source) already covers the
	// same window (boundary deltas ≤ 5 s). When yes, the entire insert is
	// skipped — both the segment_effort and the audit-row — so the user's
	// reject survives re-enrichment instead of being overwritten by a fresh
	// canonical row.
	//
	// N5b fix (Codex finding): this pre-check uses IsRejectedAnySourceInWindow
	// (symmetric, source-agnostic) rather than IsOSMRejectedInWindow
	// (source='osm' only). The asymmetric helper would let a heuristic-
	// rejected window pass this guard, insert a fresh segment_effort, and
	// then have insertAuditCandidateTx's downstream symmetric guard skip
	// the audit-row — leaving an orphan segment_effort that
	// recomputeLiftStats counts via the segment_efforts arm (NOT EXISTS a
	// rejected audit row pointing at it → it gets counted), bypassing the
	// user's reject. The symmetric guard at the early-exit shuts that gap.
	osmMatches := 0
	for _, res := range stage2 {
		ride := rides[res.RideIdx]
		switch res.Action {
		case lifts.SnapActionSnap:
			osm := osmCandidates[res.OSMCandidateIdx]
			// Pre-check the per-ride window — the audit-row writes
			// ride.Ride.StartedAt/EndedAt, so that is the window the
			// rejected-window predicate is evaluated against.
			suppressed, err := db.IsRejectedAnySourceInWindow(ctx, activityID, ride.Ride.StartedAt, ride.Ride.EndedAt)
			if err != nil {
				return fmt.Errorf("enrichStreamLifts: check rejected window: %w", err)
			}
			if suppressed {
				continue
			}
			if res.SuppressedByMerge {
				// Audit-only candidate referencing the canonical merge's
				// segment_effort_id. The canonical row creates the
				// segment_effort itself; we look it up by the merged
				// (activity, started_at) key. The lookup runs inside the
				// Tx so the just-inserted canonical row is visible.
				canonicalEffortID, err := lookupCanonicalSegmentEffortIDTx(ctx, tx, activityID, rides[res.MergedRideIndices[0]].Ride.StartedAt)
				if err != nil {
					return fmt.Errorf("enrichStreamLifts: lookup canonical merge id: %w", err)
				}
				if err := insertAuditCandidateTx(ctx, db, tx, userID, activityID, ride.Ride, canonicalEffortID); err != nil {
					return fmt.Errorf("enrichStreamLifts: insert suppressed audit candidate: %w", err)
				}
				continue
			}
			// Canonical Stage-1 snap or canonical Stage-2 merge.
			startedAt, durationS, ascentM, distanceM := canonicalRideMetrics(rides, res, osm)
			effortID, err := insertSegmentEffortTx(ctx, tx, userID, activityID, osm,
				startedAt, durationS, distanceM, ascentM, res.IsReverse,
				true /*isLiftSegmentMatch*/, false /*isMethodAOnly*/, res.IsCableHoldMerge)
			if err != nil {
				return fmt.Errorf("enrichStreamLifts: insert segment_effort: %w", err)
			}
			if err := insertAuditCandidateTx(ctx, db, tx, userID, activityID, ride.Ride, effortID); err != nil {
				return fmt.Errorf("enrichStreamLifts: insert audit candidate: %w", err)
			}
			osmMatches++

		case lifts.SnapActionPassthrough:
			if err := insertHeuristicCandidateTx(ctx, tx, userID, activityID, ride.Ride); err != nil {
				return fmt.Errorf("enrichStreamLifts: insert heuristic candidate: %w", err)
			}

		case lifts.SnapActionMultiRideGroup:
			// Stage 2 should have rewritten this; if anything slips through
			// treat as heuristic-only (defensive).
			if err := insertHeuristicCandidateTx(ctx, tx, userID, activityID, ride.Ride); err != nil {
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
		ok, err := insertStage3SegmentEffortTx(ctx, db, tx, userID, activityID, osm)
		if err != nil {
			return fmt.Errorf("enrichStreamLifts: stage 3 insert: %w", err)
		}
		if ok {
			osmMatches++
		}
	}

	// Commit the cleanup + rebuild atomically. RecomputeLiftStats runs on
	// db.Pool (its own internal Tx) afterwards: a recompute failure here
	// leaves the candidates / segment_efforts in their correct rebuilt
	// state and only activities.lift_* are stale, recoverable by re-running
	// enrichment.
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("enrichStreamLifts: commit tx: %w", err)
	}

	if err := db.RecomputeLiftStats(ctx, userID, activityID); err != nil {
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

// insertSegmentEffortTx writes one Stage-1 / Stage-2 / Stage-3 segment_efforts
// row with the Phase 4c audit flags. Returns the inserted id (UUID); the
// composite-PK hypertable design means we generate the id client-side and
// thread it back to the audit-candidate insert.
//
// Tx-bound: the INSERT must be visible to subsequent statements in the same
// Tx (lookupCanonicalSegmentEffortIDTx + ON CONFLICT semantics for
// insertAuditCandidateTx) and must roll back together with the rest of the
// rebuild on any failure.
func insertSegmentEffortTx(
	ctx context.Context,
	tx pgx.Tx,
	userID, activityID string,
	osm lifts.OSMLiftCandidate,
	startedAt time.Time,
	durationS int,
	distanceM, ascentM float64,
	isReverse, isLiftSegmentMatch, isMethodAOnly, isCableHoldMerge bool,
) (string, error) {
	effortID := uuid.NewString()
	_, err := tx.Exec(ctx, `
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

// insertAuditCandidateTx writes the activity_lift_candidates row that
// mirrors a Stage-1/2/3 segment_effort. The candidate carries source='osm',
// confirmed=true and segment_effort_id pointing at the effort. This row
// is purely for the read-side unified API; it is excluded from the
// recomputeLiftStats heuristic-only count by the partial-index dedup.
//
// Symmetric soft-reject pre-check (paired with InsertHeuristicCandidateIfNotRejected
// and the comment block above it in enrichment_db.go): before writing the
// audit-row we query for ANY soft-rejected row (regardless of source) within
// ±5 s of the new boundaries. If one exists we return nil silently — the
// rejected row stays as-is, no audit-row is upserted, and the user's reject
// persists across re-enrichment AND across OSM data imports.
//
// Why the pre-check lives here and not just at the SnapAction-entry guard
// in enrichStreamLifts: the entry guard is now also symmetric (N5b fix),
// but the upsert-site guard remains as a second line of defense. The
// failure mode it guards against is: a rejected heuristic row falls
// through both an asymmetric pre-check upstream (legacy callers, future
// callers) and the ON CONFLICT (activity_id, started_at) DO UPDATE flips
// the rejected heuristic row's source to 'osm' while keeping
// rejected_by_user=true — a once-rejected window silently poisons all
// future detection through the OSM path. Codex GPT-5.5-xhigh adversarial-
// review surfaced this as F14 on commit 2062b1465 in branch
// feature/lift-detection-go. Putting the symmetric guard at the upsert site
// makes the bug unreachable regardless of which caller path arrives here.
//
// Tx contract: the INSERT runs on tx so it commits atomically with the
// rest of the rebuild. The IsRejectedAnySourceInWindow read intentionally
// runs on db.Pool: rejected_by_user rows are preserved across the Tx's
// destructive cleanup (the DELETE filters NOT alc.rejected_by_user) and
// are not modified inside the Tx, so a separate-connection
// READ COMMITTED snapshot returns the same rows the Tx would see.
func insertAuditCandidateTx(
	ctx context.Context,
	db *database.DB,
	tx pgx.Tx,
	userID, activityID string,
	ride lifts.LiftRide,
	segmentEffortID string,
) error {
	suppressed, err := db.IsRejectedAnySourceInWindow(ctx, activityID, ride.StartedAt, ride.EndedAt)
	if err != nil {
		return fmt.Errorf("check rejected window: %w", err)
	}
	if suppressed {
		return nil
	}
	_, err = tx.Exec(ctx, `
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

// insertHeuristicCandidateTx writes a heuristic-only candidate
// (source='heuristic', segment_effort_id=NULL, confirmed=false) for a ride
// that did not snap to any OSM lift. The SQL must run inside the
// destructive-cleanup Tx so that the INSERT (and the in-Tx-only DELETEs
// that just preceded it) commit atomically; that requires routing through
// tx.Exec rather than db.Pool.Exec.
//
// The SQL is intentionally a verbatim mirror of
// database.InsertHeuristicCandidateIfNotRejected — same NOT EXISTS reject
// filter, same ±5 s boundary tolerance, same multi-tenant user_id guard,
// same ON CONFLICT (activity_id, started_at) DO NOTHING. When the
// canonical helper changes (boundary tolerance, reject-filter shape) this
// duplicate must be updated to match. The duplication is the cost of
// keeping the destructive cleanup atomic without a Tx variant of the
// canonical helper in the database package (parallel cluster N4+N7+N8
// owns enrichment_db.go in this branch; once a Tx variant of
// InsertHeuristicCandidateIfNotRejected becomes available post-merge,
// this inline SQL should be replaced with that call).
//
// Symmetric reject-filter (paired with insertAuditCandidateTx and the
// comment block above database.InsertHeuristicCandidateIfNotRejected): the
// NOT EXISTS sub-query checks rejected_by_user = true WITHOUT a source
// filter, so a rejected row of either source ('osm' or 'heuristic')
// blocks a fresh heuristic re-insert in the same window.
func insertHeuristicCandidateTx(
	ctx context.Context,
	tx pgx.Tx,
	userID, activityID string,
	ride lifts.LiftRide,
) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO activity_lift_candidates
			(activity_id, started_at, ended_at, duration_s, ascent_m, descent_m, distance_m,
			 confidence, confirmed, source)
		SELECT $1, $2, $3, $4, $5, $6, $7, $8, false, 'heuristic'
		WHERE EXISTS (SELECT 1 FROM activities a WHERE a.id = $1 AND a.user_id = $9)
		  AND NOT EXISTS (
			SELECT 1 FROM activity_lift_candidates rej
			WHERE rej.activity_id = $1
			  AND rej.rejected_by_user = true
			  AND ABS(EXTRACT(EPOCH FROM (rej.started_at - $2::timestamptz))) <= 5
			  AND ABS(EXTRACT(EPOCH FROM (rej.ended_at   - $3::timestamptz))) <= 5
		  )
		ON CONFLICT (activity_id, started_at) DO NOTHING
	`, activityID, ride.StartedAt, ride.EndedAt, ride.DurationS,
		ride.AscentM, ride.DescentM, ride.DistanceM, ride.Confidence, userID)
	if err != nil {
		return fmt.Errorf("insert: %w", err)
	}
	return nil
}

// lookupCanonicalSegmentEffortIDTx returns the id of the segment_effort
// that the canonical Stage-2 ride just inserted (matched by activity_id +
// started_at). Runs on the caller-supplied Tx so the just-inserted
// canonical row is visible (the destructive cleanup + rebuild commits
// atomically; a separate-connection lookup would not see the row yet).
// Returns the empty string when not found — caller treats that as "no
// canonical row" and skips the audit insert.
func lookupCanonicalSegmentEffortIDTx(ctx context.Context, tx pgx.Tx, activityID string, startedAt time.Time) (string, error) {
	var id string
	err := tx.QueryRow(ctx, `
		SELECT id FROM segment_efforts
		WHERE activity_id = $1 AND started_at = $2 AND is_cable_hold_merge = true
		ORDER BY created_at DESC LIMIT 1
	`, activityID, startedAt).Scan(&id)
	if err != nil {
		return "", fmt.Errorf("lookup: %w", err)
	}
	return id, nil
}

// insertStage3SegmentEffortTx runs the legacy Method-A per-lift time-
// window SQL: extracts samples within 75m of the OSM lift, derives
// duration/ascent/bearing, then INSERTs one segment_efforts row
// (is_method_a_only=true) plus the audit-only candidate. Returns true
// when a row was successfully inserted (false for "below 30s duration
// floor" or "fewer than 2 samples in window" — both legitimate skip
// cases).
//
// Tx-bound write path: insertSegmentEffortTx + insertAuditCandidateTx
// route through tx so the inserts roll back together with the rest of
// the rebuild on any failure. The activity_streams + osm_lifts read
// runs on db.Pool — pre-existing data, not modified inside the Tx.
//
// N5b fix: the rejected-window pre-check uses IsRejectedAnySourceInWindow
// (symmetric, source-agnostic) rather than IsOSMRejectedInWindow. See
// the matching N5b comment on the SnapAction-Snap pre-check above —
// same orphan-segment_effort failure mode applied to Stage-3.
func insertStage3SegmentEffortTx(
	ctx context.Context,
	db *database.DB,
	tx pgx.Tx,
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
	// Phase-4 critic F2 (Variante B): pre-check the time-window against
	// soft-rejected audit-rows. Stage 3 is the method-A coverage gap
	// path; the audit-row writes startedAt/endedAt as derived here, so
	// the rejected-window predicate uses those same boundaries.
	suppressed, err := db.IsRejectedAnySourceInWindow(ctx, activityID, startedAt, endedAt)
	if err != nil {
		return false, fmt.Errorf("check rejected window: %w", err)
	}
	if suppressed {
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

	effortID, err := insertSegmentEffortTx(ctx, tx, userID, activityID, osm,
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
	if err := insertAuditCandidateTx(ctx, db, tx, userID, activityID, auditRide, effortID); err != nil {
		return false, fmt.Errorf("insert audit candidate: %w", err)
	}
	return true, nil
}
