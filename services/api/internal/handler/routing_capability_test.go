package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/skadia-app/skadia/services/api/internal/middleware"
)

func TestCoreBlocksBadRouterDirectEndpointsWithFeatureUnavailable(t *testing.T) {
	h := &Handler{Config: &HandlerConfig{DeploymentProfile: "core"}}
	userID := "user-test"
	tests := []struct {
		name string
		req  *http.Request
		call func(http.ResponseWriter, *http.Request)
	}{
		{
			name: "route-internal",
			req:  httptest.NewRequest(http.MethodPost, "/api/v1/routing/route-internal", strings.NewReader(`{"from":[47,11],"to":[47.1,11.1]}`)),
			call: h.RouteCalculate,
		},
		{
			name: "routes-suggest",
			req:  httptest.NewRequest(http.MethodPost, "/api/v1/routes/suggest", strings.NewReader(`{"start":[11,47],"target_distance_m":10000,"profile":"hiking"}`)),
			call: h.SuggestRoutes,
		},
		{
			name: "isochrone",
			req:  httptest.NewRequest(http.MethodPost, "/api/v1/isochrone", strings.NewReader(`{"start":[11,47],"minutes":[60],"profile":"hiking"}`)),
			call: h.Isochrone,
		},
		{
			name: "poi-search",
			req:  httptest.NewRequest(http.MethodGet, "/api/v1/search/pois?q=hut", nil),
			call: h.SearchPOIs,
		},
		{
			name: "profile-defaults",
			req:  httptest.NewRequest(http.MethodGet, "/api/v1/routing/profiles/defaults/hiking", nil),
			call: h.HandleGetProfileDefaults,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := tt.req.WithContext(context.WithValue(tt.req.Context(), middleware.UserIDKey, userID))
			rec := httptest.NewRecorder()
			tt.call(rec, req)
			if rec.Code != http.StatusServiceUnavailable {
				t.Fatalf("status = %d, want 503; body: %s", rec.Code, rec.Body.String())
			}
			if !strings.Contains(rec.Body.String(), `"code":"feature_unavailable"`) {
				t.Fatalf("response missing feature_unavailable code: %s", rec.Body.String())
			}
		})
	}
}

func TestCoreRoutingStatusRedactsBadRouterArtifacts(t *testing.T) {
	h := &Handler{Config: &HandlerConfig{DeploymentProfile: "core"}}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/routing/status", nil)
	rec := httptest.NewRecorder()

	h.RoutingGraphStatus(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"reason":"core_profile"`) || !strings.Contains(rec.Body.String(), `"routing_available":false`) {
		t.Fatalf("routing status did not redact core BadRouter status: %s", rec.Body.String())
	}
}

func TestFullMissingBadRouterArtifactsReturnsFeatureUnavailable(t *testing.T) {
	h := &Handler{Config: &HandlerConfig{DeploymentProfile: "full"}}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/routing/route-internal", strings.NewReader(`{"from":[47,11],"to":[47.1,11.1]}`))
	req = req.WithContext(context.WithValue(req.Context(), middleware.UserIDKey, "user-test"))
	rec := httptest.NewRecorder()

	h.RouteCalculate(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503; body: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"code":"feature_unavailable"`) || !strings.Contains(rec.Body.String(), "missing_artifacts") {
		t.Fatalf("response missing feature_unavailable/missing_artifacts: %s", rec.Body.String())
	}
}
