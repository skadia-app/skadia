package handler

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/skadia-app/skadia/services/api/internal/middleware"
	"github.com/skadia-app/skadia/services/api/internal/model"
)

// GetMe returns the authenticated user's ID.
// GET /api/v1/me
func (h *Handler) GetMe(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "Unauthorized", "no user ID in context", r.URL.Path)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"user_id": userID})
}

// GetProfile returns the user's physiological profile.
func (h *Handler) GetProfile(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "Unauthorized", "no user ID in context", r.URL.Path)
		return
	}
	profile, err := h.DB.GetUserProfile(r.Context(), userID)
	if err != nil {
		slog.Error("get user profile failed", "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to get profile", r.URL.Path)
		return
	}
	writeJSON(w, http.StatusOK, profile)
}

// PatchProfile updates the user's physiological profile.
func (h *Handler) PatchProfile(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "Unauthorized", "no user ID in context", r.URL.Path)
		return
	}
	var update model.UserProfileUpdate
	if err := json.NewDecoder(r.Body).Decode(&update); err != nil {
		writeError(w, http.StatusBadRequest, "Bad Request", "invalid JSON body", r.URL.Path)
		return
	}
	profile, err := h.DB.UpdateUserProfile(r.Context(), userID, &update)
	if err != nil {
		slog.Error("update user profile failed", "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to update profile", r.URL.Path)
		return
	}
	writeJSON(w, http.StatusOK, profile)
}

// GetWeatherSettings returns the user's weather preferences.
func (h *Handler) GetWeatherSettings(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "Unauthorized", "no user ID in context", r.URL.Path)
		return
	}
	prefs, err := h.DB.GetWeatherPrefs(r.Context(), userID)
	if err != nil {
		slog.Error("get weather settings failed", "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to get weather settings", r.URL.Path)
		return
	}
	writeJSON(w, http.StatusOK, prefs)
}

// UpdateWeatherSettings updates the user's weather preferences.
func (h *Handler) UpdateWeatherSettings(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "Unauthorized", "no user ID in context", r.URL.Path)
		return
	}
	var prefs model.WeatherPrefs
	if err := json.NewDecoder(r.Body).Decode(&prefs); err != nil {
		writeError(w, http.StatusBadRequest, "Bad Request", "invalid JSON body", r.URL.Path)
		return
	}
	switch prefs.ReadingMode {
	case "auto", "always_one", "always_two":
	default:
		prefs.ReadingMode = "auto"
	}
	if prefs.MountainThresholdM < 50 {
		prefs.MountainThresholdM = 50
	}
	if prefs.MountainThresholdM > 3000 {
		prefs.MountainThresholdM = 3000
	}
	if prefs.LongDurationS < 1800 {
		prefs.LongDurationS = 1800
	}
	if prefs.LongDurationS > 86400 {
		prefs.LongDurationS = 86400
	}
	if err := h.DB.UpdateWeatherPrefs(r.Context(), userID, prefs); err != nil {
		slog.Error("update weather settings failed", "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to update weather settings", r.URL.Path)
		return
	}
	writeJSON(w, http.StatusOK, prefs)
}

// DetectMaxHR calculates the 90th percentile max HR from historical activity data
// and updates max_hr_detected_bpm if the result is higher than the current value.
func (h *Handler) DetectMaxHR(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "Unauthorized", "no user ID in context", r.URL.Path)
		return
	}
	detectedMaxHR, err := h.DB.GetMaxHRPercentile(r.Context(), userID)
	if err != nil {
		slog.Error("detect max HR failed", "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to detect max HR", r.URL.Path)
		return
	}
	if detectedMaxHR <= 0 {
		writeJSON(w, http.StatusOK, map[string]any{"detected_max_hr": 0, "updated": false})
		return
	}
	// Only update if higher than existing
	imp := &model.UserProfileImport{MaxHRDetected: detectedMaxHR}
	if err := h.DB.SetUserProfileFieldsIfNull(r.Context(), userID, imp); err != nil {
		slog.Warn("set max HR detected failed", "error", err)
	}
	writeJSON(w, http.StatusOK, map[string]any{"detected_max_hr": detectedMaxHR, "updated": true})
}
