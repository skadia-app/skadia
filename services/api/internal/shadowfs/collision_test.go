package shadowfs

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveCollision_MultipleCollisions(t *testing.T) {
	dir := t.TempDir()
	// Create files at base path to trigger collisions
	os.WriteFile(filepath.Join(dir, "test.fit"), []byte("a"), 0644)
	os.WriteFile(filepath.Join(dir, "test_1.fit"), []byte("b"), 0644)

	result := ResolveCollision(dir, "test.fit")
	if result != "test_2.fit" {
		t.Errorf("got %q, want test_2.fit", result)
	}
}
