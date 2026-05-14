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
	coverageDir := ""
	var importAllowedDirs []string
	deploymentProfile := "full"
	deploymentProfileSource := "implicit_legacy"
	storageMode := "local"
	storageBackend := "local"
	if h.Config != nil {
		brouterURL = h.Config.BRouterURL
		if h.Config.RoutingGraphPath != "" {
			tileDir = filepath.Join(filepath.Dir(h.Config.RoutingGraphPath), "tiles")
		}
		pmTilesDir = h.Config.TilesDir
		coverageDir = filepath.Join(h.Config.TilesDir, "coverage")
		importAllowedDirs = h.Config.ImportAllowedDirs
		deploymentProfile = h.Config.DeploymentProfile
		deploymentProfileSource = h.Config.DeploymentProfileSource
		storageMode = h.Config.StorageMode
		storageBackend = h.Config.StorageBackend
	}

	caps := capabilities.Detect(r.Context(), h.DB.Pool, capabilities.DetectInput{
		DeploymentProfile:    deploymentProfile,
		DeploymentProfileSrc: deploymentProfileSource,
		StorageMode:          storageMode,
		StorageBackend:       storageBackend,
		BRouterURL:           brouterURL,
		TileDir:              tileDir,
		PMTilesDir:           pmTilesDir,
		CoverageDir:          coverageDir,
		ImportAllowedDirs:    importAllowedDirs,
		MLProbeVersion:       capabilities.MLProbeVersion(),
	})

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
