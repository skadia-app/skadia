package routing

import (
	"math"
	"testing"

	"github.com/skadia-app/skadia/services/api/internal/routing/graph"
)

func TestDefaultTrekkingCost(t *testing.T) {
	g := &graph.Graph{
		EdgeDist:  []uint16{1000}, // 100m in dm
		EdgeTags1: []uint64{0},
	}

	cfg := DefaultTrekkingConfig()
	cost := cfg.EdgeCost(g, 0)
	if cost <= 0 {
		t.Errorf("trekking cost should be positive, got %f", cost)
	}
}

func TestDefaultTrekkingConfig_MaxLatentCost(t *testing.T) {
	cfg := DefaultTrekkingConfig()
	// MaxLatentCost = ElevMaxBuffer * max(UphillCostFactor, DownhillCostFactor)
	expected := cfg.ElevMaxBuffer * max(cfg.UphillCostFactor, cfg.DownhillCostFactor)
	if math.Abs(float64(cfg.MaxLatentCost-expected)) > 0.01 {
		t.Errorf("MaxLatentCost should be %f, got %f", expected, cfg.MaxLatentCost)
	}
}

func TestCostConfig_MotorwayBlocked(t *testing.T) {
	// Encode highway=motorway (enum 1)
	tags := graph.EncodeOSMTags(&graph.RawTags{Highway: "motorway"})
	g := &graph.Graph{
		EdgeDist:  []uint16{1000},
		EdgeTags1: []uint64{tags},
	}

	cfg := DefaultTrekkingConfig()
	cost := cfg.EdgeCost(g, 0)
	// Motorway should have very high cost (effectively blocked)
	if cost < 100000 {
		t.Errorf("motorway should have very high cost for trekking, got %f", cost)
	}
}
