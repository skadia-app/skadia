package handler

import (
	"context"
	"errors"
	"fmt"
	"hash/fnv"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/httputil"
	"github.com/skadia-app/skadia/services/api/internal/middleware"
)

// errLapSegmentCannotBeReversed is returned when an attempt is made to reverse
// a lap-type segment. Lap segments are closed loops with no direction concept;
// reversing them would corrupt leaderboard is_reverse-partitioned rankings.
var errLapSegmentCannotBeReversed = errors.New("lap segments cannot be reversed")

// ReverseSegment flips a segment's active version geometry and all effort
// is_reverse flags atomically (including leaderboard rank-recompute inside
// the same tx). Owner OR admin may reverse.
//
// POST /api/v1/segments/{id}/reverse → 200 {"efforts_updated": N} | 400 | 403 | 404 | 500
func (h *Handler) ReverseSegment(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	segID := chi.URLParam(r, "id")
	if segID == "" {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "missing segment id", r.URL.Path)
		return
	}
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	isAdmin := middleware.GetIsAdmin(ctx)

	effortsUpdated, err := reverseSegmentTx(ctx, h.DB, segID, userID, isAdmin)
	if err != nil {
		switch {
		case errors.Is(err, database.ErrSegmentNotFound):
			httputil.WriteError(w, http.StatusNotFound, "Not Found", "segment not found", r.URL.Path)
		case errors.Is(err, database.ErrSegmentNotAuthorized):
			httputil.WriteError(w, http.StatusForbidden, "Forbidden", "not owner", r.URL.Path)
		case errors.Is(err, errLapSegmentCannotBeReversed):
			httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "lap segments cannot be reversed", r.URL.Path)
		default:
			// Distinguish advisory-lock timeout / serialization failures from generic
			// errors so the client knows a retry is safe.
			//   55P03 = lock_not_available (pg_advisory_xact_lock with lock_timeout set,
			//           or explicit pg_try_advisory_xact_lock returning false)
			//   40001 = serialization_failure (snapshot conflict under REPEATABLE READ /
			//           SERIALIZABLE isolation)
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) {
				switch pgErr.Code {
				case "55P03", "40001":
					httputil.WriteError(w, http.StatusServiceUnavailable, "Service Unavailable", "lock conflict, please try again", r.URL.Path)
					return
				}
			}
			slog.Error("reverseSegmentTx failed", "segment_id", segID, "error", err)
			httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "reverse failed", r.URL.Path)
		}
		return
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]int{"efforts_updated": effortsUpdated})
}

