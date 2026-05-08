// All handlers in this file MUST be registered inside the admin middleware group.
// They do NOT perform their own admin role check — they rely on RequireAdmin() middleware.
package handler

import (
	"context"
	"fmt"
	"net/http"

	"github.com/skadia-app/skadia/services/api/internal/httputil"
	"github.com/skadia-app/skadia/services/api/internal/shutdown"
)

type allUsersBestEffortsDB interface {
	ForEachUser(ctx context.Context, fn func(userID string) error) error
	UpdateBackgroundJobProgress(ctx context.Context, jobID string, current int64) error
	CompleteBackgroundJob(ctx context.Context, jobID string, failed bool, errorText string) error
}

type allUsersBestEffortsService interface {
	RecalcAll(ctx context.Context, userID string, progressFn func(done, total int)) error
}

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
		runAllUsersBestEffortsRecalc(ctx, h.DB, svc, jobID)
	})

	httputil.WriteJSON(w, http.StatusAccepted, map[string]any{
		"status": "started",
		"job_id": jobID,
	})
}

func runAllUsersBestEffortsRecalc(ctx context.Context, db allUsersBestEffortsDB, svc allUsersBestEffortsService, jobID string) {
	var done int
	var failedUsers int
	var firstUserErr string

	err := db.ForEachUser(ctx, func(userID string) error {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err := svc.RecalcAll(ctx, userID, nil); err != nil {
			failedUsers++
			if firstUserErr == "" {
				firstUserErr = fmt.Sprintf("%s: %v", userID, err)
			}
			return nil
		}
		done++
		_ = db.UpdateBackgroundJobProgress(ctx, jobID, int64(done))
		return nil
	})

	failed := err != nil || failedUsers > 0
	errText := allUsersBestEffortsErrorText(failedUsers, firstUserErr, err)
	_ = db.CompleteBackgroundJob(context.Background(), jobID, failed, errText)
}

func allUsersBestEffortsErrorText(failedUsers int, firstUserErr string, iteratorErr error) string {
	switch {
	case failedUsers == 0 && iteratorErr == nil:
		return ""
	case failedUsers == 0:
		return iteratorErr.Error()
	}

	userSummary := fmt.Sprintf("%d user recalcs failed; first error: %s", failedUsers, firstUserErr)
	if failedUsers == 1 {
		userSummary = fmt.Sprintf("1 user recalc failed; first error: %s", firstUserErr)
	}
	if iteratorErr != nil {
		return fmt.Sprintf("%s; iteration error: %v", userSummary, iteratorErr)
	}
	return userSummary
}
