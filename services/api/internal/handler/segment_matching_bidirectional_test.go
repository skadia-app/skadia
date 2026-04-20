package handler

import (
	"testing"

	"github.com/skadia-app/skadia/services/api/internal/database"
)

// TestShouldSkipReverseTraversal locks the bidirectional-gate decision used by
// collectSegmentEfforts — see shouldSkipReverseTraversal in segment_matching.go.
func TestShouldSkipReverseTraversal(t *testing.T) {
	cases := []struct {
		name string
		seg  database.CandidateSegment
		want bool
	}{
		{"bidirectional=true → do not skip", database.CandidateSegment{Bidirectional: true}, false},
		{"bidirectional=false → skip", database.CandidateSegment{Bidirectional: false}, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := shouldSkipReverseTraversal(c.seg)
			if got != c.want {
				t.Fatalf("shouldSkipReverseTraversal(%v) = %v, want %v", c.seg.Bidirectional, got, c.want)
			}
		})
	}
}

// TODO(Phase-2): End-to-end matcher-integration test that boots a testDB, seeds
// a bidirectional=false segment + a reverse-direction stream, runs findTraversals,
// and asserts zero reverse-efforts are produced. Depends on the matcher-test
// fixture harness (testDB + stream-builder helpers) which does not yet exist in
// this package. Tracked as Phase-2 test-debt in
// docs/internal/2026-04-20-segment-editor-overhaul-plan.md.
//   - bidirectional=true  + reverse-direction stream → >=1 reverse-effort
//   - bidirectional=false + reverse-direction stream → 0 efforts
//   - bidirectional=false + forward-direction stream → >=1 forward-effort
