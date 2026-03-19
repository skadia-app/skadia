package graph

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/paulmach/orb"
	"github.com/paulmach/orb/encoding/wkb"

	"github.com/skadia-app/skadia/services/api/internal/dem"
	"github.com/skadia-app/skadia/services/api/internal/geo"
	"github.com/skadia-app/skadia/services/api/internal/routing/trafficformula"
)

// CoverageData holds enrichment values sampled from coverage grids.
// Mirrors coverage.CoverageResult but defined locally to avoid import cycles
// (coverage → graph → coverage).
type CoverageData struct {
	ForestPct      int // 0-100
	ProtectedArea  int // 0-3
	LanduseClass   int // 0-8
	NoiseScore     int // 0-100
	Urbanization   int // 0-255
	RiverProximity int // 0-255
	Water          bool

	// Traffic raw inputs for BRouter division formula (hwWeight=1.0 assumed).
	// Builder applies hwWeight based on highway type.
	TrafficPopulate   float64
	TrafficIndustrial float64
	TrafficMWDensity  float64
	TrafficHWDensity  float64
}

// CoverageReader provides coverage grid access for the graph builder.
// Defined locally to avoid import cycles (coverage package imports graph).
// Callers wrap coverage.TileGridCache with an adapter that converts
// coverage.CoverageResult → graph.CoverageData.
type CoverageReader interface {
	// SampleWay samples all coverage layers along a way geometry.
	SampleWay(geom orb.LineString) CoverageData
	// PreloadTile builds/caches coverage grids for a tile.
	// The context is forwarded to cancellable DB queries inside the preload.
	PreloadTile(ctx context.Context, tileID uint32) error
}

// computeTrafficFromCoverage applies hwWeight per highway type and classifies traffic.
// Spec section 8.5: hwWeight modulates populate, not industrial.
// Uses trafficformula.Raw/Classify (shared package, avoids import cycle with coverage/).
func computeTrafficFromCoverage(cd CoverageData, highway string) int {
	var hwWeight float64
	switch {
	case strings.HasPrefix(highway, "primary"):
		hwWeight = 1.0
	case strings.HasPrefix(highway, "secondary"):
		hwWeight = 0.6
	case strings.HasPrefix(highway, "tertiary"):
		hwWeight = 0.4
	default:
		return 0 // no traffic class for non-primary/secondary/tertiary
	}
	raw := trafficformula.Raw(cd.TrafficPopulate*hwWeight, cd.TrafficIndustrial,
		cd.TrafficMWDensity, cd.TrafficHWDensity)
	return int(trafficformula.Classify(raw))
}

// BuildFromWays constructs a complete graph from raw ways and pre-computed elevations.
func BuildFromWays(ways []RawWay, elevations []int32) *Graph {
	topo := BuildTopology(ways)

	g := BuildCSR(topo)

	if len(elevations) >= int(g.NodeCount) {
		copy(g.NodeEle, elevations[:g.NodeCount])

		isBridge := make([]bool, g.NodeCount)
		for _, e := range topo.Edges {
			if e.Tags.Bridge || e.Tags.Tunnel {
				isBridge[e.From] = true
				isBridge[e.To] = true
			}
		}
		InterpolateMissing(g.NodeEle, isBridge)
	}

	g = PruneWCC(g, 100)

	g.Grid = BuildSpatialIndex(g, 0.01)

	g.BuildTime = time.Now()
	return g
}

// bboxWhereClause returns a PostGIS ST_Intersects filter for the given bounding
// box, or an empty string if bbox is nil. The bbox array is [minLat, minLon, maxLat, maxLon].
func bboxWhereClause(bbox *[4]float64) string {
	if bbox == nil {
		return ""
	}
	return fmt.Sprintf(
		" AND ST_Intersects(geom, ST_MakeEnvelope(%f, %f, %f, %f, 4326))",
		bbox[1], bbox[0], bbox[3], bbox[2],
	)
}

