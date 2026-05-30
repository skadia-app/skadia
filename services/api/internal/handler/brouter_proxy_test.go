package handler

import (
	"context"
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/skadia-app/skadia/services/api/internal/capabilities"
	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/dem"
)

// assertInDelta is the local stdlib-only float-equal helper (testify is in
// go.sum but not used in source). Mirrors the helper from
// services/api/internal/routing/elevation_profile_builder_test.go.
func assertInDelta(t *testing.T, want, got, delta float64, msg string) {
	t.Helper()
	if math.Abs(want-got) > delta {
		t.Errorf("%s: want %v ± %v, got %v", msg, want, delta, got)
	}
}

// bc builds a brouterCoord with a usable Z value.
func bc(lon, lat, ele float64) brouterCoord {
	return brouterCoord{Lon: lon, Lat: lat, Ele: ele, EleOK: true}
}

// bcNoZ builds a 2D coord with EleOK=false (e.g. JSON null mapped through).
func bcNoZ(lon, lat float64) brouterCoord {
	return brouterCoord{Lon: lon, Lat: lat, EleOK: false}
}

// ---- buildProfileFromInlineZ ----

func TestBuildProfileFromInlineZ_RealBrouterShape(t *testing.T) {
	coords := []brouterCoord{
		bc(11.404, 47.270, 581),
		bc(11.405, 47.271, 590),
		bc(11.406, 47.272, 600),
	}
	profile := buildProfileFromInlineZ(coords)
	if len(profile) != 3 {
		t.Fatalf("want 3 profile points, got %d", len(profile))
	}
	assertInDelta(t, 0.0, profile[0]["dist_m"].(float64), 0.01, "profile[0].dist_m")
	assertInDelta(t, 581.0, profile[0]["ele_m"].(float64), 0.01, "profile[0].ele_m")
	assertInDelta(t, 590.0, profile[1]["ele_m"].(float64), 0.01, "profile[1].ele_m")
	if !(profile[1]["dist_m"].(float64) > profile[0]["dist_m"].(float64)) {
		t.Errorf("profile[1].dist_m must be > profile[0].dist_m")
	}
	if !(profile[2]["dist_m"].(float64) > profile[1]["dist_m"].(float64)) {
		t.Errorf("profile[2].dist_m must be > profile[1].dist_m")
	}
}

func TestRouteViabrouterRejectsUnknownProfileParamBeforeProxy(t *testing.T) {
	called := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusTeapot)
	}))
	defer server.Close()

	h := &Handler{Config: &HandlerConfig{BRouterURL: server.URL}}
	req := httptest.NewRequest(http.MethodPost, "/routing/route", strings.NewReader(`{
		"waypoints": [[47.259,11.394],[47.280,11.450]],
		"profile": "hiking-mountain",
		"profile_params": {"unknown_param": "1"}
	}`))
	req = req.WithContext(contextWithTestUserID(req.Context(), "00000000-0000-0000-0000-000000000001"))
	w := httptest.NewRecorder()

	h.RouteViabrouter(w, req)

	if called {
		t.Fatal("unknown profile param should be rejected before calling BRouter")
	}
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", w.Code, w.Body.String())
	}
}

func TestRouteViabrouterForwardsOfficialBRouterProfileName(t *testing.T) {
	capabilities.InvalidateCache()
	t.Cleanup(capabilities.InvalidateCache)
	var gotProfile string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/brouter" {
			w.WriteHeader(http.StatusOK)
			return
		}
		gotProfile = r.URL.Query().Get("profile")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"type":"FeatureCollection","features":[]}`))
	}))
	defer server.Close()

	h := &Handler{Config: &HandlerConfig{DeploymentProfile: "full", BRouterURL: server.URL}}
	req := newBRouterProxyTestRequest(`{
		"waypoints": [[47.259,11.394],[47.280,11.450]],
		"profile": "fastbike"
	}`)
	w := httptest.NewRecorder()

	h.RouteViabrouter(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	if gotProfile != "fastbike" {
		t.Fatalf("forwarded profile = %q, want fastbike", gotProfile)
	}
}

func TestRouteViabrouterRejectsUnsupportedBRouterProfileName(t *testing.T) {
	capabilities.InvalidateCache()
	t.Cleanup(capabilities.InvalidateCache)
	called := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusTeapot)
	}))
	defer server.Close()

	h := &Handler{Config: &HandlerConfig{DeploymentProfile: "full", BRouterURL: server.URL}}
	req := newBRouterProxyTestRequest(`{
		"waypoints": [[47.259,11.394],[47.280,11.450]],
		"profile": "made-up-profile"
	}`)
	w := httptest.NewRecorder()

	h.RouteViabrouter(w, req)

	if called {
		t.Fatal("unsupported profile should be rejected before calling BRouter")
	}
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", w.Code, w.Body.String())
	}
}

func TestRouteViabrouterMapsLegacyBRouterProfileNames(t *testing.T) {
	capabilities.InvalidateCache()
	t.Cleanup(capabilities.InvalidateCache)
	var gotProfile string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/brouter" {
			w.WriteHeader(http.StatusOK)
			return
		}
		gotProfile = r.URL.Query().Get("profile")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"type":"FeatureCollection","features":[]}`))
	}))
	defer server.Close()

	h := &Handler{Config: &HandlerConfig{DeploymentProfile: "full", BRouterURL: server.URL}}
	req := newBRouterProxyTestRequest(`{
		"waypoints": [[47.259,11.394],[47.280,11.450]],
		"profile": "cycling-road"
	}`)
	w := httptest.NewRecorder()

	h.RouteViabrouter(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	if gotProfile != "fastbike" {
		t.Fatalf("forwarded profile = %q, want fastbike", gotProfile)
	}
}

