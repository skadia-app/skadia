package graph

import "testing"

func TestFindNearEdges_ReturnsRawTags(t *testing.T) {
	tags1 := EncodeOSMTags(&RawTags{
		Highway: "track",
		Surface: "gravel",
	})
	tags2 := uint64(0x123456789)

	g := &Graph{
		NodeCount: 2,
		EdgeCount: 1,
		Offsets:   []uint32{0, 1, 1},
		Targets:   []uint32{1},
		EdgeTags1: []uint64{tags1},
		EdgeTags2: []uint64{tags2},
		NodeLat:   []int32{470000000, 470000000},
		NodeLon:   []int32{110000000, 110010000},
		// No intermediate shape points.
		EdgeGeomOff: []uint32{0, 0},
	}
	g.Grid = BuildSpatialIndex(g, 0.01)

	got := FindNearEdges(g, 47.0, 11.0005)
	if len(got) != 1 {
		t.Fatalf("FindNearEdges returned %d candidates, want 1", len(got))
	}

	if got[0].Tags1 != tags1 {
		t.Fatalf("Tags1 = %d, want %d", got[0].Tags1, tags1)
	}
	if got[0].Tags2 != tags2 {
		t.Fatalf("Tags2 = %d, want %d", got[0].Tags2, tags2)
	}
	if got[0].Highway != "track" {
		t.Fatalf("Highway = %q, want %q", got[0].Highway, "track")
	}
	if got[0].Surface != "gravel" {
		t.Fatalf("Surface = %q, want %q", got[0].Surface, "gravel")
	}
}
