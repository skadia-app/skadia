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
