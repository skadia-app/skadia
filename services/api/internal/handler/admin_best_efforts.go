// All handlers in this file MUST be registered inside the admin middleware group.
// They do NOT perform their own admin role check — they rely on RequireAdmin() middleware.
package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/skadia-app/skadia/services/api/internal/bestefforts"
	"github.com/skadia-app/skadia/services/api/internal/httputil"
	"github.com/skadia-app/skadia/services/api/internal/shutdown"
)

type allUsersBestEffortsDB interface {
	ForEachUser(ctx context.Context, fn func(userID string) error) error
	UpdateBackgroundJobProgress(ctx context.Context, jobID string, current int64) error
	UpdateBackgroundJobTotal(ctx context.Context, jobID string, total int64) error
	UpdateBackgroundJobMetadata(ctx context.Context, jobID string, metadata []byte) error
	CompleteBackgroundJob(ctx context.Context, jobID string, failed bool, errorText string) error
}

type allUsersBestEffortsService interface {
	RecalcAll(ctx context.Context, userID string, progressFn bestefforts.RecalcProgressFunc) error
	RecalcAllFromCheckpoint(ctx context.Context, userID, afterActivityID string, progressFn bestefforts.RecalcProgressFunc) error
}

type allUsersBestEffortsCheckpoint struct {
	LastUserID     string `json:"last_user_id,omitempty"`
	LastActivityID string `json:"last_activity_id,omitempty"`
	UserDone       int    `json:"user_done,omitempty"`
	UserTotal      int    `json:"user_total,omitempty"`
	DoneUsers      int    `json:"done_users"`
	TotalUsers     int    `json:"total_users"`
	FailedUsers    int    `json:"failed_users,omitempty"`
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
	runAllUsersBestEffortsRecalcFromCheckpoint(ctx, db, svc, jobID, allUsersBestEffortsCheckpoint{})
}

func runAllUsersBestEffortsRecalcFromCheckpoint(ctx context.Context, db allUsersBestEffortsDB, svc allUsersBestEffortsService, jobID string, checkpoint allUsersBestEffortsCheckpoint) {
	var users []string
	iterErr := db.ForEachUser(ctx, func(userID string) error {
		users = append(users, userID)
		return nil
	})
	totalUsers := len(users)
	_ = db.UpdateBackgroundJobTotal(ctx, jobID, int64(totalUsers))

	var doneUsers int
	var failedUsers int
	var firstUserErr string

	resumeUserID := checkpoint.LastUserID
	resumeAfterActivityID := checkpoint.LastActivityID
	for _, userID := range users {
		if ctx.Err() != nil {
			iterErr = ctx.Err()
			break
		}
		afterActivityID := ""
		if resumeUserID != "" {
			if userID != resumeUserID {
				doneUsers++
				continue
			}
			if checkpoint.UserTotal > 0 && checkpoint.UserDone >= checkpoint.UserTotal {
				doneUsers++
				resumeUserID = ""
				resumeAfterActivityID = ""
				continue
			}
			afterActivityID = resumeAfterActivityID
			resumeUserID = ""
			resumeAfterActivityID = ""
		}
		var lastProgress bestefforts.RecalcProgress
		if err := svc.RecalcAllFromCheckpoint(ctx, userID, afterActivityID, func(progress bestefforts.RecalcProgress) {
			lastProgress = progress
			updateAllUsersBestEffortsCheckpoint(ctx, db, jobID, allUsersBestEffortsCheckpoint{
				LastUserID:     userID,
				LastActivityID: progress.LastProcessedActivityID,
				UserDone:       progress.Done,
				UserTotal:      progress.Total,
				DoneUsers:      doneUsers,
				TotalUsers:     totalUsers,
				FailedUsers:    failedUsers,
			})
		}); err != nil {
			failedUsers++
			if firstUserErr == "" {
				firstUserErr = err.Error()
			}
			slog.Warn("all-users best-efforts recalc failed for user", "user_id", userID, "error", err)
		}
		doneUsers++
		_ = db.UpdateBackgroundJobProgress(ctx, jobID, int64(doneUsers))
		updateAllUsersBestEffortsCheckpoint(ctx, db, jobID, allUsersBestEffortsCheckpoint{
			LastUserID:     userID,
			LastActivityID: lastProgress.LastProcessedActivityID,
			UserDone:       lastProgress.Done,
			UserTotal:      lastProgress.Total,
			DoneUsers:      doneUsers,
			TotalUsers:     totalUsers,
			FailedUsers:    failedUsers,
		})
	}

	failed := iterErr != nil || failedUsers > 0
	errText := allUsersBestEffortsErrorText(failedUsers, firstUserErr, iterErr)
	_ = db.CompleteBackgroundJob(context.Background(), jobID, failed, errText)
}

func updateAllUsersBestEffortsCheckpoint(ctx context.Context, db allUsersBestEffortsDB, jobID string, checkpoint allUsersBestEffortsCheckpoint) {
	encoded, err := json.Marshal(checkpoint)
	if err != nil {
		slog.Warn("all-users best-efforts checkpoint marshal failed", "error", err)
		return
	}
	if err := db.UpdateBackgroundJobMetadata(ctx, jobID, encoded); err != nil {
		slog.Warn("all-users best-efforts checkpoint update failed", "job_id", jobID, "error", err)
	}
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
