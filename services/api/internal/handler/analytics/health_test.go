package analytics

import (
	"net/http/httptest"
	"testing"
)

// ---------------------------------------------------------------------------
// TestGetHRVBackwardsCompat (F004)
// GetHRV uses parseDateOnly, which silently drops activity-filter params
// (sport_type, tags, etc.). Both request variants must produce an identical
// response. No fixture database is available, so the test verifies that both
// code paths reach RequireUserID first and return 401 with an identical body,
// proving the extra-param-ignore contract is in force.
// ---------------------------------------------------------------------------

func TestGetHRVBackwardsCompat(t *testing.T) {
	h := &Handler{DB: nil}

	reqA := httptest.NewRequest("GET",
		"/analytics/hrv?sport_type=foo&tags=bar&from=2024-01-01&to=2024-12-31", nil)
	rrA := httptest.NewRecorder()
	h.GetHRV(rrA, reqA)

	reqB := httptest.NewRequest("GET",
		"/analytics/hrv?from=2024-01-01&to=2024-12-31", nil)
	rrB := httptest.NewRecorder()
	h.GetHRV(rrB, reqB)

	if rrA.Code != 401 {
		t.Errorf("GetHRV (A): expected 401, got %d", rrA.Code)
	}
	if rrB.Code != 401 {
		t.Errorf("GetHRV (B): expected 401, got %d", rrB.Code)
	}
	if rrA.Body.String() != rrB.Body.String() {
		t.Errorf("GetHRV: response body differs between with-extra and without-extra params:\n  A: %s\n  B: %s",
			rrA.Body.String(), rrB.Body.String())
	}
}

// ---------------------------------------------------------------------------
// TestGetSleepBackwardsCompat (F004)
// Same contract as GetHRV: parseDateOnly strips all non-date params.
// ---------------------------------------------------------------------------

func TestGetSleepBackwardsCompat(t *testing.T) {
	h := &Handler{DB: nil}

	reqA := httptest.NewRequest("GET",
		"/analytics/sleep?sport_type=foo&tags=bar&from=2024-01-01&to=2024-12-31", nil)
	rrA := httptest.NewRecorder()
	h.GetSleep(rrA, reqA)

	reqB := httptest.NewRequest("GET",
		"/analytics/sleep?from=2024-01-01&to=2024-12-31", nil)
	rrB := httptest.NewRecorder()
	h.GetSleep(rrB, reqB)

	if rrA.Code != 401 {
		t.Errorf("GetSleep (A): expected 401, got %d", rrA.Code)
	}
	if rrB.Code != 401 {
		t.Errorf("GetSleep (B): expected 401, got %d", rrB.Code)
	}
	if rrA.Body.String() != rrB.Body.String() {
		t.Errorf("GetSleep: response body differs between with-extra and without-extra params:\n  A: %s\n  B: %s",
			rrA.Body.String(), rrB.Body.String())
	}
}

// ---------------------------------------------------------------------------
// TestGetWeightBackwardsCompat (F004)
// Same contract as GetHRV: parseDateOnly strips all non-date params.
// ---------------------------------------------------------------------------

func TestGetWeightBackwardsCompat(t *testing.T) {
	h := &Handler{DB: nil}

	reqA := httptest.NewRequest("GET",
		"/analytics/weight?sport_type=foo&tags=bar&from=2024-01-01&to=2024-12-31", nil)
	rrA := httptest.NewRecorder()
	h.GetWeight(rrA, reqA)

	reqB := httptest.NewRequest("GET",
		"/analytics/weight?from=2024-01-01&to=2024-12-31", nil)
	rrB := httptest.NewRecorder()
	h.GetWeight(rrB, reqB)

	if rrA.Code != 401 {
		t.Errorf("GetWeight (A): expected 401, got %d", rrA.Code)
	}
	if rrB.Code != 401 {
		t.Errorf("GetWeight (B): expected 401, got %d", rrB.Code)
	}
	if rrA.Body.String() != rrB.Body.String() {
		t.Errorf("GetWeight: response body differs between with-extra and without-extra params:\n  A: %s\n  B: %s",
			rrA.Body.String(), rrB.Body.String())
	}
}
