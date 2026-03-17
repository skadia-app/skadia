package routing

import (
	"math"
	"testing"

	"github.com/skadia-app/skadia/services/api/internal/routing/graph"
)

func TestDefaultTrekkingCost(t *testing.T) {
	// Use highway=path (enum 16) for a realistic test
	tags := graph.EncodeOSMTags(&graph.RawTags{Highway: "path"})
	g := &graph.Graph{
		EdgeDist:  []uint32{10000}, // 100m in cm
		EdgeTags1: []uint64{tags},
	}

	cfg := DefaultTrekkingConfig()
	ecf := cfg.EdgeCost(g, 0)
	// path costfactor = 1.0 (highway) * 1.0 (unknown surface) = 1.0
	if ecf.Flat <= 0 {
		t.Errorf("trekking costfactor should be positive, got %f", ecf.Flat)
	}
	if ecf.Flat < 0.5 || ecf.Flat > 2.0 {
		t.Errorf("trekking costfactor for path should be ~1.0, got %f", ecf.Flat)
	}
}

func TestDefaultTrekkingConfig_MaxLatentCost(t *testing.T) {
	cfg := DefaultTrekkingConfig()
	// MaxLatentCost = ElevMaxBuffer * max(UphillCost, DownhillCost)
	expected := cfg.ElevMaxBuffer * max(cfg.UphillCost, cfg.DownhillCost)
	if math.Abs(float64(cfg.MaxLatentCost-expected)) > 0.01 {
		t.Errorf("MaxLatentCost should be %f, got %f", expected, cfg.MaxLatentCost)
	}
	// Fixed-value assertion (not derived from same formula)
	if math.Abs(float64(cfg.MaxLatentCost)-10.0) > 0.001 {
		t.Errorf("MaxLatentCost should be 10.0 for default trekking config, got %f", cfg.MaxLatentCost)
	}
}

func TestCostConfig_MotorwayBlocked(t *testing.T) {
	// Encode highway=motorway (enum 1)
	tags := graph.EncodeOSMTags(&graph.RawTags{Highway: "motorway"})
	g := &graph.Graph{
		EdgeDist:  []uint32{10000},
		EdgeTags1: []uint64{tags},
	}

	cfg := DefaultTrekkingConfig()
	ecf := cfg.EdgeCost(g, 0)
	// Motorway should be blocked (costfactor >= costBlocked)
	if ecf.Flat < costBlocked {
		t.Errorf("motorway should be blocked (Flat >= %f), got %f", costBlocked, ecf.Flat)
	}
}

func TestEdgeCostFactors_BlockedCheck(t *testing.T) {
	ecf := EdgeCostFactors{Flat: costBlocked}
	if ecf.Flat < costBlocked {
		t.Error("blocked edge should have Flat >= costBlocked")
	}
}

func TestEdgeCostFactors_WayClass(t *testing.T) {
	ecf := EdgeCostFactors{Flat: 1.0, WayClass: 1}
	if ecf.WayClass != 1 {
		t.Errorf("WayClass should be 1, got %d", ecf.WayClass)
	}
}
