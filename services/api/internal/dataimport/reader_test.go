package dataimport

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"testing"
)

// createTestZip builds a minimal in-memory ZIP containing:
//
//	manifest.json
//	columns.json
//	data/activities.json
//
// The ZIP is written to a temp file; the caller is responsible for cleanup via t.Cleanup.
func createTestZip(t *testing.T) string {
	t.Helper()

	f, err := os.CreateTemp(t.TempDir(), "testexport-*.zip")
	if err != nil {
		t.Fatalf("createTestZip: create temp file: %v", err)
	}
	defer f.Close()

	zw := zip.NewWriter(f)

	writeEntry := func(name string, v any) {
		t.Helper()
		w, err := zw.Create(name)
		if err != nil {
			t.Fatalf("createTestZip: create entry %q: %v", name, err)
		}
		if err := json.NewEncoder(w).Encode(v); err != nil {
			t.Fatalf("createTestZip: encode %q: %v", name, err)
		}
	}

	writeEntry("manifest.json", map[string]any{
		"format_version":   1,
		"skadia_version":  "2026.04",
		"schema_migration": 293,
		"exported_at":      "2026-04-12T10:00:00Z",
		"user_id":          "test-user-uuid",
		"entity_counts":    map[string]int{"activities": 42},
	})

	writeEntry("columns.json", map[string]any{
		"activities": []string{"id", "user_id", "name"},
	})

	writeEntry("data/activities.json", []map[string]any{
		{"id": "act-1", "name": "Morning Run"},
	})

	if err := zw.Close(); err != nil {
		t.Fatalf("createTestZip: close zip: %v", err)
	}

	return f.Name()
}

// TestZipSource_Manifest checks that format_version and user_id are read correctly.
func TestZipSource_Manifest(t *testing.T) {
	path := createTestZip(t)

	src, err := NewZipSource(path)
	if err != nil {
		t.Fatalf("NewZipSource: %v", err)
	}
	defer src.Close()

	m, err := src.Manifest()
	if err != nil {
		t.Fatalf("Manifest(): %v", err)
	}

	if m.FormatVersion != 1 {
		t.Errorf("FormatVersion: want 1, got %d", m.FormatVersion)
	}
	if m.UserID != "test-user-uuid" {
		t.Errorf("UserID: want %q, got %q", "test-user-uuid", m.UserID)
	}
	if m.SchemaMigration != 293 {
		t.Errorf("SchemaMigration: want 293, got %d", m.SchemaMigration)
	}
}

// TestZipSource_OpenData verifies that data/activities.json can be read from the ZIP.
func TestZipSource_OpenData(t *testing.T) {
	path := createTestZip(t)

	src, err := NewZipSource(path)
	if err != nil {
		t.Fatalf("NewZipSource: %v", err)
	}
	defer src.Close()

	rc, err := src.OpenData("data/activities.json")
	if err != nil {
		t.Fatalf("OpenData: %v", err)
	}
	defer rc.Close()

	raw, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}

	var rows []map[string]any
	if err := json.Unmarshal(raw, &rows); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	if len(rows) != 1 || rows[0]["name"] != "Morning Run" {
		t.Errorf("unexpected activities data: %v", rows)
	}
}

// TestZipSource_PathTraversal confirms that a path containing "../" is rejected.
func TestZipSource_PathTraversal(t *testing.T) {
	// Build a ZIP that contains a path-traversal entry.
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, err := zw.Create("../etc/passwd")
	if err != nil {
		t.Fatalf("create entry: %v", err)
	}
	if _, err := w.Write([]byte("root:x:0:0")); err != nil {
		t.Fatalf("write entry: %v", err)
	}
	zw.Close()

	f, err := os.CreateTemp(t.TempDir(), "traversal-*.zip")
	if err != nil {
		t.Fatalf("create temp: %v", err)
	}
	if _, err := f.Write(buf.Bytes()); err != nil {
		t.Fatalf("write zip: %v", err)
	}
	f.Close()

	src, err := NewZipSource(f.Name())
	if err != nil {
		t.Fatalf("NewZipSource: %v", err)
	}
	defer src.Close()

	// The traversal path must be rejected by OpenData.
	_, err = src.OpenData("../etc/passwd")
	if err == nil {
		t.Fatal("OpenData with path-traversal should have returned an error")
	}
}

// TestDirSource_Manifest creates a temp directory with a manifest.json and verifies parsing.
func TestDirSource_Manifest(t *testing.T) {
	dir := t.TempDir()

	manifest := map[string]any{
		"format_version":   1,
		"skadia_version":  "2026.04",
		"schema_migration": 293,
		"exported_at":      "2026-04-12T10:00:00Z",
		"user_id":          "dir-user-uuid",
	}
	raw, _ := json.Marshal(manifest)
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), raw, 0o600); err != nil {
		t.Fatalf("write manifest.json: %v", err)
	}

	src := NewDirSource(dir)
	defer src.Close()

	m, err := src.Manifest()
	if err != nil {
		t.Fatalf("Manifest(): %v", err)
	}

	if m.FormatVersion != 1 {
		t.Errorf("FormatVersion: want 1, got %d", m.FormatVersion)
	}
	if m.UserID != "dir-user-uuid" {
		t.Errorf("UserID: want %q, got %q", "dir-user-uuid", m.UserID)
	}
}
