package shadowfs

import "testing"

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
