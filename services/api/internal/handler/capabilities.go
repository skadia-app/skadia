package handler

import (
	"log/slog"
	"net/http"
	"path/filepath"

	"github.com/skadia-app/skadia/services/api/internal/capabilities"
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

	brouterURL := ""
	tileDir := ""
	pmTilesDir := ""
	var importAllowedDirs []string
	if h.Config != nil {
		brouterURL = h.Config.BRouterURL
		if h.Config.RoutingGraphPath != "" {
			tileDir = filepath.Join(filepath.Dir(h.Config.RoutingGraphPath), "tiles")
		}
		pmTilesDir = h.Config.TilesDir
		importAllowedDirs = h.Config.ImportAllowedDirs
	}

	caps := capabilities.Detect(r.Context(), h.DB.Pool, brouterURL, tileDir, pmTilesDir, importAllowedDirs)

	slog.Info("capabilities detected",
		"user_id", userID,
		"routing", caps.Routing,
		"offline_map", caps.OfflineMap,
		"dem_correction", caps.DEMCorrection,
	)

	httputil.WriteJSON(w, http.StatusOK, caps)
}
