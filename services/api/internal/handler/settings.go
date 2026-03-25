package handler

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"regexp"

	"github.com/skadia-app/skadia/services/api/internal/httputil"
	"github.com/skadia-app/skadia/services/api/internal/middleware"
	"github.com/skadia-app/skadia/services/api/internal/model"
)

// reUsername matches valid usernames: starts with alphanumeric, rest alphanumeric or underscore.
var reUsername = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_]*$`)

// UpdateMe handles PATCH /api/v1/me — updates username and/or display_name.
func (h *Handler) UpdateMe(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}

	var req struct {
		Username    *string `json:"username,omitempty"`
		DisplayName *string `json:"display_name,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid_request"}`, http.StatusBadRequest)
		return
	}

	if req.Username != nil {
		u := *req.Username
		if len(u) < 3 || len(u) > 30 {
			httputil.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "username_invalid_length"})
			return
		}
		if !reUsername.MatchString(u) {
			httputil.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "username_invalid_chars"})
			return
		}
		exists, _ := h.DB.CheckUsernameCollision(r.Context(), u, userID)
		if exists {
			httputil.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "username_taken"})
			return
		}
	}

	if req.DisplayName != nil && len(*req.DisplayName) > 100 {
		httputil.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "display_name_too_long"})
		return
	}

	if err := h.DB.UpdateAccountInfo(r.Context(), userID, req.Username, req.DisplayName); err != nil {
		slog.Error("UpdateMe: update account info failed", "error", err)
		httputil.WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal_error"})
		return
	}

	h.DB.LogAuthEvent(r.Context(), userID, "profile_update", middleware.ClientIP(r), r.UserAgent(), nil)
	httputil.WriteJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

// GetMe returns the authenticated user's full profile.
// GET /api/v1/me
func (h *Handler) GetMe(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	user, err := h.DB.GetUserByID(r.Context(), userID)
	if err != nil {
		slog.Error("GetMe: user not found", "error", err)
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}
	resp := model.MeResponse{
		UserID:             user.ID,
		Username:           user.Username,
		DisplayName:        user.DisplayName,
		Email:              user.Email,
		IsAdmin:            user.IsAdmin,
		EmailVerified:      user.EmailVerified,
		HasPassword:        user.PasswordHash != nil,
		MustChangePassword: user.MustChangePassword,
		RegistrationSource: user.RegistrationSrc,
	}
	httputil.WriteJSON(w, http.StatusOK, resp)
}

// GetProfile returns the user's physiological profile.
func (h *Handler) GetProfile(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	profile, err := h.DB.GetUserProfile(r.Context(), userID)
	if err != nil {
		slog.Error("get user profile failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to get profile", r.URL.Path)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, profile)
}

// PatchProfile updates the user's physiological profile.
func (h *Handler) PatchProfile(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	update, ok := httputil.DecodeJSON[model.UserProfileUpdate](w, r)
	if !ok {
		return
	}
	profile, err := h.DB.UpdateUserProfile(r.Context(), userID, &update)
	if err != nil {
		slog.Error("update user profile failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to update profile", r.URL.Path)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, profile)
}

// GetWeatherSettings returns the user's weather preferences.
func (h *Handler) GetWeatherSettings(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	prefs, err := h.DB.GetWeatherPrefs(r.Context(), userID)
	if err != nil {
		slog.Error("get weather settings failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to get weather settings", r.URL.Path)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, prefs)
}

// UpdateWeatherSettings updates the user's weather preferences.
func (h *Handler) UpdateWeatherSettings(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	prefs, ok := httputil.DecodeJSON[model.WeatherPrefs](w, r)
	if !ok {
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
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to update weather settings", r.URL.Path)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, prefs)
}

// DetectMaxHR calculates the 90th percentile max HR from historical activity data
// and updates max_hr_detected_bpm if the result is higher than the current value.
func (h *Handler) DetectMaxHR(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	detectedMaxHR, err := h.DB.GetMaxHRPercentile(r.Context(), userID)
	if err != nil {
		slog.Error("detect max HR failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to detect max HR", r.URL.Path)
		return
	}
	if detectedMaxHR <= 0 {
		httputil.WriteJSON(w, http.StatusOK, map[string]any{"detected_max_hr": 0, "updated": false})
		return
	}
	// Only update if higher than existing
	imp := &model.UserProfileImport{MaxHRDetected: detectedMaxHR}
	if err := h.DB.SetUserProfileFieldsIfNull(r.Context(), userID, imp); err != nil {
		slog.Warn("set max HR detected failed", "error", err)
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]any{"detected_max_hr": detectedMaxHR, "updated": true})
}
