//go:build integration

package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/event"
)

// These tests pin the readiness-trigger contract added in Phase 6: each
// listed handler must call AnalyticsInvalidator.PublishAnalyticsInvalidate
// with the authenticated user's ID after a successful write. Coverage of
// the remaining trigger sites (garmin_import.go:135/195, recompute.go:1147,
// manual_activity.go:231) is deferred — those handlers depend on Garmin
// ZIP fixtures, full PMC pipeline state, or sport-import wiring that this
// segregated-interface refactor does not unblock on its own.

// ingestHealthBody helper builds a JSON body for /import/health-data.
func ingestHealthBody(t *testing.T, typ, date string, data any) []byte {
	t.Helper()
	raw, err := json.Marshal(data)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	body, err := json.Marshal(map[string]any{
		"type": typ,
		"date": date,
		"data": json.RawMessage(raw),
	})
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	return body
}

// TestIngestHealthData_HRV_TriggersAnalyticsInvalidate verifies health_data.go:215
// fires PublishAnalyticsInvalidate after a successful HRV ingest.
func TestIngestHealthData_HRV_TriggersAnalyticsInvalidate(t *testing.T) {
	pool := fusionTestPool(t)
	uid := insertTestUser(t, pool)

	fake := &event.FakeInvalidator{}
	h := &Handler{
		DB:                   &database.DB{Pool: pool},
		AnalyticsInvalidator: fake,
	}

	rmssd := 42.5
	body := ingestHealthBody(t, "hrv", "2026-04-30", map[string]any{
		"rmssd_ms": rmssd,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/import/health-data", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(withUserAndAdmin(req.Context(), uid, false))
	rec := httptest.NewRecorder()
	h.IngestHealthData(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("HRV ingest: code = %d body = %s, want 200", rec.Code, rec.Body.String())
	}
	calls := fake.CallsCopy()
	if len(calls) != 1 {
		t.Fatalf("FakeInvalidator.Calls = %d, want 1", len(calls))
	}
	if calls[0] != uid {
		t.Errorf("FakeInvalidator.Calls[0] = %q, want %q", calls[0], uid)
	}
}

// TestIngestHealthData_RestingHR_TriggersAnalyticsInvalidate verifies the
// resting_hr branch also fires the readiness trigger.
func TestIngestHealthData_RestingHR_TriggersAnalyticsInvalidate(t *testing.T) {
	pool := fusionTestPool(t)
	uid := insertTestUser(t, pool)

	fake := &event.FakeInvalidator{}
	h := &Handler{
		DB:                   &database.DB{Pool: pool},
		AnalyticsInvalidator: fake,
	}

	hr := 52
	body := ingestHealthBody(t, "resting_hr", "2026-04-30", map[string]any{
		"resting_hr_bpm": hr,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/import/health-data", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(withUserAndAdmin(req.Context(), uid, false))
	rec := httptest.NewRecorder()
	h.IngestHealthData(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("resting_hr ingest: code = %d body = %s, want 200", rec.Code, rec.Body.String())
	}
	if got := fake.CallsCopy(); len(got) != 1 || got[0] != uid {
		t.Fatalf("FakeInvalidator.Calls = %v, want [%q]", got, uid)
	}
}

// TestIngestHealthData_Weight_DoesNotTriggerInvalidate ensures only readiness-
// adjacent ingests (hrv/sleep/resting_hr) fire the publisher; weight must not.
func TestIngestHealthData_Weight_DoesNotTriggerInvalidate(t *testing.T) {
	pool := fusionTestPool(t)
	uid := insertTestUser(t, pool)

	fake := &event.FakeInvalidator{}
	h := &Handler{
		DB:                   &database.DB{Pool: pool},
		AnalyticsInvalidator: fake,
	}

	body := ingestHealthBody(t, "weight", "2026-04-30", map[string]any{
		"weight_g": 75000,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/import/health-data", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(withUserAndAdmin(req.Context(), uid, false))
	rec := httptest.NewRecorder()
	h.IngestHealthData(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("weight ingest: code = %d, want 200", rec.Code)
	}
	if got := fake.CallsCopy(); len(got) != 0 {
		t.Errorf("FakeInvalidator.Calls = %v, want []", got)
	}
}

// TestReverseSegment_TriggersAnalyticsInvalidate verifies segment_reverse.go:78
// fires PublishAnalyticsInvalidate after a successful flip.
func TestReverseSegment_TriggersAnalyticsInvalidate(t *testing.T) {
	pool := fusionTestPool(t)
	uid := insertTestUser(t, pool)
	segmentID := insertTestSegment(t, pool, uid, TestSegmentOpts{
		Track: "LINESTRING(0 0, 1 1, 2 2)",
	})

	fake := &event.FakeInvalidator{}
	h := &Handler{
		DB:                   &database.DB{Pool: pool},
		AnalyticsInvalidator: fake,
	}
	r := chi.NewRouter()
	r.Post("/api/v1/segments/{id}/reverse", h.ReverseSegment)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/segments/"+segmentID+"/reverse", nil)
	req = req.WithContext(withUserAndAdmin(req.Context(), uid, false))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("reverse: code = %d body = %s, want 200", rec.Code, rec.Body.String())
	}
	if got := fake.CallsCopy(); len(got) != 1 || got[0] != uid {
		t.Fatalf("FakeInvalidator.Calls = %v, want [%q]", got, uid)
	}
}
