// tiledreader.go
package routing

import (
	"container/list"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"sync/atomic"

	"golang.org/x/sync/singleflight"

	"github.com/skadia-app/skadia/services/api/internal/routing/graph"
)

// borderCacheKey uniquely identifies a (coordKey, neighborTileID) pair.
type borderCacheKey struct {
	coordKey       int64
	neighborTileID uint32
}

// borderCacheEntry records the result of a BorderIndex.Lookup.
type borderCacheEntry struct {
	localID uint32
	found   bool
}

// TiledGraphReader implements GraphReader with on-demand tile loading,
// LRU cache, singleflight dedup, and transparent border-node expansion.
type TiledGraphReader struct {
	tileDir     string
	cache       *tiledCache
	cachedLMeta atomic.Pointer[graph.LandmarkMetaData]
	borderCache sync.Map // borderCacheKey → borderCacheEntry
}

// NewTiledGraphReader creates a reader with configurable cache size.
func NewTiledGraphReader(tileDir string, maxBytes int64) *TiledGraphReader {
	return &TiledGraphReader{
		tileDir: tileDir,
		cache:   newTiledCache(maxBytes),
	}
}

// tile loads (or retrieves from cache) a tile graph and opportunistically
// populates the reader-level LandmarkMeta atomic on first encounter (W2).
func (r *TiledGraphReader) tile(tileID uint32) *graph.Graph {
	g := r.cache.getOrLoad(tileID, r.tileDir)
	if g != nil && g.LandmarkMeta != nil && r.cachedLMeta.Load() == nil {
		r.cachedLMeta.CompareAndSwap(nil, g.LandmarkMeta)
	}
	return g
}

func (r *TiledGraphReader) ForEachInEdge(nodeID uint64, fn func(EdgeView)) {
	tileID, localID := SplitGlobalNodeID(nodeID)
	g := r.tile(tileID)
	if g == nil {
		return
	}
	g.EnsureReverseAdj()
	if g.RevOffsets == nil { // empty graph (0 nodes/edges)
		return
	}
	if localID >= g.NodeCount {
		return
	}

	// Local reverse edges within the tile
	for ri := g.RevOffsets[localID]; ri < g.RevOffsets[localID+1]; ri++ {
		srcLocal := g.RevTargets[ri]
		ei := g.RevEdgeIdx[ri]
		fn(EdgeView{
			Target:     MakeGlobalNodeID(tileID, srcLocal),
			TargetLat:  g.NodeLat[srcLocal],
			TargetLon:  g.NodeLon[srcLocal],
			Dist:       g.EdgeDist[ei],
			Tags1:      g.EdgeTags1[ei],
			Tags2:      g.EdgeTags2[ei],
			ElevHandle: uint64(tileID)<<32 | uint64(ei),
			DeltaEle:   g.NodeEle[srcLocal] - g.NodeEle[localID],
		})
	}

	// Border-node expansion: check neighbor tiles' reverse edges
	if g.IsBorder != nil && localID < uint32(len(g.IsBorder)) && g.IsBorder[localID] {
		coordKey := graph.CoordKeyToInt64(graph.CoordKey{
			Lat: g.NodeLat[localID],
			Lon: g.NodeLon[localID],
		})

		// expandNeighborRevEdges emits all reverse edges from neighborLocalID in gNeighbor.
		expandNeighborRevEdges := func(gNeighbor *graph.Graph, neighborTileID, neighborLocalID uint32) {
			gNeighbor.EnsureReverseAdj()
			if gNeighbor.RevOffsets == nil { // empty graph
				return
			}
			for ri := gNeighbor.RevOffsets[neighborLocalID]; ri < gNeighbor.RevOffsets[neighborLocalID+1]; ri++ {
				srcLocal := gNeighbor.RevTargets[ri]
				ei := gNeighbor.RevEdgeIdx[ri]
				fn(EdgeView{
					Target:     MakeGlobalNodeID(neighborTileID, srcLocal),
					TargetLat:  gNeighbor.NodeLat[srcLocal],
					TargetLon:  gNeighbor.NodeLon[srcLocal],
					Dist:       gNeighbor.EdgeDist[ei],
					Tags1:      gNeighbor.EdgeTags1[ei],
					Tags2:      gNeighbor.EdgeTags2[ei],
					ElevHandle: uint64(neighborTileID)<<32 | uint64(ei),
					DeltaEle:   gNeighbor.NodeEle[srcLocal] - gNeighbor.NodeEle[neighborLocalID],
				})
			}
		}

		nids, ncount := graph.NeighborTileIDs(tileID)
		for i := range ncount {
			neighborTileID := nids[i]
			cacheKey := borderCacheKey{coordKey: coordKey, neighborTileID: neighborTileID}

			if cached, ok := r.borderCache.Load(cacheKey); ok {
				entry := cached.(borderCacheEntry)
				if !entry.found {
					continue
				}
				gNeighbor := r.tile(neighborTileID)
				if gNeighbor == nil {
					continue
				}
				expandNeighborRevEdges(gNeighbor, neighborTileID, entry.localID)
			} else {
				gNeighbor := r.tile(neighborTileID)
				if gNeighbor == nil || gNeighbor.BorderIndex == nil {
					r.borderCache.Store(cacheKey, borderCacheEntry{found: false})
					continue
				}
				neighborLocalID, ok := gNeighbor.BorderIndex.Lookup(coordKey)
				r.borderCache.Store(cacheKey, borderCacheEntry{localID: neighborLocalID, found: ok})
				if !ok {
					continue
				}
				expandNeighborRevEdges(gNeighbor, neighborTileID, neighborLocalID)
			}
		}
	}
}

