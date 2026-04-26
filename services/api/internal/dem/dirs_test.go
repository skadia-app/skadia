package dem

import (
	"path/filepath"
	"testing"
)

func TestHGTDirsForConfig_GoldenList(t *testing.T) {
	const base = "/data/dem"
	got := HGTDirsForConfig(base)

	want := []string{
		filepath.Join(base, "sonny-05"),
		filepath.Join(base, "sonny-1"),
		filepath.Join(base, "glo30"),
	}

	if len(got) != len(want) {
		t.Fatalf("HGTDirsForConfig: got %d dirs, want %d", len(got), len(want))
	}
	for i, g := range got {
		if g != want[i] {
			t.Errorf("HGTDirsForConfig[%d] = %q, want %q", i, g, want[i])
		}
	}
}

func TestHGTDirsForConfig_EmptyBase(t *testing.T) {
	got := HGTDirsForConfig("")
	if len(got) != 3 {
		t.Fatalf("HGTDirsForConfig with empty base: got %d dirs, want 3", len(got))
	}
	// With empty base, filepath.Join("", "sonny-05") == "sonny-05"
	if got[0] != "sonny-05" {
		t.Errorf("HGTDirsForConfig(\"\")[0] = %q, want %q", got[0], "sonny-05")
	}
}
