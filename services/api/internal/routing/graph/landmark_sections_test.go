package graph

import "testing"

func TestSelectLandmarksBasic(t *testing.T) {
	tiles := []TileScanResult{
		{TileID: 1, NodeCount: 4,
			NodeLats: []int32{470000000, 471000000, 472000000, 473000000},
			NodeLons: []int32{80000000, 81000000, 82000000, 83000000},
			NodeEles: []int32{5000, 10000, 20000, 30000}},
		{TileID: 2, NodeCount: 4,
			NodeLats: []int32{474000000, 475000000, 476000000, 477000000},
			NodeLons: []int32{84000000, 85000000, 86000000, 87000000},
			NodeEles: []int32{1000, 2000, 25000, 35000}},
		{TileID: 3, NodeCount: 4,
			NodeLats: []int32{478000000, 479000000, 480000000, 481000000},
			NodeLons: []int32{88000000, 89000000, 90000000, 91000000},
			NodeEles: []int32{500, 800, 15000, 40000}},
	}

	meta, err := SelectLandmarks(tiles, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if meta.LandmarkCount < 4 {
		t.Errorf("expected at least 4 landmarks, got %d", meta.LandmarkCount)
	}
	found := false
	for i := range meta.LandmarkCount {
		if meta.Lats[i] == 481000000 {
			found = true
		}
	}
	if !found {
		t.Error("maxLat BBox corner node should be a landmark")
	}
}

func TestSelectLandmarksWithOSMPass(t *testing.T) {
	tiles := []TileScanResult{
		{TileID: 1, NodeCount: 6,
			NodeLats: []int32{470000000, 471000000, 472000000, 473000000, 474000000, 475000000},
			NodeLons: []int32{110000000, 111000000, 112000000, 113000000, 114000000, 115000000},
			NodeEles: []int32{5000, 10000, 20000, 30000, 13700, 8000}},
	}
	// Node at local index 4 (TileID=1, local=4) has GlobalNodeID = MakeGlobalNodeID(1, 4)
	passNodeID := MakeGlobalNodeID(1, 4)
	passCandidates := []LandmarkCandidate{
		{GlobalNodeID: passNodeID, Lat: 474000000, Lon: 114000000, Ele: 13700},
	}
	meta, err := SelectLandmarks(tiles, nil, passCandidates, nil)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	slotIdx := -1
	for i := range meta.LandmarkCount {
		if meta.NodeIDs[i] == passNodeID {
			found = true
			slotIdx = i
		}
	}
	if !found {
		t.Error("OSM pass candidate not selected as landmark")
	}
	if slotIdx < 4 || slotIdx > 7 {
		t.Errorf("OSM pass in slot %d, expected 4-7", slotIdx)
	}
}

func TestFindNearestCosLat(t *testing.T) {
	tiles := []TileScanResult{
		{TileID: 1, NodeCount: 2,
			NodeLats: []int32{470001000, 470002000},
			NodeLons: []int32{80002000, 80001000},
			NodeEles: []int32{1000, 2000}},
	}
	best := findNearestCosLat(tiles, 470000000, 80000000, 0.68, nil)
	if best.Lat != 470001000 {
		t.Errorf("expected node 0 (lat=470001000), got lat=%d", best.Lat)
	}
}

func TestDenseIndex(t *testing.T) {
	tiles := []TileScanResult{
		{TileID: 10, NodeCount: 100},
		{TileID: 20, NodeCount: 200},
	}
	idx := BuildDenseIndex(tiles)
	if idx.TotalNodes != 300 {
		t.Errorf("expected 300 total nodes, got %d", idx.TotalNodes)
	}
	if idx.TileOffset[10] != 0 || idx.TileOffset[20] != 100 {
		t.Errorf("wrong offsets: %v", idx.TileOffset)
	}
	// ToDense: tile 20, local 50 → 100 + 50 = 150
	gid := MakeGlobalNodeID(20, 50)
	dense := idx.ToDense(gid)
	if dense != 150 {
		t.Errorf("expected dense=150, got %d", dense)
	}
}
