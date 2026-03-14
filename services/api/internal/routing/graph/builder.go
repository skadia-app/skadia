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
func Build(ctx context.Context, pool *pgxpool.Pool, outputPath string,
	progressFn func(step string, pct int)) (*Graph, error) {

	report := func(step string, pct int) {
		slog.Info("graph build", "step", step, "pct", pct)
		if progressFn != nil {
			progressFn(step, pct)
		}
	}

	report("load_ways", 0)
	ways, err := LoadWays(ctx, pool)
	if err != nil {
		return nil, fmt.Errorf("load ways: %w", err)
	}
	slog.Info("loaded ways", "count", len(ways))
	report("load_ways", 100)

	report("topology", 0)
	topo := BuildTopology(ways)
	slog.Info("topology built", "nodes", topo.NodeCount, "edges", len(topo.Edges))
	report("topology", 100)

	ways = nil

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
	g := BuildCSR(topo)
	copy(g.NodeEle, elevations)
	report("build_csr", 100)

	topo = TopologyResult{}

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

// LoadWays queries osm_ways from PostGIS and returns RawWays.
func LoadWays(ctx context.Context, pool *pgxpool.Pool) ([]RawWay, error) {
	rows, err := pool.Query(ctx, `
		SELECT
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
			COALESCE(landuse_class, 0)
		FROM osm_ways
		WHERE highway IS NOT NULL
		  AND highway != 'construction'
		  AND (tags->>'area') IS DISTINCT FROM 'yes'
		ORDER BY osm_id`)
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
		)
		if err != nil {
			return nil, fmt.Errorf("scan way: %w", err)
		}

		w.Tags = RawTags{
			Highway: highway, Surface: surface, Tracktype: tracktype,
			Smoothness: smoothness, SacScale: sacScale, MtbScale: mtbScale,
			Access: access, Foot: foot, Bicycle: bicycle, Horse: horse,
			Cycleway: cw, CyclewayLeft: cwLeft, CyclewayRight: cwRight,
			CyclewayBoth: cwBoth, Maxspeed: maxspeed, Lanes: lanes,
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
		if w.Coords == nil && geomWKT != "" {
			slog.Warn("skipped non-LINESTRING way geometry", "osm_id", w.OSMID,
				"geom_prefix", geomWKT[:min(30, len(geomWKT))])
		}
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
