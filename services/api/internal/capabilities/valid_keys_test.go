package capabilities

import "testing"

// TestValidKey covers the source-of-truth bridge between the Capabilities
// struct (JSON tags) and map_layers.requires_capability admin validation.
func TestValidKey(t *testing.T) {
	known := []string{
		"routing", "brouter", "badrouter", "offline_map",
		"surface_enrichment", "dem_correction", "isochrone",
		"route_suggestion", "ml_service", "import_path_enabled",
	}
	for _, k := range known {
		if !ValidKey(k) {
			t.Errorf("expected %q to be a valid capability key", k)
		}
	}
	bogus := []string{"", "totally_made_up", "Routing", "OFFLINE_MAP"}
	for _, k := range bogus {
		if ValidKey(k) {
			t.Errorf("expected %q to be rejected", k)
		}
	}
	if got := len(ValidKeysSorted()); got < len(known) {
		t.Errorf("ValidKeysSorted returned %d keys, want >= %d", got, len(known))
	}
}
