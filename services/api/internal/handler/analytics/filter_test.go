package analytics

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestParseFilters_DateRange(t *testing.T) {
	r := httptest.NewRequest("GET", "/analytics/test?from=2025-01-01&to=2025-12-31", nil)
	f := ParseFilters(r)

	if f.From.Format("2006-01-02") != "2025-01-01" {
		t.Errorf("expected from 2025-01-01, got %s", f.From.Format("2006-01-02"))
	}
	if f.To.Format("2006-01-02") != "2025-12-31" {
		t.Errorf("expected to 2025-12-31, got %s", f.To.Format("2006-01-02"))
	}
}

func TestParseFilters_SportTypes(t *testing.T) {
	r := httptest.NewRequest("GET", "/test?sport_type=cycling,running", nil)
	f := ParseFilters(r)

	if len(f.SportTypes) != 2 || f.SportTypes[0] != "cycling" || f.SportTypes[1] != "running" {
		t.Errorf("expected [cycling, running], got %v", f.SportTypes)
	}
}

func TestParseFilters_SubTypes(t *testing.T) {
	r := httptest.NewRequest("GET", "/test?sport_type=cycling&sub_type=mountain,gravel_cycling", nil)
	f := ParseFilters(r)

	if len(f.SubTypes) != 2 || f.SubTypes[0] != "mountain" || f.SubTypes[1] != "gravel_cycling" {
		t.Fatalf("SubTypes = %v, want [mountain gravel_cycling]", f.SubTypes)
	}
}

func TestBuildWhereClause_WithCompositeSportTuples(t *testing.T) {
	r := httptest.NewRequest("GET", "/test?sport_type=cycling:mountain,running:trail", nil)
	f := ParseFilters(r)
	clause, args := f.BuildWhereClause(1)

	if strings.Contains(clause, "a.sport_type = ANY") || strings.Contains(clause, "a.sub_type = ANY") {
		t.Fatalf("expected tuple OR predicate, got cross-product clause %q", clause)
	}
	if !strings.Contains(clause, "a.sport_type = $2 AND COALESCE(a.sub_type, '') = $3") {
		t.Fatalf("missing cycling mountain tuple in %q", clause)
	}
	if !strings.Contains(clause, "a.sport_type = $4 AND COALESCE(a.sub_type, '') = $5") {
		t.Fatalf("missing running trail tuple in %q", clause)
	}
	if len(args) < 5 || args[1] != "cycling" || args[2] != "mountain" || args[3] != "running" || args[4] != "trail" {
		t.Fatalf("args = %#v, want cycling/mountain and running/trail first", args)
	}
}

func TestBuildWhereClause_WithMixedBareAndCompositeSports(t *testing.T) {
	r := httptest.NewRequest("GET", "/test?sport_type=cycling,running:trail", nil)
	f := ParseFilters(r)
	clause, args := f.BuildWhereClause(1)

	if strings.Contains(clause, "a.sub_type = ANY") {
		t.Fatalf("expected tuple OR predicate without subtype cross-product, got %q", clause)
	}
	if !strings.Contains(clause, "a.sport_type = $2") {
		t.Fatalf("missing bare cycling predicate in %q", clause)
	}
	if !strings.Contains(clause, "a.sport_type = $3 AND COALESCE(a.sub_type, '') = $4") {
		t.Fatalf("missing running trail predicate in %q", clause)
	}
	if len(args) < 4 || args[1] != "cycling" || args[2] != "running" || args[3] != "trail" {
		t.Fatalf("args = %#v, want cycling and running/trail first", args)
	}
}

func TestParseFilters_IndoorModifier(t *testing.T) {
	r := httptest.NewRequest("GET", "/test?is_indoor=true", nil)
	f := ParseFilters(r)

	if f.IsIndoor == nil || *f.IsIndoor != true {
		t.Fatalf("IsIndoor = %#v, want true", f.IsIndoor)
	}
}

func TestParseFilters_Seasons(t *testing.T) {
	r := httptest.NewRequest("GET", "/test?season=sommer,winter", nil)
	f := ParseFilters(r)

	if len(f.Seasons) != 2 {
		t.Errorf("expected 2 seasons, got %d", len(f.Seasons))
	}
}

func TestBuildWhereClause_Empty(t *testing.T) {
	f := AnalyticsFilter{}
	clause, args := f.BuildWhereClause(1)
	// Even with no explicit filters, two defensive filters are always applied:
	//   - exclude_from_stats (virtual + manual activities flagged via 6eef1a19)
	//   - is_group_secondary (recording-group secondaries via fc87504e, so the
	//     stats don't double-count members of the same clique)
	// Use Contains checks rather than an exact-string match so future default
	// filters don't break this test mechanically — the point is that both
	// defaults are present and there are no bind args.
	if !strings.Contains(clause, "NOT a.exclude_from_stats") {
		t.Errorf("expected clause to include 'NOT a.exclude_from_stats', got %q", clause)
	}
	if !strings.Contains(clause, "NOT a.is_group_secondary") {
		t.Errorf("expected clause to include 'NOT a.is_group_secondary', got %q", clause)
	}
	if len(args) != 0 {
		t.Errorf("expected zero bind args for empty filter, got %d", len(args))
	}
}

