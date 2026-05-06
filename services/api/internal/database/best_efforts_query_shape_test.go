package database

import (
	"os"
	"strings"
	"testing"
)

func TestBestEffortQueriesUseSharedEligibilityPredicate(t *testing.T) {
	source, err := os.ReadFile("best_efforts.go")
	if err != nil {
		t.Fatalf("read best_efforts.go: %v", err)
	}
	const predicateNeedle = "NOT COALESCE(%s.exclude_from_stats, false)"
	if !strings.Contains(string(source), predicateNeedle) {
		t.Fatalf("best_efforts.go must define the shared eligibility predicate format %q", predicateNeedle)
	}
	if got := strings.Count(string(source), "NOT COALESCE(a.exclude_from_stats, false)"); got != 0 {
		t.Fatalf("best-effort queries repeat concrete activity eligibility %d times; use sharedBestEffortActivityEligibilitySQL", got)
	}
}
