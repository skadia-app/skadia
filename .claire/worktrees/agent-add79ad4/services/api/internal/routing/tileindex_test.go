package routing

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/skadia-app/skadia/services/api/internal/routing/graph"
)

func TestTileIndex_Scan(t *testing.T) {
	dir := t.TempDir()
	for _, id := range []uint32{49508, 49509, 49868} {
		path := filepath.Join(dir, graph.TileFileName(id))
		if err := os.WriteFile(path, []byte("dummy"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	idx, err := ScanTileIndex(dir)
	if err != nil {
		t.Fatal(err)
	}
	if idx.Count() != 3 {
		t.Errorf("count = %d, want 3", idx.Count())
	}
	path, ok := idx.TilePath(49508)
	if !ok {
		t.Fatal("tile 49508 not found")
	}
	if filepath.Base(path) != graph.TileFileName(49508) {
		t.Errorf("path = %s", path)
	}
	_, ok = idx.TilePath(99999)
	if ok {
		t.Error("tile 99999 should not exist")
	}
}

func TestTileIndex_TileForCoord(t *testing.T) {
	dir := t.TempDir()
	id := graph.TileIDFromCoord(47.37, 8.54)
	path := filepath.Join(dir, graph.TileFileName(id))
	os.WriteFile(path, []byte("dummy"), 0o644)
	idx, _ := ScanTileIndex(dir)
	tid, ok := idx.TileForCoord(47.37, 8.54)
	if !ok || tid != id {
		t.Errorf("TileForCoord(47.37, 8.54) = %d, %v; want %d, true", tid, ok, id)
	}
}