func TestRouteViabrouterForwardsExistingCustomBRouterProfileName(t *testing.T) {
	capabilities.InvalidateCache()
	t.Cleanup(capabilities.InvalidateCache)
	profilesDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(profilesDir, "custom-trail.brf"), []byte("assign validForBikes 1\n"), 0o644); err != nil {
		t.Fatalf("write custom profile: %v", err)
	}
	var gotProfile string
	var gotProcessUnusedTags string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/brouter" {
			w.WriteHeader(http.StatusOK)
			return
		}
		gotProfile = r.URL.Query().Get("profile")
		gotProcessUnusedTags = r.URL.Query().Get("profile:processUnusedTags")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"type":"FeatureCollection","features":[]}`))
	}))
	defer server.Close()

	h := &Handler{Config: &HandlerConfig{DeploymentProfile: "full", BRouterURL: server.URL, BRouterProfilesDir: profilesDir}}
	req := newBRouterProxyTestRequest(`{
		"waypoints": [[47.259,11.394],[47.280,11.450]],
		"profile": "custom-trail"
	}`)
	w := httptest.NewRecorder()

	h.RouteViabrouter(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	if gotProfile != "custom-trail" {
		t.Fatalf("forwarded profile = %q, want custom-trail", gotProfile)
	}
	if gotProcessUnusedTags != "1" {
		t.Fatalf("profile:processUnusedTags = %q, want 1", gotProcessUnusedTags)
	}
}

func TestRouteViabrouterRequiresAvailableBRouterBeforeOutboundCall(t *testing.T) {
	capabilities.InvalidateCache()
	t.Cleanup(capabilities.InvalidateCache)
	var routeCalls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/brouter" && r.URL.Query().Get("lonlats") != "0,0|0.001,0.001" {
			routeCalls++
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	h := &Handler{Config: &HandlerConfig{DeploymentProfile: "full", BRouterURL: server.URL}}
	req := newBRouterProxyTestRequest(`{
		"waypoints": [[47.259,11.394],[47.280,11.450]],
		"profile": "hiking-mountain"
	}`)
	w := httptest.NewRecorder()

	h.RouteViabrouter(w, req)

	if routeCalls != 0 {
		t.Fatalf("proxy made %d route /brouter calls after unavailable capability probe", routeCalls)
	}
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503; body=%s", w.Code, w.Body.String())
	}
	if body := w.Body.String(); !strings.Contains(body, `"code":"feature_unavailable"`) || !strings.Contains(body, "missing_service") {
		t.Fatalf("body %q missing feature_unavailable/missing_service", body)
	}
}

func TestRouteViabrouterEmptyBRouterURLUsesCapabilityUnavailableBehavior(t *testing.T) {
	capabilities.InvalidateCache()
	t.Cleanup(capabilities.InvalidateCache)
	h := &Handler{Config: &HandlerConfig{DeploymentProfile: "full"}}
	req := newBRouterProxyTestRequest(`{
		"waypoints": [[47.259,11.394],[47.280,11.450]],
		"profile": "hiking-mountain"
	}`)
	w := httptest.NewRecorder()

	h.RouteViabrouter(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503; body=%s", w.Code, w.Body.String())
	}
	if body := w.Body.String(); !strings.Contains(body, `"code":"feature_unavailable"`) || !strings.Contains(body, "missing_service") {
		t.Fatalf("body %q missing feature_unavailable/missing_service", body)
	}
}

func newBRouterProxyTestRequest(body string) *http.Request {
	req := httptest.NewRequest(http.MethodPost, "/routing/route", strings.NewReader(body))
	return req.WithContext(contextWithTestUserID(req.Context(), "00000000-0000-0000-0000-000000000001"))
}

func TestInjectBRouterWayTagEnrichmentBuildsSegmentsAndRanges(t *testing.T) {
	raw := `{
	  "type":"FeatureCollection",
	  "features":[{
	    "type":"Feature",
	    "properties":{
	      "messages":[
	        ["Longitude","Latitude","Elevation","Distance","CostPerKm","ElevCost","TurnCost","NodeCost","InitialCost","WayTags","NodeTags","Time","Energy"],
	        ["11576298","48137118","523","14","5050","0","0","0","0","highway=footway surface=paving_stones sac_scale=mountain_hiking","","2","221"],
	        ["11576413","48137263","523","19","7050","0","72","0","0","highway=path surface=ground mtb:scale=2 mtb:scale:uphill=1 trail_visibility=good","","5","523"]
	      ]
	    },
	    "geometry":{"type":"LineString","coordinates":[
	      [11.576124,48.137154,523],
	      [11.576298,48.137118,523],
	      [11.576413,48.137263,523]
	    ]}
	  }]
	}`
	var gj map[string]any
	if err := json.Unmarshal([]byte(raw), &gj); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	injectBRouterWayTagEnrichment(gj)

	props := gj["features"].([]any)[0].(map[string]any)["properties"].(map[string]any)
	segments, ok := props["segments"].([]map[string]any)
	if !ok {
		t.Fatalf("segments type = %T, want []map[string]any", props["segments"])
	}
	if len(segments) != 2 {
		t.Fatalf("segments len = %d, want 2", len(segments))
	}
	if segments[0]["from_idx"] != 0 || segments[0]["to_idx"] != 1 {
		t.Fatalf("first segment indexes = %v..%v, want 0..1", segments[0]["from_idx"], segments[0]["to_idx"])
	}
	if segments[0]["highway"] != "footway" || segments[0]["surface"] != "paving_stones" || segments[0]["sac_scale"] != "T2" {
		t.Fatalf("first segment tags = %#v", segments[0])
	}
	if segments[1]["from_idx"] != 1 || segments[1]["to_idx"] != 2 {
		t.Fatalf("second segment indexes = %v..%v, want 1..2", segments[1]["from_idx"], segments[1]["to_idx"])
	}
	if segments[1]["highway"] != "path" || segments[1]["surface"] != "ground" || segments[1]["mtb_scale"] != "S2" {
		t.Fatalf("second segment tags = %#v", segments[1])
	}
	if segments[1]["mtb_scale_uphill"] != "S1" || segments[1]["trail_visibility"] != "good" {
		t.Fatalf("second segment extended tags = %#v", segments[1])
	}
	if segments[1]["from_dist_m"] != 14.0 || segments[1]["to_dist_m"] != 33.0 {
		t.Fatalf("second segment distances = %v..%v, want 14..33", segments[1]["from_dist_m"], segments[1]["to_dist_m"])
	}

	sacRanges := props["sac_ranges"].(map[string][]CoordRange)
	if got := sacRanges["T2"]; len(got) != 1 || got[0] != (CoordRange{From: 0, To: 2}) {
		t.Fatalf("sac_ranges[T2] = %#v, want [{0 2}]", got)
	}
	mtbRanges := props["mtb_ranges"].(map[string][]CoordRange)
	if got := mtbRanges["S2"]; len(got) != 1 || got[0] != (CoordRange{From: 1, To: 3}) {
		t.Fatalf("mtb_ranges[S2] = %#v, want [{1 3}]", got)
	}
	if _, has := props["forest_pct"]; has {
		t.Fatal("BRouter enrichment must not emit forest_pct")
	}
	if _, has := props["protected_pct"]; has {
		t.Fatal("BRouter enrichment must not emit protected_pct")
	}
}

func TestInjectBRouterWayTagEnrichmentKeepsSafePrefixOnUnmatchedTail(t *testing.T) {
	raw := `{
	  "type":"FeatureCollection",
	  "features":[{
	    "type":"Feature",
	    "properties":{"messages":[
	      ["Longitude","Latitude","Elevation","Distance","WayTags"],
	      ["11576298","48137118","523","14","highway=footway surface=paving_stones"],
	      ["99999999","99999999","523","19","highway=path surface=ground"]
	    ]},
	    "geometry":{"type":"LineString","coordinates":[[11.576124,48.137154,523],[11.576298,48.137118,523]]}
	  }]
	}`
	var gj map[string]any
	if err := json.Unmarshal([]byte(raw), &gj); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	injectBRouterWayTagEnrichment(gj)

	props := gj["features"].([]any)[0].(map[string]any)["properties"].(map[string]any)
	segments := props["segments"].([]map[string]any)
	if len(segments) != 1 {
		t.Fatalf("segments len = %d, want safe prefix len 1", len(segments))
	}
	if segments[0]["highway"] != "footway" {
		t.Fatalf("safe prefix segment = %#v", segments[0])
	}
}

func TestInjectBRouterWayTagEnrichmentNoOpsOnMissingHeader(t *testing.T) {
	raw := `{"type":"FeatureCollection","features":[{
	  "type":"Feature",
	  "properties":{"messages":[["Longitude","Latitude","Distance"],["11576298","48137118","14"]]},
	  "geometry":{"type":"LineString","coordinates":[[11.576124,48.137154,523],[11.576298,48.137118,523]]}
	}]}`
	var gj map[string]any
	if err := json.Unmarshal([]byte(raw), &gj); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	injectBRouterWayTagEnrichment(gj)

	props := gj["features"].([]any)[0].(map[string]any)["properties"].(map[string]any)
	if _, has := props["segments"]; has {
		t.Fatalf("segments must be absent for incomplete messages header, got %#v", props["segments"])
	}
}

func TestInjectBRouterSummitsAddsSummitsPassedFromGeometry(t *testing.T) {
	raw := `{"type":"FeatureCollection","features":[{
	  "type":"Feature",
	  "properties":{"creator":"BRouter-1.7.8"},
	  "geometry":{"type":"LineString","coordinates":[[11.576124,48.137154,523],[11.576298,48.137118,523]]}
	}]}`
	var gj map[string]any
	if err := json.Unmarshal([]byte(raw), &gj); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	called := false
	injectBRouterSummits(context.Background(), gj, 80, func(ctx context.Context, geomWKB []byte, radiusM float64) ([]database.PassedSummit, error) {
		called = true
		if len(geomWKB) == 0 {
			t.Fatal("summit lookup received empty WKB")
		}
		if radiusM != 80 {
			t.Fatalf("radiusM = %v, want 80", radiusM)
		}
		return []database.PassedSummit{{
			OsmID:    42,
			Name:     "Test Summit",
			Category: "summit",
			DistM:    12,
			Lat:      48.137154,
			Lng:      11.576124,
		}}, nil
	})

	if !called {
		t.Fatal("summit lookup was not called")
	}
	props := gj["features"].([]any)[0].(map[string]any)["properties"].(map[string]any)
	summits, ok := props["summits_passed"].([]ResponsePassedSummit)
	if !ok {
		t.Fatalf("summits_passed missing or wrong type: %#v", props["summits_passed"])
	}
	if len(summits) != 1 {
		t.Fatalf("len(summits_passed) = %d, want 1", len(summits))
	}
	if summits[0].Name != "Test Summit" || summits[0].DistM != 12 {
		t.Fatalf("unexpected summit DTO: %#v", summits[0])
	}
}

func TestRouteViabrouterAddsProcessUnusedTagsDefaultAndEnrichesResponse(t *testing.T) {
	capabilities.InvalidateCache()
	t.Cleanup(capabilities.InvalidateCache)
	var gotProcessUnusedTags string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/brouter" {
			w.WriteHeader(http.StatusOK)
			return
		}
		gotProcessUnusedTags = r.URL.Query().Get("profile:processUnusedTags")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
		  "type":"FeatureCollection",
		  "features":[{
		    "type":"Feature",
		    "properties":{
		      "creator":"BRouter-1.7.8",
		      "track-length":"14",
		      "messages":[
		        ["Longitude","Latitude","Elevation","Distance","WayTags"],
		        ["11576298","48137118","523","14","highway=footway surface=paving_stones"]
		      ]
		    },
		    "geometry":{"type":"LineString","coordinates":[[11.576124,48.137154,523],[11.576298,48.137118,523]]}
		  }]
		}`))
	}))
	defer server.Close()

	h := &Handler{Config: &HandlerConfig{DeploymentProfile: "full", BRouterURL: server.URL}}
	req := newBRouterProxyTestRequest(`{
		"waypoints": [[48.137154,11.576124],[48.137118,11.576298]],
		"profile": "trekking"
	}`)
	w := httptest.NewRecorder()

	h.RouteViabrouter(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	if gotProcessUnusedTags != "1" {
		t.Fatalf("profile:processUnusedTags = %q, want 1", gotProcessUnusedTags)
	}
	var gj map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &gj); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	props := gj["features"].([]any)[0].(map[string]any)["properties"].(map[string]any)
	if props["elevation_profile"] == nil {
		t.Fatal("elevation_profile must still be injected")
	}
	if _, has := props["segments"]; !has {
		t.Fatalf("segments missing from enriched response properties: %#v", props)
	}
}