// reverseSegmentTx performs the atomic flip inside a single transaction.
// Owner-or-admin check is done TOCTOU-safely inside the same tx via a
// SELECT ... FOR UPDATE, so a concurrent DeleteSegment / UpdateSegment cannot
// win the race. Guarded by pg_advisory_xact_lock to serialize concurrent
// reverse-calls on the same segment. Returns number of segment_efforts rows
// updated.
//
// Schema reality (verified 2026-04-20 against migrate/migrations/000032):
// segments has no geometry columns — geometry lives in segment_versions.track
// with start_point / end_point. A reverse inserts a NEW segment_versions row
// with ST_Reverse(track) and ST_EndPoint(old.track) as the new start_point
// (and vice versa). All subsequent lookups that pick the active version via
// ORDER BY version_number DESC LIMIT 1 automatically see the reversed track.
//
// NOTE: segments has no updated_at column (verified against migration 000032 and
// all subsequent ALTER TABLE segments migrations). No timestamp bump on the parent
// row is performed; the new segment_versions row has its own created_at.
func reverseSegmentTx(ctx context.Context, db *database.DB, segID, userID string, isAdmin bool) (int, error) {
	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("begin: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	// Advisory lock: segment-scoped, namespaced via FNV-1a("segment.reverse." + id).
	// pg_advisory_xact_lock releases on commit/rollback — no defer-unlock needed.
	lockKey := advisoryLockKeyForSegmentReverse(segID)
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock($1)`, lockKey); err != nil {
		return 0, fmt.Errorf("advisory lock: %w", err)
	}

	// TOCTOU-safe owner-or-admin check in the same tx: SELECT created_by FOR
	// UPDATE holds a row-lock through the subsequent INSERT/UPDATEs, so a
	// concurrent DeleteSegment / UpdateSegment cannot win the race.
	var ownerID, segmentType string
	if err := tx.QueryRow(ctx,
		`SELECT created_by, segment_type FROM segments WHERE id = $1 FOR UPDATE`, segID,
	).Scan(&ownerID, &segmentType); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, database.ErrSegmentNotFound
		}
		return 0, fmt.Errorf("select segment: %w", err)
	}
	if !isAdmin && ownerID != userID {
		return 0, database.ErrSegmentNotAuthorized
	}
	// Lap segments are closed loops with no meaningful direction concept.
	// Reversing them would corrupt leaderboard direction-partitioning.
	if segmentType == "lap" {
		return 0, errLapSegmentCannotBeReversed
	}

	// Insert a new segment_versions row with ST_Reverse(track) and swapped
	// start_point / end_point. Copies the rest of the active version (including
	// segment_type_meta which is a column on segment_versions — verified per
	// database/segments.go createNewSegmentVersion at line 446). The parent
	// segments row's effort_count/athlete_count are untouched by the geometry flip.
	if _, err := tx.Exec(ctx, `
		INSERT INTO segment_versions (segment_id, version_number, track, start_point, end_point,
		                              valid_from, created_by, segment_type_meta)
		SELECT sv.segment_id,
		       (SELECT COALESCE(MAX(version_number), 0) + 1 FROM segment_versions WHERE segment_id = $1),
		       ST_Reverse(sv.track),
		       sv.end_point,
		       sv.start_point,
		       CURRENT_DATE,
		       $2,
		       sv.segment_type_meta
		FROM segment_versions sv
		WHERE sv.segment_id = $1
		ORDER BY sv.version_number DESC
		LIMIT 1`, segID, userID); err != nil {
		return 0, fmt.Errorf("insert reversed version: %w", err)
	}

	// Flip is_reverse on all efforts for this segment.
	ct, err := tx.Exec(ctx, `UPDATE segment_efforts SET is_reverse = NOT is_reverse WHERE segment_id = $1`, segID)
	if err != nil {
		return 0, fmt.Errorf("update efforts: %w", err)
	}

	// Rank-recompute — literal copy of the two queries from
	// database/segments.go:678 (UpdateSegmentEffortRanks). Copied inline because
	// that method uses db.Pool.Exec directly and has no Tx-variant. A future
	// refactor could introduce an Execer parameter so it accepts either pool
	// or tx; for now we duplicate to keep the change surface small and make the
	// reverse atomic (geometry + is_reverse + ranks either all commit or all roll back).
	//
	// IF YOU CHANGE EITHER QUERY HERE, mirror it in
	// database/segments.go:UpdateSegmentEffortRanks (and vice-versa).
	if _, err := tx.Exec(ctx, `
		WITH
		global_ranked AS (
			SELECT se.id, se.started_at,
			       ROW_NUMBER() OVER (
			           PARTITION BY se.sport_type, se.is_reverse
			           ORDER BY se.elapsed_time_s ASC
			       ) AS rk,
			       (ROW_NUMBER() OVER (
			           PARTITION BY se.sport_type, se.is_reverse
			           ORDER BY se.elapsed_time_s ASC
			       ) = 1) AS is_record
			FROM segment_efforts se
			JOIN activities a ON a.id = se.activity_id AND a.visibility = 'public' AND NOT a.is_group_secondary
			WHERE se.segment_id = $1 AND se.in_leaderboard = true
		),
		personal_ranked AS (
			SELECT id, started_at,
			       ROW_NUMBER() OVER (
			           PARTITION BY sport_type, is_reverse, user_id
			           ORDER BY elapsed_time_s ASC
			       ) AS user_rk
			FROM segment_efforts
			WHERE segment_id = $1 AND in_leaderboard = true
		)
		UPDATE segment_efforts se
		SET rank_overall      = gr.rk,
		    is_personal_best  = (pr.user_rk = 1),
		    personal_rank     = CASE WHEN pr.user_rk <= 3 THEN pr.user_rk::smallint ELSE NULL END,
		    is_segment_record = COALESCE(gr.is_record, false)
		FROM personal_ranked pr
		LEFT JOIN global_ranked gr ON gr.id = pr.id AND gr.started_at = pr.started_at
		WHERE se.id = pr.id AND se.started_at = pr.started_at`, segID); err != nil {
		return 0, fmt.Errorf("recompute ranks: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		UPDATE segments SET
		    effort_count  = (SELECT COUNT(*) FROM segment_efforts se
		                     JOIN activities a ON a.id = se.activity_id AND a.visibility = 'public' AND NOT a.is_group_secondary
		                     WHERE se.segment_id = $1),
		    athlete_count = (SELECT COUNT(DISTINCT se.user_id) FROM segment_efforts se
		                     JOIN activities a ON a.id = se.activity_id AND a.visibility = 'public' AND NOT a.is_group_secondary
		                     WHERE se.segment_id = $1)
		WHERE id = $1`, segID); err != nil {
		return 0, fmt.Errorf("update segment counts: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("commit: %w", err)
	}
	return int(ct.RowsAffected()), nil
}

// advisoryLockKeyForSegmentReverse derives a stable int64 from the segment UUID,
// namespaced with "segment.reverse." so it cannot collide with advisory-locks
// used elsewhere (fusion_merge, shadowfs_settings, commute_rule, activity_edit_ops).
func advisoryLockKeyForSegmentReverse(segmentID string) int64 {
	h := fnv.New64a()
	h.Write([]byte("segment.reverse."))
	h.Write([]byte(segmentID))
	return int64(h.Sum64())
}
