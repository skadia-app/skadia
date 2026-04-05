// All handlers in this file MUST be registered inside the admin middleware group.
// They do NOT perform their own admin role check — they rely on RequireAdmin() middleware.
package handler

import (
	"context"
	"net/http"

	"github.com/skadia-app/skadia/services/api/internal/httputil"
	"github.com/skadia-app/skadia/services/api/internal/shutdown"
)

// RecalcAllUsersBestEfforts starts a background job that recomputes best efforts
// for every user. Uses a fixed "system" user ID as job guard so only one run can
// be active at a time regardless of which admin triggers it.
//
// POST /api/v1/admin/best-efforts/recalc-all
func (h *Handler) RecalcAllUsersBestEfforts(w http.ResponseWriter, r *http.Request) {
	if h.BestEfforts == nil {
		httputil.WriteError(w, http.StatusServiceUnavailable, "Service Unavailable", "best efforts service not available", r.URL.Path)
		return
	}

	entityName := "Bestleistungen (alle User)"
	// Use fixed "system" user so the job guard prevents parallel runs regardless of which admin triggers it.
	systemUser := "00000000-0000-0000-0000-000000000000"
	jobID, created, err := h.DB.InsertBackgroundJobGuarded(r.Context(), systemUser,
		"recalc_best_efforts_all", nil, &entityName, 0)
	if err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, "Job creation failed", err.Error(), r.URL.Path)
		return
	}
	if !created {
		httputil.WriteJSON(w, http.StatusOK, map[string]any{
			"status": "already_running",
			"job_id": jobID,
		})
		return
	}

	svc := h.BestEfforts
	shutdown.Go(jobID, func(ctx context.Context) {
		var done int
		err := h.DB.ForEachUser(ctx, func(userID string) error {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			_ = svc.RecalcAll(ctx, userID, nil)
			done++
			_ = h.DB.UpdateBackgroundJobProgress(ctx, jobID, int64(done))
			return nil
		})
		failed := err != nil
		errText := ""
		if failed {
			errText = err.Error()
		}
		_ = h.DB.CompleteBackgroundJob(context.Background(), jobID, failed, errText)
	})

	httputil.WriteJSON(w, http.StatusAccepted, map[string]any{
		"status": "started",
		"job_id": jobID,
	})
}
