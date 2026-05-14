package handler

import (
	"log/slog"
	"net/http"

	"github.com/skadia-app/skadia/services/api/internal/httputil"
)

// GetCapabilities probes the infrastructure and returns which routing and
// geo features are available at runtime (Lite vs. Full mode).
// GET /api/v1/capabilities
func (h *Handler) GetCapabilities(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}

	caps := h.detectCapabilitiesForGate(r.Context())

	slog.Info("capabilities detected",
		"user_id", userID,
		"deployment_profile", caps.DeploymentProfile,
		"storage_mode", caps.StorageMode,
		"routing", caps.Routing,
		"offline_map", caps.OfflineMap,
		"dem_correction", caps.DEMCorrection,
	)

	httputil.WriteJSON(w, http.StatusOK, caps)
}
