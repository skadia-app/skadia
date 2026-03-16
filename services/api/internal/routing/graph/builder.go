package graph

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

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

// Build constructs the complete graph from PostGIS data.
// Uses cursor-based streaming to avoid loading all ways into memory at once.
func Build(ctx context.Context, pool *pgxpool.Pool, outputPath string,
	progressFn func(step string, pct int)) (*Graph, error) {

	report := func(step string, pct int) {
		slog.Info("graph build", "step", step, "pct", pct)
		if progressFn != nil {
			progressFn(step, pct)
		}
	}

	var totalWays int64
	if err := pool.QueryRow(ctx, "SELECT COUNT(*) FROM osm_ways WHERE highway IS NOT NULL AND highway != 'construction' AND (tags->>'area') IS DISTINCT FROM 'yes'").Scan(&totalWays); err != nil {
		return nil, fmt.Errorf("count ways: %w", err)
	}
	slog.Info("ways to load", "count", totalWays)

	report("load_topology", 0)
	topo, err := StreamTopology(ctx, pool, totalWays, func(loaded int64) {
		if totalWays > 0 {
			report("load_topology", int(loaded*100/totalWays))
		}
	})
	if err != nil {
		return nil, fmt.Errorf("stream topology: %w", err)
	}
	slog.Info("topology built", "nodes", topo.NodeCount, "edges", len(topo.Edges))
	report("load_topology", 100)

	report("build_csr", 0)
	g := BuildCSRCompact(topo)
	report("build_csr", 100)

	report("elevation_nodes", 0)
	// Detect bridge/tunnel from encoded tags — O(E) scan
	isBridge := make([]bool, g.NodeCount)
	for ni := uint32(0); ni < g.NodeCount; ni++ {
		for ei := g.Offsets[ni]; ei < g.Offsets[ni+1]; ei++ {
			if DecodeBool(g.EdgeTags1[ei], 38) || DecodeBool(g.EdgeTags1[ei], 39) {
				isBridge[ni] = true
				isBridge[g.Targets[ei]] = true
			}
		}
	}
	elevations, err := SampleElevation(ctx, pool, g.NodeLat, g.NodeLon, isBridge,
		func(done, total int) {
			report("elevation_nodes", done*100/total)
		})
	if err != nil {
		return nil, fmt.Errorf("sample elevation: %w", err)
	}
	copy(g.NodeEle, elevations)
	report("elevation_nodes", 100)

	report("elevation_edges", 0)
	if err := SampleEdgeElevationProfiles(ctx, pool, g); err != nil {
		slog.Warn("edge elevation profiles failed (non-fatal)", "error", err)
	}
	report("elevation_edges", 100)

	*topo = CompactTopologyResult{} // free topology memory

	report("prune_wcc", 0)
	beforeNodes := g.NodeCount
	g = PruneWCC(g, 100)
	slog.Info("WCC pruned", "before", beforeNodes, "after", g.NodeCount,
		"removed", beforeNodes-g.NodeCount)
	report("prune_wcc", 100)

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

// waysQuery is the SQL for loading routing-eligible ways from PostGIS.
const waysQuery = `
	SELECT DISTINCT ON (osm_id)
		osm_id,
		ST_AsText(geom) AS geom_wkt,
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
		COALESCE(sidewalk, '')
	FROM osm_ways
	WHERE highway IS NOT NULL
	  AND highway != 'construction'
	  AND (tags->>'area') IS DISTINCT FROM 'yes'
	ORDER BY osm_id, way_id DESC`

// StreamTopology streams ways from PostGIS in two passes and builds a compact
// topology with transfer-node elimination (TNE).
//
// Pass 1: geometry-only query to count vertex degrees and identify network nodes
// (endpoints, intersections). Pass 2: full query with tags, immediately encoded
// to uint64 — no string copies in CompactEdge (36 bytes vs ~300 bytes).
// Intermediate vertices become edge geometry instead of graph nodes.
func StreamTopology(ctx context.Context, pool *pgxpool.Pool, totalWays int64,
	progressFn func(loaded int64)) (*CompactTopologyResult, error) {

	// === Pass 1: Degree counting (geometry only, no tags) ===
	slog.Info("stream topology: pass 1 — counting vertex degrees")
	vd := NewVertexDegree()

	rows1, err := pool.Query(ctx, `
		SELECT DISTINCT ON (osm_id) ST_AsText(geom)
		FROM osm_ways
		WHERE highway IS NOT NULL AND highway != 'construction'
		  AND (tags->>'area') IS DISTINCT FROM 'yes'
		ORDER BY osm_id, way_id DESC`)
	if err != nil {
		return nil, fmt.Errorf("pass 1 query: %w", err)
	}
	var pass1Count int64
	for rows1.Next() {
		var geomWKT string
		if err := rows1.Scan(&geomWKT); err != nil {
			rows1.Close()
			return nil, fmt.Errorf("pass 1 scan: %w", err)
		}
		coords := parseLineStringWKT(geomWKT)
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

	// Compact: merge endpoints + shared vertices into single set, free large maps.
	// This reclaims ~4-5 GB of Go map overhead before Pass 2 allocates its own maps.
	vd.Compact()
	slog.Info("vertex degree compacted", "network_nodes", len(vd.networkNodes))

	// === Pass 2: Build compact edges with immediate tag encoding ===
	slog.Info("stream topology: pass 2 — building edges with TNE")

	nodeIndex := make(map[CoordKey]uint32, len(vd.endpoints))
	var nodes []Coord
	var nodeTags []uint8
	var edges []CompactEdge
	var edgeGeomOffset []uint32
	var edgeGeomCoords []Coord

	assignNode := func(c Coord) uint32 {
		key := NewCoordKey(c.Lat, c.Lon)
		if id, ok := nodeIndex[key]; ok {
			return id
		}
		id := uint32(len(nodes))
		nodeIndex[key] = id
		nodes = append(nodes, c)
		nodeTags = append(nodeTags, BarrierNone)
		return id
	}

	rows2, err := pool.Query(ctx, waysQuery)
	if err != nil {
		return nil, fmt.Errorf("pass 2 query: %w", err)
	}

	var pass2Count int64
	for rows2.Next() {
		var osmID int64
		var geomWKT string
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
		var mtbScaleUphill, viaFerrataScale, trailVisibility string
		var width, incline, sidewalk string

		err := rows2.Scan(
			&osmID, &geomWKT,
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
		enrichment := RawEnrichment{
			ForestPct: forestPct, NoiseScore: noiseScore,
			TrafficClass: trafficClass, ProtectedArea: protectedArea,
			IsCycleroute: isCycleroute, IsHikeroute: isHikeroute,
			CyclerouteNetwork: cyclerouteNetwork, HikerouteNetwork: hikerouteNetwork,
			LanduseClass: landuseClass,
		}
		tags1 := EncodeOSMTags(&tags)
		tags2 := EncodeEdgeTags2(&tags, &enrichment)

		coords := parseLineStringWKT(geomWKT)
		if len(coords) < 2 {
			pass2Count++
			continue
		}

		isOneway := oneway

		// Assign network-node IDs (only for endpoints + intersections).
		for _, c := range coords {
			key := NewCoordKey(c.Lat, c.Lon)
			if vd.IsNetworkNode(key) {
				assignNode(c)
			}
		}

		// Generate edges between consecutive network nodes.
		var geomBuf []Coord
		lastNetIdx := 0

		for i := 1; i < len(coords); i++ {
			key := NewCoordKey(coords[i].Lat, coords[i].Lon)
			if !vd.IsNetworkNode(key) {
				geomBuf = append(geomBuf, coords[i])
				continue
			}

			fromKey := NewCoordKey(coords[lastNetIdx].Lat, coords[lastNetIdx].Lon)
			toKey := key
			fromID := nodeIndex[fromKey]
			toID := nodeIndex[toKey]

			if fromID != toID {
				dist := edgeDistanceCm(coords[lastNetIdx : i+1])

				fullGeom := make([]Coord, 0, len(geomBuf)+2)
				fullGeom = append(fullGeom, coords[lastNetIdx])
				fullGeom = append(fullGeom, geomBuf...)
				fullGeom = append(fullGeom, coords[i])

				geomStart := uint32(len(edgeGeomCoords))
				edgeGeomCoords = append(edgeGeomCoords, fullGeom...)

				edgeGeomOffset = append(edgeGeomOffset, geomStart)
				edges = append(edges, CompactEdge{
					From: fromID, To: toID, OSMID: osmID,
					Tags1: tags1, Tags2: tags2, Dist: dist,
				})

				if !isOneway {
					revGeomStart := uint32(len(edgeGeomCoords))
					for j := len(fullGeom) - 1; j >= 0; j-- {
						edgeGeomCoords = append(edgeGeomCoords, fullGeom[j])
					}
					edgeGeomOffset = append(edgeGeomOffset, revGeomStart)
					edges = append(edges, CompactEdge{
						From: toID, To: fromID, OSMID: osmID,
						Tags1: tags1, Tags2: tags2, Dist: dist,
					})
				}
			} else if fromID == toID && len(geomBuf) > 0 {
				// Circular way: force midpoint as network node.
				midIdx := lastNetIdx + (i-lastNetIdx)/2
				midCoord := coords[midIdx]
				midKey := NewCoordKey(midCoord.Lat, midCoord.Lon)
				if _, ok := nodeIndex[midKey]; !ok {
					assignNode(midCoord)
				}
				midID := nodeIndex[midKey]

				// First half: [lastNetIdx..midIdx]
				dist1 := edgeDistanceCm(coords[lastNetIdx : midIdx+1])
				geom1 := make([]Coord, 0, midIdx-lastNetIdx+1)
				geom1 = append(geom1, coords[lastNetIdx])
				for k := lastNetIdx + 1; k < midIdx; k++ {
					geom1 = append(geom1, coords[k])
				}
				geom1 = append(geom1, coords[midIdx])
				gs1 := uint32(len(edgeGeomCoords))
				edgeGeomCoords = append(edgeGeomCoords, geom1...)
				edgeGeomOffset = append(edgeGeomOffset, gs1)
				edges = append(edges, CompactEdge{
					From: fromID, To: midID, OSMID: osmID,
					Tags1: tags1, Tags2: tags2, Dist: dist1,
				})
				if !isOneway {
					rgs1 := uint32(len(edgeGeomCoords))
					for j := len(geom1) - 1; j >= 0; j-- {
						edgeGeomCoords = append(edgeGeomCoords, geom1[j])
					}
					edgeGeomOffset = append(edgeGeomOffset, rgs1)
					edges = append(edges, CompactEdge{
						From: midID, To: fromID, OSMID: osmID,
						Tags1: tags1, Tags2: tags2, Dist: dist1,
					})
				}

				// Second half: [midIdx..i]
				dist2 := edgeDistanceCm(coords[midIdx : i+1])
				geom2 := make([]Coord, 0, i-midIdx+1)
				geom2 = append(geom2, coords[midIdx])
				for k := midIdx + 1; k < i; k++ {
					geom2 = append(geom2, coords[k])
				}
				geom2 = append(geom2, coords[i])
				gs2 := uint32(len(edgeGeomCoords))
				edgeGeomCoords = append(edgeGeomCoords, geom2...)
				edgeGeomOffset = append(edgeGeomOffset, gs2)
				edges = append(edges, CompactEdge{
					From: midID, To: toID, OSMID: osmID,
					Tags1: tags1, Tags2: tags2, Dist: dist2,
				})
				if !isOneway {
					rgs2 := uint32(len(edgeGeomCoords))
					for j := len(geom2) - 1; j >= 0; j-- {
						edgeGeomCoords = append(edgeGeomCoords, geom2[j])
					}
					edgeGeomOffset = append(edgeGeomOffset, rgs2)
					edges = append(edges, CompactEdge{
						From: toID, To: midID, OSMID: osmID,
						Tags1: tags1, Tags2: tags2, Dist: dist2,
					})
				}
			}

			geomBuf = geomBuf[:0]
			lastNetIdx = i
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

	// Final sentinel for EdgeGeomOffset.
	edgeGeomOffset = append(edgeGeomOffset, uint32(len(edgeGeomCoords)))

	slog.Info("stream topology complete",
		"ways", pass2Count,
		"network_nodes", len(nodes),
		"edges", len(edges),
		"geom_points", len(edgeGeomCoords))

	return &CompactTopologyResult{
		Nodes:          nodes,
		Edges:          edges,
		NodeIndex:      nodeIndex,
		NodeTags:       nodeTags,
		NodeCount:      len(nodes),
		EdgeGeomOffset: edgeGeomOffset,
		EdgeGeomCoords: edgeGeomCoords,
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
		var geomWKT string
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
		var mtbScaleUphill, viaFerrataScale, trailVisibility string
		var width, incline, sidewalk string

		err := rows.Scan(
			&w.OSMID, &geomWKT,
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
			LanduseClass: landuseClass,
		}
		w.Coords = parseLineStringWKT(geomWKT)
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
		total += haversineDistM(coords[i-1].Lat, coords[i-1].Lon, coords[i].Lat, coords[i].Lon)
	}
	cm := uint32(total * 100)
	if cm == 0 && total > 0 {
		cm = 1
	}
	return cm
}

// haversineDistM returns the haversine distance between two points in meters.
func haversineDistM(lat1, lon1, lat2, lon2 float64) float64 {
	const R = 6_371_000.0
	dLat := (lat2 - lat1) * math.Pi / 180
	dLon := (lon2 - lon1) * math.Pi / 180
	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(lat1*math.Pi/180)*math.Cos(lat2*math.Pi/180)*
			math.Sin(dLon/2)*math.Sin(dLon/2)
	return R * 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
}

func parseLineStringWKT(wkt string) []Coord {
	const prefix = "LINESTRING("
	if len(wkt) < len(prefix)+1 || wkt[:len(prefix)] != prefix {
		return nil
	}
	inner := wkt[len(prefix) : len(wkt)-1]

	var coords []Coord
	for _, pair := range splitComma(inner) {
		parts := splitSpace(pair)
		if len(parts) != 2 {
			continue
		}
		lon := parseFloat(parts[0])
		lat := parseFloat(parts[1])
		coords = append(coords, Coord{Lat: lat, Lon: lon})
	}
	return coords
}

func splitComma(s string) []string {
	var parts []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == ',' {
			parts = append(parts, s[start:i])
			start = i + 1
		}
	}
	parts = append(parts, s[start:])
	return parts
}

func splitSpace(s string) []string {
	var parts []string
	start := -1
	for i := 0; i < len(s); i++ {
		if s[i] == ' ' {
			if start >= 0 {
				parts = append(parts, s[start:i])
				start = -1
			}
		} else if start < 0 {
			start = i
		}
	}
	if start >= 0 {
		parts = append(parts, s[start:])
	}
	return parts
}

func parseFloat(s string) float64 {
	v, _ := strconv.ParseFloat(s, 64)
	return v
}
