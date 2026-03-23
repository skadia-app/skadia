package routing

import (
	"log/slog"
	"math"

	"github.com/skadia-app/skadia/services/api/internal/routing/graph"
)

// FindFlankLandmarks places 2 landmarks on opposite sides of a crossing.
//
// Algorithm:
//  1. Find the 2 deepest reachable nodes in different directions (valley detection)
//  2. Compute split bearing perpendicular to the valley axis
//  3. Find the farthest node in each half-plane (15-50km, >=300m lower)
//
// Parameters:
//   - crossingSeed: the crossing node (snapped to graph, with GlobalNodeID)
//   - costs: Dijkstra cost array indexed by dense node ID (from budget Dijkstra)
//   - tiles: TileScanResult for coordinate/elevation access
//   - nodeMapping: tileID -> localID -> denseID
//
// Returns flankA, flankB. Either may have GlobalNodeID=0 if no suitable node found.
func FindFlankLandmarks(
	crossingSeed graph.LandmarkCandidate,
	costs []float32,
	tiles []graph.TileScanResult,
	nodeMapping map[uint32][]uint32,
) (flankA, flankB graph.LandmarkCandidate) {

	seedLat := float64(crossingSeed.Lat)
	seedLon := float64(crossingSeed.Lon)

	// cos(lat) correction for longitude distances.
	// Without this, bearings are distorted by ~15-20 deg at 47 deg N (Alps).
	// E-W oriented passes (Arlberg) and diagonal passes (Julier) would get
	// wrong split bearings, placing flanks in the wrong half-plane.
	midLatRad := seedLat * 1.745329252e-9 // E7 -> radians
	cosLat := math.Cos(midLatRad)

	// --- Phase 1: Valley detection ---
	// Find the 2 deepest reachable nodes in different directions (>60 deg apart)
	// Cost range 5000-30000 (~5-30km) for valley detection
	type dirCandidate struct {
		bearing float64
		ele     int32
		gid     uint64
	}
	var deepest [2]dirCandidate
	deepest[0].ele = math.MaxInt32
	deepest[1].ele = math.MaxInt32

	for _, tile := range tiles {
		mapping := nodeMapping[tile.TileID]
		if mapping == nil {
			continue
		}
		for i := range tile.NodeCount {
			if i >= uint32(len(mapping)) {
				continue
			}
			denseID := mapping[i]
			if denseID >= uint32(len(costs)) {
				continue
			}
			cost := costs[denseID]
			if cost < 5000 || cost > 30000 {
				continue
			}
			if tile.NodeEles[i] >= crossingSeed.Ele {
				continue // must be lower than crossing
			}

			bearing := math.Atan2(
				(float64(tile.NodeLons[i])-seedLon)*cosLat,
				float64(tile.NodeLats[i])-seedLat,
			)

			if tile.NodeEles[i] < deepest[0].ele {
				deepest[1] = deepest[0]
				deepest[0] = dirCandidate{bearing, tile.NodeEles[i], graph.MakeGlobalNodeID(tile.TileID, i)}
			} else if tile.NodeEles[i] < deepest[1].ele {
				// Only accept if bearing is sufficiently different (>60 deg)
				if angleDiff(bearing, deepest[0].bearing) > math.Pi/3 {
					deepest[1] = dirCandidate{bearing, tile.NodeEles[i], graph.MakeGlobalNodeID(tile.TileID, i)}
				}
			}
		}
	}

	// --- Phase 2: Compute split bearing ---
	// Perpendicular to the line connecting the 2 valleys
	var splitBearing float64
	noValleyFound := deepest[1].ele == math.MaxInt32
	if noValleyFound {
		// Only 1 valley direction found: use fixed E/W split (N/S half-planes)
		slog.Warn("flank: only 1 valley direction found, using N/S fallback",
			"seed_ele", crossingSeed.Ele,
			"deepest_ele", deepest[0].ele)
		splitBearing = 0
	} else {
		// Circular mean of the 2 valley bearings
		midBearing := math.Atan2(
			math.Sin(deepest[0].bearing)+math.Sin(deepest[1].bearing),
			math.Cos(deepest[0].bearing)+math.Cos(deepest[1].bearing),
		)
		splitBearing = midBearing + math.Pi/2.0
	}

	// --- Phase 3: Find farthest nodes in each half-plane ---
	// Sweet spot: 15-50km graph distance, at least 300m lower (3000 dm) than crossing.
	// In N/S fallback (no valley detected), skip elevation filtering — place landmarks
	// at the farthest reachable nodes regardless of elevation (better than nothing).
	minEleDiff := int32(3000) // 300m in dm
	var bestACost, bestBCost float32

	for _, tile := range tiles {
		mapping := nodeMapping[tile.TileID]
		if mapping == nil {
			continue
		}
		for i := range tile.NodeCount {
			if i >= uint32(len(mapping)) {
				continue
			}
			denseID := mapping[i]
			if denseID >= uint32(len(costs)) {
				continue
			}
			cost := costs[denseID]
			if cost < 15000 || cost > 50000 {
				continue
			}
			// Elevation filtering: in normal mode (valley detected), require node to be
			// at least minEleDiff (300m) below crossing. If crossing is at low elevation
			// (< 500m, i.e. < 5000dm), only require node to be at or below crossing.
			// In N/S fallback mode (no valley detected), require node to be at or below
			// crossing (prevents placing landmarks above the crossing).
			if noValleyFound {
				if tile.NodeEles[i] > crossingSeed.Ele {
					continue // node is higher than crossing — skip
				}
			} else if crossingSeed.Ele >= 5000 {
				if tile.NodeEles[i] > crossingSeed.Ele-minEleDiff {
					continue // not deep enough in valley
				}
			} else {
				if tile.NodeEles[i] > crossingSeed.Ele {
					continue // low crossing: only require node to be below crossing
				}
			}

			bearing := math.Atan2(
				(float64(tile.NodeLons[i])-seedLon)*cosLat,
				float64(tile.NodeLats[i])-seedLat,
			)

			gid := graph.MakeGlobalNodeID(tile.TileID, i)
			candidate := graph.LandmarkCandidate{
				GlobalNodeID: gid,
				Lat:          tile.NodeLats[i],
				Lon:          tile.NodeLons[i],
				Ele:          tile.NodeEles[i],
			}

			// Determine half-plane via sin(bearing - splitBearing)
			side := math.Sin(bearing - splitBearing)
			if side >= 0 {
				if cost > bestACost {
					bestACost = cost
					flankA = candidate
				}
			} else {
				if cost > bestBCost {
					bestBCost = cost
					flankB = candidate
				}
			}
		}
	}

	return flankA, flankB
}

// angleDiff returns the absolute angular difference in [0, pi].
func angleDiff(a, b float64) float64 {
	d := math.Abs(a - b)
	if d > math.Pi {
		d = 2*math.Pi - d
	}
	return d
}
