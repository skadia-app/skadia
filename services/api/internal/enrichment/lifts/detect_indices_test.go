package lifts

import (
	"testing"
	"time"
)

func TestDetectHeuristicWithIndicesMatchesPublicDetector(t *testing.T) {
	cfg := DefaultDetectorConfig("ski")
	start := time.Date(2026, 1, 1, 8, 0, 0, 0, time.UTC)
	rows := makeStream(start, 220, 47.0, 12.0, 1200, 1.0, 5e-5, 5.0)

	indexed := DetectHeuristicWithIndices(rows, cfg)
	public := DetectHeuristic(rows, cfg)

	if len(indexed) != 1 {
		t.Fatalf("DetectHeuristicWithIndices returned %d rides, want 1", len(indexed))
	}
	if len(public) != len(indexed) {
		t.Fatalf("DetectHeuristic returned %d rides, indexed returned %d", len(public), len(indexed))
	}

	got := indexed[0]
	if got.StartSampleIdx < 0 || got.StartSampleIdx >= len(rows) {
		t.Fatalf("StartSampleIdx = %d outside stream length %d", got.StartSampleIdx, len(rows))
	}
	if got.EndSampleIdx <= got.StartSampleIdx || got.EndSampleIdx >= len(rows) {
		t.Fatalf("EndSampleIdx = %d not after start %d within stream length %d", got.EndSampleIdx, got.StartSampleIdx, len(rows))
	}
	if got.Ride != public[0] {
		t.Fatalf("indexed ride %+v does not match public ride %+v", got.Ride, public[0])
	}
	if !got.Ride.StartedAt.Equal(rows[got.StartSampleIdx].Time) {
		t.Fatalf("StartSampleIdx points at %s, ride starts at %s", rows[got.StartSampleIdx].Time, got.Ride.StartedAt)
	}
	if !got.Ride.EndedAt.Equal(rows[got.EndSampleIdx].Time) {
		t.Fatalf("EndSampleIdx points at %s, ride ends at %s", rows[got.EndSampleIdx].Time, got.Ride.EndedAt)
	}
}

func TestDetectHeuristicWrapperPreservesNilForTooFewSamples(t *testing.T) {
	cfg := DefaultDetectorConfig("ski")
	rows := makeStream(time.Unix(0, 0).UTC(), cfg.MinSamples-1, 47.0, 12.0, 1200, 1.0, 5e-5, 5.0)

	if got := DetectHeuristicWithIndices(rows, cfg); got != nil {
		t.Fatalf("DetectHeuristicWithIndices returned %#v, want nil", got)
	}
	if got := DetectHeuristic(rows, cfg); got != nil {
		t.Fatalf("DetectHeuristic returned %#v, want nil", got)
	}
}