func TestEnsureBRouterWayTagProfileParamsNormalizesBooleanProcessUnusedTags(t *testing.T) {
	params := ensureBRouterWayTagProfileParams(map[string]string{"processUnusedTags": "true"})
	if params["processUnusedTags"] != "1" {
		t.Fatalf("true processUnusedTags normalized to %q, want 1", params["processUnusedTags"])
	}

	params = ensureBRouterWayTagProfileParams(map[string]string{"processUnusedTags": "false"})
	if params["processUnusedTags"] != "0" {
		t.Fatalf("false processUnusedTags normalized to %q, want 0", params["processUnusedTags"])
	}

	params = ensureBRouterWayTagProfileParams(map[string]string{"processUnusedTags": "0"})
	if params["processUnusedTags"] != "0" {
		t.Fatalf("explicit numeric processUnusedTags overwritten to %q, want 0", params["processUnusedTags"])
	}
}

func TestBuildProfileFromInlineZ_NoZ_ReturnsNil(t *testing.T) {
	coords := []brouterCoord{bcNoZ(11.404, 47.270), bcNoZ(11.405, 47.271)}
	if got := buildProfileFromInlineZ(coords); got != nil {
		t.Errorf("want nil for no-Z coords, got %v", got)
	}
}

func TestBuildProfileFromInlineZ_AllZeroZ_ReturnsNil(t *testing.T) {
	coords := []brouterCoord{bc(11.404, 47.270, 0), bc(11.405, 47.271, 0)}
	if got := buildProfileFromInlineZ(coords); got != nil {
		t.Errorf("want nil for all-zero Z, got %v", got)
	}
}

