package settings

import "testing"

func TestAllKeysIncludesLibraryKeys(t *testing.T) {
	expected := []string{
		"library.user_can_edit_dir_name",
		"library.user_can_edit_schemas",
		"library.user_map_layer_limit",
		"library.user_map_layer_rate_limit_per_10min",
		"library.default_schema.activity",
		"library.default_schema.media",
		"library.default_schema.document",
		"library.default_schema.health",
		"library.default_schema.route",
	}
	for _, k := range expected {
		if _, ok := KeysByKey[k]; !ok {
			t.Errorf("expected key %q in KeysByKey, not found", k)
		}
	}
	// Verify Group grouping: 3 pre-existing (keep_versions, sync_enabled, sync_auto_import)
	// + 7 admin-model keys + 2 user-map-layer keys = 12 total.
	lib := KeysByGroup["library"]
	if len(lib) != 12 {
		t.Errorf("expected 12 library-group keys, got %d", len(lib))
	}
}

func TestLibraryBoolValidation(t *testing.T) {
	kd := KeysByKey["library.user_can_edit_dir_name"]
	if err := kd.Validate("true"); err != nil {
		t.Errorf("validate 'true' failed: %v", err)
	}
	if err := kd.Validate("false"); err != nil {
		t.Errorf("validate 'false' failed: %v", err)
	}
	if err := kd.Validate("yes"); err == nil {
		t.Error("expected validate 'yes' to fail")
	}
}
