package handler

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/skadia-app/skadia/services/api/internal/capabilities"
	"github.com/skadia-app/skadia/services/api/internal/enrichment/lifts"
	"github.com/skadia-app/skadia/services/api/internal/httputil"
)

// liftDetectionConfigResponse is the JSON shape returned by GetLiftDetectionConfig.
type liftDetectionConfigResponse struct {
	Effective     liftDetectionPair     `json:"effective"`
	UserSetting   liftDetectionPair     `json:"user_setting"`
	Override      liftDetectionOverride `json:"override"`
	MasterEnabled bool                  `json:"master_enabled"`
}

type liftDetectionPair struct {
	OSM       bool `json:"osm"`
	Heuristic bool `json:"heuristic"`
}

type liftDetectionOverride struct {
	OSM       *bool `json:"osm"`
	Heuristic *bool `json:"heuristic"`
}

// GetLiftDetectionConfig handles GET /api/v1/activities/{id}/lifts/detection-config.
// Returns the effective resolved options, the raw user-level setting, and the
// per-activity override pointers. User B accessing User A's activity → 404.
func (h *Handler) GetLiftDetectionConfig(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	activityID := chi.URLParam(r, "id")

	activity, err := h.DB.GetActivity(r.Context(), activityID)
	if err != nil {
		slog.Error("GetLiftDetectionConfig: get activity failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "", r.URL.Path)
		return
	}
	if activity == nil || activity.UserID != userID {
		httputil.WriteError(w, http.StatusNotFound, "Not Found", "activity not found", r.URL.Path)
		return
	}

	settings, err := h.DB.GetEnrichmentSettings(r.Context(), userID)
	if err != nil {
		slog.Error("GetLiftDetectionConfig: get enrichment settings failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "", r.URL.Path)
		return
	}

	overrideOSM := activity.LiftDetectionOSMOverride
	overrideHeuristic := activity.LiftDetectionHeuristicOverride

	effective := lifts.EffectiveLiftDetectionOpts(
		settings.EnableLiftDetection,
		settings.EnableLiftDetectionOSM,
		settings.EnableLiftDetectionHeuristic,
		overrideOSM,
		overrideHeuristic,
	)
	if h.featureNotAvailableReasonWithContext(r.Context(), capabilities.KeyOSMLiftSnap) != "" {
		effective.OSMSnap = false
	}

	resp := liftDetectionConfigResponse{
		MasterEnabled: settings.EnableLiftDetection,
		UserSetting: liftDetectionPair{
			OSM:       settings.EnableLiftDetectionOSM,
			Heuristic: settings.EnableLiftDetectionHeuristic,
		},
		Override: liftDetectionOverride{
			OSM:       overrideOSM,
			Heuristic: overrideHeuristic,
		},
		Effective: liftDetectionPair{
			OSM:       effective.OSMSnap,
			Heuristic: effective.Heuristic,
		},
	}
	httputil.WriteJSON(w, http.StatusOK, resp)
}

// PutLiftDetectionConfig handles PUT /api/v1/activities/{id}/lifts/detection-config.
// Persists per-activity override values. Partial body accepted: omit a key to
// leave its column unchanged (COALESCE in DB layer). Does not trigger re-enrichment.
func (h *Handler) PutLiftDetectionConfig(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	activityID := chi.URLParam(r, "id")

	activity, err := h.DB.GetActivity(r.Context(), activityID)
	if err != nil {
		slog.Error("PutLiftDetectionConfig: get activity failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "", r.URL.Path)
		return
	}
	if activity == nil || activity.UserID != userID {
		httputil.WriteError(w, http.StatusNotFound, "Not Found", "activity not found", r.URL.Path)
		return
	}

	var body struct {
		OSM       *bool `json:"osm"`
		Heuristic *bool `json:"heuristic"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "invalid JSON", r.URL.Path)
		return
	}
	if body.OSM != nil && *body.OSM {
		if reason := h.featureNotAvailableReasonWithContext(r.Context(), capabilities.KeyOSMLiftSnap); reason != "" {
			writeFeatureUnavailable(w, r, capabilities.KeyOSMLiftSnap, reason)
			return
		}
	}

	// F2: SetLiftDetectionOverride is now user-scoped; the pre-check above
	// already guards ownership, but passing userID into the UPDATE is
	// defence-in-depth.
	// F18: a no-op PUT (body matches the stored values) intentionally returns
	// RowsAffected=0; we still respond 204 because the resulting state is what
	// the client requested.
	if _, err := h.DB.SetLiftDetectionOverride(r.Context(), userID, activityID, body.OSM, body.Heuristic); err != nil {
		slog.Error("PutLiftDetectionConfig: set override failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "", r.URL.Path)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// DeleteLiftDetectionConfig handles DELETE /api/v1/activities/{id}/lifts/detection-config.
// Resets both per-activity override columns to NULL so the pipeline uses the
// user-level setting again. Idempotent — deleting already-null overrides → 204.
func (h *Handler) DeleteLiftDetectionConfig(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	activityID := chi.URLParam(r, "id")

	activity, err := h.DB.GetActivity(r.Context(), activityID)
	if err != nil {
		slog.Error("DeleteLiftDetectionConfig: get activity failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "", r.URL.Path)
		return
	}
	if activity == nil || activity.UserID != userID {
		httputil.WriteError(w, http.StatusNotFound, "Not Found", "activity not found", r.URL.Path)
		return
	}

	if err := h.DB.ResetLiftDetectionOverride(r.Context(), userID, activityID); err != nil {
		slog.Error("DeleteLiftDetectionConfig: reset override failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "", r.URL.Path)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
