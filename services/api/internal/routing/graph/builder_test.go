package graph

import (
	"testing"
	"time"
)

func TestBuildFromSyntheticWays(t *testing.T) {
	ways := []RawWay{
		{
			OSMID:  100,
			Coords: []Coord{{Lat: 47.0, Lon: 11.0}, {Lat: 47.001, Lon: 11.0}, {Lat: 47.002, Lon: 11.0}},
			Tags:   RawTags{Highway: "residential", Surface: "asphalt", Maxspeed: "50"},
			Enrichment: RawEnrichment{ForestPct: 20, NoiseScore: 60},
		},
		{
			OSMID:  200,
			Coords: []Coord{{Lat: 47.001, Lon: 10.999}, {Lat: 47.001, Lon: 11.0}, {Lat: 47.001, Lon: 11.001}},
			Tags:   RawTags{Highway: "tertiary", Surface: "gravel", Oneway: true},
			Enrichment: RawEnrichment{TrafficClass: 1},
		},
	}

	elevations := []int32{5000, 5100, 5200, 4950, 5100}

	g := BuildFromWays(ways, elevations)

	if g == nil {
		t.Fatal("BuildFromWays returned nil")
	}

	if g.NodeCount != 5 {
		t.Errorf("NodeCount: got %d, want 5", g.NodeCount)
	}

	if g.EdgeCount != 6 {
		t.Errorf("EdgeCount: got %d, want 6", g.EdgeCount)
	}

	for i := uint32(0); i < g.NodeCount; i++ {
		if g.NodeEle[i] == 0 {
			t.Errorf("NodeEle[%d] = 0, expected non-zero", i)
		}
	}

	if g.Grid == nil {
		t.Error("Grid should not be nil")
	}

	nodeID, _ := NearestNode(g, 47.001, 11.0)
	nodeLat := float64(g.NodeLat[nodeID]) / 1e7
	if nodeLat < 47.0005 || nodeLat > 47.0015 {
		t.Errorf("nearest node lat: got %.4f, want ~47.001", nodeLat)
	}

	for i := uint32(0); i < g.EdgeCount; i++ {
		hw := DecodeHighway(g.EdgeTags1[i])
		if hw == 0 {
			t.Errorf("EdgeTags1[%d] highway = 0 (unknown), expected known value", i)
		}
	}

	if g.BuildTime.IsZero() {
		t.Error("BuildTime should be set")
	}
}

func TestBuildFromWaysPreservesBarriers(t *testing.T) {
	ways := []RawWay{
		{
			OSMID:  100,
			Coords: []Coord{{Lat: 47.0, Lon: 11.0}, {Lat: 47.001, Lon: 11.0}},
			Tags:   RawTags{Highway: "path"},
			BarrierNodes: map[int]uint8{1: BarrierGate},
		},
	}

	g := BuildFromWays(ways, []int32{5000, 5100})

	found := false
	for i := uint32(0); i < g.NodeCount; i++ {
		if g.NodeTags[i] == BarrierGate {
			found = true
			break
		}
	}
	if !found {
		t.Error("barrier node not found in built graph")
	}
}

func init() {
	_ = time.Now
}
