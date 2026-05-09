package applicablemodifiers

import (
	"os"
	"path/filepath"
	"testing"
)

func registryPath(t *testing.T) string {
	t.Helper()
	return filepath.Join("..", "..", "..", "..", "config", "applicable-modifiers", "subsystems.yaml")
}

func TestLoadSubsystemRegistryCountAndNames(t *testing.T) {
	subs, err := LoadSubsystemRegistry(registryPath(t))
	if err != nil {
		t.Fatalf("LoadSubsystemRegistry: %v", err)
	}
	if len(subs) != 8 {
		t.Fatalf("got %d subsystems, want 8", len(subs))
	}

	// Verify order matches config/applicable-modifiers/subsystems.yaml — the
	// registry YAML is the single source of truth for subsystem ordering;
	// scripts/generate-applicable-modifiers.sh consumes it via --registry.
	wantNames := []string{"gear", "rules", "dataimport", "shadowfs", "routefilter", "bestefforts", "heatmap", "quick-edit"}
	for i, want := range wantNames {
		if subs[i].Name != want {
			t.Errorf("subsystems[%d].Name = %q, want %q", i, subs[i].Name, want)
		}
	}
}

func TestLoadSubsystemRegistrySampleFields(t *testing.T) {
	subs, err := LoadSubsystemRegistry(registryPath(t))
	if err != nil {
		t.Fatalf("LoadSubsystemRegistry: %v", err)
	}

	// gear: has both Go and TS output
	gear := subs[0]
	if gear.GoPackage != "gear" {
		t.Errorf("gear.GoPackage = %q, want gear", gear.GoPackage)
	}
	if gear.GoOut == "" {
		t.Error("gear.GoOut must be non-empty")
	}
	if gear.TSExportPrefix != "Gear" {
		t.Errorf("gear.TSExportPrefix = %q, want Gear", gear.TSExportPrefix)
	}
	if gear.TSOut == "" {
		t.Error("gear.TSOut must be non-empty")
	}

	// dataimport: has Go output but no TS
	dataimport := subs[2]
	if dataimport.GoOut == "" {
		t.Error("dataimport.GoOut must be non-empty")
	}
	if dataimport.TSOut != "" {
		t.Errorf("dataimport.TSOut = %q, want empty", dataimport.TSOut)
	}

	// quick-edit: has TS output but no Go
	quickEdit := subs[7]
	if quickEdit.GoOut != "" {
		t.Errorf("quick-edit.GoOut = %q, want empty", quickEdit.GoOut)
	}
	if quickEdit.GoPackage != "" {
		t.Errorf("quick-edit.GoPackage = %q, want empty", quickEdit.GoPackage)
	}
	if quickEdit.TSOut == "" {
		t.Error("quick-edit.TSOut must be non-empty")
	}
	if quickEdit.TSExportPrefix != "QuickEdit" {
		t.Errorf("quick-edit.TSExportPrefix = %q, want QuickEdit", quickEdit.TSExportPrefix)
	}

	// routefilter: export prefix differs from package name
	routefilter := subs[4]
	if routefilter.TSExportPrefix != "Routing" {
		t.Errorf("routefilter.TSExportPrefix = %q, want Routing", routefilter.TSExportPrefix)
	}
}

func TestLoadSubsystemRegistryValidatesAgainstSchema(t *testing.T) {
	subs, err := LoadSubsystemRegistry(registryPath(t))
	if err != nil {
		t.Fatalf("LoadSubsystemRegistry schema validation failed: %v", err)
	}
	// Schema validation is built into LoadSubsystemRegistry.
	// If we get here with 8 subsystems the schema accepted the document.
	if len(subs) == 0 {
		t.Fatal("empty registry after successful load")
	}
}

func TestLoadSubsystemRegistryRejectsInvalidDocument(t *testing.T) {
	// Write a temp file missing the required ts_export_prefix field.
	tmp := t.TempDir()
	badPath := tmp + "/bad.yaml"
	badYAML := []byte("subsystems:\n  - name: foo\n")
	if err := os.WriteFile(badPath, badYAML, 0o644); err != nil {
		t.Fatalf("write bad yaml: %v", err)
	}
	_, err := LoadSubsystemRegistry(badPath)
	if err == nil {
		t.Fatal("expected error for invalid document, got nil")
	}
}

func TestLoadSubsystemRegistryRejectsUnknownFields(t *testing.T) {
	tmp := t.TempDir()
	path := tmp + "/bad.yaml"
	badYAML := []byte("subsystems:\n  - name: foo\n    ts_export_prefix: Foo\n    unknown_field: bar\n")
	if err := os.WriteFile(path, badYAML, 0o644); err != nil {
		t.Fatalf("write bad yaml: %v", err)
	}
	_, err := LoadSubsystemRegistry(path)
	if err == nil {
		t.Fatal("expected error for unknown field, got nil")
	}
}
