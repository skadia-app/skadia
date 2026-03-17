package graph

import "testing"

func TestIdentifyBorderNodes(t *testing.T) {
	tileID := TileIDFromCoord(47.5, 8.5)
	nodeLat := []int32{
		int32(47.003 * 1e7),
		int32(47.5 * 1e7),
		int32(47.5 * 1e7),
	}
	nodeLon := []int32{
		int32(8.5 * 1e7),
		int32(8.5 * 1e7),
		int32(8.996 * 1e7),
	}
	borderIDs := IdentifyBorderNodes(tileID, nodeLat, nodeLon, 0.0045)
	if len(borderIDs) != 2 {
		t.Fatalf("expected 2 border nodes, got %d: %v", len(borderIDs), borderIDs)
	}
	if borderIDs[0] != 0 || borderIDs[1] != 2 {
		t.Errorf("border nodes = %v, want [0, 2]", borderIDs)
	}
}

func TestBorderNodeIndex_RoundTrip(t *testing.T) {
	idx := &BorderNodeIndex{
		Entries: []BorderNodeEntry{
			{CoordKey: CoordKeyToInt64(CoordKey{Lat: 470030000, Lon: 85000000}), LocalNodeID: 0},
			{CoordKey: CoordKeyToInt64(CoordKey{Lat: 475000000, Lon: 89960000}), LocalNodeID: 2},
		},
	}
	data := idx.Serialize()
	idx2, err := DeserializeBorderNodeIndex(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(idx2.Entries) != 2 {
		t.Fatalf("deserialized %d entries, want 2", len(idx2.Entries))
	}
	if idx2.Entries[0].CoordKey != idx.Entries[0].CoordKey {
		t.Errorf("entry 0 coord key mismatch")
	}
}

func TestBorderNodeIndex_Lookup(t *testing.T) {
	idx := &BorderNodeIndex{
		Entries: []BorderNodeEntry{
			{CoordKey: 100, LocalNodeID: 5},
			{CoordKey: 200, LocalNodeID: 10},
			{CoordKey: 300, LocalNodeID: 15},
		},
	}
	if id, ok := idx.Lookup(200); !ok || id != 10 {
		t.Errorf("Lookup(200) = %d, %v; want 10, true", id, ok)
	}
	if _, ok := idx.Lookup(150); ok {
		t.Error("Lookup(150) should return false")
	}
}
