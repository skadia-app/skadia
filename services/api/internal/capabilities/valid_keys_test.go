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

// TestValidKey_PinsKnownKeys hard-pins specific well-known capability keys.
// A refactor that accidentally drops the `json:"offline_map"` (etc.) tag from
// the Capabilities struct would silently shrink the admin-allow-list; this
// test fails loudly in that case so the regression surfaces in CI rather than
// at runtime when an admin tries to assign the capability to a map layer.
func TestValidKey_PinsKnownKeys(t *testing.T) {
	pinned := []string{"offline_map", "brouter", "dem_correction"}
	for _, k := range pinned {
		if !ValidKey(k) {
			t.Errorf("pinned capability key %q missing — JSON tag dropped from Capabilities struct?", k)
		}
	}
}
