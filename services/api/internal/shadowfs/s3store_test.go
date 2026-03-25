package shadowfs

import (
	"os"
	"path/filepath"
	"testing"
)

// TODO: Add mock-based S3ShadowFS tests
func TestS3ShadowFS_PutGet(t *testing.T) {
	t.Skip("requires S3 mock — implement with mock S3 client")
}

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

func TestDetectContentType_Various(t *testing.T) {
	tests := []struct {
		name     string
		fileName string
		explicit string
		magic    []byte
		want     string
	}{
		{"explicit wins", "test.txt", "application/json", nil, "application/json"},
		{"extension", "photo.jpg", "", nil, "image/jpeg"},
		{"magic bytes", "unknown", "", []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}, "image/png"},
		{"fallback", "unknown", "", nil, "application/octet-stream"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DetectContentType(tt.fileName, tt.explicit, tt.magic)
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}
