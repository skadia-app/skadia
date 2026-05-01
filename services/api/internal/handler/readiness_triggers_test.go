//go:build integration

package handler

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

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

// buildGarminGDPRZip returns a minimal Garmin GDPR export ZIP containing a
// user_profile.json file so that only the ProfileImporter is detected.
// ProfileImporter needs no ShadowFS or ImportService — it only updates the
// users row, making it ideal for integration tests without full service wiring.
func buildGarminGDPRZip(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)

	profileJSON := `{"gender":"MALE","birthDate":"1990-01-01"}`
	w, err := zw.Create("DI_CONNECT/DI-Connect-User/user_profile.json")
	if err != nil {
		t.Fatalf("buildGarminGDPRZip: create zip entry: %v", err)
	}
	if _, err := w.Write([]byte(profileJSON)); err != nil {
		t.Fatalf("buildGarminGDPRZip: write zip entry: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("buildGarminGDPRZip: close zip: %v", err)
	}
	return buf.Bytes()
}

// buildGarminMultipart wraps zipBytes in a multipart/form-data body with field
// name "file" as expected by UploadGarminZIP → parseUploadedZIP.
func buildGarminMultipart(t *testing.T, zipBytes []byte) (*bytes.Buffer, string) {
	t.Helper()
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	fw, err := mw.CreateFormFile("file", "export.zip")
	if err != nil {
		t.Fatalf("buildGarminMultipart: CreateFormFile: %v", err)
	}
	if _, err := fw.Write(zipBytes); err != nil {
		t.Fatalf("buildGarminMultipart: write: %v", err)
	}
	if err := mw.Close(); err != nil {
		t.Fatalf("buildGarminMultipart: close: %v", err)
	}
	return &body, mw.FormDataContentType()
}

// waitForInvalidate polls fake until it records at least one call, or until
// deadline. Returns true when a call was recorded in time.
func waitForInvalidate(t *testing.T, fake *event.FakeInvalidator, timeout time.Duration) bool {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if len(fake.CallsCopy()) > 0 {
			return true
		}
		time.Sleep(20 * time.Millisecond)
	}
	return false
}

// TestUploadGarminZIP_TriggersAnalyticsInvalidate verifies garmin_import.go:195
// calls AnalyticsInvalidator.PublishAnalyticsInvalidate after a successful ZIP
// upload job. The goroutine inside startImportAsync is async, so the test
// polls via waitForInvalidate with a 10 s deadline.
func TestUploadGarminZIP_TriggersAnalyticsInvalidate(t *testing.T) {
	pool := fusionTestPool(t)
	uid := insertTestUser(t, pool)
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(),
			`DELETE FROM garmin_import_jobs WHERE user_id = $1`, uid)
	})

	fake := &event.FakeInvalidator{}
	h := &Handler{
		DB:                   &database.DB{Pool: pool},
		AnalyticsInvalidator: fake,
		// ShadowFS and ImportService left nil: ProfileImporter does not need them.
	}

	zipBytes := buildGarminGDPRZip(t)
	body, ct := buildGarminMultipart(t, zipBytes)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/import/garmin/upload-zip", body)
	req.Header.Set("Content-Type", ct)
	req = req.WithContext(withUserAndAdmin(req.Context(), uid, false))
	rec := httptest.NewRecorder()
	h.UploadGarminZIP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("UploadGarminZIP: code = %d body = %s, want 202", rec.Code, rec.Body.String())
	}

	// The async goroutine runs the import and calls Invalidate when done.
	if !waitForInvalidate(t, fake, 10*time.Second) {
		t.Fatalf("FakeInvalidator.Calls still empty after 10 s — Invalidate not triggered")
	}
	calls := fake.CallsCopy()
	if len(calls) != 1 {
		t.Fatalf("FakeInvalidator.Calls = %d, want 1", len(calls))
	}
	if calls[0] != uid {
		t.Errorf("FakeInvalidator.Calls[0] = %q, want %q", calls[0], uid)
	}
}

// TestUploadGarminZIP_MissingFile_NoInvalidate verifies that a malformed
// multipart request (no "file" field) returns 400 before any job is created
// and therefore never fires PublishAnalyticsInvalidate.
func TestUploadGarminZIP_MissingFile_NoInvalidate(t *testing.T) {
	pool := fusionTestPool(t)
	uid := insertTestUser(t, pool)

	fake := &event.FakeInvalidator{}
	h := &Handler{
		DB:                   &database.DB{Pool: pool},
		AnalyticsInvalidator: fake,
	}

	// Build multipart with a field other than "file" so FormFile("file") fails.
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	_ = mw.WriteField("other", "value")
	_ = mw.Close()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/import/garmin/upload-zip", &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req = req.WithContext(withUserAndAdmin(req.Context(), uid, false))
	rec := httptest.NewRecorder()
	h.UploadGarminZIP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("missing-file: code = %d, want 400", rec.Code)
	}
	// No goroutine was started — Invalidate must never fire.
	time.Sleep(100 * time.Millisecond)
	if got := fake.CallsCopy(); len(got) != 0 {
		t.Errorf("FakeInvalidator.Calls = %v, want [] (no job should have started)", got)
	}
}
