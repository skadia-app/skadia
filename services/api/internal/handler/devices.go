package handler

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"

	"github.com/skadia-app/skadia/services/api/internal/httputil"
	"github.com/skadia-app/skadia/services/api/internal/model"
)

// registerDeviceRequest is the JSON body for POST /devices/register.
type registerDeviceRequest struct {
	Token      string  `json:"token"`
	Platform   string  `json:"platform"`    // "android" or "ios"
	DeviceName *string `json:"device_name"` // optional
}

// RegisterDevice registers or updates an FCM/APNs device token.
//
// POST /api/v1/devices/register
func (h *Handler) RegisterDevice(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}

	req, ok := httputil.DecodeJSON[registerDeviceRequest](w, r)
	if !ok {
		return
	}
	if req.Token == "" {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "token required", r.URL.Path)
		return
	}
	if req.Platform != "android" && req.Platform != "ios" {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "platform must be 'android' or 'ios'", r.URL.Path)
		return
	}

	deviceName := ""
	if req.DeviceName != nil {
		deviceName = *req.DeviceName
	}

	if err := h.DB.RegisterDevice(r.Context(), userID, req.Token, req.Platform, deviceName); err != nil {
		slog.Error("register device failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to register device", r.URL.Path)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]string{"status": "registered"})
}

// ListDevices returns all registered device tokens for the authenticated user.
//
// GET /api/v1/devices
func (h *Handler) ListDevices(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}

	devices, err := h.DB.ListDevices(r.Context(), userID)
	if err != nil {
		slog.Error("list devices failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to list devices", r.URL.Path)
		return
	}
	if devices == nil {
		devices = []model.DeviceToken{}
	}
	httputil.WriteJSON(w, http.StatusOK, devices)
}

// DeleteDevice removes a registered device token.
//
// DELETE /api/v1/devices/{id}
func (h *Handler) DeleteDevice(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	deviceID := chi.URLParam(r, "id")

	if err := h.DB.DeleteDevice(r.Context(), deviceID, userID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			httputil.WriteError(w, http.StatusNotFound, "Not Found", "device not found", r.URL.Path)
			return
		}
		slog.Error("delete device failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to delete device", r.URL.Path)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
