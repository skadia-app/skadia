package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRenderSystemTagsEmitsCanonicalForm(t *testing.T) {
	repoRoot := repoRootFromTestPath(t)
	yamlPath := filepath.Join(repoRoot, "config", "tags", "system_tags.yaml")
	schemaPath := filepath.Join(repoRoot, "config", "tags", "schema", "system_tags.schema.json")
	output, err := generateGo(yamlPath, schemaPath)
	if err != nil {
		t.Fatalf("generateGo: %v", err)
	}
	for _, needle := range []string{
		"package tags",
		"type SystemTagSync struct",
		"var SystemTagSyncs = []SystemTagSync{",
		`ModifierKey:   "is_electric"`,
		`ModifierKey:   "is_indoor"`,
		`ModifierKey:   "is_virtual"`,
		`ModifierKey:   "is_commute"`,
		"func SystemTagSyncByModifierKey(key string) (SystemTagSync, bool)",
	} {
		if !strings.Contains(output, needle) {
			t.Fatalf("generated Go missing %q\ngot:\n%s", needle, output)
		}
	}
}

func TestRenderSystemTagsEmitsLocalizedTSDescriptions(t *testing.T) {
	repoRoot := repoRootFromTestPath(t)
	yamlPath := filepath.Join(repoRoot, "config", "tags", "system_tags.yaml")
	schemaPath := filepath.Join(repoRoot, "config", "tags", "schema", "system_tags.schema.json")
	output, err := generateTSBytes(yamlPath, schemaPath)
	if err != nil {
		t.Fatalf("generateTSBytes: %v", err)
	}
	for _, needle := range []string{
		`import { i18n } from '@/i18n/i18n'`,
		`get description() { return i18n.t('systemTags.descriptions.is_electric') }`,
		`get description() { return i18n.t('systemTags.descriptions.is_indoor') }`,
		`get description() { return i18n.t('systemTags.descriptions.is_virtual') }`,
		`get description() { return i18n.t('systemTags.descriptions.is_commute') }`,
	} {
		if !strings.Contains(string(output), needle) {
			t.Fatalf("generated TS missing %q\ngot:\n%s", needle, output)
		}
	}
}

func TestRenderSystemTagsRejectsUnknownModifierKey(t *testing.T) {
	schemaPath := filepath.Join(repoRootFromTestPath(t), "config", "tags", "schema", "system_tags.schema.json")
	tmp := filepath.Join(t.TempDir(), "fixture.yaml")
	err := os.WriteFile(tmp, []byte(`system_tags:
  - modifier_key: is_typoed
    system_tag: Typoed
    color: "#AABBCC"
    description: Test
    readonly_sync: false
`), 0o644)
	if err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	_, err = generateGo(tmp, schemaPath)
	if err == nil {
		t.Fatal("expected error for unknown modifier key, got nil")
	}
	if !strings.Contains(err.Error(), "is_typoed") {
		t.Fatalf("error should mention is_typoed; got: %v", err)
	}
}

func TestRenderSystemTagsRejectsBadColor(t *testing.T) {
	schemaPath := filepath.Join(repoRootFromTestPath(t), "config", "tags", "schema", "system_tags.schema.json")
	tmp := filepath.Join(t.TempDir(), "fixture.yaml")
	err := os.WriteFile(tmp, []byte(`system_tags:
  - modifier_key: is_indoor
    system_tag: Indoor
    color: "red"
    description: Test
    readonly_sync: true
`), 0o644)
	if err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	_, err = generateGo(tmp, schemaPath)
	if err == nil {
		t.Fatal("expected schema validation error for bad color, got nil")
	}
}

func TestCheckFileReportsDiffForStaleGeneratedOutput(t *testing.T) {
	path := filepath.Join(t.TempDir(), "generated.go")
	if err := os.WriteFile(path, []byte("package old\n"), 0o644); err != nil {
		t.Fatalf("write stale file: %v", err)
	}
	err := checkFile(path, []byte("package new\n"))
	if err == nil {
		t.Fatal("nil for stale output")
	}
	for _, want := range []string{"is stale", "+package new", "-package old"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("missing %q in error: %v", want, err)
		}
	}
}

func TestCheckFilePassesForCurrentOutput(t *testing.T) {
	path := filepath.Join(t.TempDir(), "generated.go")
	content := []byte("package current\n")
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	if err := checkFile(path, content); err != nil {
		t.Fatalf("unexpected error for current output: %v", err)
	}
}

// repoRootFromTestPath locates the repo root by walking up from the test binary.
func repoRootFromTestPath(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			// go.mod is in services/api; repo root is two levels up
			return filepath.Join(dir, "..", "..")
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("could not find repo root from test path")
		}
		dir = parent
	}
}
