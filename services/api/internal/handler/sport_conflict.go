package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"

	"github.com/skadia-app/skadia/services/api/internal/middleware"
	"github.com/skadia-app/skadia/services/api/internal/sport"
	"github.com/go-chi/chi/v5"
)

type resolveSportConflictRequest struct {
	SportType string `json:"sport_type"`
}

// ResolveSportConflict handles PATCH /api/v1/activities/{id}/resolve-sport-conflict
func (h *Handler) ResolveSportConflict(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "Unauthorized", "no user ID in context", r.URL.Path)
		return
	}
	activityID := chi.URLParam(r, "id")

	// Ownership + existence check
	activity, err := h.DB.GetActivity(r.Context(), activityID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to load activity", r.URL.Path)
		return
	}
	if activity == nil || activity.UserID != userID {
		writeError(w, http.StatusNotFound, "Not Found", "activity not found", r.URL.Path)
		return
	}

	// Already resolved?
	if !activity.SportConflictPending {
		writeJSON(w, http.StatusOK, map[string]string{"status": "already_resolved"})
		return
	}

	var req resolveSportConflictRequest
	if r.Body != nil {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil && err != io.EOF {
			slog.Warn("sport conflict resolve: invalid request body", "error", err)
		}
	}

	// Validate sport type against taxonomy (prevent garbage via direct API calls)
	if req.SportType != "" {
		if _, ok := sport.GetSportDef(req.SportType); !ok {
			writeError(w, http.StatusBadRequest, "Bad Request", "unknown sport type", r.URL.Path)
			return
		}
	}

	// Resolve via DB method (TOCTOU-safe: WHERE sport_conflict_pending = true)
	var sportPtr *string
	if req.SportType != "" {
		sportPtr = &req.SportType
	}
	affected, err := h.DB.ResolveSportConflict(r.Context(), activityID, userID, sportPtr)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to resolve conflict", r.URL.Path)
		return
	}
	if !affected {
		// Another request resolved first (TOCTOU)
		writeJSON(w, http.StatusOK, map[string]string{"status": "already_resolved"})
		return
	}

	// Resolve queue entry (graceful if missing)
	_ = h.DB.ResolveImportQueueByActivity(r.Context(), userID, activityID)

	// Trigger recompute if sport changed (reuses existing reprocessActivity from recompute.go)
	if req.SportType != "" && req.SportType != activity.SportType {
		// Create background_jobs entry so ActiveJobsIndicator shows progress
		entityID := activityID
		activityTitle := activityID
		if activity.Title != nil && *activity.Title != "" {
			activityTitle = *activity.Title
		} else if activity.TitleAuto != nil && *activity.TitleAuto != "" {
			activityTitle = *activity.TitleAuto
		}
		entityName := fmt.Sprintf("Sport-Konflikt: %s", activityTitle)
		jobID, jobErr := h.DB.InsertBackgroundJob(r.Context(), userID, "activity_recompute", &entityID, &entityName, 1)
		if jobErr != nil {
			slog.Warn("sport conflict: failed to create background job", "error", jobErr)
		}

		go func() {
			failed := false
			defer func() {
				if rec := recover(); rec != nil {
					slog.Error("sport conflict resolve: recompute panicked", "id", activityID, "panic", rec)
					failed = true
				}
				if jobID != "" {
					_ = h.DB.CompleteBackgroundJob(context.Background(), jobID, failed)
				}
			}()
			ctx := context.Background()
			if reErr := h.reprocessActivity(ctx, activityID, userID, nil, 0); reErr != nil {
				slog.Warn("sport conflict resolve: recompute failed", "id", activityID, "error", reErr)
				failed = true
			}
		}()
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "resolved"})
}
