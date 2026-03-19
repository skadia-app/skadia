package geo

import (
	"math"
	"testing"

	"github.com/paulmach/orb"
)

func TestDistanceHaversine(t *testing.T) {
	tests := []struct {
		name     string
		lat1     float64
		lng1     float64
		lat2     float64
		lng2     float64
		wantKm   float64
		tolerKm  float64
	}{
		{
			name:    "Zürich to Bern",
			lat1:    47.3769, lng1: 8.5417,
			lat2:    46.9480, lng2: 7.4474,
			wantKm:  95.4,
			tolerKm: 1.0,
		},
		{
			name:    "null distance same point",
			lat1:    47.3769, lng1: 8.5417,
			lat2:    47.3769, lng2: 8.5417,
			wantKm:  0.0,
			tolerKm: 0.0001,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DistanceHaversine(tt.lat1, tt.lng1, tt.lat2, tt.lng2)
			gotKm := got / 1000.0
			if math.Abs(gotKm-tt.wantKm) > tt.tolerKm {
				t.Errorf("DistanceHaversine() = %.3f km, want %.3f km (±%.4f km)", gotKm, tt.wantKm, tt.tolerKm)
			}
		})
	}
}

func TestLength(t *testing.T) {
	t.Run("triangle ~2km", func(t *testing.T) {
		// Small triangle near Zürich, roughly 2km perimeter
		ls := orb.LineString{
			{8.5417, 47.3769}, // Zürich (lng, lat)
			{8.5517, 47.3769}, // ~800m east
			{8.5417, 47.3869}, // ~1100m north
			{8.5417, 47.3769}, // back to start
		}
		got := Length(ls)
		gotKm := got / 1000.0
		// Triangle with ~800m east side, ~1100m north side, ~1360m hypotenuse ≈ 3.26km
		if gotKm < 1.5 || gotKm > 5.0 {
			t.Errorf("Length() = %.3f km, expected roughly 2-4 km triangle", gotKm)
		}
	})

	t.Run("empty LineString returns 0", func(t *testing.T) {
		ls := orb.LineString{}
		got := Length(ls)
		if got != 0.0 {
			t.Errorf("Length(empty) = %f, want 0", got)
		}
	})

	t.Run("single point returns 0", func(t *testing.T) {
		ls := orb.LineString{{8.5417, 47.3769}}
		got := Length(ls)
		if got != 0.0 {
			t.Errorf("Length(single point) = %f, want 0", got)
		}
	})
}
