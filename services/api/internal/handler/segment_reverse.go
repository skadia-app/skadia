package handler

import (
	"context"
	"errors"
	"fmt"
	"hash/fnv"
	"log/slog"
	"net/http"
	"time"

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

// ReverseSegment creates a reversed active segment version atomically. Existing
// efforts remain frozen on the version and direction that originally matched.
// Owner OR admin may reverse.
//
// POST /api/v1/segments/{id}/reverse → 200 {"efforts_updated": 0} | 400 | 403 | 404 | 500
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

// reverseSegmentTx performs the atomic geometry flip inside a single transaction.
// Owner-or-admin check is done TOCTOU-safely inside the same tx via a
// SELECT ... FOR UPDATE, so a concurrent DeleteSegment / UpdateSegment cannot
// win the race. Guarded by pg_advisory_xact_lock to serialize concurrent
// reverse-calls on the same segment. Existing segment_efforts are not updated;
// the return value remains part of the HTTP contract and is currently always 0.
//
// Schema reality (verified 2026-04-20 against migrate/migrations/000032):
// segments has no geometry columns — geometry lives in segment_versions.track
// with start_point / end_point. A reverse inserts a NEW segment_versions row
// with ST_Reverse(track) and ST_EndPoint(old.track) as the new start_point
// (and vice versa). Subsequent current-display lookups see the reversed track
// through the course_gps_v1 validity selector.
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

	operationID := fmt.Sprintf("reverse:%s:%d:%d", segID, lockKey, time.Now().UnixNano())
	if _, _, err := db.ReverseActiveSegmentVersionTx(ctx, tx, segID, userID, operationID); err != nil {
		return 0, err
	}

	if err := database.UpdateSegmentEffortRanksTx(ctx, tx, segID); err != nil {
		return 0, fmt.Errorf("recompute ranks: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("commit: %w", err)
	}
	return 0, nil
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
