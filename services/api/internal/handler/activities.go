package handler

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/skadia-app/skadia/services/api/internal/middleware"
	"github.com/skadia-app/skadia/services/api/internal/model"
)

func (h *Handler) ListActivities(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())

	cursor := r.URL.Query().Get("cursor")
	limit := 20
	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 && parsed <= 100 {
			limit = parsed
		}
	}

	sortBy := r.URL.Query().Get("sort_by")
	if sortBy == "" {
		sortBy = "started_at"
	}
	sortDir := r.URL.Query().Get("sort_dir")
	if sortDir == "" {
		sortDir = "desc"
	}

	// Parse filter parameters
	var filters model.ActivityFilters
	if st := r.URL.Query().Get("sport_type"); st != "" {
		filters.SportTypes = strings.Split(st, ",")
	}
	if sub := r.URL.Query().Get("sub_type"); sub != "" {
		filters.SubTypes = strings.Split(sub, ",")
	}
	filters.DateFrom = r.URL.Query().Get("date_from")
	filters.DateTo = r.URL.Query().Get("date_to")
	if v := r.URL.Query().Get("is_indoor"); v != "" {
		b := v == "true"
		filters.IsIndoor = &b
	}
	if v := r.URL.Query().Get("has_gps"); v != "" {
		b := v == "true"
		filters.HasGPS = &b
	}
	filters.GearID = r.URL.Query().Get("gear_id")
	filters.Search = r.URL.Query().Get("search")

	list, err := h.DB.ListActivities(r.Context(), userID, cursor, limit, sortBy, sortDir, filters)
	if err != nil {
		slog.Error("list activities failed", "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to list activities", r.URL.Path)
		return
	}

	writeJSON(w, http.StatusOK, list)
}

func (h *Handler) GetActivity(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	activity, err := h.DB.GetActivity(r.Context(), id)
	if err != nil {
		slog.Error("get activity failed", "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to get activity", r.URL.Path)
		return
	}
	if activity == nil {
		writeError(w, http.StatusNotFound, "Not Found", "activity not found", r.URL.Path)
		return
	}

	writeJSON(w, http.StatusOK, activity)
}

func (h *Handler) UpdateActivity(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var update model.ActivityUpdate
	if err := json.NewDecoder(r.Body).Decode(&update); err != nil {
		writeError(w, http.StatusBadRequest, "Bad Request", "invalid JSON body", r.URL.Path)
		return
	}

	activity, err := h.DB.UpdateActivity(r.Context(), id, &update)
	if err != nil {
		slog.Error("update activity failed", "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to update activity", r.URL.Path)
		return
	}
	if activity == nil {
		writeError(w, http.StatusNotFound, "Not Found", "activity not found", r.URL.Path)
		return
	}

	writeJSON(w, http.StatusOK, activity)
}

func (h *Handler) DeleteActivity(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	if err := h.DB.DeleteActivity(r.Context(), id); err != nil {
		if err.Error() == "activity not found" {
			writeError(w, http.StatusNotFound, "Not Found", "activity not found", r.URL.Path)
			return
		}
		slog.Error("delete activity failed", "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to delete activity", r.URL.Path)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
