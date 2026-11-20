package coverage

import (
	"context"
	"testing"
)

func TestDiskCoverageReaderRejectsInvalidLayer(t *testing.T) {
	r, err := NewDiskCoverageReader(t.TempDir(), 4)
	if err != nil {
		t.Fatalf("NewDiskCoverageReader: %v", err)
	}
	defer r.Close()

	for _, bad := range []string{"", ".", "..", "../etc", "forest/../../etc", "unknown_layer"} {
		if r.HasTile(bad, 1) {
			t.Errorf("HasTile(%q) = true, want false", bad)
		}
		if _, err := r.LoadLayer(bad, 1); err == nil {
			t.Errorf("LoadLayer(%q) succeeded, want error", bad)
		}
		if _, err := r.SamplePointByLayer(context.Background(), bad, 47.0, 11.0); err == nil {
			t.Errorf("SamplePointByLayer(%q) succeeded, want error", bad)
		}
	}
}