func TestBuildProfileFromInlineZ_TooFewPoints_ReturnsNil(t *testing.T) {
	if got := buildProfileFromInlineZ(nil); got != nil {
		t.Errorf("want nil for nil coords, got %v", got)
	}
	if got := buildProfileFromInlineZ([]brouterCoord{bc(11.4, 47.2, 580)}); got != nil {
		t.Errorf("want nil for single coord, got %v", got)
	}
}

func TestBuildProfileFromInlineZ_PartialZ_DistanceContinues(t *testing.T) {
	coords := []brouterCoord{
		bc(11.404, 47.270, 581),
		bcNoZ(11.405, 47.271),
		bc(11.406, 47.272, 600),
	}
	profile := buildProfileFromInlineZ(coords)
	if len(profile) != 2 {
		t.Fatalf("want 2 profile points (mid-point dropped), got %d", len(profile))
	}
	if !(profile[1]["dist_m"].(float64) > profile[0]["dist_m"].(float64)) {
		t.Errorf("dist_m must keep increasing across dropped middle point")
	}
}

func TestBuildProfileFromInlineZ_SentinelZ_Filtered(t *testing.T) {
	coords := []brouterCoord{
		bc(11.404, 47.270, -9999),
		bc(11.405, 47.271, 590),
		bc(11.406, 47.272, 99999),
		bc(11.407, 47.273, 600),
	}
	profile := buildProfileFromInlineZ(coords)
	if len(profile) != 2 {
		t.Fatalf("want 2 profile points (sentinels filtered), got %d", len(profile))
	}
	assertInDelta(t, 590.0, profile[0]["ele_m"].(float64), 0.01, "profile[0].ele_m")
	assertInDelta(t, 600.0, profile[1]["ele_m"].(float64), 0.01, "profile[1].ele_m")
}

