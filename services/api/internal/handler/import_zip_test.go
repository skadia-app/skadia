package handler

import (
	"archive/zip"
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

func TestExtractZipToTempDirRejectsTraversalAndSymlinks(t *testing.T) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)

	addEntry := func(name, content string, mode fs.FileMode) {
		t.Helper()
		hdr := &zip.FileHeader{Name: name}
		hdr.SetMode(mode)
		w, err := zw.CreateHeader(hdr)
		if err != nil {
			t.Fatalf("create zip entry %q: %v", name, err)
		}
		if _, err := w.Write([]byte(content)); err != nil {
			t.Fatalf("write zip entry %q: %v", name, err)
		}
	}

	addEntry("nested/valid.txt", "safe", 0o644)
	addEntry("../escape.txt", "evil", 0o644)
	addEntry("nested/../../escape2.txt", "evil", 0o644)
	addEntry("/abs/escape.txt", "evil", 0o644)
	addEntry(`..\win-escape.txt`, "evil", 0o644)
	addEntry(".", "dot-file", 0o644)
	addEntry("link.txt", "/etc/passwd", fs.ModeSymlink|0o777)

	if err := zw.Close(); err != nil {
		t.Fatalf("close zip: %v", err)
	}

	tmpDir, err := extractZipToTempDir(&buf)
	if err != nil {
		t.Fatalf("extractZipToTempDir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	data, err := os.ReadFile(filepath.Join(tmpDir, "nested", "valid.txt"))
	if err != nil {
		t.Fatalf("expected valid nested file to be extracted: %v", err)
	}
	if string(data) != "safe" {
		t.Fatalf("valid file content = %q, want %q", string(data), "safe")
	}

	if _, err := os.Lstat(filepath.Join(tmpDir, "link.txt")); !os.IsNotExist(err) {
		t.Fatalf("symlink entry should not have been extracted, stat err=%v", err)
	}
	parentEscape := filepath.Join(filepath.Dir(tmpDir), "escape.txt")
	if _, err := os.Lstat(parentEscape); !os.IsNotExist(err) {
		t.Fatalf("traversal entry escaped temp dir: %s", parentEscape)
	}
}
