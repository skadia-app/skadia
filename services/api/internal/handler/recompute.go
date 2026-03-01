package handler

import (
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/skadia-app/skadia/services/api/internal/activityutil"
	"github.com/skadia-app/skadia/services/api/internal/middleware"
)

// RecomputeActivity recomputes derived fields (currently moving_time_s) for a
// single activity from its raw stream data and writes the result back to the DB.
//
// POST /api/v1/activities/{id}/recompute
func (h *Handler) RecomputeActivity(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "Unauthorized", "no user ID in context", r.URL.Path)
		return
	}

	id := chi.URLParam(r, "id")

	// Load the activity to verify ownership and get the sport type.
	activity, err := h.DB.GetActivity(r.Context(), id)
	if err != nil {
		slog.Error("recompute: get activity failed", "id", id, "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to load activity", r.URL.Path)
		return
	}
	if activity == nil {
		writeError(w, http.StatusNotFound, "Not Found", "activity not found", r.URL.Path)
		return
	}
	if activity.UserID != userID {
		writeError(w, http.StatusNotFound, "Not Found", "activity not found", r.URL.Path)
		return
	}

	// Load stream points for the computation.
	streams, err := h.DB.GetStreamPoints(r.Context(), id)
	if err != nil {
		slog.Error("recompute: get stream points failed", "id", id, "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to load stream data", r.URL.Path)
		return
	}

	// Recompute moving time.
	movingTimeS := activityutil.CalcMovingTime(streams, activity.SportType)

	// Persist the updated value.
	if err := h.DB.UpdateMovingTime(r.Context(), id, userID, movingTimeS); err != nil {
		slog.Error("recompute: update moving time failed", "id", id, "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to update activity", r.URL.Path)
		return
	}

	slog.Info("recompute: activity updated",
		"id", id,
		"sport", activity.SportType,
		"streams", len(streams),
		"moving_time_s", movingTimeS,
	)

	// Return the full updated activity.
	updated, err := h.DB.GetActivity(r.Context(), id)
	if err != nil || updated == nil {
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to reload activity", r.URL.Path)
		return
	}

	writeJSON(w, http.StatusOK, updated)
}

// RecomputeAllActivities recomputes derived fields for every activity owned by
// the authenticated user. Errors on individual activities are logged but do not
// abort the overall run.
//
// POST /api/v1/activities/recompute-all
func (h *Handler) RecomputeAllActivities(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "Unauthorized", "no user ID in context", r.URL.Path)
		return
	}

	// Fetch all activity IDs for this user.
	ids, err := h.DB.ListActivityIDsForUser(r.Context(), userID)
	if err != nil {
		slog.Error("recompute-all: list activity ids failed", "user_id", userID, "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to list activities", r.URL.Path)
		return
	}

	var processed, errors int

	for _, id := range ids {
		// Load the activity for its sport type.
		activity, err := h.DB.GetActivity(r.Context(), id)
		if err != nil || activity == nil {
			slog.Error("recompute-all: get activity failed", "id", id, "error", err)
			errors++
			continue
		}

		// Load stream points.
		streams, err := h.DB.GetStreamPoints(r.Context(), id)
		if err != nil {
			slog.Error("recompute-all: get stream points failed", "id", id, "error", err)
			errors++
			continue
		}

		// Recompute and persist.
		movingTimeS := activityutil.CalcMovingTime(streams, activity.SportType)
		if err := h.DB.UpdateMovingTime(r.Context(), id, userID, movingTimeS); err != nil {
			slog.Error("recompute-all: update moving time failed", "id", id, "error", err)
			errors++
			continue
		}

		processed++
	}

	slog.Info("recompute-all: finished",
		"user_id", userID,
		"processed", processed,
		"errors", errors,
	)

	writeJSON(w, http.StatusOK, map[string]int{
		"processed": processed,
		"errors":    errors,
	})
}