func TestBuildProfileFromInlineZ_AllSentinel_ReturnsNil(t *testing.T) {
	coords := []brouterCoord{bc(11.404, 47.270, -9999), bc(11.405, 47.271, 99999)}
	if got := buildProfileFromInlineZ(coords); got != nil {
		t.Errorf("want nil for all-sentinel Z, got %v", got)
	}
}

// ---- extractCoordinates ----

func TestExtractCoordinates_NullZIsPerCoord_NotGlobalDiscard(t *testing.T) {
	raw := `{"geometry":{"type":"LineString","coordinates":[
	  [11.404, 47.270, 581],
	  [11.405, 47.271, null],
	  [11.406, 47.272, 600]
	]}}`
	var feat map[string]any
	if err := json.Unmarshal([]byte(raw), &feat); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	coords := extractCoordinates(feat)
	if len(coords) != 3 {
		t.Fatalf("want 3 coords (null Z preserved as EleOK=false), got %d", len(coords))
	}
	if !coords[0].EleOK {
		t.Errorf("coords[0].EleOK want true")
	}
	if coords[1].EleOK {
		t.Errorf("coords[1].EleOK want false (null Z)")
	}
	if !coords[2].EleOK {
		t.Errorf("coords[2].EleOK want true")
	}
}

// ---- injectElevationProfileFromInlineZ ----

func TestInjectElevationProfileFromInlineZ_RealBrouterShape(t *testing.T) {
	raw := `{
	  "type":"FeatureCollection",
	  "features":[{
	    "type":"Feature",
	    "properties":{"creator":"BRouter-1.7.8","track-length":"118","cost":"155"},
	    "geometry":{"type":"LineString","coordinates":[[11.404,47.270,581],[11.405,47.271,590]]}
	  }]
	}`
	var gj map[string]any
	if err := json.Unmarshal([]byte(raw), &gj); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	injectElevationProfileFromInlineZ(gj, nil)

	feat := gj["features"].([]any)[0].(map[string]any)
	props := feat["properties"].(map[string]any)
	profileRaw, has := props["elevation_profile"]
	if !has {
		t.Fatal("elevation_profile key must be present")
	}
	if profileRaw == nil {
		t.Fatal("elevation_profile must NOT be nil for 3D BRouter coords")
	}
	profile := profileRaw.([]map[string]any)
	if len(profile) != 2 {
		t.Fatalf("want 2 profile points, got %d", len(profile))
	}
	assertInDelta(t, 581.0, profile[0]["ele_m"].(float64), 0.01, "profile[0].ele_m")
	assertInDelta(t, 590.0, profile[1]["ele_m"].(float64), 0.01, "profile[1].ele_m")
	// Original properties must be preserved.
	if props["creator"] != "BRouter-1.7.8" {
		t.Errorf("original property 'creator' lost, got %v", props["creator"])
	}
	if props["cost"] != "155" {
		t.Errorf("original property 'cost' lost, got %v", props["cost"])
	}
}

func TestInjectElevationProfileFromInlineZ_PlainWrapperNoZ_NullProfile(t *testing.T) {
	raw := `{"type":"FeatureCollection","features":[{
	  "type":"Feature","properties":{"name":"a"},
	  "geometry":{"type":"LineString","coordinates":[[11.404,47.270],[11.405,47.271]]}
	}]}`
	var gj map[string]any
	if err := json.Unmarshal([]byte(raw), &gj); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	injectElevationProfileFromInlineZ(gj, nil)
	feat := gj["features"].([]any)[0].(map[string]any)
	props := feat["properties"].(map[string]any)
	val, has := props["elevation_profile"]
	if !has {
		t.Fatal("elevation_profile key must be present")
	}
	if val != nil {
		t.Errorf("no Z available → elevation_profile must be nil, got %v", val)
	}
}

