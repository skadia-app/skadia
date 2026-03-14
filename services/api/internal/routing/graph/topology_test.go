package graph

import (
	"math"
	"testing"
)

func TestCoordKey(t *testing.T) {
	k1 := NewCoordKey(47.1234567, 11.9876543)
	k2 := NewCoordKey(47.1234567, 11.9876543)
	if k1 != k2 {
		t.Errorf("same coords should produce same key: %v != %v", k1, k2)
	}
	k3 := NewCoordKey(47.1234568, 11.9876543)
	if k1 == k3 {
		t.Error("different coords should produce different keys")
	}
}

func TestBuildTopologyStraightWay(t *testing.T) {
	ways := []RawWay{
		{
			OSMID: 100,
			Coords: []Coord{
				{Lat: 47.0, Lon: 11.0},
				{Lat: 47.001, Lon: 11.0},
				{Lat: 47.002, Lon: 11.0},
				{Lat: 47.003, Lon: 11.0},
			},
			Tags: RawTags{Highway: "residential"},
		},
	}
	result := BuildTopology(ways)
	if result.NodeCount != 4 {
		t.Errorf("node count: got %d, want 4", result.NodeCount)
	}
	if len(result.Edges) != 6 {
		t.Errorf("edge count: got %d, want 6", len(result.Edges))
	}
}

func TestBuildTopologyCrossing(t *testing.T) {
	ways := []RawWay{
		{
			OSMID:  100,
			Coords: []Coord{{Lat: 47.0, Lon: 11.0}, {Lat: 47.001, Lon: 11.0}, {Lat: 47.002, Lon: 11.0}},
			Tags:   RawTags{Highway: "residential"},
		},
		{
			OSMID:  200,
			Coords: []Coord{{Lat: 47.001, Lon: 10.999}, {Lat: 47.001, Lon: 11.0}, {Lat: 47.001, Lon: 11.001}},
			Tags:   RawTags{Highway: "tertiary"},
		},
	}
	result := BuildTopology(ways)
	if result.NodeCount != 5 {
		t.Errorf("node count: got %d, want 5", result.NodeCount)
	}
	crossingKey := NewCoordKey(47.001, 11.0)
	nodeID, ok := result.NodeIndex[crossingKey]
	if !ok {
		t.Fatal("crossing node not found in index")
	}
	degree := 0
	for _, e := range result.Edges {
		if e.From == nodeID || e.To == nodeID {
			degree++
		}
	}
	if degree < 4 {
		t.Errorf("crossing node degree: got %d, want >= 4", degree)
	}
}

func TestBuildTopologyOneway(t *testing.T) {
	ways := []RawWay{
		{
			OSMID:  100,
			Coords: []Coord{{Lat: 47.0, Lon: 11.0}, {Lat: 47.001, Lon: 11.0}},
			Tags:   RawTags{Highway: "residential", Oneway: true},
		},
	}
	result := BuildTopology(ways)
	if len(result.Edges) != 1 {
		t.Fatalf("oneway should produce 1 edge, got %d", len(result.Edges))
	}
	fromKey := NewCoordKey(47.0, 11.0)
	toKey := NewCoordKey(47.001, 11.0)
	fromID := result.NodeIndex[fromKey]
	toID := result.NodeIndex[toKey]
	if result.Edges[0].From != fromID || result.Edges[0].To != toID {
		t.Errorf("oneway edge direction: got %d→%d, want %d→%d",
			result.Edges[0].From, result.Edges[0].To, fromID, toID)
	}
}

func TestBuildTopologyReverseOneway(t *testing.T) {
	ways := []RawWay{
		{
			OSMID:  100,
			Coords: []Coord{{Lat: 47.0, Lon: 11.0}, {Lat: 47.001, Lon: 11.0}},
			Tags:   RawTags{Highway: "residential", Oneway: true},
			ReverseOneway: true,
		},
	}
	result := BuildTopology(ways)
	if len(result.Edges) != 1 {
		t.Fatalf("reverse oneway should produce 1 edge, got %d", len(result.Edges))
	}
	fromKey := NewCoordKey(47.001, 11.0)
	toKey := NewCoordKey(47.0, 11.0)
	fromID := result.NodeIndex[fromKey]
	toID := result.NodeIndex[toKey]
	if result.Edges[0].From != fromID || result.Edges[0].To != toID {
		t.Errorf("reverse oneway edge direction: got %d→%d, want %d→%d",
			result.Edges[0].From, result.Edges[0].To, fromID, toID)
	}
}

func coordsEqual(a, b Coord) bool {
	return math.Abs(a.Lat-b.Lat) < 1e-9 && math.Abs(a.Lon-b.Lon) < 1e-9
}
