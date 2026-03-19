package handler

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"net/http"
	"sort"
	"strings"

	"github.com/paulmach/orb"
	"github.com/paulmach/orb/encoding/wkb"
	"github.com/paulmach/orb/geojson"

	"github.com/skadia-app/skadia/services/api/internal/middleware"
	"github.com/skadia-app/skadia/services/api/internal/routing"
)

// ---------------------------------------------------------------------------
// Request / response types
// ---------------------------------------------------------------------------

// IsochroneRequest describes the POST body for /api/v1/isochrone.
type IsochroneRequest struct {
	// Start is a [longitude, latitude] coordinate pair.
	Start [2]float64 `json:"start"`
	// Minutes lists the travel-time thresholds (in minutes) for each isochrone
	// band, e.g. [60, 120, 180].  At least one value is required.
	Minutes []int `json:"minutes"`
	// Profile selects the routing cost function, e.g. "hiking", "mtb", "road".
	Profile string `json:"profile"`
	// POITypes is an optional list of OSM categories to discover within the
	// reachable area, e.g. ["peak", "gastronomy", "accommodation"].
	POITypes []string `json:"poi_types"`
}

// IsochronePolygon is one isochrone band in the response.
type IsochronePolygon struct {
	Minutes  int             `json:"minutes"`
	Geometry json.RawMessage `json:"geometry"` // GeoJSON Polygon
}

// IsochronePOI is a point of interest found within the outermost isochrone.
type IsochronePOI struct {
	Name    string  `json:"name"`
	Type    string  `json:"type"`
	Lat     float64 `json:"lat"`
	Lng     float64 `json:"lng"`
	OsmID   int64   `json:"osm_id,omitempty"`
	Minutes float64 `json:"minutes"`
}

// IsochroneResponse is the JSON body returned by POST /api/v1/isochrone.
type IsochroneResponse struct {
	Polygons  []IsochronePolygon `json:"polygons"`
	POIs      []IsochronePOI     `json:"pois"`
	Truncated bool               `json:"truncated"`
}

// ---------------------------------------------------------------------------
// Handler
// ---------------------------------------------------------------------------

