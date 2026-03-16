package routing

import (
	"encoding/json"
	"math"
	"slices"

	"github.com/skadia-app/skadia/services/api/internal/routing/graph"
)

// RouteResult holds the complete routing result.
type RouteResult struct {
	Coordinates []LatLonEle    `json:"-"`
	DistanceM   float64        `json:"distance_m"`
	AscentM     float64        `json:"ascent_m"`
	DescentM    float64        `json:"descent_m"`
	Cost        float64        `json:"cost"`
	Segments    []RouteSegment `json:"segments"`
	PassStats   PassStats      `json:"stats"`
}

// LatLonEle is a coordinate with elevation.
type LatLonEle struct {
	Lat float64 `json:"lat"`
	Lon float64 `json:"lon"`
	Ele float64 `json:"ele"`
}

// RouteSegment describes a contiguous section with the same OSM way properties.
type RouteSegment struct {
	FromIdx   int    `json:"from_idx"`
	ToIdx     int    `json:"to_idx"`
	Highway   string `json:"highway"`
	Surface   string `json:"surface"`
	SacScale  string `json:"sac_scale,omitempty"`
	MtbScale  string `json:"mtb_scale,omitempty"`
	Tracktype string `json:"tracktype,omitempty"`
	OsmWayID  int64  `json:"osm_way_id"`
}

// PassStats contains timing/expansion metrics for debugging.
type PassStats struct {
	Pass0Expansions int   `json:"pass0_expansions"`
	Pass0DurationMs int64 `json:"pass0_duration_ms"`
	Pass1Expansions int   `json:"pass1_expansions"`
	Pass1DurationMs int64 `json:"pass1_duration_ms"`
	TotalDurationMs int64 `json:"total_duration_ms"`
}

// LatLon is a simple lat/lon pair for via-point routing.
type LatLon struct {
	Lat float64
	Lon float64
}

// ToGeoJSON converts a RouteResult to a GeoJSON FeatureCollection.
func (r *RouteResult) ToGeoJSON() json.RawMessage {
	coords := make([][]float64, len(r.Coordinates))
	for i, c := range r.Coordinates {
		coords[i] = []float64{c.Lon, c.Lat, c.Ele} // GeoJSON: [lon, lat, ele]
	}

	feature := map[string]any{
		"type": "FeatureCollection",
		"features": []map[string]any{{
			"type": "Feature",
			"geometry": map[string]any{
				"type":        "LineString",
				"coordinates": coords,
			},
			"properties": map[string]any{
				"distance_m": int(r.DistanceM),
				"ascent_m":   int(r.AscentM),
				"descent_m":  int(r.DescentM),
				"cost":       int(r.Cost),
				"segments":   r.Segments,
				"stats":      r.PassStats,
			},
		}},
	}

	b, err := json.Marshal(feature)
	if err != nil {
		// Fallback: return minimal valid GeoJSON
		return json.RawMessage(`{"type":"FeatureCollection","features":[]}`)
	}
	return b
}

// mergeLegs concatenates multiple RouteResults (via-point legs) into one.
func mergeLegs(legs []*RouteResult) *RouteResult {
	if len(legs) == 0 {
		return &RouteResult{}
	}
	if len(legs) == 1 {
		return legs[0]
	}

	result := &RouteResult{}
	for i, leg := range legs {
		coordsBefore := len(result.Coordinates)

		if i == 0 {
			result.Coordinates = append(result.Coordinates, leg.Coordinates...)
		} else if len(leg.Coordinates) > 0 {
			// Skip first coordinate (duplicate of previous leg's last)
			result.Coordinates = append(result.Coordinates, leg.Coordinates[1:]...)
		}
		result.DistanceM += leg.DistanceM
		result.AscentM += leg.AscentM
		result.DescentM += leg.DescentM
		result.Cost += leg.Cost

		segOffset := 0
		if i > 0 {
			segOffset = coordsBefore - 1
		}
		for _, seg := range leg.Segments {
			adjusted := seg
			adjusted.FromIdx += segOffset
			adjusted.ToIdx += segOffset
			result.Segments = append(result.Segments, adjusted)
		}

		result.PassStats.Pass0Expansions += leg.PassStats.Pass0Expansions
		result.PassStats.Pass1Expansions += leg.PassStats.Pass1Expansions
		result.PassStats.Pass0DurationMs += leg.PassStats.Pass0DurationMs
		result.PassStats.Pass1DurationMs += leg.PassStats.Pass1DurationMs
		result.PassStats.TotalDurationMs += leg.PassStats.TotalDurationMs
	}
	return result
}

