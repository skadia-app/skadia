package enrichment

import (
	"context"
	"math"
	"reflect"
	"testing"

	"github.com/skadia-app/skadia/services/api/internal/enrichment/edgetags"
	"github.com/skadia-app/skadia/services/api/internal/model"
	"github.com/skadia-app/skadia/services/api/internal/routing/graph"
)

func TestEnrichTrackAnalysis_DisabledNoOp(t *testing.T) {
	settings := &model.EnrichmentSettings{
		EnableSurfaceDetection: false,
		EnableDifficultyScales: false,
	}

	if err := EnrichTrackAnalysis(context.Background(), nil, "activity-id", settings, nil, nil); err != nil {
		t.Fatalf("EnrichTrackAnalysis disabled no-op returned error: %v", err)
	}
}

func TestEnrichTrackAnalysis_NilSettingsNoOp(t *testing.T) {
	if err := EnrichTrackAnalysis(context.Background(), nil, "activity-id", nil, nil, nil); err != nil {
		t.Fatalf("EnrichTrackAnalysis nil settings no-op returned error: %v", err)
	}
}

// With active settings the nil db must surface as an error rather than silently
// no-oping — otherwise misconfigured callers would skip enrichment unnoticed.
func TestEnrichTrackAnalysis_NilDBWithActiveSettingsReturnsError(t *testing.T) {
	settings := &model.EnrichmentSettings{
		EnableSurfaceDetection: true,
		EnableDifficultyScales: false,
	}

	err := EnrichTrackAnalysis(context.Background(), nil, "activity-id", settings, nil, nil)
	if err == nil {
		t.Fatalf("EnrichTrackAnalysis with nil db + active settings: expected error, got nil")
	}
}

func TestExpandCoalescedStarts_MapsMatchedIndicesToTrackIndices(t *testing.T) {
	got := expandCoalescedStarts([]int{0, 2, 3}, []int{4, 5, 12, 20})
	want := []int{4, 12, 20}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("expandCoalescedStarts() = %v, want %v", got, want)
	}
}

func TestBuildMatchedEdgeRanges_SplitsAtUnmatchedGaps(t *testing.T) {
	t1 := graph.EncodeOSMTags(&graph.RawTags{SacScale: "hiking"})

	starts, tags1, tags2 := buildMatchedEdgeRanges(
		[]uint32{7, 7},
		[]uint64{t1, t1},
		[]uint64{0, 0},
		[]int{0, 2},
		3,
	)

	wantStarts := []int{0, 1, 2, 3}
	if !reflect.DeepEqual(starts, wantStarts) {
		t.Fatalf("starts = %v, want %v", starts, wantStarts)
	}
	wantTags1 := []uint64{t1, 0, t1, 0}
	if !reflect.DeepEqual(tags1, wantTags1) {
		t.Fatalf("tags1 = %v, want %v", tags1, wantTags1)
	}
	if !reflect.DeepEqual(tags2, []uint64{0, 0, 0, 0}) {
		t.Fatalf("tags2 = %v, want all zero sentinels around matched tags", tags2)
	}

	ranges := edgetags.ComputeAllRanges(starts, tags1, tags2, 3)
	wantRanges := []edgetags.CoordRange{{From: 0, To: 1}, {From: 2, To: 3}}
	if !reflect.DeepEqual(ranges.Sac["T1"], wantRanges) {
		t.Fatalf("Sac[T1] ranges = %v, want %v", ranges.Sac["T1"], wantRanges)
	}
}

func TestComputeScalePercentagesByDistance_UsesRangeDistance(t *testing.T) {
	coords := make([]edgetags.LatLng, 11)
	for i := range coords {
		coords[i] = edgetags.LatLng{Lat: 47.0, Lng: 11.0 + float64(i)*0.0001}
	}
	t1 := graph.EncodeOSMTags(&graph.RawTags{SacScale: "hiking"})
	t2 := graph.EncodeOSMTags(&graph.RawTags{SacScale: "mountain_hiking"})

	got := computeScalePercentagesByDistance(
		coords,
		[]int{0, 1},
		[]uint64{t1, t2},
		[]uint64{0, 0},
		10,
	)

	if math.Abs(got.Sac["T1"]-10.0) > 0.001 {
		t.Fatalf("Sac[T1] = %.6f, want 10.0", got.Sac["T1"])
	}
	if math.Abs(got.Sac["T2"]-90.0) > 0.001 {
		t.Fatalf("Sac[T2] = %.6f, want 90.0", got.Sac["T2"])
	}
}
