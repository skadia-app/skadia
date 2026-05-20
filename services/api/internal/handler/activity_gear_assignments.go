package handler

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/httputil"
	"github.com/go-chi/chi/v5"
)

type activityGearAssignmentRequest struct {
	GearID  string `json:"gear_id"`
	RoleKey string `json:"role_key,omitempty"`
}

func (h *Handler) AddActivityGearAssignment(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	activityID := chi.URLParam(r, "id")

	existing, err := h.DB.GetActivity(r.Context(), activityID)
	if err != nil {
		slog.Error("add activity gear: load activity failed", "activity_id", activityID, "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to load activity", r.URL.Path)
		return
	}
	if existing == nil || existing.UserID != userID {
		httputil.WriteError(w, http.StatusNotFound, "Not Found", "activity not found", r.URL.Path)
		return
	}

	req, ok := httputil.DecodeJSON[activityGearAssignmentRequest](w, r)
	if !ok {
		return
	}
	assignment, err := h.DB.AddActivityGearAssignment(r.Context(), userID, activityID, req.GearID, req.RoleKey)
	if err != nil {
		if errors.Is(err, database.ErrNotFound) {
			httputil.WriteError(w, http.StatusNotFound, "Not Found", "gear or activity not found", r.URL.Path)
			return
		}
		slog.Error("add activity gear failed", "activity_id", activityID, "gear_id", req.GearID, "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to add activity gear", r.URL.Path)
		return
	}
	httputil.WriteJSON(w, http.StatusCreated, assignment)
}

func (h *Handler) RemoveActivityGearAssignment(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	activityID := chi.URLParam(r, "id")
	assignmentID := chi.URLParam(r, "assignmentID")

	if err := h.DB.RemoveActivityGearAssignment(r.Context(), userID, activityID, assignmentID); err != nil {
		if errors.Is(err, database.ErrNotFound) {
			httputil.WriteError(w, http.StatusNotFound, "Not Found", "activity gear assignment not found", r.URL.Path)
			return
		}
		slog.Error("remove activity gear failed", "activity_id", activityID, "assignment_id", assignmentID, "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to remove activity gear", r.URL.Path)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
