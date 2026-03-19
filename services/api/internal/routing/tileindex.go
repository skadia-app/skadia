package routing

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/skadia-app/skadia/services/api/internal/routing/graph"
)

type TileIndex struct {
	dir   string
	tiles map[uint32]string
}

func ScanTileIndex(dir string) (*TileIndex, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("scan tile dir %s: %w", dir, err)
	}
	idx := &TileIndex{dir: dir, tiles: make(map[uint32]string)}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasPrefix(name, "tile_") || !strings.HasSuffix(name, ".bin") {
			continue
		}
		// Parse tile_LAT_LON.bin format (e.g. tile_46_14.bin)
		idStr := strings.TrimPrefix(strings.TrimSuffix(name, ".bin"), "tile_")
		parts := strings.SplitN(idStr, "_", 2)
		if len(parts) == 2 {
			lat, err1 := strconv.Atoi(parts[0])
			lon, err2 := strconv.Atoi(parts[1])
			if err1 == nil && err2 == nil {
				tileID := uint32((lat+90)*360 + (lon + 180))
				idx.tiles[tileID] = filepath.Join(dir, name)
				continue
			}
		}
		// Fallback: try numeric tile ID (legacy format)
		id, err := strconv.ParseUint(idStr, 10, 32)
		if err != nil {
			continue
		}
		idx.tiles[uint32(id)] = filepath.Join(dir, name)
	}
	return idx, nil
}

func (idx *TileIndex) Count() int { return len(idx.tiles) }

func (idx *TileIndex) TilePath(tileID uint32) (string, bool) {
	p, ok := idx.tiles[tileID]
	return p, ok
}

func (idx *TileIndex) TileForCoord(lat, lon float64) (uint32, bool) {
	id := graph.TileIDFromCoord(lat, lon)
	_, ok := idx.tiles[id]
	return id, ok
}

func (idx *TileIndex) AllTileIDs() []uint32 {
	ids := make([]uint32, 0, len(idx.tiles))
	for id := range idx.tiles {
		ids = append(ids, id)
	}
	return ids
}