func (r *TiledGraphReader) ForEachEdge(nodeID uint64, fn func(EdgeView)) {
	tileID, localID := SplitGlobalNodeID(nodeID)
	g := r.tile(tileID)
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

		// expandNeighborEdges emits all edges from neighborLocalID in gNeighbor.
		// Extracted as a closure to avoid duplicating the edge-iteration loop
		// across the cache-hit and cache-miss paths below.
		expandNeighborEdges := func(gNeighbor *graph.Graph, neighborTileID, neighborLocalID uint32) {
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

		nids, ncount := graph.NeighborTileIDs(tileID)
		for i := range ncount {
			neighborTileID := nids[i]
			cacheKey := borderCacheKey{coordKey: coordKey, neighborTileID: neighborTileID}

			if cached, ok := r.borderCache.Load(cacheKey); ok {
				entry := cached.(borderCacheEntry)
				if !entry.found {
					continue // Known negative — skip tile load entirely
				}
				// Positive hit: tile still needed for edge data, but skip binary search
				gNeighbor := r.tile(neighborTileID)
				if gNeighbor == nil {
					continue
				}
				expandNeighborEdges(gNeighbor, neighborTileID, entry.localID)
			} else {
				// Cache miss: load tile and perform lookup
				gNeighbor := r.tile(neighborTileID)
				if gNeighbor == nil || gNeighbor.BorderIndex == nil {
					r.borderCache.Store(cacheKey, borderCacheEntry{found: false})
					continue
				}
				neighborLocalID, ok := gNeighbor.BorderIndex.Lookup(coordKey)
				r.borderCache.Store(cacheKey, borderCacheEntry{localID: neighborLocalID, found: ok})
				if !ok {
					continue
				}
				expandNeighborEdges(gNeighbor, neighborTileID, neighborLocalID)
			}
		}
	}
}

func (r *TiledGraphReader) NodeCoord(nodeID uint64) (lat, lon, ele int32) {
	tileID, localID := SplitGlobalNodeID(nodeID)
	g := r.tile(tileID)
	if g == nil {
		return 0, 0, 0
	}
	return g.NodeLat[localID], g.NodeLon[localID], g.NodeEle[localID]
}