// Build constructs the complete graph from PostGIS data.
// Uses cursor-based streaming to avoid loading all ways into memory at once.
// Edges are written to a temp file by StreamTopology, then read back by
// BuildCSRFromFile in two passes (degree counting + CSR placement).
// If bbox is non-nil, only ways intersecting the bounding box are included.
// If coverageReader is non-nil, enrichment values are sampled from coverage
// grids instead of SQL columns (Phase 2, eliminates DB roundtrip).
func Build(ctx context.Context, pool *pgxpool.Pool, outputPath string,
	bbox *[4]float64, demReader dem.Reader,
	coverageReader CoverageReader,
	progressFn func(step string, pct int)) (*Graph, error) {

	report := func(step string, pct int) {
		slog.Info("graph build", "step", step, "pct", pct)
		if progressFn != nil {
			progressFn(step, pct)
		}
	}

	var totalWays int64
	countQuery := `SELECT COUNT(*) FROM osm_ways WHERE highway IS NOT NULL AND highway != 'construction' AND (tags->>'area') IS DISTINCT FROM 'yes'` + bboxWhereClause(bbox)
	if err := pool.QueryRow(ctx, countQuery).Scan(&totalWays); err != nil {
		return nil, fmt.Errorf("count ways: %w", err)
	}
	slog.Info("ways to load", "count", totalWays)

	// Determine temp file path — ensure directory exists
	var edgeTmpPath string
	if outputPath != "" {
		dir := filepath.Dir(outputPath)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("mkdir %s: %w", dir, err)
		}
		edgeTmpPath = filepath.Join(dir, "edges.tmp")
	} else {
		tmpFile, err := os.CreateTemp("", "badrouter-edges-*.tmp")
		if err != nil {
			return nil, fmt.Errorf("create temp: %w", err)
		}
		edgeTmpPath = tmpFile.Name()
		tmpFile.Close()
	}
	defer os.Remove(edgeTmpPath)

	report("load_topology", 0)
	topo, err := StreamTopology(ctx, pool, totalWays, edgeTmpPath, bbox, coverageReader, func(loaded int64) {
		if totalWays > 0 {
			report("load_topology", int(loaded*100/totalWays))
		}
	})
	if err != nil {
		return nil, fmt.Errorf("stream topology: %w", err)
	}
	slog.Info("topology built", "nodes", topo.NodeCount)
	report("load_topology", 100)

	// WCC pruning: DISABLED for outdoor routing.
	// Trails, via ferratas, summit paths and hut approaches are legitimately
	// disconnected from the main network (often just 2-5 nodes). Pruning them
	// removes exactly the destinations this router exists to reach.
	// "No route found" is the correct answer for genuinely unreachable points.
	// FilterWCCEdgeFile() is available if ever needed for road-cycling profiles.
	report("prune_wcc", 100)

	report("build_csr", 0)
	g, err := BuildCSRFromFile(topo.Nodes, topo.NodeTags, edgeTmpPath)
	if err != nil {
		return nil, fmt.Errorf("build CSR: %w", err)
	}
	*topo = CompactTopologyResult{} // free nodes memory
	report("build_csr", 100)

	// DEM elevation: sample from dem.Reader (HGT mmap, no PostGIS).
	if demReader != nil {
		report("elevation_nodes", 0)
		g.NodeEle = SampleElevationFromReader(demReader, g.NodeLat, g.NodeLon,
			func(done, total int) {
				if total > 0 {
					report("elevation_nodes", done*100/total)
				}
			})
		report("elevation_nodes", 100)

		report("elevation_edges", 0)
		SampleEdgeElevationProfilesFromReader(demReader, g)
		report("elevation_edges", 100)
	} else {
		slog.Info("no DEM reader provided, skipping elevation",
			"nodes", g.NodeCount)
		report("elevation_nodes", 100)
		report("elevation_edges", 100)
	}

	// K07: ElevOff = copy of GeomOff BEFORE DP changes GeomOff.
	// SampleEdgeElevationProfilesFromReader already copies GeomOff→ElevOff,
	// but this explicit copy ensures correctness even without DEM.
	g.EdgeElevOff = make([]uint32, len(g.EdgeGeomOff))
	copy(g.EdgeElevOff, g.EdgeGeomOff)

	// Compute ElevSegDist BEFORE DP (needs original coordinates).
	report("compute_elev_seg_dist", 0)
	centerLat := (g.BBox[0] + g.BBox[2]) / 2.0
	cosLat := math.Cos(centerLat * math.Pi / 180.0)

	var allElevSegDist []uint16
	for ei := range g.EdgeCount {
		start := g.EdgeGeomOff[ei]
		end := g.EdgeGeomOff[ei+1]
		segDists := ComputeElevSegDist(g.EdgeGeomLat[start:end], g.EdgeGeomLon[start:end], cosLat)
		allElevSegDist = append(allElevSegDist, segDists...)
	}
	g.EdgeElevSegDist = allElevSegDist
	report("compute_elev_seg_dist", 100)

	// Build assertion (Spec section 4.3): ElevSegDist count = ElevProf count - EdgeCount.
	if len(g.EdgeElevProf) > 0 {
		expectedLen := len(g.EdgeElevProf) - int(g.EdgeCount)
		if len(g.EdgeElevSegDist) != expectedLen {
			return nil, fmt.Errorf("ElevSegDist/ElevProf mismatch: %d != %d",
				len(g.EdgeElevSegDist), expectedLen)
		}
	}

	// Douglas-Peucker on EdgeGeom (2D, 0.4m threshold).
	report("douglas_peucker", 0)
	newGeomOff, newGeomLat, newGeomLon := ApplyDPToEdgeGeometry(
		g.EdgeCount, g.EdgeGeomOff, g.EdgeGeomLat, g.EdgeGeomLon,
		cosLat, 0.4)

	savedGeomPts := len(g.EdgeGeomLat)
	g.EdgeGeomOff = newGeomOff
	g.EdgeGeomLat = newGeomLat
	g.EdgeGeomLon = newGeomLon

	slog.Info("douglas-peucker complete",
		"before", savedGeomPts, "after", len(newGeomLat),
		"reduction_pct", 100-len(newGeomLat)*100/max(1, savedGeomPts))
	report("douglas_peucker", 100)

	report("spatial_index", 0)
	g.Grid = BuildSpatialIndex(g, 0.01)
	report("spatial_index", 100)

	g.BuildTime = time.Now()

	if outputPath != "" {
		report("serialize", 0)
		if err := Serialize(g, outputPath); err != nil {
			return nil, fmt.Errorf("serialize: %w", err)
		}
		report("serialize", 100)
	}

	return g, nil
}