// Isochrone computes reachable areas from a start point using
// UnifiedRouter.FindReachableWithCoords (Dijkstra with cost budget).
// One concave (alpha-shape) polygon is returned for each requested time band.
// POIs within the outermost band are optionally returned when poi_types is set.
//
// POST /api/v1/isochrone
func (h *Handler) Isochrone(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "Unauthorized", "no user ID", r.URL.Path)
		return
	}

	var req IsochroneRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Bad Request", "invalid JSON body", r.URL.Path)
		return
	}

	if len(req.Minutes) == 0 {
		writeError(w, http.StatusBadRequest, "Bad Request", "minutes must contain at least one value", r.URL.Path)
		return
	}

	// Validate coordinate range.
	if req.Start[0] < -180 || req.Start[0] > 180 || req.Start[1] < -90 || req.Start[1] > 90 {
		writeError(w, http.StatusBadRequest, "Bad Request", "start coordinates out of range", r.URL.Path)
		return
	}

	// Default profile.
	if req.Profile == "" {
		req.Profile = "hiking"
	}

	// ----- Router availability -----
	if h.GraphManager == nil {
		writeError(w, http.StatusServiceUnavailable, "No Routing Engine",
			"routing graph is not configured", r.URL.Path)
		return
	}
	router := h.GraphManager.UnifiedRouter()
	if router == nil {
		writeError(w, http.StatusServiceUnavailable, "No Routing Engine",
			"routing graph has not been loaded yet", r.URL.Path)
		return
	}

	// ----- Build cost config -----
	cfg, err := routing.BuildCostConfig(routing.ProfileConfig{Profile: req.Profile})
	if err != nil {
		writeError(w, http.StatusBadRequest, "Bad Request",
			fmt.Sprintf("unknown profile %q", req.Profile), r.URL.Path)
		return
	}

	// ----- Compute budget -----
	// Sort minutes descending so we can iterate largest-first.
	sortedMinutes := make([]int, len(req.Minutes))
	copy(sortedMinutes, req.Minutes)
	sort.Sort(sort.Reverse(sort.IntSlice(sortedMinutes)))

	maxMinutes := sortedMinutes[0]
	speedKmh := isochroneSpeed(req.Profile)
	radiusKm := (float64(maxMinutes) / 60.0) * speedKmh
	budget := float32(radiusKm) * cfg.AvgCostPerKm

	// ----- Dijkstra expansion -----
	startLat := req.Start[1]
	startLon := req.Start[0]

	reachable, truncated, err := router.FindReachableWithCoords(
		r.Context(), startLat, startLon, cfg, budget, nil,
	)
	if err != nil {
		slog.Warn("isochrone: FindReachableWithCoords failed",
			"error", err, "user_id", userID)
		writeError(w, http.StatusServiceUnavailable, "No Trail Network",
			"no routing graph nodes near the start point", r.URL.Path)
		return
	}

	if len(reachable) == 0 {
		writeJSON(w, http.StatusOK, IsochroneResponse{
			Polygons:  []IsochronePolygon{},
			POIs:      []IsochronePOI{},
			Truncated: truncated,
		})
		return
	}

	// ----- Build polygons per time band -----
	polygons := make([]IsochronePolygon, 0, len(sortedMinutes))
	var largestBandPoints []orb.Point // for POI bounding hull

	for _, mins := range sortedMinutes {
		// Proportional cost threshold for this band.
		bandBudget := budget * (float32(mins) / float32(maxMinutes))

		points := make([]orb.Point, 0, len(reachable)/2)
		for _, rn := range reachable {
			if rn.Cost <= bandBudget {
				points = append(points, orb.Point{rn.Lon, rn.Lat})
			}
		}

		if largestBandPoints == nil {
			largestBandPoints = points // first iteration = largest band
		}

		if len(points) < 3 {
			// Not enough points for a polygon; emit an empty geometry.
			polygons = append(polygons, IsochronePolygon{
				Minutes:  mins,
				Geometry: json.RawMessage(`null`),
			})
			continue
		}

		ring := routing.AlphaShape(points, 0) // auto-alpha
		if ring == nil {
			// Fallback to convex hull.
			ring = routing.ConvexHull(points)
		}
		if ring == nil {
			polygons = append(polygons, IsochronePolygon{
				Minutes:  mins,
				Geometry: json.RawMessage(`null`),
			})
			continue
		}

		poly := orb.Polygon{ring}
		geom := geojson.NewGeometry(poly)
		gjBytes, err := geom.MarshalJSON()
		if err != nil {
			slog.Warn("isochrone: GeoJSON marshal failed", "error", err, "minutes", mins)
			polygons = append(polygons, IsochronePolygon{
				Minutes:  mins,
				Geometry: json.RawMessage(`null`),
			})
			continue
		}

		polygons = append(polygons, IsochronePolygon{
			Minutes:  mins,
			Geometry: gjBytes,
		})
	}

	// Reverse polygons so they go from smallest to largest minutes in the response.
	for i, j := 0, len(polygons)-1; i < j; i, j = i+1, j-1 {
		polygons[i], polygons[j] = polygons[j], polygons[i]
	}

	// ----- POI search -----
	pois := make([]IsochronePOI, 0)

	if len(req.POITypes) > 0 && len(largestBandPoints) >= 3 {
		hullRing := routing.ConvexHull(largestBandPoints)
		if hullRing != nil {
			found, poiErr := h.queryPOIsInHull(r, hullRing, req.POITypes)
			if poiErr != nil {
				slog.Warn("isochrone: POI lookup failed",
					"error", poiErr, "user_id", userID)
			} else {
				// Build spatial grid once; then estimate minutes per POI via
				// 3×3 cell neighbourhood lookup instead of O(N) linear scan.
				grid := buildReachableGrid(reachable)
				for i := range found {
					found[i].Minutes = estimatePOIMinutes(
						found[i].Lat, found[i].Lng,
						reachable, grid, budget, float64(maxMinutes),
					)
				}
				pois = found
			}
		}
	}

	writeJSON(w, http.StatusOK, IsochroneResponse{
		Polygons:  polygons,
		POIs:      pois,
		Truncated: truncated,
	})
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// isochroneSpeed returns the expected average speed (km/h) for a profile.
func isochroneSpeed(profile string) float64 {
	switch profile {
	case "road":
		return 25.0
	case "gravel", "trekking":
		return 20.0
	case "mtb":
		return 12.0
	default: // hiking
		return 5.0
	}
}

