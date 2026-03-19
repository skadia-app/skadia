// tiledreader.go
package routing

import (
	"container/list"
	"path/filepath"
	"strconv"
	"sync"

	"golang.org/x/sync/singleflight"

	"github.com/skadia-app/skadia/services/api/internal/routing/graph"
)

// TiledGraphReader implements GraphReader with on-demand tile loading,
// LRU cache, singleflight dedup, and transparent border-node expansion.
type TiledGraphReader struct {
	tileDir string
	cache   *tiledCache
}

// NewTiledGraphReader creates a reader with configurable cache size.
func NewTiledGraphReader(tileDir string, maxBytes int64) *TiledGraphReader {
	return &TiledGraphReader{
		tileDir: tileDir,
		cache:   newTiledCache(maxBytes),
	}
}

func (r *TiledGraphReader) ForEachEdge(nodeID uint64, fn func(EdgeView)) {
	tileID, localID := SplitGlobalNodeID(nodeID)
	g := r.cache.getOrLoad(tileID, r.tileDir)
	if g == nil {
		return
	}
	if localID >= g.NodeCount {
		return // invalid node ID — corrupted parent pointer or stale cache
	}

	// 1. Local edges within the tile
	for ei := g.Offsets[localID]; ei < g.Offsets[localID+1]; ei++ {
		target := g.Targets[ei]
		fn(EdgeView{
			Target:     MakeGlobalNodeID(tileID, target),
			TargetLat:  g.NodeLat[target],
			TargetLon:  g.NodeLon[target],
			Dist:       g.EdgeDist[ei],
			Tags1:      g.EdgeTags1[ei],
			Tags2:      g.EdgeTags2[ei],
			ElevHandle: uint64(tileID)<<32 | uint64(ei),
			DeltaEle:   g.NodeEle[target] - g.NodeEle[localID],
		})
	}

	// 2. Border-node expansion (transparent for A*)
	if g.IsBorder != nil && localID < uint32(len(g.IsBorder)) && g.IsBorder[localID] {
		coordKey := graph.CoordKeyToInt64(graph.CoordKey{
			Lat: g.NodeLat[localID],
			Lon: g.NodeLon[localID],
		})

		for _, neighborTileID := range graph.NeighborTileIDs(tileID) {
			gNeighbor := r.cache.getOrLoad(neighborTileID, r.tileDir)
			if gNeighbor == nil || gNeighbor.BorderIndex == nil {
				continue
			}

			neighborLocalID, ok := gNeighbor.BorderIndex.Lookup(coordKey)
			if !ok {
				continue
			}

			for ei := gNeighbor.Offsets[neighborLocalID]; ei < gNeighbor.Offsets[neighborLocalID+1]; ei++ {
				target := gNeighbor.Targets[ei]
				fn(EdgeView{
					Target:     MakeGlobalNodeID(neighborTileID, target),
					TargetLat:  gNeighbor.NodeLat[target],
					TargetLon:  gNeighbor.NodeLon[target],
					Dist:       gNeighbor.EdgeDist[ei],
					Tags1:      gNeighbor.EdgeTags1[ei],
					Tags2:      gNeighbor.EdgeTags2[ei],
					ElevHandle: uint64(neighborTileID)<<32 | uint64(ei),
					DeltaEle:   gNeighbor.NodeEle[target] - gNeighbor.NodeEle[neighborLocalID],
				})
			}
		}
	}
}

func (r *TiledGraphReader) NodeCoord(nodeID uint64) (lat, lon, ele int32) {
	tileID, localID := SplitGlobalNodeID(nodeID)
	g := r.cache.getOrLoad(tileID, r.tileDir)
	if g == nil {
		return 0, 0, 0
	}
	return g.NodeLat[localID], g.NodeLon[localID], g.NodeEle[localID]
}

func (r *TiledGraphReader) NearestNode(lat, lon float64) (uint64, float64, bool) {
	primaryTileID := graph.TileIDFromCoord(lat, lon)
	g := r.cache.getOrLoad(primaryTileID, r.tileDir)
	if g == nil {
		return 0, 0, false
	}

	bestLocal, bestDistDm := graph.NearestNode(g, lat, lon)
	bestDist := float64(bestDistDm) / 10.0
	bestTile := primaryTileID

	if bestDistDm == 65535 {
		bestDist = 1e9
	}

	// Edge case: near tile border -> also check neighbor tiles
	bbox := graph.TileBBox(primaryTileID)
	const borderCheckDist = 0.02 // ~2.2 km, matches BorderBuffer
	nearBorder := (lat-bbox[0] < borderCheckDist || bbox[2]-lat < borderCheckDist ||
		lon-bbox[1] < borderCheckDist || bbox[3]-lon < borderCheckDist)

	if nearBorder {
		for _, neighborTileID := range graph.NeighborTileIDs(primaryTileID) {
			gNeighbor := r.cache.getOrLoad(neighborTileID, r.tileDir)
			if gNeighbor == nil {
				continue
			}
			candidateLocal, candidateDistDm := graph.NearestNode(gNeighbor, lat, lon)
			candidateDist := float64(candidateDistDm) / 10.0
			if candidateDistDm != 65535 && candidateDist < bestDist {
				bestLocal = candidateLocal
				bestDist = candidateDist
				bestTile = neighborTileID
			}
		}
	}

	if bestDist > 1e8 {
		return 0, 0, false
	}
	return MakeGlobalNodeID(bestTile, bestLocal), bestDist, true
}

