package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSearchAddressReturnsExternalStatusWithoutConfiguredForwardProvider(t *testing.T) {
	h := &Handler{}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/geo/search?q=Example&type=all&limit=5&external=1", nil)
	rec := httptest.NewRecorder()

	h.SearchAddress(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
	var body struct {
		Results        []map[string]any `json:"results"`
		Count          int              `json:"count"`
		LocalStatus    string           `json:"local_status"`
		ExternalStatus string           `json:"external_status"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Results == nil {
		t.Fatal("results is nil, want empty array")
	}
	if body.LocalStatus != "unavailable" {
		t.Fatalf("local_status = %q, want unavailable", body.LocalStatus)
	}
	if body.ExternalStatus != "disabled" {
		t.Fatalf("external_status = %q, want disabled", body.ExternalStatus)
	}
}
