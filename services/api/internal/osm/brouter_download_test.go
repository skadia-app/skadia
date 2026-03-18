package osm

import "testing"

func TestFormatBRouterTileID(t *testing.T) {
	tests := []struct {
		lon, lat int
		want     string
	}{
		{5, 45, "E5_N45"},
		{0, 0, "E0_N0"},
		{-20, 25, "W20_N25"},
		{10, -5, "E10_S5"},
		{-15, -10, "W15_S10"},
	}
	for _, tt := range tests {
		got := formatBRouterTileID(tt.lon, tt.lat)
		if got != tt.want {
			t.Errorf("formatBRouterTileID(%d, %d) = %q, want %q", tt.lon, tt.lat, got, tt.want)
		}
	}
}

func TestBRouterTileIDs(t *testing.T) {
	tiles := BRouterTileIDs([]Region{{MinLon: 5.96, MinLat: 45.82, MaxLon: 10.49, MaxLat: 47.81}})
	if len(tiles) != 2 {
		t.Fatalf("expected 2 tiles for Switzerland, got %d: %v", len(tiles), tiles)
	}
	has := func(id string) bool {
		for _, tile := range tiles {
			if tile == id {
				return true
			}
		}
		return false
	}
	if !has("E5_N45") {
		t.Error("missing E5_N45")
	}
	if !has("E10_N45") {
		t.Error("missing E10_N45")
	}
}

func TestBRouterTileIDs_Deduplication(t *testing.T) {
	tiles := BRouterTileIDs([]Region{
		{MinLon: 5.96, MinLat: 45.82, MaxLon: 10.49, MaxLat: 47.81},
		{MinLon: 9.53, MinLat: 46.61, MaxLon: 17.16, MaxLat: 49.02},
	})
	seen := map[string]int{}
	for _, tile := range tiles {
		seen[tile]++
	}
	for id, count := range seen {
		if count > 1 {
			t.Errorf("duplicate tile ID: %s (count=%d)", id, count)
		}
	}
}
