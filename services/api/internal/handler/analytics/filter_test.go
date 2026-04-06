package analytics

import (
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
