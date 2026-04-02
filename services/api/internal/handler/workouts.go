package handler

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/skadia-app/skadia/services/api/internal/httputil"
)

// HandleListWorkouts returns all workouts for the authenticated user.
// GET /api/v1/workouts
func (h *Handler) HandleListWorkouts(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	limit, offset := httputil.ParsePagination(r, 20, 100)
	workouts, err := h.DB.ListWorkouts(r.Context(), userID, limit, offset)
	if httputil.HandleDBErr(w, r, err, "workouts not found") {
		return
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]any{"workouts": workouts})
}

// HandleGetWorkout returns a single workout with its steps.
// GET /api/v1/workouts/{id}
func (h *Handler) HandleGetWorkout(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	workoutID := chi.URLParam(r, "id")
	if workoutID == "" {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "missing workout ID", r.URL.Path)
		return
	}
	workout, steps, activities, err := h.DB.GetWorkout(r.Context(), workoutID, userID)
	if httputil.HandleDBErr(w, r, err, "workout not found") {
		return
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]any{"workout": workout, "steps": steps, "activities": activities})
}