// waysQueryBase is the SQL for loading routing-eligible ways from PostGIS,
// without the ORDER BY clause — allows injecting additional WHERE filters.
const waysQueryBase = `
	SELECT DISTINCT ON (osm_id)
		osm_id,
		ST_AsBinary(geom) AS geom_wkb,
		highway,
		COALESCE(surface, ''), COALESCE(tracktype, ''), COALESCE(smoothness, ''),
		COALESCE(sac_scale, ''), COALESCE(mtb_scale, ''),
		COALESCE(access, ''), COALESCE(foot, ''), COALESCE(bicycle, ''), COALESCE(horse, ''),
		COALESCE(cycleway, ''), COALESCE(cycleway_left, ''), COALESCE(cycleway_right, ''), COALESCE(cycleway_both, ''),
		COALESCE(maxspeed, ''), COALESCE(lanes, 0),
		COALESCE(oneway, false), COALESCE(oneway_bicycle, false),
		COALESCE(bridge, false), COALESCE(tunnel, false),
		COALESCE(ford, false), COALESCE(lit, false),
		COALESCE(segregated, false), COALESCE(bicycle_road, false),
		COALESCE(forest_pct, 0),
		COALESCE(noise_score, 0),
		COALESCE(traffic_class, 0),
		COALESCE(protected_area, 0),
		COALESCE(is_cycleroute, false),
		COALESCE(is_hikeroute, false),
		COALESCE(cycleroute_network, ''),
		COALESCE(hikeroute_network, ''),
		COALESCE(landuse_class, 0),
		COALESCE(mtb_scale_uphill, ''),
		COALESCE(via_ferrata_scale, ''),
		COALESCE(trail_visibility, ''),
		COALESCE(width, ''),
		COALESCE(incline, ''),
		COALESCE(sidewalk, ''),
		COALESCE(urbanization, 0),
		COALESCE(river_proximity, 0)
	FROM osm_ways
	WHERE highway IS NOT NULL
	  AND highway != 'construction'
	  AND (tags->>'area') IS DISTINCT FROM 'yes'`

// waysQuery is the full SQL including ORDER BY (used by LoadWays).
const waysQuery = waysQueryBase + `
	ORDER BY osm_id, way_id DESC`

