package handler

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/skadia-app/skadia/services/api/internal/middleware"
	"github.com/skadia-app/skadia/services/api/internal/model"
)

func (h *Handler) ListGear(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	includeRetired := r.URL.Query().Get("retired") == "true"

	items, err := h.DB.ListGear(r.Context(), userID, includeRetired)
	if err != nil {
		slog.Error("list gear failed", "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to list gear", r.URL.Path)
		return
	}

	writeJSON(w, http.StatusOK, items)
}

func (h *Handler) CreateGear(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())

	var create model.GearCreate
	if err := json.NewDecoder(r.Body).Decode(&create); err != nil {
		writeError(w, http.StatusBadRequest, "Bad Request", "invalid JSON body", r.URL.Path)
		return
	}
	if create.Name == "" {
		writeError(w, http.StatusBadRequest, "Bad Request", "name is required", r.URL.Path)
		return
	}
	if create.Type == "" {
		writeError(w, http.StatusBadRequest, "Bad Request", "type is required", r.URL.Path)
		return
	}

	gear, err := h.DB.CreateGear(r.Context(), userID, &create)
	if err != nil {
		slog.Error("create gear failed", "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to create gear", r.URL.Path)
		return
	}

	writeJSON(w, http.StatusCreated, gear)
}

func (h *Handler) GetGear(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	gear, err := h.DB.GetGear(r.Context(), id)
	if err != nil {
		slog.Error("get gear failed", "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to get gear", r.URL.Path)
		return
	}
	if gear == nil {
		writeError(w, http.StatusNotFound, "Not Found", "gear not found", r.URL.Path)
		return
	}

	writeJSON(w, http.StatusOK, gear)
}

func (h *Handler) UpdateGear(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var update model.GearUpdate
	if err := json.NewDecoder(r.Body).Decode(&update); err != nil {
		writeError(w, http.StatusBadRequest, "Bad Request", "invalid JSON body", r.URL.Path)
		return
	}

	gear, err := h.DB.UpdateGear(r.Context(), id, &update)
	if err != nil {
		slog.Error("update gear failed", "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to update gear", r.URL.Path)
		return
	}
	if gear == nil {
		writeError(w, http.StatusNotFound, "Not Found", "gear not found", r.URL.Path)
		return
	}

	writeJSON(w, http.StatusOK, gear)
}

func (h *Handler) DeleteGear(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	if err := h.DB.RetireGear(r.Context(), id); err != nil {
		if err.Error() == "gear not found" {
			writeError(w, http.StatusNotFound, "Not Found", "gear not found", r.URL.Path)
			return
		}
		slog.Error("retire gear failed", "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to retire gear", r.URL.Path)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) CreateComponent(w http.ResponseWriter, r *http.Request) {
	gearID := chi.URLParam(r, "id")

	// Verify gear exists
	gear, err := h.DB.GetGear(r.Context(), gearID)
	if err != nil {
		slog.Error("get gear for component failed", "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to get gear", r.URL.Path)
		return
	}
	if gear == nil {
		writeError(w, http.StatusNotFound, "Not Found", "gear not found", r.URL.Path)
		return
	}

	var create model.GearComponentCreate
	if err := json.NewDecoder(r.Body).Decode(&create); err != nil {
		writeError(w, http.StatusBadRequest, "Bad Request", "invalid JSON body", r.URL.Path)
		return
	}
	if create.Name == "" {
		writeError(w, http.StatusBadRequest, "Bad Request", "name is required", r.URL.Path)
		return
	}
	if create.Type == "" {
		writeError(w, http.StatusBadRequest, "Bad Request", "type is required", r.URL.Path)
		return
	}

	component, err := h.DB.CreateComponent(r.Context(), gearID, &create)
	if err != nil {
		slog.Error("create component failed", "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to create component", r.URL.Path)
		return
	}

	writeJSON(w, http.StatusCreated, component)
}

func (h *Handler) UpdateComponent(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var req struct {
		Name      *string `json:"name,omitempty"`
		Type      *string `json:"type,omitempty"`
		Brand     *string `json:"brand,omitempty"`
		Model     *string `json:"model,omitempty"`
		Notes     *string `json:"notes,omitempty"`
		IsRetired *bool   `json:"is_retired,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Bad Request", "invalid JSON body", r.URL.Path)
		return
	}

	component, err := h.DB.UpdateComponent(r.Context(), id, req.Name, req.Type, req.Brand, req.Model, req.Notes, req.IsRetired)
	if err != nil {
		slog.Error("update component failed", "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to update component", r.URL.Path)
		return
	}
	if component == nil {
		writeError(w, http.StatusNotFound, "Not Found", "component not found", r.URL.Path)
		return
	}

	writeJSON(w, http.StatusOK, component)
}

func (h *Handler) DeleteComponent(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	if err := h.DB.DeleteComponent(r.Context(), id); err != nil {
		if err.Error() == "component not found" {
			writeError(w, http.StatusNotFound, "Not Found", "component not found", r.URL.Path)
			return
		}
		slog.Error("delete component failed", "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to delete component", r.URL.Path)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) GetMaintenanceAlerts(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())

	alerts, err := h.DB.GetMaintenanceAlerts(r.Context(), userID)
	if err != nil {
		slog.Error("get maintenance alerts failed", "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to get maintenance alerts", r.URL.Path)
		return
	}

	writeJSON(w, http.StatusOK, alerts)
}
