package handler

import (
	"testing"

	"github.com/skadia-app/skadia/services/api/internal/model"
)

func TestBestEffortGroupingInputChanged(t *testing.T) {
	trail := "trail"
	treadmill := "treadmill"
	title := "New title"
	tests := []struct {
		name     string
		existing model.Activity
		update   model.ActivityUpdate
		want     bool
	}{
		{
			name:     "sport type changed",
			existing: model.Activity{SportType: "running"},
			update:   model.ActivityUpdate{SportType: stringPtrForBestEffortTest("cycling")},
			want:     true,
		},
		{
			name:     "sub type changed",
			existing: model.Activity{SportType: "running", SubType: &trail},
			update:   model.ActivityUpdate{SubType: &treadmill},
			want:     true,
		},
		{
			name:     "indoor changed",
			existing: model.Activity{SportType: "cycling", IsIndoor: false},
			update:   model.ActivityUpdate{IsIndoor: boolPtr(true)},
			want:     true,
		},
		{
			name:     "virtual changed",
			existing: model.Activity{SportType: "running", IsVirtual: false},
			update:   model.ActivityUpdate{IsVirtual: boolPtr(true)},
			want:     true,
		},
		{
			name:     "electric changed",
			existing: model.Activity{SportType: "cycling", IsElectric: false},
			update:   model.ActivityUpdate{IsElectric: boolPtr(true)},
			want:     true,
		},
		{
			name:     "exclude from stats changed",
			existing: model.Activity{SportType: "running", ExcludeFromStats: false},
			update:   model.ActivityUpdate{ExcludeFromStats: boolPtr(true)},
			want:     true,
		},
		{
			name:     "title only",
			existing: model.Activity{SportType: "running"},
			update:   model.ActivityUpdate{Title: &title},
			want:     false,
		},
		{
			name:     "same grouping values",
			existing: model.Activity{SportType: "running", SubType: &trail, IsIndoor: true, IsVirtual: false, IsElectric: false},
			update: model.ActivityUpdate{
				SportType:  stringPtrForBestEffortTest("running"),
				SubType:    &trail,
				IsIndoor:   boolPtr(true),
				IsVirtual:  boolPtr(false),
				IsElectric: boolPtr(false),
			},
			want: false,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := bestEffortGroupingInputChanged(tc.update, tc.existing); got != tc.want {
				t.Fatalf("bestEffortGroupingInputChanged() = %v, want %v", got, tc.want)
			}
		})
	}
}

func stringPtrForBestEffortTest(v string) *string {
	return &v
}
