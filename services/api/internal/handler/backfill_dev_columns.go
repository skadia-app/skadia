package handler

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/skadia-app/skadia/services/api/internal/httputil"
)

// BackfillDeveloperFieldColumns promotes Suunto developer-field values from
// activity_streams.developer_fields JSONB into dedicated activity columns.
//
// Stream columns (radar_vehicle_count, radar_nearest_range_m, etc.) cannot be
// backfilled via direct SQL UPDATE because activity_streams uses TimescaleDB
// columnar compression; UPDATEs on compressed chunks are not supported.
// For those columns, a full recompute from the original FIT file is needed.
//
// POST /api/v1/admin/backfill-developer-field-columns
// Optional query param: ?user_id= to restrict to a single user.
func (h *Handler) BackfillDeveloperFieldColumns(w http.ResponseWriter, r *http.Request) {
	_, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}

	ctx := r.Context()

	// Resolve the set of user IDs to process.
	targetUserID := r.URL.Query().Get("user_id")
	var userIDs []string
	if targetUserID != "" {
		userIDs = []string{targetUserID}
	} else {
		var err error
		userIDs, err = h.DB.ListActiveUserIDs(ctx)
		if err != nil {
			slog.Error("backfill dev columns: list active users failed", "error", err)
			httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to list users", r.URL.Path)
			return
		}
	}

	if len(userIDs) == 0 {
		httputil.WriteJSON(w, http.StatusOK, map[string]any{"updated_activities": 0})
		return
	}

	// Return immediately; run backfill in background.
	httputil.WriteJSON(w, http.StatusAccepted, map[string]any{
		"users_queued": len(userIDs),
		"note":         "stream columns (radar, running_smoothness, glucose) require a full recompute from FIT — only activity-level Suunto columns are backfilled here",
	})

	go func() {
		bgCtx := context.Background()
		totalUpdated := int64(0)

		for _, uid := range userIDs {
			updated, err := backfillSuuntoColumnsForUser(bgCtx, h, uid)
			if err != nil {
				slog.Error("backfill dev columns: suunto update failed",
					"user_id", uid, "error", err)
				continue
			}
			totalUpdated += updated
		}

		slog.Info("backfill dev columns: completed",
			"users", len(userIDs),
			"updated_activities", totalUpdated,
		)
	}()
}

// backfillSuuntoColumnsForUser runs the Suunto CTE-based UPDATE for a single user.
// Returns the number of activity rows updated.
func backfillSuuntoColumnsForUser(ctx context.Context, h *Handler, userID string) (int64, error) {
	tag, err := h.DB.Pool.Exec(ctx, `
		WITH suunto_vals AS (
		    SELECT activity_id,
		        (array_agg(developer_fields->>'estimated_vo2_max' ORDER BY time DESC)
		            FILTER (WHERE developer_fields->>'estimated_vo2_max' IS NOT NULL))[1]::real AS vo2max,
		        (array_agg(developer_fields->>'peak_epoc' ORDER BY time DESC)
		            FILTER (WHERE developer_fields->>'peak_epoc' IS NOT NULL))[1]::real AS epoc,
		        (array_agg(developer_fields->>'recovery_time' ORDER BY time DESC)
		            FILTER (WHERE developer_fields->>'recovery_time' IS NOT NULL))[1]::real AS recovery
		    FROM activity_streams
		    WHERE activity_id IN (SELECT id FROM activities WHERE user_id = $1)
		      AND developer_fields IS NOT NULL
		    GROUP BY activity_id
		)
		UPDATE activities a SET
		    suunto_vo2max        = sv.vo2max,
		    suunto_peak_epoc     = sv.epoc,
		    suunto_recovery_time_h = sv.recovery
		FROM suunto_vals sv
		WHERE a.id = sv.activity_id
		  AND (sv.vo2max IS NOT NULL OR sv.epoc IS NOT NULL OR sv.recovery IS NOT NULL)
	`, userID)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}