// StreamTopology streams ways from PostGIS in two passes and builds a compact
// topology with transfer-node elimination (TNE).
//
// Pass 1: geometry-only query to count vertex degrees and identify network nodes
// (endpoints, intersections). Pass 2: full query with tags, immediately encoded
// to uint64 — no string copies in CompactEdge (36 bytes vs ~300 bytes).
// Intermediate vertices become edge geometry instead of graph nodes.
// If coverageReader is non-nil, enrichment values are sampled from grids
// instead of SQL columns (Phase 2 CoverageReader integration).
func StreamTopology(ctx context.Context, pool *pgxpool.Pool, totalWays int64,
	edgeTmpPath string, bbox *[4]float64,
	coverageReader CoverageReader,
	progressFn func(loaded int64)) (*CompactTopologyResult, error) {

	// === Pass 1: Degree counting (geometry only, no tags) ===
	slog.Info("stream topology: pass 1 — counting vertex degrees")
	vd := NewVertexDegree()

	pass1Query := `
		SELECT DISTINCT ON (osm_id) ST_AsBinary(geom)
		FROM osm_ways
		WHERE highway IS NOT NULL AND highway != 'construction'
		  AND (tags->>'area') IS DISTINCT FROM 'yes'` + bboxWhereClause(bbox) + `
		ORDER BY osm_id, way_id DESC`
	rows1, err := pool.Query(ctx, pass1Query)
	if err != nil {
		return nil, fmt.Errorf("pass 1 query: %w", err)
	}
	var pass1Count int64
	for rows1.Next() {
		var geomWKB []byte
		if err := rows1.Scan(&geomWKB); err != nil {
			rows1.Close()
			return nil, fmt.Errorf("pass 1 scan: %w", err)
		}
		geomRaw, err := wkb.Unmarshal(geomWKB)
		if err != nil {
			pass1Count++
			continue
		}
		ls, ok := geomRaw.(orb.LineString)
		if !ok {
			pass1Count++
			continue
		}
		coords := make([]Coord, len(ls))
		for i, pt := range ls {
			coords[i] = Coord{Lat: pt[1], Lon: pt[0]}
		}
		if len(coords) >= 2 {
			vd.AddWay(coords)
		}
		pass1Count++
		if pass1Count%500_000 == 0 {
			slog.Info("pass 1 progress", "ways", pass1Count)
		}
	}
	rows1.Close()
	if err := rows1.Err(); err != nil {
		return nil, fmt.Errorf("pass 1 rows: %w", err)
	}
	slog.Info("pass 1 complete", "ways", pass1Count,
		"endpoints", len(vd.endpoints), "interior_shared", len(vd.counts))

	// Compact: convert to sorted slice (~200 MB), free large maps (~7 GB).
	vd.Compact()
	runtime.GC()
	debug.FreeOSMemory()
	slog.Info("vertex degree compacted", "network_nodes", vd.NetworkNodeCount())

	// === Pass 2: Build compact edges with immediate tag encoding ===
	slog.Info("stream topology: pass 2 — building edges with TNE")

	// Pre-build nodes from sorted network node keys (no map needed — saves ~2.5 GB).
	// Node ID = index in sortedNodes (assigned by Compact/sort order).
	numNodes := vd.NetworkNodeCount()
	nodes := make([]Coord, numNodes)
	for i, key := range vd.sortedNodes {
		nodes[i] = Coord{
			Lat: float64(key.Lat) / 1e7,
			Lon: float64(key.Lon) / 1e7,
		}
	}
	nodeTags := make([]uint8, numNodes)

	ew, err := NewEdgeFileWriter(edgeTmpPath)
	if err != nil {
		return nil, fmt.Errorf("create edge writer: %w", err)
	}
	defer func() {
		if ew != nil {
			ew.CloseWithNodeCount(uint32(len(nodes)))
		}
	}()

	// Small overflow map for circular-way midpoints (very few entries).
	overflowNodes := make(map[CoordKey]uint32)
	assignOverflow := func(c Coord) uint32 {
		key := NewCoordKey(c.Lat, c.Lon)
		if id, ok := overflowNodes[key]; ok {
			return id
		}
		id := uint32(len(nodes))
		overflowNodes[key] = id
		nodes = append(nodes, c)
		nodeTags = append(nodeTags, BarrierNone)
		return id
	}

	// lookupNode returns the node ID for a coordinate.
	// First checks the sorted network nodes (binary search), then overflow.
	lookupNode := func(key CoordKey) (uint32, bool) {
		if id, ok := vd.SearchNodeID(key); ok {
			return id, true
		}
		if id, ok := overflowNodes[key]; ok {
			return id, true
		}
		return 0, false
	}

	pass2Query := waysQueryBase + bboxWhereClause(bbox) + `
	ORDER BY osm_id, way_id DESC`
	rows2, err := pool.Query(ctx, pass2Query)
	if err != nil {
		return nil, fmt.Errorf("pass 2 query: %w", err)
	}

	var pass2Count int64
	for rows2.Next() {
		var osmID int64
		var geomWKB []byte
		var highway, surface, tracktype, smoothness string
		var sacScale, mtbScale string
		var access, foot, bicycle, horse string
		var cw, cwLeft, cwRight, cwBoth string
		var maxspeed string
		var lanes int
		var oneway, onewayBicycle, bridge, tunnel, ford, lit bool
		var segregated, bicycleRoad bool
		var forestPct, noiseScore, trafficClass, protectedArea int
		var isCycleroute, isHikeroute bool
		var cyclerouteNetwork, hikerouteNetwork string
		var landuseClass int
		var urbanization, riverProximity int
		var mtbScaleUphill, viaFerrataScale, trailVisibility string
		var width, incline, sidewalk string

		err := rows2.Scan(
			&osmID, &geomWKB,
			&highway, &surface, &tracktype, &smoothness,
			&sacScale, &mtbScale,
			&access, &foot, &bicycle, &horse,
			&cw, &cwLeft, &cwRight, &cwBoth,
			&maxspeed, &lanes,
			&oneway, &onewayBicycle, &bridge, &tunnel, &ford, &lit,
			&segregated, &bicycleRoad,
			&forestPct, &noiseScore, &trafficClass, &protectedArea,
			&isCycleroute, &isHikeroute,
			&cyclerouteNetwork, &hikerouteNetwork,
			&landuseClass,
			&mtbScaleUphill, &viaFerrataScale, &trailVisibility,
			&width, &incline, &sidewalk,
			&urbanization, &riverProximity,
		)
		if err != nil {
			rows2.Close()
			return nil, fmt.Errorf("pass 2 scan: %w", err)
		}

		// IMMEDIATELY encode tags — no string copies in edges.
		tags := RawTags{
			Highway: highway, Surface: surface, Tracktype: tracktype,
			Smoothness: smoothness, SacScale: sacScale, MtbScale: mtbScale,
			MtbScaleUphill: mtbScaleUphill, ViaFerrataScale: viaFerrataScale,
			TrailVisibility: trailVisibility,
			Access: access, Foot: foot, Bicycle: bicycle, Horse: horse,
			Cycleway: cw, CyclewayLeft: cwLeft, CyclewayRight: cwRight,
			CyclewayBoth: cwBoth, Maxspeed: maxspeed,
			Width: width, Incline: incline, Sidewalk: sidewalk,
			Lanes: lanes,
			Oneway: oneway, OnewayBicycle: onewayBicycle,
			Bridge: bridge, Tunnel: tunnel, Ford: ford, Lit: lit,
			Segregated: segregated, BicycleRoad: bicycleRoad,
		}
		var coords []Coord
		if geomRaw2, err2 := wkb.Unmarshal(geomWKB); err2 == nil {
			if ls2, ok2 := geomRaw2.(orb.LineString); ok2 {
				coords = make([]Coord, len(ls2))
				for i, pt := range ls2 {
					coords[i] = Coord{Lat: pt[1], Lon: pt[0]}
				}
			}
		}
		if len(coords) < 2 {
			pass2Count++
			continue
		}

		enrichment := RawEnrichment{
			ForestPct: forestPct, NoiseScore: noiseScore,
			TrafficClass: trafficClass, ProtectedArea: protectedArea,
			IsCycleroute: isCycleroute, IsHikeroute: isHikeroute,
			CyclerouteNetwork: cyclerouteNetwork, HikerouteNetwork: hikerouteNetwork,
			LanduseClass:   landuseClass,
			Urbanization:   urbanization,
			RiverProximity: riverProximity,
		}

		// Phase 2: override enrichment from coverage grids (no DB roundtrip).
		// Route-membership (cycleroute/hikeroute) still comes from SQL.
		if coverageReader != nil {
			ls := geo.LineStringFromLatLng(coords, func(c Coord) (lat, lng float64) {
				return c.Lat, c.Lon
			})
			cd := coverageReader.SampleWay(ls)
			enrichment.ForestPct = cd.ForestPct
			enrichment.NoiseScore = cd.NoiseScore
			enrichment.ProtectedArea = cd.ProtectedArea
			enrichment.LanduseClass = cd.LanduseClass
			enrichment.Urbanization = cd.Urbanization
			enrichment.RiverProximity = cd.RiverProximity
			enrichment.TrafficClass = computeTrafficFromCoverage(cd, highway)
		}

		tags1 := EncodeOSMTags(&tags)
		tags2 := EncodeEdgeTags2(&tags, &enrichment)

		isOneway := oneway

		// Generate edges between consecutive network nodes.
		// Node IDs are looked up via binary search (no map needed).
		var waySegments []WaySegment
		var geomBuf []Coord
		lastNetIdx := 0

		for i := 1; i < len(coords); i++ {
			key := NewCoordKey(coords[i].Lat, coords[i].Lon)
			if !vd.IsNetworkNode(key) {
				geomBuf = append(geomBuf, coords[i])
				continue
			}

			fromKey := NewCoordKey(coords[lastNetIdx].Lat, coords[lastNetIdx].Lon)
			fromID, _ := lookupNode(fromKey)
			toID, _ := lookupNode(key)

			if fromID != toID {
				dist := edgeDistanceCm(coords[lastNetIdx : i+1])
				seg := WaySegment{FromID: fromID, ToID: toID, Dist: dist}
				for k := lastNetIdx; k <= i; k++ {
					seg.GeomLat = append(seg.GeomLat, int32(math.Round(coords[k].Lat*1e7)))
					seg.GeomLon = append(seg.GeomLon, int32(math.Round(coords[k].Lon*1e7)))
				}
				waySegments = append(waySegments, seg)
			} else if fromID == toID && len(geomBuf) > 0 {
				// Circular way: force midpoint as network node.
				midIdx := lastNetIdx + (i-lastNetIdx)/2
				midCoord := coords[midIdx]
				midKey := NewCoordKey(midCoord.Lat, midCoord.Lon)
				midID, ok := lookupNode(midKey)
				if !ok {
					midID = assignOverflow(midCoord)
				}

				// First half
				seg1 := WaySegment{FromID: fromID, ToID: midID, Dist: edgeDistanceCm(coords[lastNetIdx : midIdx+1])}
				for k := lastNetIdx; k <= midIdx; k++ {
					seg1.GeomLat = append(seg1.GeomLat, int32(math.Round(coords[k].Lat*1e7)))
					seg1.GeomLon = append(seg1.GeomLon, int32(math.Round(coords[k].Lon*1e7)))
				}
				waySegments = append(waySegments, seg1)

				// Second half
				seg2 := WaySegment{FromID: midID, ToID: toID, Dist: edgeDistanceCm(coords[midIdx : i+1])}
				for k := midIdx; k <= i; k++ {
					seg2.GeomLat = append(seg2.GeomLat, int32(math.Round(coords[k].Lat*1e7)))
					seg2.GeomLon = append(seg2.GeomLon, int32(math.Round(coords[k].Lon*1e7)))
				}
				waySegments = append(waySegments, seg2)
			}

			geomBuf = geomBuf[:0]
			lastNetIdx = i
		}

		if len(waySegments) > 0 {
			if err := ew.WriteWay(osmID, tags1, tags2, isOneway, waySegments); err != nil {
				rows2.Close()
				return nil, fmt.Errorf("write way: %w", err)
			}
			waySegments = waySegments[:0]
		}

		pass2Count++
		if pass2Count%500_000 == 0 && progressFn != nil {
			progressFn(pass2Count)
		}
	}
	rows2.Close()
	if err := rows2.Err(); err != nil {
		return nil, fmt.Errorf("pass 2 rows: %w", err)
	}

	if err := ew.CloseWithNodeCount(uint32(len(nodes))); err != nil {
		return nil, fmt.Errorf("close edge writer: %w", err)
	}
	ew = nil // prevent defer double-close

	slog.Info("stream topology complete",
		"ways", pass2Count,
		"network_nodes", len(nodes),
		"edge_file", edgeTmpPath,
		"overflow_nodes", len(overflowNodes))

	return &CompactTopologyResult{
		Nodes:     nodes,
		NodeTags:  nodeTags,
		NodeCount: len(nodes),
	}, nil
}

