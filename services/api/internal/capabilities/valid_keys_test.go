package capabilities

import "testing"

// TestValidKey covers the source-of-truth bridge between the Capabilities
// struct (JSON tags) and map_layers.requires_capability admin validation.
func TestValidKey(t *testing.T) {
	known := []string{
		"deployment_profile", "storage_mode", "storage_backend",
		"library_paths", "library_schemas", "library_sync",
		"import_path_enabled", "direct_upload_lifecycle",
		"route_photo_media", "gdpr_artifacts", "osm_import",
		"osm_enrichment", "brouter", "brouter_segment_download",
		"brouter_segment_build", "badrouter", "routing",
		"routing_tiles", "dem_correction", "coverage",
		"coverage_tiles", "system_layers_vector",
		"system_layers_coverage", "offline_map", "geo_poi_enrichment",
		"surface_enrichment", "activity_track_summary_geo",
		"activity_track_summary_surface", "segments",
		"segment_recompute", "segment_publish",
		"segment_osm_lift_import", "segment_enrichment_suggest",
		"osm_lift_snap", "isochrone", "route_suggestion",
		"local_poi_search", "ml_service", "document_ml",
		"photo_embeddings", "photo_embedding_storage_access",
		"ai_agent",
	}
	for _, k := range known {
		if !ValidKey(k) {
			t.Errorf("expected %q to be a valid capability key", k)
		}
	}
	bogus := []string{"", "totally_made_up", "Routing", "OFFLINE_MAP", "route-suggestion"}
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
	pinned := []string{"offline_map", "brouter", "dem_correction", "storage_mode", "photo_embedding_storage_access"}
	for _, k := range pinned {
		if !ValidKey(k) {
			t.Errorf("pinned capability key %q missing — JSON tag dropped from Capabilities struct?", k)
		}
	}
}