func (r *TiledGraphReader) EdgeElevProfile(handle uint64) ([]int32, []uint16) {
	tileID, edgeIdx := SplitGlobalNodeID(handle)
	g := r.cache.getOrLoad(tileID, r.tileDir)
	if g == nil {
		return nil, nil
	}

	elevStart := g.EdgeElevOff[edgeIdx]
	elevEnd := g.EdgeElevOff[edgeIdx+1]
	nPoints := elevEnd - elevStart

	elevs := g.EdgeElevProf[elevStart:elevEnd]

	if nPoints <= 1 || len(g.EdgeElevSegDist) == 0 {
		return elevs, nil
	}

	// C05: ElevSegDist offset = elevStart - edgeIdx
	segDistStart := elevStart - edgeIdx
	segDists := g.EdgeElevSegDist[segDistStart : segDistStart+nPoints-1]

	return elevs, segDists
}

func (r *TiledGraphReader) EdgeGeometry(handle uint64) ([]int32, []int32) {
	tileID, edgeIdx := SplitGlobalNodeID(handle)
	g := r.cache.getOrLoad(tileID, r.tileDir)
	if g == nil {
		return nil, nil
	}

	geomStart := g.EdgeGeomOff[edgeIdx]
	geomEnd := g.EdgeGeomOff[edgeIdx+1]
	nPts := geomEnd - geomStart

	// A04: return only intermediate points (exclude start/end node coords)
	if nPts <= 2 {
		return nil, nil
	}
	return g.EdgeGeomLat[geomStart+1 : geomEnd-1], g.EdgeGeomLon[geomStart+1 : geomEnd-1]
}

func (r *TiledGraphReader) EdgeOSMID(handle uint64) int64 {
	tileID, edgeIdx := SplitGlobalNodeID(handle)
	g := r.cache.getOrLoad(tileID, r.tileDir)
	if g == nil || len(g.EdgeOSMIdx) == 0 || len(g.OSMIDPool) == 0 {
		return 0
	}
	osmIdx := g.EdgeOSMIdx[edgeIdx]
	if int(osmIdx) >= len(g.OSMIDPool) {
		return 0
	}
	return g.OSMIDPool[osmIdx]
}

// HasTiles returns true if any tiles exist in the directory.
func (r *TiledGraphReader) HasTiles() bool {
	matches, _ := filepath.Glob(filepath.Join(r.tileDir, "tile_*.bin"))
	return len(matches) > 0
}

// --- Tiled LRU Cache with singleflight ---

type tiledCacheEntry struct {
	tileID    uint32
	g         *graph.Graph
	sizeBytes int64
}

type tiledCache struct {
	mu        sync.Mutex
	maxBytes  int64
	usedBytes int64
	items     map[uint32]*list.Element
	order     *list.List
	inflight  singleflight.Group
}

func newTiledCache(maxBytes int64) *tiledCache {
	return &tiledCache{
		maxBytes: maxBytes,
		items:    make(map[uint32]*list.Element),
		order:    list.New(),
	}
}

func (tc *tiledCache) flush() {
	tc.mu.Lock()
	defer tc.mu.Unlock()
	tc.items = make(map[uint32]*list.Element)
	tc.order.Init()
	tc.usedBytes = 0
}

// Flush evicts all cached tiles from the TiledGraphReader.
// Call after a tile rebuild to ensure stale tiles are not served.
func (r *TiledGraphReader) Flush() {
	r.cache.flush()
}

func (tc *tiledCache) getOrLoad(tileID uint32, tileDir string) *graph.Graph {
	tc.mu.Lock()
	if elem, ok := tc.items[tileID]; ok {
		tc.order.MoveToFront(elem)
		g := elem.Value.(*tiledCacheEntry).g
		tc.mu.Unlock()
		return g
	}
	tc.mu.Unlock()

	// singleflight: only one load per tileID concurrently
	key := strconv.FormatUint(uint64(tileID), 10)
	val, _, _ := tc.inflight.Do(key, func() (any, error) {
		path := filepath.Join(tileDir, tileName(tileID))
		g, err := graph.DeserializeMmap(path)
		if err != nil {
			return nil, err
		}
		return g, nil
	})
	if val == nil {
		return nil
	}

	g := val.(*graph.Graph)

	tc.mu.Lock()
	defer tc.mu.Unlock()

	// Double-checked locking: another waiter may have already inserted
	if elem, ok := tc.items[tileID]; ok {
		tc.order.MoveToFront(elem)
		return elem.Value.(*tiledCacheEntry).g
	}

	size := estimateTileBytes(g)
	entry := &tiledCacheEntry{tileID: tileID, g: g, sizeBytes: size}
	elem := tc.order.PushFront(entry)
	tc.items[tileID] = elem
	tc.usedBytes += size

	// Evict LRU until under budget
	for tc.usedBytes > tc.maxBytes && tc.order.Len() > 1 {
		back := tc.order.Back()
		if back == nil {
			break
		}
		evicted := back.Value.(*tiledCacheEntry)
		tc.order.Remove(back)
		delete(tc.items, evicted.tileID)
		tc.usedBytes -= evicted.sizeBytes
	}

	return g
}

// R11: Conservative estimate — 48 bytes/edge accounts for all parallel arrays.
func estimateTileBytes(g *graph.Graph) int64 {
	if g == nil {
		return 0
	}
	edgeBytes := int64(g.EdgeCount) * 48
	nodeBytes := int64(g.NodeCount) * 13
	geomBytes := int64(len(g.EdgeGeomLat)) * 8
	elevBytes := int64(len(g.EdgeElevProf)) * 4
	segDistBytes := int64(len(g.EdgeElevSegDist)) * 2
	return edgeBytes + nodeBytes + geomBytes + elevBytes + segDistBytes
}

func tileName(tileID uint32) string {
	lat := int(tileID/360) - 90
	lon := int(tileID%360) - 180
	return "tile_" + strconv.Itoa(lat) + "_" + strconv.Itoa(lon) + ".bin"
}
