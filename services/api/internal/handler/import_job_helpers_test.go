package handler

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/skadia-app/skadia/services/api/internal/importutil"
)

func TestIsTempSourcePathStrictPrefix(t *testing.T) {
	tmpRoot := os.TempDir()
	if isTempSourcePath(tmpRoot) {
		t.Fatalf("isTempSourcePath(%q) = true, must never match OS temp root itself", tmpRoot)
	}
	if isTempSourcePath(filepath.Join(tmpRoot, "other-dir")) {
		t.Fatal("isTempSourcePath matched unrelated temp subdirectory")
	}
	if isTempSourcePath(filepath.Join(tmpRoot, "skadia-import-123", "nested")) {
		t.Fatal("isTempSourcePath matched nested child instead of direct skadia-import-* dir")
	}

	dir, err := os.MkdirTemp("", "skadia-import-*")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	defer os.RemoveAll(dir)

	if !isTempSourcePath(dir) {
		t.Fatalf("isTempSourcePath(%q) = false, want true", dir)
	}

	if _, err := importutil.ValidateImportSourcePath(tmpRoot); err == nil {
		t.Fatalf("ValidateImportSourcePath(%q) succeeded, must reject bare OS temp root", tmpRoot)
	}
	if _, err := importutil.ValidateImportSourcePath(dir); err != nil {
		t.Fatalf("ValidateImportSourcePath(%q) failed: %v", dir, err)
	}
}
