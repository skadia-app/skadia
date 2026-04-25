package enrichment

import (
	"context"
	"reflect"
	"testing"

	"github.com/skadia-app/skadia/services/api/internal/model"
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

func TestExpandCoalescedStarts_MapsMatchedIndicesToTrackIndices(t *testing.T) {
	got := expandCoalescedStarts([]int{0, 2, 3}, []int{4, 5, 12, 20})
	want := []int{4, 12, 20}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("expandCoalescedStarts() = %v, want %v", got, want)
	}
}
