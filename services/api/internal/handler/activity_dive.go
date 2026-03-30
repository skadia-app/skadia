package handler

import (
	"log/slog"
	"net/http"

	"github.com/skadia-app/skadia/services/api/internal/httputil"
	"github.com/skadia-app/skadia/services/api/internal/model"
)

// DiveInfo bundles settings + gases for a single response.
type DiveInfo struct {
	Settings *model.DiveSettings `json:"settings,omitempty"`
	Gases    []model.DiveGas     `json:"gases"`
}

// GetActivityDiveInfo returns the dive settings and gas mixes for an activity.
// GET /api/v1/activities/{id}/dive
func (h *Handler) GetActivityDiveInfo(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	_, activityID, ok := h.requireActivity(w, r, userID)
	if !ok {
		return
	}

	settings, err := h.DB.GetDiveSettings(r.Context(), activityID)
	if err != nil {
		slog.Error("get dive settings failed", "activity_id", activityID, "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to get dive settings", r.URL.Path)
		return
	}
	gases, err := h.DB.GetDiveGases(r.Context(), activityID)
	if err != nil {
		slog.Error("get dive gases failed", "activity_id", activityID, "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to get dive gases", r.URL.Path)
		return
	}
	if gases == nil {
		gases = []model.DiveGas{}
	}

	httputil.WriteJSON(w, http.StatusOK, DiveInfo{Settings: settings, Gases: gases})
}