func TestInjectElevationProfileFromInlineZ_AlternativesMultiFeature(t *testing.T) {
	raw := `{"type":"FeatureCollection","features":[
	  {"type":"Feature","properties":{"name":"a"},"geometry":{"type":"LineString","coordinates":[[11.4,47.2,580],[11.41,47.21,590]]}},
	  {"type":"Feature","properties":{"name":"b"},"geometry":{"type":"LineString","coordinates":[[11.4,47.2,580],[11.41,47.21,595]]}}
	]}`
	var gj map[string]any
	if err := json.Unmarshal([]byte(raw), &gj); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	injectElevationProfileFromInlineZ(gj, nil)
	for i, f := range gj["features"].([]any) {
		props := f.(map[string]any)["properties"].(map[string]any)
		profileRaw, has := props["elevation_profile"]
		if !has {
			t.Errorf("feature %d missing elevation_profile key", i)
		}
		if profileRaw == nil {
			t.Errorf("feature %d must have non-nil profile", i)
		}
	}
}

func TestInjectElevationProfileFromInlineZ_NoFeaturesKey_NoOp(t *testing.T) {
	gj := map[string]any{"type": "FeatureCollection"}
	injectElevationProfileFromInlineZ(gj, nil) // must not panic
	if _, has := gj["features"]; has {
		t.Errorf("features key must remain absent")
	}
}

func TestInjectElevationProfileFromInlineZ_FeatureWithoutProperties(t *testing.T) {
	raw := `{"type":"FeatureCollection","features":[
	  {"type":"Feature","geometry":{"type":"LineString","coordinates":[[11.4,47.2,580],[11.41,47.21,590]]}}
	]}`
	var gj map[string]any
	if err := json.Unmarshal([]byte(raw), &gj); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	injectElevationProfileFromInlineZ(gj, nil)
	feat := gj["features"].([]any)[0].(map[string]any)
	props, ok := feat["properties"].(map[string]any)
	if !ok {
		t.Fatal("properties must be a map after injection")
	}
	if _, has := props["elevation_profile"]; !has {
		t.Errorf("elevation_profile must be present")
	}
}

func TestInjectElevationProfileFromInlineZ_PropertiesIsNull_Defensive(t *testing.T) {
	raw := `{"type":"FeatureCollection","features":[
	  {"type":"Feature","properties":null,"geometry":{"type":"LineString","coordinates":[[11.4,47.2,580],[11.41,47.21,590]]}}
	]}`
	var gj map[string]any
	if err := json.Unmarshal([]byte(raw), &gj); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	injectElevationProfileFromInlineZ(gj, nil)
	feat := gj["features"].([]any)[0].(map[string]any)
	props, ok := feat["properties"].(map[string]any)
	if !ok {
		t.Fatal("properties must become a map (was null)")
	}
	if _, has := props["elevation_profile"]; !has {
		t.Errorf("elevation_profile must be present")
	}
}

func TestInjectElevationProfileFromInlineZ_PropertiesIsArray_Defensive(t *testing.T) {
	raw := `{"type":"FeatureCollection","features":[
	  {"type":"Feature","properties":["unexpected","array"],"geometry":{"type":"LineString","coordinates":[[11.4,47.2,580]]}}
	]}`
	var gj map[string]any
	if err := json.Unmarshal([]byte(raw), &gj); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	injectElevationProfileFromInlineZ(gj, nil)
	feat := gj["features"].([]any)[0].(map[string]any)
	propsArr, isArr := feat["properties"].([]any)
	if !isArr {
		t.Fatal("properties array must be preserved")
	}
	if len(propsArr) != 2 {
		t.Errorf("want 2 elements in preserved array, got %d", len(propsArr))
	}
}

func TestInjectElevationProfileFromInlineZ_PropertiesIsString_Defensive(t *testing.T) {
	raw := `{"type":"FeatureCollection","features":[
	  {"type":"Feature","properties":"unexpected-string","geometry":{"type":"LineString","coordinates":[[11.4,47.2,580]]}}
	]}`
	var gj map[string]any
	if err := json.Unmarshal([]byte(raw), &gj); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	injectElevationProfileFromInlineZ(gj, nil)
	feat := gj["features"].([]any)[0].(map[string]any)
	if feat["properties"] != "unexpected-string" {
		t.Errorf("string properties must be preserved, got %v", feat["properties"])
	}
}

func TestInjectElevationProfileFromInlineZ_NonFeatureCollection_NoOp(t *testing.T) {
	gj := map[string]any{"error": "no route possible"}
	injectElevationProfileFromInlineZ(gj, nil)
	if gj["error"] != "no route possible" {
		t.Errorf("error key must remain unchanged, got %v", gj["error"])
	}
}