func (r *TiledGraphReader) NearestNode(lat, lon float64) (uint64, float64, bool) {
	primaryTileID := graph.TileIDFromCoord(lat, lon)
	g := r.tile(primaryTileID)
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
		nnids, nncount := graph.NeighborTileIDs(primaryTileID)
		for i := range nncount {
			neighborTileID := nnids[i]
			gNeighbor := r.tile(neighborTileID)
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

func (r *TiledGraphReader) FindNearestEdge(lat, lon float64, filter func(tags1 uint64) bool) (graph.EdgeProjection, bool) {
	primaryTileID := graph.TileIDFromCoord(lat, lon)
	g := r.tile(primaryTileID)
	if g == nil {
		return graph.EdgeProjection{}, false
	}

	best, found := g.FindNearestEdgeLocal(lat, lon, filter)

	// Cross-tile probe: if near tile border (< 500m ≈ 0.005°), also check neighbor tile
	bbox := graph.TileBBox(primaryTileID)
	const borderCheckDist = 0.005 // ~500m
	nearBorder := (lat-bbox[0] < borderCheckDist || bbox[2]-lat < borderCheckDist ||
		lon-bbox[1] < borderCheckDist || bbox[3]-lon < borderCheckDist)

	if nearBorder {
		nids, ncount := graph.NeighborTileIDs(primaryTileID)
		for i := range ncount {
			neighborTileID := nids[i]
			gNeighbor := r.tile(neighborTileID)
			if gNeighbor == nil {
				continue
			}
			candidate, candidateFound := gNeighbor.FindNearestEdgeLocal(lat, lon, filter)
			if candidateFound && (!found || candidate.DistanceM < best.DistanceM) {
				best = candidate
				found = true
			}
		}
	}

	return best, found
}

func (r *TiledGraphReader) EdgeElevProfile(handle uint64) ([]int32, []uint16) {
	tileID, edgeIdx := SplitGlobalNodeID(handle)
	g := r.tile(tileID)
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
	g := r.tile(tileID)
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
	g := r.tile(tileID)
	if g == nil || len(g.EdgeOSMIdx) == 0 || len(g.OSMIDPool) == 0 {
		return 0
	}
	osmIdx := g.EdgeOSMIdx[edgeIdx]
	if int(osmIdx) >= len(g.OSMIDPool) {
		return 0
	}
	return g.OSMIDPool[osmIdx]
}

func (r *TiledGraphReader) EdgeTags2(handle uint64) uint64 {
	tileID, edgeIdx := SplitGlobalNodeID(handle)
	g := r.tile(tileID)
	if g == nil || int(edgeIdx) >= len(g.EdgeTags2) {
		return 0
	}
	return g.EdgeTags2[edgeIdx]
}

func (r *TiledGraphReader) EdgeDistanceCm(handle uint64) uint32 {
	tileID, edgeIdx := SplitGlobalNodeID(handle)
	g := r.tile(tileID)
	if g == nil || int(edgeIdx) >= len(g.EdgeDist) {
		return 0
	}
	return g.EdgeDist[edgeIdx]
}

func (r *TiledGraphReader) LandmarkDists(setID LandmarkSetID, nodeID uint64) []float32 {
	tileID, localID := SplitGlobalNodeID(nodeID)
	g := r.tile(tileID)
	if g == nil {
		return nil
	}
	return g.LandmarkDists(graph.LandmarkSetID(setID), localID)
}

func (r *TiledGraphReader) LandmarkDistsInto(setID LandmarkSetID, nodeID uint64, dst []float32, byteBuf []byte) []float32 {
	tileID, localID := SplitGlobalNodeID(nodeID)
	g := r.tile(tileID)
	if g == nil {
		return nil
	}
	return g.LandmarkDistsInto(graph.LandmarkSetID(setID), localID, dst, byteBuf)
}

func (r *TiledGraphReader) LandmarkDistsPageInto(setID LandmarkSetID, tileID uint32, startLocalNode, nodeCount uint32, dst []float32, byteBuf []byte) []float32 {
	g := r.tile(tileID)
	if g == nil {
		return nil
	}
	return g.LandmarkDistsPageInto(graph.LandmarkSetID(setID), startLocalNode, nodeCount, dst, byteBuf)
}

func (r *TiledGraphReader) LandmarkMeta() *graph.LandmarkMetaData {
	if m := r.cachedLMeta.Load(); m != nil {
		return m
	}
	// Fall through to cache scan — only happens until the first tile with
	// landmark data is loaded, after which the atomic is populated.
	r.cache.mu.Lock()
	defer r.cache.mu.Unlock()
	for _, elem := range r.cache.items {
		ce := elem.Value.(*tiledCacheEntry)
		if ce.g != nil && ce.g.LandmarkMeta != nil {
			r.cachedLMeta.Store(ce.g.LandmarkMeta)
			return ce.g.LandmarkMeta
		}
	}
	return nil
}

// GraphAt returns the graph for the tile containing (lat, lon), or nil.
func (r *TiledGraphReader) GraphAt(lat, lon float64) *graph.Graph {
	return r.tile(graph.TileIDFromCoord(lat, lon))
}

// HasTilesFor checks if tiles exist covering the bounding box corners.
func (r *TiledGraphReader) HasTilesFor(minLat, minLon, maxLat, maxLon float64) bool {
	// Check all four corners — if all corner tiles load, the bbox is covered.
	corners := [][2]float64{
		{minLat, minLon}, {minLat, maxLon},
		{maxLat, minLon}, {maxLat, maxLon},
	}
	for _, c := range corners {
		if r.tile(graph.TileIDFromCoord(c[0], c[1])) == nil {
			return false
		}
	}
	return true
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
	hits      atomic.Int64
	misses    atomic.Int64
	evictions atomic.Int64
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

// SetCacheBudget changes the cache budget at runtime.
// Evicts LRU tiles if the new budget is smaller than current usage.
func (tc *tiledCache) SetCacheBudget(newBytes int64) {
	tc.mu.Lock()
	tc.maxBytes = newBytes
	for tc.usedBytes > tc.maxBytes && tc.order.Len() > 1 {
		back := tc.order.Back()
		if back == nil {
			break
		}
		evicted := back.Value.(*tiledCacheEntry)
		tc.order.Remove(back)
		delete(tc.items, evicted.tileID)
		tc.usedBytes -= evicted.sizeBytes
		tc.evictions.Add(1)
	}
	tc.mu.Unlock()
}

// Flush evicts all cached tiles from the TiledGraphReader.
// Call after a tile rebuild to ensure stale tiles are not served.
func (r *TiledGraphReader) Flush() {
	r.cachedLMeta.Store(nil)
	r.cache.flush()
	r.borderCache.Range(func(key, _ any) bool {
		r.borderCache.Delete(key)
		return true
	})
}

// CacheStats returns cache hit/miss/eviction counters and byte usage.
// usedBytes is an approximate snapshot (not mutex-protected).
func (r *TiledGraphReader) CacheStats() (hits, misses, evictions, usedBytes, budgetBytes int64) {
	return r.cache.hits.Load(), r.cache.misses.Load(), r.cache.evictions.Load(),
		r.cache.usedBytes, r.cache.maxBytes
}

// SetCacheBudget adjusts the tile cache budget at runtime.
func (r *TiledGraphReader) SetCacheBudget(newBytes int64) {
	r.cache.SetCacheBudget(newBytes)
}

func (tc *tiledCache) getOrLoad(tileID uint32, tileDir string) *graph.Graph {
	tc.mu.Lock()
	if elem, ok := tc.items[tileID]; ok {
		tc.order.MoveToFront(elem)
		g := elem.Value.(*tiledCacheEntry).g
		tc.mu.Unlock()
		tc.hits.Add(1)
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

	tc.misses.Add(1)

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
		tc.evictions.Add(1)
	}

	return g
}

var tileEstimateLegacy = os.Getenv("TILE_ESTIMATE_LEGACY") == "true"

// estimateTileBytes returns the heap memory footprint of a loaded tile graph.
// mmap sections (EdgeGeomLat/Lon, EdgeElevOff, EdgeElevProf, EdgeOSMIdx) are
// excluded because they are zero-copy mapped from disk and do not consume heap.
// Set TILE_ESTIMATE_LEGACY=true to revert to the old formula (overestimates ~2.27x).
func estimateTileBytes(g *graph.Graph) int64 {
	if g == nil {
		return 0
	}
	if tileEstimateLegacy {
		// Old formula: conservative, overestimates ~2.27x but safe as upper bound.
		edgeBytes := int64(g.EdgeCount) * 48
		nodeBytes := int64(g.NodeCount) * 13
		geomBytes := int64(len(g.EdgeGeomLat)) * 8
		elevBytes := int64(len(g.EdgeElevProf)) * 4
		segDistBytes := int64(len(g.EdgeElevSegDist)) * 2
		return edgeBytes + nodeBytes + geomBytes + elevBytes + segDistBytes
	}
	// Only heap sections:
	// CSR: Offsets (NodeCount+1)*4, Targets EdgeCount*4, EdgeDist EdgeCount*4,
	//      EdgeTags1 EdgeCount*8, EdgeTags2 EdgeCount*8, EdgeGeomOff EdgeCount*4 (not mmap)
	csr := int64(g.NodeCount+1)*4 + int64(g.EdgeCount)*(4+4+8+8) + int64(g.EdgeCount+1)*4
	// Nodes: NodeLat*4 + NodeLon*4 + NodeEle*4 + NodeTags*1 + IsBorder*1
	nodes := int64(g.NodeCount) * (4 + 4 + 4 + 1 + 1)
	segDist := int64(len(g.EdgeElevSegDist)) * 2
	osmPool := int64(len(g.OSMIDPool)) * 8
	var border int64
	if g.BorderIndex != nil {
		border = int64(len(g.BorderIndex.Entries)) * 12
	}
	var grid int64
	if g.Grid != nil {
		grid = int64(len(g.Grid.CellStart))*4 + int64(len(g.Grid.NodeOrder))*4
	}
	// Reverse adjacency is lazy-built (EnsureReverseAdj via ForEachInEdge).
	// Only count it if it has actually been built for this tile.
	var revAdj int64
	if g.RevOffsets != nil {
		revAdj = int64(len(g.RevOffsets))*4 + int64(len(g.RevTargets))*4 + int64(len(g.RevEdgeIdx))*4
	}
	return csr + nodes + segDist + osmPool + border + grid + revAdj
}

func tileName(tileID uint32) string {
	lat := int(tileID/360) - 90
	lon := int(tileID%360) - 180
	return "tile_" + strconv.Itoa(lat) + "_" + strconv.Itoa(lon) + ".bin"
}
