package handler

import (
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/skadia-app/skadia/services/api/internal/httputil"
	"github.com/skadia-app/skadia/services/api/internal/model"
)

// requireActivity loads an activity by URL param "id" and checks ownership.
// Returns (activity, activityID, true) on success, or (nil, "", false) with error written.
func (h *Handler) requireActivity(w http.ResponseWriter, r *http.Request, userID string) (*model.Activity, string, bool) {
	activityID := chi.URLParam(r, "id")
	if activityID == "" {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "missing activity ID", r.URL.Path)
		return nil, "", false
	}
	activity, err := h.DB.GetActivity(r.Context(), activityID)
	if err != nil {
		slog.Error("load activity failed", "id", activityID, "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to load activity", r.URL.Path)
		return nil, "", false
	}
	if activity == nil || activity.UserID != userID {
		httputil.WriteError(w, http.StatusNotFound, "Not Found", "activity not found", r.URL.Path)
		return nil, "", false
	}
	return activity, activityID, true
}

// startResetJob creates a background job and starts it in a goroutine.
// Writes 202 Accepted with job_id on success.
func (h *Handler) startResetJob(
	w http.ResponseWriter, r *http.Request,
	userID, activityID string, activityName *string, jobType string,
	progressTotal int64,
	runFn func(jobID, activityID, userID string),
) {
	jobID, err := h.DB.InsertBackgroundJob(r.Context(), userID, jobType, &activityID, activityName, progressTotal)
	if err != nil {
		slog.Error("insert background job failed", "type", jobType, "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "could not create job", r.URL.Path)
		return
	}
	go runFn(jobID, activityID, userID)
	httputil.WriteJSON(w, http.StatusAccepted, map[string]string{"job_id": jobID, "status": "started"})
}
