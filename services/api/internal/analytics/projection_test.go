package analytics

import "testing"

func TestProjectionTrendForSlopeKeepsLegacyTrendAndStableKey(t *testing.T) {
	tests := []struct {
		name       string
		slope      float64
		wantLegacy string
		wantKey    string
	}{
		{name: "building", slope: 0.051, wantLegacy: "aufbau", wantKey: "building"},
		{name: "detraining", slope: -0.051, wantLegacy: "abbau", wantKey: "detraining"},
		{name: "plateau", slope: 0.049, wantLegacy: "plateau", wantKey: "plateau"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := legacyProjectionTrendForSlope(tt.slope); got != tt.wantLegacy {
				t.Fatalf("legacyProjectionTrendForSlope(%v) = %q, want %q", tt.slope, got, tt.wantLegacy)
			}
			if got := projectionTrendKeyForSlope(tt.slope); got != tt.wantKey {
				t.Fatalf("projectionTrendKeyForSlope(%v) = %q, want %q", tt.slope, got, tt.wantKey)
			}
		})
	}
}