func TestBuildWhereClause_WithFilters(t *testing.T) {
	r := httptest.NewRequest("GET", "/test?sport_type=cycling&is_commute=false", nil)
	f := ParseFilters(r)
	clause, args := f.BuildWhereClause(2)

	if clause == "" {
		t.Error("expected non-empty clause")
	}
	if len(args) < 2 {
		t.Errorf("expected at least 2 args, got %d", len(args))
	}
}

func TestBuildWhereClause_WithSubTypeFilter(t *testing.T) {
	r := httptest.NewRequest("GET", "/test?sport_type=cycling&sub_type=mountain", nil)
	f := ParseFilters(r)
	clause, args := f.BuildWhereClause(1)

	if !strings.Contains(clause, "a.sub_type = ANY") {
		t.Fatalf("expected subtype predicate in %q", clause)
	}
	if len(args) < 2 {
		t.Fatalf("expected sport and subtype args, got %d", len(args))
	}
}

// ---------------------------------------------------------------------------
// parseDateOnly — backwards-compat tests for pure-aggregate handlers
// ---------------------------------------------------------------------------

// TestParseDateOnly_ExtractsFromTo verifies that from/to are parsed correctly.
func TestParseDateOnly_ExtractsFromTo(t *testing.T) {
	r := httptest.NewRequest("GET", "/analytics/hrv?from=2024-01-01&to=2024-12-31", nil)
	from, to := parseDateOnly(r)
	if from.Format("2006-01-02") != "2024-01-01" {
		t.Errorf("from = %s, want 2024-01-01", from.Format("2006-01-02"))
	}
	if to.Format("2006-01-02") != "2024-12-31" {
		t.Errorf("to = %s, want 2024-12-31", to.Format("2006-01-02"))
	}
}

// TestParseDateOnly_IgnoresActivityFilters verifies that extra params (sport_type, tags, etc.)
// have no effect — the returned from/to are identical to the variant without them.
// This is the backwards-compat guarantee for pure-aggregate handlers.
func TestParseDateOnly_IgnoresActivityFilters(t *testing.T) {
	withExtra := httptest.NewRequest("GET", "/analytics/hrv?sport_type=foo&tags=bar&from=2024-01-01&to=2024-12-31", nil)
	withoutExtra := httptest.NewRequest("GET", "/analytics/hrv?from=2024-01-01&to=2024-12-31", nil)

	fromA, toA := parseDateOnly(withExtra)
	fromB, toB := parseDateOnly(withoutExtra)

	if !fromA.Equal(fromB) {
		t.Errorf("from mismatch: with-extra=%s, without-extra=%s", fromA, fromB)
	}
	if !toA.Equal(toB) {
		t.Errorf("to mismatch: with-extra=%s, without-extra=%s", toA, toB)
	}
}

// TestParseDateOnly_DefaultToNow verifies that a missing "to" defaults to today (not zero).
func TestParseDateOnly_DefaultToNow(t *testing.T) {
	r := httptest.NewRequest("GET", "/analytics/hrv?from=2024-01-01", nil)
	_, to := parseDateOnly(r)
	if to.IsZero() {
		t.Error("to must default to time.Now(), got zero value")
	}
}

// TestParseDateOnly_EmptyFromIsZero verifies that a missing "from" stays zero (no lower bound).
func TestParseDateOnly_EmptyFromIsZero(t *testing.T) {
	r := httptest.NewRequest("GET", "/analytics/hrv", nil)
	from, _ := parseDateOnly(r)
	if !from.IsZero() {
		t.Errorf("from must be zero when absent, got %s", from)
	}
}

// ---------------------------------------------------------------------------
// Handler 401 early-return — pure-aggregate handlers (no DB needed)
// ---------------------------------------------------------------------------

// TestPureAggregateHandlers_NoUserID verifies that HRV, Sleep, and Weight handlers
// return 401 before any DB call when no user identity is present. This also confirms
// that the parseDateOnly refactor does not introduce any panic on missing user.
func TestPureAggregateHandlers_NoUserID(t *testing.T) {
	h := &Handler{DB: nil}

	handlers := []struct {
		name string
		fn   func(http.ResponseWriter, *http.Request)
	}{
		{"GetHRV", h.GetHRV},
		{"GetSleep", h.GetSleep},
		{"GetWeight", h.GetWeight},
	}

	for _, hh := range handlers {
		t.Run(hh.name, func(t *testing.T) {
			// Variant A: extra ignored params present.
			reqA := httptest.NewRequest("GET", "/analytics?sport_type=foo&tags=bar&from=2024-01-01&to=2024-12-31", nil)
			rrA := httptest.NewRecorder()
			hh.fn(rrA, reqA)
			if rrA.Code != 401 {
				t.Errorf("%s (A): expected 401, got %d", hh.name, rrA.Code)
			}

			// Variant B: only date params.
			reqB := httptest.NewRequest("GET", "/analytics?from=2024-01-01&to=2024-12-31", nil)
			rrB := httptest.NewRecorder()
			hh.fn(rrB, reqB)
			if rrB.Code != 401 {
				t.Errorf("%s (B): expected 401, got %d", hh.name, rrB.Code)
			}
		})
	}
}
