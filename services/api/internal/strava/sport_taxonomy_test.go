package strava

import (
	"testing"

	"github.com/skadia-app/skadia/services/api/internal/model"
)

func TestNormalizeStravaSportFields(t *testing.T) {
	tests := []struct {
		name         string
		input        string
		wantSport    string
		wantSubType  string
		wantElectric bool
		wantIndoor   bool
		wantVirtual  bool
	}{
		{
			name:        "mountain bike ride",
			input:       "Mountain Bike Ride",
			wantSport:   "cycling",
			wantSubType: "mountain",
		},
		{
			name:         "e mountain bike ride",
			input:        "E-Mountain Bike Ride",
			wantSport:    "cycling",
			wantSubType:  "mountain",
			wantElectric: true,
		},
		{
			name:        "treadmill",
			input:       "Treadmill",
			wantSport:   "running",
			wantSubType: "indoor_running",
			wantIndoor:  true,
		},
		{
			name:        "gravel",
			input:       "Gravel Ride",
			wantSport:   "cycling",
			wantSubType: "gravel_cycling",
		},
		{
			name:        "trail",
			input:       "Trail Run",
			wantSport:   "running",
			wantSubType: "trail",
		},
		{
			name:        "open water swim",
			input:       "Open Water Swim",
			wantSport:   "swimming",
			wantSubType: "open_water",
		},
		{
			name:        "virtual ride",
			input:       "Virtual Ride",
			wantSport:   "cycling",
			wantVirtual: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := normalizeStravaSportFields(tt.input)
			if err != nil {
				t.Fatalf("normalizeStravaSportFields(%q): %v", tt.input, err)
			}
			if got.SportType != tt.wantSport {
				t.Errorf("SportType: got %q, want %q", got.SportType, tt.wantSport)
			}
			if got.SubType != tt.wantSubType {
				t.Errorf("SubType: got %q, want %q", got.SubType, tt.wantSubType)
			}
			if got.IsElectric != tt.wantElectric {
				t.Errorf("IsElectric: got %v, want %v", got.IsElectric, tt.wantElectric)
			}
			if got.IsIndoor != tt.wantIndoor {
				t.Errorf("IsIndoor: got %v, want %v", got.IsIndoor, tt.wantIndoor)
			}
			if got.IsVirtual != tt.wantVirtual {
				t.Errorf("IsVirtual: got %v, want %v", got.IsVirtual, tt.wantVirtual)
			}
		})
	}
}

func TestMergeActivityDataPropagatesParsedModifiers(t *testing.T) {
	base := &model.ActivityData{
		SportType: "cycling",
		SubType:   "",
	}
	parsed := &model.ActivityData{
		SportType:  "cycling",
		SubType:    "indoor_cycling",
		IsElectric: true,
		IsIndoor:   true,
		IsVirtual:  true,
	}

	mergeActivityData(base, parsed)

	if base.SubType != "indoor_cycling" {
		t.Fatalf("SubType = %q, want indoor_cycling", base.SubType)
	}
	if !base.IsElectric {
		t.Error("IsElectric was not propagated")
	}
	if !base.IsIndoor {
		t.Error("IsIndoor was not propagated")
	}
	if !base.IsVirtual {
		t.Error("IsVirtual was not propagated")
	}
}
