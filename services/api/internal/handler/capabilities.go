package handler

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"path/filepath"

	"github.com/skadia-app/skadia/services/api/internal/capabilities"
	"github.com/skadia-app/skadia/services/api/internal/middleware"
)

// GetCapabilities probes the infrastructure and returns which routing and
// geo features are available at runtime (Lite vs. Full mode).
// GET /api/v1/capabilities
func (h *Handler) GetCapabilities(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "Unauthorized", "no user ID", r.URL.Path)
		return
	}

	brouterURL := ""
	tileDir := ""
	if h.Config != nil {
		brouterURL = h.Config.BRouterURL
		if h.Config.RoutingGraphPath != "" {
			tileDir = filepath.Join(filepath.Dir(h.Config.RoutingGraphPath), "tiles")
		}
	}

	caps := capabilities.Detect(r.Context(), h.DB.Pool, brouterURL, tileDir)

	slog.Info("capabilities detected",
		"user_id", userID,
		"routing", caps.Routing,
		"offline_map", caps.OfflineMap,
		"dem_correction", caps.DEMCorrection,
	)

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(caps); err != nil {
		slog.Error("failed to encode capabilities response", "error", err)
	}
}