// TestExtractCoordinates_MalformedCoords_PreservesDistanceConsistency verifies
// Critic finding #11 (MAJOR): when BRouter emits a null or non-array element
// inside geometry.coordinates, the distance axis in the injected elevation_profile
// is internally consistent with the wire coordinate list received by the frontend.
//
// Architecture note: extractCoordinates drops malformed coord entries (null /
// non-array / non-numeric lon-lat) because they have no valid position and
// cannot participate in Haversine accumulation. The frontend receives the original
// geometry.coordinates unchanged — including the null entry. Both sides therefore
// skip the same unusable point, so profile dist_m values match what the frontend
// would compute over the non-null wire coords. No drift.
//
// This test confirms three invariants:
//  1. geometry.coordinates in the wire GeoJSON is NOT mutated (still contains
//     the null entry at index 1).
//  2. extractCoordinates returns only the 2 valid coords (null dropped).
//  3. Profile dist_m[1] equals the Haversine distance between the 2 valid coords
//     (i.e. the distance axis is consistent with the reduced coord list that
//     both the proxy and any null-aware frontend consumer would use).
func TestExtractCoordinates_MalformedCoords_PreservesDistanceConsistency(t *testing.T) {
	// BRouter edge-case: null element between two valid 3D coords.
	raw := `{"type":"FeatureCollection","features":[{
	  "type":"Feature",
	  "properties":{},
	  "geometry":{"type":"LineString","coordinates":[
	    [11.404, 47.270, 581],
	    null,
	    [11.406, 47.272, 600]
	  ]}
	}]}`

	var gj map[string]any
	if err := json.Unmarshal([]byte(raw), &gj); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	// Invariant 1: geometry.coordinates is unchanged after inject.
	injectElevationProfileFromInlineZ(gj, nil)

	feat := gj["features"].([]any)[0].(map[string]any)
	geom := feat["geometry"].(map[string]any)
	wireCoordsRaw := geom["coordinates"].([]any)
	if len(wireCoordsRaw) != 3 {
		t.Fatalf("wire geometry.coordinates: want 3 elements (incl. null), got %d", len(wireCoordsRaw))
	}
	if wireCoordsRaw[1] != nil {
		t.Errorf("wire coordinates[1] must remain nil (null in JSON), got %v", wireCoordsRaw[1])
	}

	// Invariant 2: extractCoordinates drops the null, returns 2 valid coords.
	coords := extractCoordinates(feat)
	if len(coords) != 2 {
		t.Fatalf("extractCoordinates: want 2 valid coords (null dropped), got %d", len(coords))
	}

	// Invariant 3: profile dist_m[1] = Haversine(coord0, coord1) — i.e. the
	// distance axis uses only the coords that extractCoordinates produced.
	props := feat["properties"].(map[string]any)
	profileRaw := props["elevation_profile"]
	if profileRaw == nil {
		t.Fatal("elevation_profile must not be nil (both valid coords have usable Z)")
	}
	profile := profileRaw.([]map[string]any)
	if len(profile) != 2 {
		t.Fatalf("profile: want 2 points, got %d", len(profile))
	}
	assertInDelta(t, 0.0, profile[0]["dist_m"].(float64), 0.01, "profile[0].dist_m")

	// Expected distance: Haversine between the two valid extracted coords.
	wantDist := haversineM(coords[0].Lat, coords[0].Lon, coords[1].Lat, coords[1].Lon)
	assertInDelta(t, wantDist, profile[1]["dist_m"].(float64), 1.0,
		"profile[1].dist_m must equal Haversine(coord0, coord1) — no drift")
}

// ---- DEM fallback (fillMissingElevFromDEM / injectElevationProfileFromInlineZ with DEMSvc) ----

// stubBrouterDEM implements dem.Servicer for BRouter proxy tests. It returns
// a per-position slice of decimeter values supplied at construction time.
// A position entry of dem.NoData signals absent coverage.
type stubBrouterDEM struct {
	// values is indexed by call order (i.e. the order of missing coords passed
	// in a single ElevationBatch call). Panic on out-of-bounds is intentional —
	// it catches bugs in how many coords are requested.
	values []int32
	err    error
}

func (s *stubBrouterDEM) ElevationBatch(lats, lons []int32) ([]int32, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.values[:len(lats)], nil
}

func (s *stubBrouterDEM) Health() dem.Health { return dem.Health{} }
func (s *stubBrouterDEM) Close()             {}

// TestBRouterProxy_MissingInlineZ_FallbackToService verifies that when BRouter
// sends a 2D-only response (no inline-Z), dem.Service fills the full profile.
func TestBRouterProxy_MissingInlineZ_FallbackToService(t *testing.T) {
	raw := `{"type":"FeatureCollection","features":[{
	  "type":"Feature","properties":{"name":"test"},
	  "geometry":{"type":"LineString","coordinates":[[11.404,47.270],[11.405,47.271],[11.406,47.272]]}
	}]}`
	var gj map[string]any
	if err := json.Unmarshal([]byte(raw), &gj); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	// DEM returns 5810, 5900, 6000 decimeters → 581.0, 590.0, 600.0 m
	svc := &stubBrouterDEM{values: []int32{5810, 5900, 6000}}
	injectElevationProfileFromInlineZ(gj, svc)

	feat := gj["features"].([]any)[0].(map[string]any)
	props := feat["properties"].(map[string]any)
	profileRaw := props["elevation_profile"]
	if profileRaw == nil {
		t.Fatal("elevation_profile must not be nil after DEM fallback")
	}
	profile := profileRaw.([]map[string]any)
	if len(profile) != 3 {
		t.Fatalf("want 3 profile points, got %d", len(profile))
	}
	assertInDelta(t, 581.0, profile[0]["ele_m"].(float64), 0.01, "profile[0].ele_m")
	assertInDelta(t, 590.0, profile[1]["ele_m"].(float64), 0.01, "profile[1].ele_m")
	assertInDelta(t, 600.0, profile[2]["ele_m"].(float64), 0.01, "profile[2].ele_m")
}

