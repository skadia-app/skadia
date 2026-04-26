package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/skadia-app/skadia/services/api/internal/dem"
)

// TestHealth_ReportsDemCorruptCells verifies that Healthz includes a "dem"
// key in its response, and that a Service with corrupt cells sets the overall
// status to "degraded".
func TestHealth_ReportsDemCorruptCells(t *testing.T) {
	// Build a service whose opener always returns a corrupt error.
	corruptErr := &demCorruptErr{}
	svc := dem.NewServiceWithOpenerForTest(nil, func(_ []string, _ string) (*dem.HGTGrid, error) {
		return nil, corruptErr
	})
	defer svc.Close()

	// Trigger an open so the corrupt cell is recorded.
	_, _ = svc.ElevationAt(47.5, 8.5)

	h := &Handler{DEMSvc: svc}

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	w := httptest.NewRecorder()
	h.Healthz(w, req)

	resp := w.Result()
	defer resp.Body.Close()

	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	// Overall status must be "degraded" because of corrupt DEM cells.
	if got, ok := body["status"].(string); !ok || got != "degraded" {
		t.Errorf("status = %q, want \"degraded\"", body["status"])
	}

	// "dem" key must be present.
	demRaw, ok := body["dem"]
	if !ok {
		t.Fatal("response missing \"dem\" key")
	}
	demMap, ok := demRaw.(map[string]any)
	if !ok {
		t.Fatalf("dem value type = %T, want map", demRaw)
	}

	// corrupt_cells must be non-empty.
	cells, ok := demMap["corrupt_cells"]
	if !ok {
		t.Fatal("dem.corrupt_cells missing")
	}
	cellSlice, ok := cells.([]any)
	if !ok || len(cellSlice) == 0 {
		t.Errorf("dem.corrupt_cells = %v, want non-empty slice", cells)
	}
}

// TestHealth_DemOk verifies that a clean Service reports dem.status = "ok"
// and does not degrade overall status.
func TestHealth_DemOk(t *testing.T) {
	svc := dem.NewServiceWithOpenerForTest(nil, func(_ []string, _ string) (*dem.HGTGrid, error) {
		return nil, dem.ErrNoCoverage
	})
	defer svc.Close()

	h := &Handler{DEMSvc: svc}
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	w := httptest.NewRecorder()
	h.Healthz(w, req)

	var body map[string]any
	if err := json.NewDecoder(w.Result().Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}

	demRaw := body["dem"]
	demMap, ok := demRaw.(map[string]any)
	if !ok {
		t.Fatalf("dem value type = %T, want map", demRaw)
	}
	if s, _ := demMap["status"].(string); s != "ok" {
		t.Errorf("dem.status = %q, want \"ok\"", s)
	}
}

type demCorruptErr struct{}

func (e *demCorruptErr) Error() string { return "corrupt HGT for test" }
