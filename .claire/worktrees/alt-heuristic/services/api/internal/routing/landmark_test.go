package routing

import (
	"testing"

	"github.com/skadia-app/skadia/services/api/internal/routing/graph"
)

func TestSelectLandmarkSet(t *testing.T) {
	meta := &graph.LandmarkMetaData{
		UphillThreshold:   7.0,
		DownhillThreshold: 7.0,
	}

	tests := []struct {
		uphill, downhill float32
		want             LandmarkSetID
	}{
		{7, 7, LandmarkSetA},
		{10, 10, LandmarkSetA},
		{7, 0, LandmarkSetB},
		{40, 5, LandmarkSetB},
		{0, 60, LandmarkSetC},
		{0, 7, LandmarkSetC},
		{0, 0, LandmarkSetD},
		{3, 3, LandmarkSetD},
	}

	for _, tt := range tests {
		cfg := &CostConfig{UphillCost: tt.uphill, DownhillCost: tt.downhill}
		got := SelectLandmarkSet(cfg, meta)
		if got != tt.want {
			t.Errorf("uphill=%.0f downhill=%.0f: got %d, want %d", tt.uphill, tt.downhill, got, tt.want)
		}
	}
}

func TestSelectLandmarkSetNilMeta(t *testing.T) {
	cfg := &CostConfig{UphillCost: 7, DownhillCost: 7}
	got := SelectLandmarkSet(cfg, nil)
	if got != LandmarkSetD {
		t.Errorf("nil meta should return SetD, got %d", got)
	}
}

func TestEquirectDistKm(t *testing.T) {
	lat1 := int32(47.37 * 1e7)
	lon1 := int32(8.54 * 1e7)
	lat2 := int32(46.95 * 1e7)
	lon2 := int32(7.45 * 1e7)
	d := equirectDistKm(lat1, lon1, lat2, lon2)
	if d < 80 || d > 110 {
		t.Errorf("Zürich-Bern distance: got %.1f km, expected ~95 km", d)
	}
}