// LoadWays queries osm_ways from PostGIS and returns RawWays.
// Deprecated: Use StreamTopology for large datasets. This function loads
// everything into memory and is only kept for tests with small synthetic data.
func LoadWays(ctx context.Context, pool *pgxpool.Pool) ([]RawWay, error) {
	rows, err := pool.Query(ctx, waysQuery)
	if err != nil {
		return nil, fmt.Errorf("query osm_ways: %w", err)
	}
	defer rows.Close()

	var ways []RawWay
	for rows.Next() {
		var w RawWay
		var geomWKB []byte
		var highway, surface, tracktype, smoothness string
		var sacScale, mtbScale string
		var access, foot, bicycle, horse string
		var cw, cwLeft, cwRight, cwBoth string
		var maxspeed string
		var lanes int
		var oneway, onewayBicycle, bridge, tunnel, ford, lit bool
		var segregated, bicycleRoad bool
		var forestPct, noiseScore, trafficClass, protectedArea int
		var isCycleroute, isHikeroute bool
		var cyclerouteNetwork, hikerouteNetwork string
		var landuseClass int
		var urbanization, riverProximity int
		var mtbScaleUphill, viaFerrataScale, trailVisibility string
		var width, incline, sidewalk string

		err := rows.Scan(
			&w.OSMID, &geomWKB,
			&highway, &surface, &tracktype, &smoothness,
			&sacScale, &mtbScale,
			&access, &foot, &bicycle, &horse,
			&cw, &cwLeft, &cwRight, &cwBoth,
			&maxspeed, &lanes,
			&oneway, &onewayBicycle, &bridge, &tunnel, &ford, &lit,
			&segregated, &bicycleRoad,
			&forestPct, &noiseScore, &trafficClass, &protectedArea,
			&isCycleroute, &isHikeroute,
			&cyclerouteNetwork, &hikerouteNetwork,
			&landuseClass,
			&mtbScaleUphill, &viaFerrataScale, &trailVisibility,
			&width, &incline, &sidewalk,
			&urbanization, &riverProximity,
		)
		if err != nil {
			return nil, fmt.Errorf("scan way: %w", err)
		}

		w.Tags = RawTags{
			Highway: highway, Surface: surface, Tracktype: tracktype,
			Smoothness: smoothness, SacScale: sacScale, MtbScale: mtbScale,
			MtbScaleUphill: mtbScaleUphill, ViaFerrataScale: viaFerrataScale,
			TrailVisibility: trailVisibility,
			Access: access, Foot: foot, Bicycle: bicycle, Horse: horse,
			Cycleway: cw, CyclewayLeft: cwLeft, CyclewayRight: cwRight,
			CyclewayBoth: cwBoth, Maxspeed: maxspeed,
			Width: width, Incline: incline, Sidewalk: sidewalk,
			Lanes: lanes,
			Oneway: oneway, OnewayBicycle: onewayBicycle,
			Bridge: bridge, Tunnel: tunnel, Ford: ford, Lit: lit,
			Segregated: segregated, BicycleRoad: bicycleRoad,
		}
		w.Enrichment = RawEnrichment{
			ForestPct: forestPct, NoiseScore: noiseScore,
			TrafficClass: trafficClass, ProtectedArea: protectedArea,
			IsCycleroute: isCycleroute, IsHikeroute: isHikeroute,
			CyclerouteNetwork: cyclerouteNetwork, HikerouteNetwork: hikerouteNetwork,
			LanduseClass:   landuseClass,
			Urbanization:   urbanization,
			RiverProximity: riverProximity,
		}
		if geomRaw3, err3 := wkb.Unmarshal(geomWKB); err3 == nil {
			if ls3, ok3 := geomRaw3.(orb.LineString); ok3 {
				w.Coords = make([]Coord, len(ls3))
				for i, pt := range ls3 {
					w.Coords[i] = Coord{Lat: pt[1], Lon: pt[0]}
				}
			}
		}
		ways = append(ways, w)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows: %w", err)
	}

	return ways, nil
}

// edgeDistanceCm calculates total haversine distance along a coordinate
// sequence, returning the result in centimeters as uint32.
func edgeDistanceCm(coords []Coord) uint32 {
	var total float64
	for i := 1; i < len(coords); i++ {
		total += geo.DistanceHaversine(coords[i-1].Lat, coords[i-1].Lon, coords[i].Lat, coords[i].Lon)
	}
	cm := uint32(total * 100)
	if cm == 0 && total > 0 {
		cm = 1
	}
	return cm
}

