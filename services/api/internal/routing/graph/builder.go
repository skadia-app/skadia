package graph

import (
	"context"
	"fmt"
	"log/slog"
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

	// Get total count for progress reporting.
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

	report("elevation", 0)
	isBridge := make([]bool, topo.NodeCount)
	for _, e := range topo.Edges {
		if e.Tags.Bridge {
			isBridge[e.From] = true
			isBridge[e.To] = true
		}
		if e.Tags.Tunnel {
			isBridge[e.From] = true
			isBridge[e.To] = true
		}
	}
	lats := make([]int32, topo.NodeCount)
	lons := make([]int32, topo.NodeCount)
	for i, c := range topo.Nodes {
		lats[i] = NewCoordKey(c.Lat, c.Lon).Lat
		lons[i] = NewCoordKey(c.Lat, c.Lon).Lon
	}
	elevations, err := SampleElevation(ctx, pool, lats, lons, isBridge,
		func(done, total int) {
			report("elevation", done*100/total)
		})
	if err != nil {
		return nil, fmt.Errorf("sample elevation: %w", err)
	}
	report("elevation", 100)

	report("build_csr", 0)
	g := BuildCSR(*topo)
	copy(g.NodeEle, elevations)
	report("build_csr", 100)

	*topo = TopologyResult{} // release topology memory

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

// StreamTopology streams ways from PostGIS via pgx cursor and builds topology
// incrementally. Unlike LoadWays+BuildTopology, this never holds all RawWay
// objects in memory — each row is scanned, processed into topology edges, then
// discarded. Peak memory is dominated by the TopologyResult (nodes + edges),
// not the input data.
func StreamTopology(ctx context.Context, pool *pgxpool.Pool, totalWays int64,
	progressFn func(loaded int64)) (*TopologyResult, error) {

	rows, err := pool.Query(ctx, waysQuery)
	if err != nil {
		return nil, fmt.Errorf("query osm_ways: %w", err)
	}
	defer rows.Close()

	// Topology state — built incrementally.
	nodeIndex := make(map[CoordKey]uint32, int(totalWays)*4)
	var nodes []Coord
	var nodeTags []uint8
	var edges []TopologyEdge

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

	var loaded int64
	for rows.Next() {
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

		err := rows.Scan(
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
			return nil, fmt.Errorf("scan way: %w", err)
		}

		coords := parseLineStringWKT(geomWKT)
		if len(coords) < 2 {
			loaded++
			continue
		}

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

		// Assign node IDs for all coordinates.
		for _, c := range coords {
			assignNode(c)
		}

		// Generate directed edges.
		isOneway := tags.Oneway
		for j := 0; j < len(coords)-1; j++ {
			fromKey := NewCoordKey(coords[j].Lat, coords[j].Lon)
			toKey := NewCoordKey(coords[j+1].Lat, coords[j+1].Lon)
			fromID := nodeIndex[fromKey]
			toID := nodeIndex[toKey]
			if fromID == toID {
				continue
			}

			if isOneway {
				edges = append(edges, TopologyEdge{
					From: fromID, To: toID, OSMID: osmID,
					Tags: tags, Enrichment: enrichment,
				})
			} else {
				edges = append(edges, TopologyEdge{
					From: fromID, To: toID, OSMID: osmID,
					Tags: tags, Enrichment: enrichment,
				})
				edges = append(edges, TopologyEdge{
					From: toID, To: fromID, OSMID: osmID,
					Tags: tags, Enrichment: enrichment,
				})
			}
		}

		loaded++
		if loaded%500_000 == 0 && progressFn != nil {
			progressFn(loaded)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows: %w", err)
	}

	slog.Info("stream topology complete", "ways", loaded, "nodes", len(nodes), "edges", len(edges))

	return &TopologyResult{
		Nodes:     nodes,
		Edges:     edges,
		NodeIndex: nodeIndex,
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