// TestBRouterProxy_PartialInlineZ_OnlyMissingFilled verifies that only coords
// lacking plausible inline-Z are replaced by DEM; coords with valid Z are kept.
func TestBRouterProxy_PartialInlineZ_OnlyMissingFilled(t *testing.T) {
	// coord[1] has Z=0 (implausible per isPlausibleSrtmEle) → DEM fills it.
	// coord[0] and coord[2] have valid inline-Z and must not be changed.
	raw := `{"type":"FeatureCollection","features":[{
	  "type":"Feature","properties":{},
	  "geometry":{"type":"LineString","coordinates":[[11.404,47.270,581],[11.405,47.271,0],[11.406,47.272,600]]}
	}]}`
	var gj map[string]any
	if err := json.Unmarshal([]byte(raw), &gj); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	// DEM is consulted only for the single missing coord[1] → 5900 dm = 590.0 m.
	svc := &stubBrouterDEM{values: []int32{5900}}
	injectElevationProfileFromInlineZ(gj, svc)

	feat := gj["features"].([]any)[0].(map[string]any)
	props := feat["properties"].(map[string]any)
	profile := props["elevation_profile"].([]map[string]any)
	if len(profile) != 3 {
		t.Fatalf("want 3 profile points (gap filled), got %d", len(profile))
	}
	assertInDelta(t, 581.0, profile[0]["ele_m"].(float64), 0.01, "profile[0].ele_m — inline-Z unchanged")
	assertInDelta(t, 590.0, profile[1]["ele_m"].(float64), 0.01, "profile[1].ele_m — from DEM")
	assertInDelta(t, 600.0, profile[2]["ele_m"].(float64), 0.01, "profile[2].ele_m — inline-Z unchanged")
}

// TestBRouterProxy_NilDEMSvc_FallbackSkipped verifies that nil DEMSvc causes
// no crash and leaves the profile as built from inline-Z (or nil if no Z).
func TestBRouterProxy_NilDEMSvc_FallbackSkipped(t *testing.T) {
	// 2D-only BRouter response → no inline-Z.
	raw := `{"type":"FeatureCollection","features":[{
	  "type":"Feature","properties":{},
	  "geometry":{"type":"LineString","coordinates":[[11.404,47.270],[11.405,47.271]]}
	}]}`
	var gj map[string]any
	if err := json.Unmarshal([]byte(raw), &gj); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	// Must not panic and profile must stay nil (no inline-Z, no DEM service).
	injectElevationProfileFromInlineZ(gj, nil)

	feat := gj["features"].([]any)[0].(map[string]any)
	props := feat["properties"].(map[string]any)
	val := props["elevation_profile"]
	if val != nil {
		t.Errorf("nil DEMSvc → elevation_profile must remain nil, got %v", val)
	}
}

// TestBRouterProxy_DEMNoData_NullEleM verifies that DEM NoData responses
// cause the corresponding coord to be absent from the profile (EleOK=false
// → buildProfileFromInlineZ omits the point).
func TestBRouterProxy_DEMNoData_NullEleM(t *testing.T) {
	// All three coords lack inline-Z; DEM returns NoData for coord[1].
	raw := `{"type":"FeatureCollection","features":[{
	  "type":"Feature","properties":{},
	  "geometry":{"type":"LineString","coordinates":[[11.404,47.270],[11.405,47.271],[11.406,47.272]]}
	}]}`
	var gj map[string]any
	if err := json.Unmarshal([]byte(raw), &gj); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	// coord[1] → NoData; coord[0] and coord[2] get real values.
	svc := &stubBrouterDEM{values: []int32{5810, dem.NoData, 6000}}
	injectElevationProfileFromInlineZ(gj, svc)

	feat := gj["features"].([]any)[0].(map[string]any)
	props := feat["properties"].(map[string]any)
	profileRaw := props["elevation_profile"]
	if profileRaw == nil {
		t.Fatal("elevation_profile must not be nil (two coords have DEM data)")
	}
	profile := profileRaw.([]map[string]any)
	// coord[1] is omitted due to NoData — only coord[0] and coord[2] remain.
	if len(profile) != 2 {
		t.Fatalf("want 2 profile points (NoData coord omitted), got %d", len(profile))
	}
	assertInDelta(t, 581.0, profile[0]["ele_m"].(float64), 0.01, "profile[0].ele_m")
	assertInDelta(t, 600.0, profile[1]["ele_m"].(float64), 0.01, "profile[1].ele_m")
}

// haversineM computes the Haversine distance in metres between two lat/lon points.
// Mirrors the geo.DistanceHaversine formula used in buildProfileFromInlineZ so
// we can assert the expected value without importing the geo package.
func haversineM(lat1, lon1, lat2, lon2 float64) float64 {
	const R = 6371000.0
	toRad := func(d float64) float64 { return d * math.Pi / 180 }
	dLat := toRad(lat2 - lat1)
	dLon := toRad(lon2 - lon1)
	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(toRad(lat1))*math.Cos(toRad(lat2))*math.Sin(dLon/2)*math.Sin(dLon/2)
	return R * 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
}
