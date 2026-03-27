package graph

import "testing"

func TestTileID(t *testing.T) {
	tests := []struct {
		name     string
		lat, lon float64
		wantID   uint32
	}{
		{"Zürich", 47.37, 8.54, uint32((47+90)*360 + (8 + 180))},
		{"Equator/Greenwich", 0.5, 0.5, uint32(90*360 + 180)},
		{"South Pole edge", -89.5, -179.5, uint32(0*360 + 0)},
		{"Boundary lat=47.0 → floor=47", 47.0, 8.5, uint32((47+90)*360 + (8 + 180))},
		{"Negative coords", -33.9, -70.6, uint32((-34+90)*360 + (-71 + 180))},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := TileIDFromCoord(tt.lat, tt.lon)
			if got != tt.wantID {
				t.Errorf("TileIDFromCoord(%v, %v) = %d, want %d", tt.lat, tt.lon, got, tt.wantID)
			}
		})
	}
}

func TestTileBBox(t *testing.T) {
	id := TileIDFromCoord(47.37, 8.54)
	bbox := TileBBox(id)
	if bbox[0] != 47.0 || bbox[1] != 8.0 || bbox[2] != 48.0 || bbox[3] != 9.0 {
		t.Errorf("TileBBox(%d) = %v, want [47 8 48 9]", id, bbox)
	}
}

func TestTileBBoxExpanded(t *testing.T) {
	id := TileIDFromCoord(47.37, 8.54)
	bbox := TileBBoxExpanded(id, 0.02)
	if bbox[0] != 46.98 || bbox[1] != 7.98 || bbox[2] != 48.02 || bbox[3] != 9.02 {
		t.Errorf("TileBBoxExpanded(%d, 0.02) = %v, want [46.98 7.98 48.02 9.02]", id, bbox)
	}
}

func TestNeighborTileIDs(t *testing.T) {
	id := TileIDFromCoord(47.37, 8.54)
	neighbors, count := NeighborTileIDs(id)
	if count != 8 {
		t.Fatalf("NeighborTileIDs: got %d, want 8", count)
	}
	north := TileIDFromCoord(48.5, 8.5)
	found := false
	for i := range count {
		if neighbors[i] == north {
			found = true
		}
	}
	if !found {
		t.Errorf("north neighbor %d not in %v", north, neighbors[:count])
	}
}
