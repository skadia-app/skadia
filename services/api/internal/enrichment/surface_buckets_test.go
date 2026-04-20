package enrichment

import "testing"

func TestSurfaceIDToGroup(t *testing.T) {
	cases := []struct {
		id   uint8
		want string
	}{
		{0, ""},                   // unknown
		{1, "Asphalt"},            // asphalt
		{2, "Beton"},              // concrete
		{3, "Pflaster"},           // paving_stones
		{4, "Pflaster"},           // sett
		{5, "Pflaster"},           // cobblestone
		{6, "Metall"},             // metal
		{7, "Holz"},               // wood
		{8, "Schotter (fein)"},    // compacted
		{9, "Schotter (grob)"},    // fine_gravel
		{10, "Schotter (grob)"},   // gravel
		{11, "Schotter (grob)"},   // pebblestone
		{12, "Naturweg"},          // dirt
		{13, "Naturweg"},          // earth
		{14, "Gras"},              // grass
		{15, "Gras"},              // grass_paver
		{16, "Naturweg"},          // ground
		{17, "Naturweg"},          // mud
		{18, "Sand"},              // sand
		{19, "Holz"},              // woodchips
		{20, "Schnee"},            // snow
		{21, "Schnee"},            // ice
		{22, "Fels"},              // rock
		{23, "Fels"},              // stone
		{24, "Pflaster"},          // unhewn_cobblestone
		{25, "Naturweg"},          // clay
		{26, "Sportbelag"},        // tartan
		{27, "Sportbelag"},        // artificial_turf
		{28, "Sportbelag"},        // decoturf
		{29, "Sportbelag"},        // carpet
		{30, "Sportbelag"},        // rubber
		{31, "Sportbelag"},        // acrylic
		{32, "Pflaster"},          // brick
		{33, "Asphalt"},           // chipseal
		{34, "Beton"},             // concrete_plates
		{35, "Beton"},             // concrete_lanes
		{36, "Metall"},            // metal_grid
		{37, "Naturweg"},          // stepping_stones
		{99, ""},                  // out-of-range
	}
	for _, c := range cases {
		got := surfaceIDToGroup(c.id)
		if got != c.want {
			t.Errorf("surfaceIDToGroup(%d) = %q, want %q", c.id, got, c.want)
		}
	}
}

// surfaceIDToGroupForSegment collapses buckets that the segment-editor
// dropdown does not offer. Diffs vs surfaceIDToGroup:
//   - concrete (2), concrete_plates (34), concrete_lanes (35) → "Asphalt"
//   - metal (6), wood (7), woodchips (19), metal_grid (36) → "" (skipped)
// All other IDs delegate to the canonical mapper unchanged.
func TestSurfaceIDToGroupForSegment(t *testing.T) {
	cases := []struct {
		id   uint8
		want string
	}{
		// Collapsed / excluded for segments:
		{2, "Asphalt"}, {34, "Asphalt"}, {35, "Asphalt"},
		{6, ""}, {7, ""}, {19, ""}, {36, ""},
		// Spot-check unchanged IDs delegate to surfaceIDToGroup:
		{1, "Asphalt"},
		{3, "Pflaster"},
		{8, "Schotter (fein)"},
		{9, "Schotter (grob)"},
		{12, "Naturweg"},
		{14, "Gras"},
		{18, "Sand"},
		{20, "Schnee"},
		{22, "Fels"},
		{26, "Sportbelag"},
		{33, "Asphalt"},
		{37, "Naturweg"},
		{0, ""}, {99, ""},
	}
	for _, c := range cases {
		got := surfaceIDToGroupForSegment(c.id)
		if got != c.want {
			t.Errorf("surfaceIDToGroupForSegment(%d) = %q, want %q", c.id, got, c.want)
		}
	}
}
