package handler

import (
	"net/http"

	"github.com/skadia-app/skadia/services/api/internal/capabilities"
	"github.com/skadia-app/skadia/services/api/internal/httputil"
)

func (h *Handler) requireFeature(w http.ResponseWriter, r *http.Request, key string) bool {
	if h.featureUnavailableReason(key) == "" {
		return true
	}
	writeFeatureUnavailable(w, r, key, h.featureUnavailableReason(key))
	return false
}

func (h *Handler) featureUnavailableReason(key string) string {
	if h.deploymentProfile() == "core" {
		switch key {
		case capabilities.KeyBadRouter,
			capabilities.KeyRoutingTiles,
			capabilities.KeyIsochrone,
			capabilities.KeyRouteSuggestion,
			capabilities.KeyLocalPOISearch,
			capabilities.KeyBRouterSegmentBuild:
			return capabilities.ReasonCoreProfile
		}
	}
	return ""
}

func writeFeatureUnavailable(w http.ResponseWriter, r *http.Request, key, reason string) {
	httputil.WriteProblem(w, http.StatusServiceUnavailable, "Feature Unavailable", "feature "+key+" is unavailable: "+reason, "feature_unavailable", r.URL.Path)
}
