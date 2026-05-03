package enrichment

import (
	"testing"

	"github.com/skadia-app/skadia/services/api/internal/model"
)

func TestShouldSyncEBikeSystemTagOnlyForCyclingElectricActivities(t *testing.T) {
	tests := []struct {
		name string
		data *model.ActivityData
		want bool
	}{
		{name: "cycling electric", data: &model.ActivityData{SportType: "cycling", IsElectric: true}, want: true},
		{name: "cycling non electric", data: &model.ActivityData{SportType: "cycling", IsElectric: false}, want: false},
		{name: "running electric", data: &model.ActivityData{SportType: "running", IsElectric: true}, want: false},
		{name: "blank electric", data: &model.ActivityData{IsElectric: true}, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := shouldSyncEBikeSystemTag(tt.data); got != tt.want {
				t.Fatalf("shouldSyncEBikeSystemTag() = %v, want %v", got, tt.want)
			}
		})
	}
}
