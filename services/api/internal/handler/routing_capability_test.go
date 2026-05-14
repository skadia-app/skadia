package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/skadia-app/skadia/services/api/internal/capabilities"
	"github.com/skadia-app/skadia/services/api/internal/middleware"
	"github.com/skadia-app/skadia/services/api/internal/model"
	"github.com/go-chi/chi/v5"
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

func TestCoreBlocksAdminGeodataAndCoverageEndpoints(t *testing.T) {
	h := &Handler{Config: &HandlerConfig{DeploymentProfile: "core"}}
	userID := "user-test"
	tests := []struct {
		name string
		req  *http.Request
		call func(http.ResponseWriter, *http.Request)
	}{
		{name: "osm-status", req: httptest.NewRequest(http.MethodGet, "/api/v1/admin/osm-status", nil), call: h.GetOSMStatus},
		{name: "osm-regions-status", req: httptest.NewRequest(http.MethodGet, "/api/v1/admin/osm-regions/status", nil), call: h.GetOSMRegionStatus},
		{name: "osm-disk-usage", req: httptest.NewRequest(http.MethodGet, "/api/v1/admin/osm-disk-usage", nil), call: h.GetOSMDiskUsage},
		{name: "osm-region-delete", req: withRouteParam(httptest.NewRequest(http.MethodDelete, "/api/v1/admin/osm-regions/austria", nil), "regionID", "austria"), call: h.DeleteOSMRegion},
		{name: "osm-region-update", req: withRouteParam(httptest.NewRequest(http.MethodPost, "/api/v1/admin/osm-regions/austria/update", nil), "regionID", "austria"), call: h.TriggerOSMRegionUpdate},
		{name: "osm-update-all", req: httptest.NewRequest(http.MethodPost, "/api/v1/admin/osm-regions/update-all", nil), call: h.BatchUpdateAllRegions},
		{name: "osm-import-start", req: httptest.NewRequest(http.MethodPost, "/api/v1/admin/osm-import/start", strings.NewReader(`{"regions":["austria"],"categories":["ways"]}`)), call: h.StartOSMImport},
		{name: "osm-import-cancel", req: withRouteParam(httptest.NewRequest(http.MethodPost, "/api/v1/admin/osm-import/cancel/job-1", nil), "jobID", "job-1"), call: h.CancelOSMImport},
		{name: "osm-enrichment", req: httptest.NewRequest(http.MethodPost, "/api/v1/admin/osm-import/retry-enrichment", nil), call: h.RetryOSMEnrichment},
		{name: "osm-schedule-get", req: httptest.NewRequest(http.MethodGet, "/api/v1/admin/osm-schedule", nil), call: h.GetOSMSchedule},
		{name: "osm-schedule-set", req: httptest.NewRequest(http.MethodPost, "/api/v1/admin/osm-schedule", strings.NewReader(`{"interval":"manual"}`)), call: h.SetOSMSchedule},
		{name: "dem-status", req: httptest.NewRequest(http.MethodGet, "/api/v1/admin/dem-status", nil), call: h.GetDEMStatus},
		{name: "dem-download", req: httptest.NewRequest(http.MethodPost, "/api/v1/admin/dem/download", nil), call: h.DownloadDEM},
		{name: "dem-region-status", req: httptest.NewRequest(http.MethodGet, "/api/v1/admin/dem/status", nil), call: h.GetDEMRegionStatus},
		{name: "routing-tiles", req: httptest.NewRequest(http.MethodPost, "/api/v1/admin/routing/rebuild-tiles", nil), call: h.RebuildRoutingTiles},
		{name: "routing-landmarks", req: httptest.NewRequest(http.MethodPost, "/api/v1/admin/routing/rebuild-landmarks", nil), call: h.RebuildLandmarks},
		{name: "brouter-build", req: httptest.NewRequest(http.MethodPost, "/api/v1/admin/routing/build-brouter-segments", nil), call: h.BuildBRouterSegments},
		{name: "coverage-build", req: httptest.NewRequest(http.MethodPost, "/api/v1/admin/build-coverage", nil), call: h.BuildCoverage},
		{name: "coverage-info", req: httptest.NewRequest(http.MethodGet, "/api/v1/coverage/info", nil), call: h.CoverageBuildInfo},
		{name: "coverage-tile", req: httptest.NewRequest(http.MethodGet, "/api/v1/coverage/tiles/forest/10/535/362.png", nil), call: h.CoverageTile},
		{name: "coverage-query", req: httptest.NewRequest(http.MethodGet, "/internal/coverage/query?lat=47.2&lon=11.4&layers=forest", nil), call: h.QueryCoverage},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := tt.req.WithContext(context.WithValue(tt.req.Context(), middleware.UserIDKey, userID))
			rec := httptest.NewRecorder()
			tt.call(rec, req)
			assertFeatureUnavailable(t, rec)
		})
	}
}

func TestCoreSystemLayerKeysExposeAvailabilityAndPatchBlocksFullOnlyKeys(t *testing.T) {
	h := &Handler{Config: &HandlerConfig{DeploymentProfile: "core"}}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/system-layers/keys", nil)
	rec := httptest.NewRecorder()
	h.GetSystemLayerKeys(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body.String())
	}
	var layers []struct {
		model.SystemLayerDescriptor
		Available bool   `json:"available"`
		Reason    string `json:"reason"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &layers); err != nil {
		t.Fatalf("decode system layers: %v", err)
	}
	seenSegments := false
	seenSummitsBlocked := false
	seenCoverageBlocked := false
	for _, layer := range layers {
		switch layer.Key {
		case "segments_lines":
			seenSegments = true
			if !layer.Available || layer.RequiredCapability != "" {
				t.Fatalf("segments_lines = %#v, want Core-capable without required capability", layer)
			}
		case "summits":
			seenSummitsBlocked = true
			if layer.Available || layer.RequiredCapability != capabilities.KeySystemLayersVector || layer.Reason != capabilities.ReasonCoreProfile {
				t.Fatalf("summits = %#v, want vector unavailable in Core", layer)
			}
		case "coverage_forest":
			seenCoverageBlocked = true
			if layer.Available || layer.RequiredCapability != capabilities.KeySystemLayersCoverage || layer.Reason != capabilities.ReasonCoreProfile {
				t.Fatalf("coverage_forest = %#v, want coverage unavailable in Core", layer)
			}
		}
	}
	if !seenSegments || !seenSummitsBlocked || !seenCoverageBlocked {
		t.Fatalf("missing expected system layer descriptors: segments=%v summits=%v coverage=%v", seenSegments, seenSummitsBlocked, seenCoverageBlocked)
	}

	body := bytes.NewReader([]byte(`{"enabled":false}`))
	patchReq := withRouteParam(httptest.NewRequest(http.MethodPatch, "/api/v1/admin/system-layer-visibility/summits", body), "key", "summits")
	patchReq.Header.Set("Content-Type", "application/json")
	patchReq = patchReq.WithContext(context.WithValue(patchReq.Context(), middleware.UserIDKey, "user-test"))
	patchRec := httptest.NewRecorder()
	h.PatchSystemLayerVisibility(patchRec, patchReq)
	assertFeatureUnavailable(t, patchRec)
}

func withRouteParam(req *http.Request, key, value string) *http.Request {
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add(key, value)
	return req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
}

func assertFeatureUnavailable(t *testing.T, rec *httptest.ResponseRecorder) {
	t.Helper()
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503; body: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"code":"feature_unavailable"`) {
		t.Fatalf("response missing feature_unavailable code: %s", rec.Body.String())
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
