package applicablemodifiers

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestLoadForSubsystemFiltersFromModifiersYaml(t *testing.T) {
	keys, err := LoadForSubsystem(
		"../../../../config/sport-taxonomy/modifiers.yaml",
		"heatmap",
	)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"is_electric", "is_commute"}
	if !reflect.DeepEqual(keys, want) {
		t.Errorf("got %v, want %v", keys, want)
	}
}

func writeFixture(t *testing.T, content string) string {
	t.Helper()
	tmp := t.TempDir()
	path := filepath.Join(tmp, "applicable_modifiers.yaml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	return path
}

func TestLoadValidApplicableModifiersFile(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "applicable_modifiers.yaml")
	content := []byte("applicable_modifiers:\n  - is_electric\n  - is_indoor\n  - is_virtual\n")
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	want := []string{"is_electric", "is_indoor", "is_virtual"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Load = %v, want %v", got, want)
	}
}

func TestLoadRejectsMissingApplicableModifiersKey(t *testing.T) {
	path := writeFixture(t, "other_key: foo\n")
	_, err := Load(path)
	if err == nil {
		t.Fatal("expected error for missing applicable_modifiers key")
	}
	if !strings.Contains(err.Error(), "applicable_modifiers") {
		t.Fatalf("error should mention applicable_modifiers, got: %v", err)
	}
}

func TestLoadRejectsAdditionalProperties(t *testing.T) {
	path := writeFixture(t, "applicable_modifiers:\n  - is_electric\nfoo: bar\n")
	_, err := Load(path)
	if err == nil {
		t.Fatal("expected error for additional properties")
	}
	// jsonschema/v6 reports "additional properties 'foo' not allowed"
	if !strings.Contains(err.Error(), "additional properties") {
		t.Fatalf("error should mention additional properties, got: %v", err)
	}
}

func TestLoadRejectsNonStringEntries(t *testing.T) {
	path := writeFixture(t, "applicable_modifiers:\n  - 42\n  - true\n")
	_, err := Load(path)
	if err == nil {
		t.Fatal("expected error for non-string entries")
	}
	if !strings.Contains(err.Error(), "string") {
		t.Fatalf("error should mention string type, got: %v", err)
	}
}
