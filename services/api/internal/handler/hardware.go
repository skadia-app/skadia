package handler

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/skadia-app/skadia/services/api/internal/middleware"
	"github.com/skadia-app/skadia/services/api/internal/model"
	"github.com/skadia-app/skadia/services/api/internal/httputil"
	"github.com/go-chi/chi/v5"
)

// ListHardware handles GET /api/v1/hardware?type=recorder|sensor
func (h *Handler) ListHardware(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	deviceType := r.URL.Query().Get("type")

	devices, err := h.DB.ListHWDevices(r.Context(), userID, deviceType)
	if err != nil {
		slog.Error("list hardware devices failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to list hardware devices", r.URL.Path)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, devices)
}

// GetHardware handles GET /api/v1/hardware/{id}
func (h *Handler) GetHardware(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	id := chi.URLParam(r, "id")

	device, err := h.DB.GetHWDevice(r.Context(), id)
	if err != nil {
		slog.Error("get hardware device failed", "error", err, "id", id)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to get hardware device", r.URL.Path)
		return
	}
	if device == nil || device.UserID != userID {
		httputil.WriteError(w, http.StatusNotFound, "Not Found", "hardware device not found", r.URL.Path)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, device)
}

// PatchHardware handles PATCH /api/v1/hardware/{id}
func (h *Handler) PatchHardware(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	id := chi.URLParam(r, "id")

	// Ownership check
	device, err := h.DB.GetHWDevice(r.Context(), id)
	if err != nil {
		slog.Error("get hardware device failed", "error", err, "id", id)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to get hardware device", r.URL.Path)
		return
	}
	if device == nil || device.UserID != userID {
		httputil.WriteError(w, http.StatusNotFound, "Not Found", "hardware device not found", r.URL.Path)
		return
	}

	var req model.DeviceUpdate
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "invalid request body", r.URL.Path)
		return
	}

	customName := ""
	if req.CustomName != nil {
		customName = *req.CustomName
	}

	updated, err := h.DB.UpdateHWDeviceCustomName(r.Context(), id, customName)
	if err != nil {
		slog.Error("update hardware device custom name failed", "error", err, "id", id)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to update hardware device", r.URL.Path)
		return
	}
	if updated == nil {
		httputil.WriteError(w, http.StatusNotFound, "Not Found", "hardware device not found", r.URL.Path)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, updated)
}

// DeleteHardware handles DELETE /api/v1/hardware/{id}
func (h *Handler) DeleteHardware(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	id := chi.URLParam(r, "id")

	// Ownership check
	device, err := h.DB.GetHWDevice(r.Context(), id)
	if err != nil {
		slog.Error("get hardware device failed", "error", err, "id", id)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to get hardware device", r.URL.Path)
		return
	}
	if device == nil || device.UserID != userID {
		httputil.WriteError(w, http.StatusNotFound, "Not Found", "hardware device not found", r.URL.Path)
		return
	}

	// FK on activities.recording_device_id and activity_sensors.device_id are ON DELETE SET NULL.
	_, err = h.DB.Pool.Exec(r.Context(), `DELETE FROM devices WHERE id = $1`, id)
	if err != nil {
		slog.Error("delete hardware device failed", "error", err, "id", id)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to delete hardware device", r.URL.Path)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// MergeHardware handles POST /api/v1/hardware/merge
func (h *Handler) MergeHardware(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())

	var req model.DeviceMerge
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "invalid request body", r.URL.Path)
		return
	}
	if req.SourceID == "" || req.TargetID == "" {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "source_id and target_id are required", r.URL.Path)
		return
	}

	if err := h.DB.MergeHWDevices(r.Context(), userID, req.SourceID, req.TargetID); err != nil {
		slog.Error("merge hardware devices failed", "error", err, "source", req.SourceID, "target", req.TargetID)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to merge hardware devices", r.URL.Path)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]string{"status": "merged"})
}

// ListHardwareActivities handles GET /api/v1/hardware/{id}/activities?after=cursor
func (h *Handler) ListHardwareActivities(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	id := chi.URLParam(r, "id")
	after := r.URL.Query().Get("after")

	// Ownership check
	device, err := h.DB.GetHWDevice(r.Context(), id)
	if err != nil {
		slog.Error("get hardware device failed", "error", err, "id", id)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to get hardware device", r.URL.Path)
		return
	}
	if device == nil || device.UserID != userID {
		httputil.WriteError(w, http.StatusNotFound, "Not Found", "hardware device not found", r.URL.Path)
		return
	}

	activities, err := h.DB.ListDeviceActivities(r.Context(), id, 50, after)
	if err != nil {
		slog.Error("list device activities failed", "error", err, "id", id)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to list device activities", r.URL.Path)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, activities)
}