// buildRetrackResult constructs a RouteResult from parent-pointer arrays.
// Called during re-tracking phase.
func buildRetrackResult(g *graph.Graph, src, dst uint32,
	parentNode, parentEdge []uint32, nodeGen []uint32, gen uint32) *RouteResult {

	// Walk backwards from dst to src (with safety limit)
	var path []uint32
	var edges []uint32
	node := dst
	maxIter := int(g.NodeCount)
	for node != src && maxIter > 0 {
		if nodeGen[node] != gen {
			break // should not happen if route was found
		}
		path = append(path, node)
		edges = append(edges, parentEdge[node])
		node = parentNode[node]
		maxIter--
	}
	path = append(path, src)

	// Reverse to get src→dst order
	slices.Reverse(path)
	slices.Reverse(edges)

	// Build coordinates and compute metrics — expand edge geometry
	var coords []LatLonEle
	var totalDist, ascent, descent float64
	// coordIdx tracks the coordinate index for segment boundary mapping
	// edgeCoordStart[i] = coordinate index where edge i starts (the network node before it)
	edgeCoordStart := make([]int, len(edges))

	for i, n := range path {
		// Add the network node itself
		coords = append(coords, LatLonEle{
			Lat: float64(g.NodeLat[n]) / 1e7,
			Lon: float64(g.NodeLon[n]) / 1e7,
			Ele: float64(g.NodeEle[n]) / 10.0, // dm → m
		})

		// If not the last node, record edge start and expand edge geometry.
		// Skip first and last geometry points — they are the start/end
		// network nodes which are already added by the outer loop.
		if i < len(edges) {
			edgeCoordStart[i] = len(coords) - 1

			ei := edges[i]
			geomStart := g.EdgeGeomOff[ei]
			geomEnd := g.EdgeGeomOff[ei+1]
			if geomEnd > geomStart+2 { // at least one interior point
				for gi := geomStart + 1; gi < geomEnd-1; gi++ {
					lat := float64(g.EdgeGeomLat[gi]) / 1e7
					lon := float64(g.EdgeGeomLon[gi]) / 1e7
					var ele float64
					profStart := g.EdgeElevOff[ei]
					profIdx := gi - geomStart
					if profStart+profIdx < g.EdgeElevOff[ei+1] {
						ele = float64(g.EdgeElevProf[profStart+profIdx]) / 10.0
					}
					coords = append(coords, LatLonEle{Lat: lat, Lon: lon, Ele: ele})
				}
			}
		}
	}

	// Compute distance and elevation from expanded coords
	for i := 1; i < len(coords); i++ {
		totalDist += haversineDist(coords[i-1].Lat, coords[i-1].Lon,
			coords[i].Lat, coords[i].Lon)
		dEle := coords[i].Ele - coords[i-1].Ele
		if dEle > 0 {
			ascent += dEle
		} else {
			descent -= dEle
		}
	}

	// Build segments using expanded coordinate indices
	var segments []RouteSegment
	if len(edges) > 0 {
		curOSMIdx := g.EdgeOSMIdx[edges[0]]
		curTags := g.EdgeTags1[edges[0]]
		segFromCoord := edgeCoordStart[0]

		for i := 1; i < len(edges); i++ {
			osmIdx := g.EdgeOSMIdx[edges[i]]
			if osmIdx != curOSMIdx {
				seg := makeSegment(g, segFromCoord, edgeCoordStart[i], curTags, curOSMIdx)
				segments = append(segments, seg)
				segFromCoord = edgeCoordStart[i]
				curOSMIdx = osmIdx
				curTags = g.EdgeTags1[edges[i]]
			}
		}
		seg := makeSegment(g, segFromCoord, len(coords)-1, curTags, curOSMIdx)
		segments = append(segments, seg)
	}

	return &RouteResult{
		Coordinates: coords,
		DistanceM:   totalDist,
		AscentM:     ascent,
		DescentM:    descent,
		Segments:    segments,
	}
}

func makeSegment(g *graph.Graph, fromIdx, toIdx int, tags uint64, osmIdx uint32) RouteSegment {
	var osmWayID int64
	if int(osmIdx) < len(g.OSMIDPool) {
		osmWayID = g.OSMIDPool[osmIdx]
	}
	return RouteSegment{
		FromIdx:   fromIdx,
		ToIdx:     toIdx,
		Highway:   graph.HighwayName(graph.DecodeHighway(tags)),
		Surface:   graph.SurfaceName(graph.DecodeSurface(tags)),
		SacScale:  graph.SacScaleName(uint8(graph.DecodeField(tags, 14, 3))),
		MtbScale:  graph.MtbScaleName(uint8(graph.DecodeField(tags, 17, 3))),
		Tracktype: graph.TrackTypeName(uint8(graph.DecodeField(tags, 11, 3))),
		OsmWayID:  osmWayID,
	}
}

// haversineDist computes accurate distance in meters between two points.
// Used only in re-tracking (not performance-critical).
func haversineDist(lat1, lon1, lat2, lon2 float64) float64 {
	const R = 6_371_000.0
	dLat := (lat2 - lat1) * math.Pi / 180
	dLon := (lon2 - lon1) * math.Pi / 180
	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(lat1*math.Pi/180)*math.Cos(lat2*math.Pi/180)*
			math.Sin(dLon/2)*math.Sin(dLon/2)
	return R * 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
}