// queryPOIsInHull queries osm_pois within the convex hull polygon, filtered
// by the requested POI categories.
func (h *Handler) queryPOIsInHull(r *http.Request, hull orb.Ring, poiTypes []string) ([]IsochronePOI, error) {
	poly := orb.Polygon{hull}
	wkbBytes, err := wkb.Marshal(poly)
	if err != nil {
		return nil, fmt.Errorf("WKB marshal: %w", err)
	}

	// Build category filter with parameterized placeholders.
	// poiTypes are category names from osm_pois.category.
	placeholders := make([]string, len(poiTypes))
	args := make([]any, 0, len(poiTypes)+1)
	args = append(args, wkbBytes) // $1
	for i, pt := range poiTypes {
		args = append(args, pt)
		placeholders[i] = fmt.Sprintf("$%d", i+2)
	}

	query := fmt.Sprintf(`
		SELECT osm_id, COALESCE(name, ''), category,
		       ST_Y(geom), ST_X(geom)
		FROM osm_pois
		WHERE ST_Intersects(geom, ST_GeomFromWKB($1, 4326))
		  AND category IN (%s)
		ORDER BY name
		LIMIT 200
	`, strings.Join(placeholders, ", "))

	rows, err := h.DB.Pool.Query(r.Context(), query, args...)
	if err != nil {
		return nil, fmt.Errorf("POI query: %w", err)
	}
	defer rows.Close()

	var pois []IsochronePOI
	for rows.Next() {
		var p IsochronePOI
		if err := rows.Scan(&p.OsmID, &p.Name, &p.Type, &p.Lat, &p.Lng); err != nil {
			return nil, fmt.Errorf("POI scan: %w", err)
		}
		pois = append(pois, p)
	}
	return pois, rows.Err()
}

// ---------------------------------------------------------------------------
// Spatial grid index for O(1) nearest-node lookup
// ---------------------------------------------------------------------------

// cellSize is ~500 m in lat/lon degrees.
const cellSize = 0.005

// cellKey is an integer grid coordinate.
type cellKey struct{ latIdx, lonIdx int }

// buildReachableGrid constructs a spatial grid that maps each cell to the
// indices of reachable nodes whose coordinates fall inside it.  The grid is
// built once and reused for every POI lookup, reducing the overall complexity
// from O(N*M) to O(N + M*9k) where k is the average nodes per cell.
func buildReachableGrid(reachable []routing.ReachableNode) map[cellKey][]int {
	grid := make(map[cellKey][]int, len(reachable)/4+1)
	for i, rn := range reachable {
		key := cellKey{
			latIdx: int(math.Floor(rn.Lat / cellSize)),
			lonIdx: int(math.Floor(rn.Lon / cellSize)),
		}
		grid[key] = append(grid[key], i)
	}
	return grid
}

// nearestReachableNode searches the 3×3 cell neighbourhood around (lat, lon)
// and returns the cost of the closest reachable node.  If no node is found in
// adjacent cells the function falls back to a full linear scan so correctness
// is preserved even in sparse areas.
func nearestReachableNode(lat, lon float64, reachable []routing.ReachableNode, grid map[cellKey][]int) (float32, bool) {
	centerKey := cellKey{
		latIdx: int(math.Floor(lat / cellSize)),
		lonIdx: int(math.Floor(lon / cellSize)),
	}

	bestDist := math.MaxFloat64
	var bestCost float32
	found := false

	for dlat := -1; dlat <= 1; dlat++ {
		for dlon := -1; dlon <= 1; dlon++ {
			key := cellKey{centerKey.latIdx + dlat, centerKey.lonIdx + dlon}
			for _, idx := range grid[key] {
				rn := reachable[idx]
				dx := rn.Lon - lon
				dy := rn.Lat - lat
				d := dx*dx + dy*dy
				if d < bestDist {
					bestDist = d
					bestCost = rn.Cost
					found = true
				}
			}
		}
	}

	if found {
		return bestCost, true
	}

	// Fallback: full linear scan for sparse regions where the 3×3 window is
	// empty (e.g. POI very close to the edge of the reachable area).
	for _, rn := range reachable {
		dx := rn.Lon - lon
		dy := rn.Lat - lat
		d := dx*dx + dy*dy
		if d < bestDist {
			bestDist = d
			bestCost = rn.Cost
			found = true
		}
	}
	return bestCost, found
}

// estimatePOIMinutes converts a POI's nearest-node cost to estimated minutes.
// The caller must build the grid with buildReachableGrid before calling this.
func estimatePOIMinutes(poiLat, poiLon float64, reachable []routing.ReachableNode, grid map[cellKey][]int, budget float32, maxMinutes float64) float64 {
	bestCost, ok := nearestReachableNode(poiLat, poiLon, reachable, grid)
	if !ok || budget <= 0 {
		return maxMinutes
	}
	minutes := (float64(bestCost) / float64(budget)) * maxMinutes
	return math.Round(minutes*10) / 10 // round to 1 decimal
}
