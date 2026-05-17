package handler

import (
	"context"
	"net/http"
	"path/filepath"

	"github.com/skadia-app/skadia/services/api/internal/capabilities"
	"github.com/skadia-app/skadia/services/api/internal/httputil"
	"github.com/jackc/pgx/v5/pgxpool"
)

func (h *Handler) requireFeature(w http.ResponseWriter, r *http.Request, key string) bool {
	reason := h.featureUnavailableReasonWithContext(r.Context(), key)
	if reason == "" {
		return true
	}
	writeFeatureUnavailable(w, r, key, reason)
	return false
}

func (h *Handler) requireFeatureAvailable(w http.ResponseWriter, r *http.Request, key string) bool {
	return h.requireFeature(w, r, key)
}

func (h *Handler) requireFeatureConfigured(w http.ResponseWriter, r *http.Request, key string) bool {
	reason := h.featureNotConfiguredReasonWithContext(r.Context(), key)
	if reason == "" {
		return true
	}
	writeFeatureUnavailable(w, r, key, reason)
	return false
}

func (h *Handler) featureUnavailableReason(key string) string {
	return h.featureUnavailableReasonWithContext(context.Background(), key)
}

func (h *Handler) featureUnavailableReasonWithContext(ctx context.Context, key string) string {
	return h.featureNotAvailableReasonWithContext(ctx, key)
}

func (h *Handler) featureNotConfiguredReasonWithContext(ctx context.Context, key string) string {
	if !capabilities.ValidKey(key) {
		return capabilities.ReasonNotConfigured
	}
	caps := h.detectCapabilitiesForGate(ctx)
	state, ok := caps.Features[key]
	if !ok {
		return capabilities.ReasonNotConfigured
	}
	if !state.Configured {
		if state.Reason != "" {
			return state.Reason
		}
		return capabilities.ReasonNotConfigured
	}
	if state.Available {
		return ""
	}
	return ""
}

func (h *Handler) featureNotAvailableReasonWithContext(ctx context.Context, key string) string {
	if !capabilities.ValidKey(key) {
		return capabilities.ReasonNotConfigured
	}
	caps := h.detectCapabilitiesForGate(ctx)
	state, ok := caps.Features[key]
	if !ok {
		return capabilities.ReasonNotConfigured
	}
	if !state.Configured {
		if state.Reason != "" {
			return state.Reason
		}
		return capabilities.ReasonNotConfigured
	}
	if state.Available {
		return ""
	}
	if state.Reason != "" {
		return state.Reason
	}
	return capabilities.ReasonMissingArtifacts
}

func (h *Handler) detectCapabilitiesForGate(ctx context.Context) capabilities.Capabilities {
	var pool *pgxpool.Pool
	if h != nil && h.DB != nil {
		pool = h.DB.Pool
	}
	input := h.detectInputForGate()
	caps := capabilities.Detect(ctx, pool, input)
	if h != nil && h.GraphManager != nil && h.GraphManager.UnifiedRouter() != nil {
		for _, key := range []string{
			capabilities.KeyBadRouter,
			capabilities.KeyRouting,
			capabilities.KeyRoutingTiles,
			capabilities.KeyIsochrone,
			capabilities.KeyRouteSuggestion,
		} {
			state, ok := caps.Features[key]
			if ok && state.Configured {
				state.Available = true
				state.Reason = ""
				caps.Features[key] = state
			}
		}
		caps.ApplyLegacyBooleans()
	}
	return caps
}

func (h *Handler) detectInputForGate() capabilities.DetectInput {
	input := capabilities.DetectInput{}
	if h == nil || h.Config == nil {
		return input
	}
	input.DeploymentProfile = h.Config.DeploymentProfile
	input.DeploymentProfileSrc = h.Config.DeploymentProfileSource
	input.StorageMode = h.Config.StorageMode
	input.StorageBackend = h.Config.StorageBackend
	input.BRouterURL = h.Config.BRouterURL
	input.BadRouterProfilesKnown = h.Config.BadRouterProfilesKnown
	input.BadRouterProfilesConfigured = h.Config.BadRouterProfilesConfigured
	input.BadRouterProfilesAvailable = h.Config.BadRouterProfilesAvailable
	input.BadRouterProfilesReason = h.Config.BadRouterProfilesReason
	if h.Config.RoutingGraphPath != "" {
		input.TileDir = filepath.Join(filepath.Dir(h.Config.RoutingGraphPath), "tiles")
		if h.CoverageBaseDir != "" {
			input.CoverageDir = h.CoverageBaseDir
		} else {
			input.CoverageDir = filepath.Join(filepath.Dir(h.Config.RoutingGraphPath), "coverage")
		}
	}
	input.PMTilesDir = h.Config.TilesDir
	input.ImportAllowedDirs = h.Config.ImportAllowedDirs
	input.MLProbeVersion = capabilities.MLProbeVersion()
	return input
}

func (h *Handler) coreProfile() bool {
	if h.deploymentProfile() == "core" {
		return true
	}
	return false
}

func (h *Handler) requireImportPathAvailable(w http.ResponseWriter, r *http.Request, sourcePath string) bool {
	if isTempSourcePath(sourcePath) {
		return true
	}
	return h.requireFeatureAvailable(w, r, capabilities.KeyImportPathEnabled)
}

func writeFeatureUnavailable(w http.ResponseWriter, r *http.Request, key, reason string) {
	httputil.WriteProblem(w, http.StatusServiceUnavailable, "Feature Unavailable", "feature "+key+" is unavailable: "+reason, "feature_unavailable", r.URL.Path)
}
