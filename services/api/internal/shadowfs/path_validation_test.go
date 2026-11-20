package shadowfs

import (
	"os"
	"path/filepath"
	"testing"
)

func TestValidateRelPathRejectsSiblingPrefixSymlinkEscape(t *testing.T) {
	parent := t.TempDir()
	base := filepath.Join(parent, "library")
	sibling := filepath.Join(parent, "library_secret")
	if err := os.MkdirAll(base, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(sibling, 0o755); err != nil {
		t.Fatal(err)
	}
	secretFile := filepath.Join(sibling, "data.txt")
	if err := os.WriteFile(secretFile, []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(secretFile, filepath.Join(base, "escape.txt")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(sibling, filepath.Join(base, "escape_dir")); err != nil {
		t.Fatal(err)
	}

	if err := ValidateRelPath(base, "escape.txt"); err == nil {
		t.Fatal("ValidateRelPath allowed symlink escaping to sibling prefix directory")
	}
	if err := ValidateRelPath(base, "escape_dir/new.txt"); err == nil {
		t.Fatal("ValidateRelPath allowed parent symlink escaping to sibling prefix directory")
	}
	if err := ValidateRelPath(base, "nested/ok.txt"); err != nil {
		t.Fatalf("ValidateRelPath rejected valid nested path: %v", err)
	}
}

func TestWatcherManagerEnableUserRejectsTraversal(t *testing.T) {
	wm := NewWatcherManager(t.TempDir(), func(string, string) error { return nil }, nil)
	defer wm.StopAll()

	for _, bad := range []string{"", ".", "..", "../escape", "a/b", `a\b`} {
		if err := wm.EnableUser(bad, true); err == nil {
			t.Errorf("EnableUser(%q) succeeded, want error", bad)
		}
	}
}
