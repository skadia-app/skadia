package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestQueryCoverageRequiresInternalSecretWhenConfigured(t *testing.T) {
	h := &Handler{Config: &HandlerConfig{MLCallbackSecret: "secret"}}
	req := httptest.NewRequest(http.MethodGet, "/internal/coverage/query?lat=47.2&lon=11.4&layers=forest", nil)
	rec := httptest.NewRecorder()

	h.QueryCoverage(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401; body=%s", rec.Code, rec.Body.String())
	}
}

func TestQueryCoverageRejectsInvalidCoordinatesBeforeReader(t *testing.T) {
	h := &Handler{}
	req := httptest.NewRequest(http.MethodGet, "/internal/coverage/query?lat=91&lon=11.4&layers=forest", nil)
	rec := httptest.NewRecorder()

	h.QueryCoverage(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", rec.Code, rec.Body.String())
	}
}

func TestQueryCoverageRejectsInvalidLayerBeforeReader(t *testing.T) {
	h := &Handler{}
	req := httptest.NewRequest(http.MethodGet, "/internal/coverage/query?lat=47.2&lon=11.4&layers=../secret", nil)
	rec := httptest.NewRecorder()

	h.QueryCoverage(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", rec.Code, rec.Body.String())
	}
}

func TestParseCoverageQueryLayersDedupesAndCaps(t *testing.T) {
	layers, err := parseCoverageQueryLayers("forest, traffic,forest")
	if err != nil {
		t.Fatalf("parseCoverageQueryLayers: %v", err)
	}
	if len(layers) != 2 || layers[0] != "forest" || layers[1] != "traffic" {
		t.Fatalf("layers = %#v, want deduped ordered layers", layers)
	}

	if _, err := parseCoverageQueryLayers("forest,landuse,water,protected,urbanization,river_proximity,noise,traffic,populate"); err == nil {
		t.Fatal("parseCoverageQueryLayers accepted too many layers")
	}
}
