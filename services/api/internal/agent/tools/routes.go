package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/routing"
	"github.com/skadia-app/skadia/services/api/internal/sport"
	"github.com/google/uuid"
)

var (
	routeHTTPClient = &http.Client{Timeout: 30 * time.Second}
	routeAPIBaseURL = "http://api:8080"
)

const (
	maxPlanRouteWaypoints     = 25
	maxPlanRouteURLBytes      = 8192
	maxPlanRouteResponseBytes = 1 << 20
	maxCoverageLayers         = 8
	maxCoverageResponseBytes  = 64 << 10
	maxRouteGeometryBytes     = 500_000
	maxRouteGeometryVertices  = 2_000
	maxRouteGeometryLengthM   = 200_000
	maxRoutePOIBufferM        = 5_000
	maxIsochroneMinutes       = 180
	maxIsochronePoints        = 1_000
)

type searchRoutesArgs struct {
	LocationName string  `json:"location_name,omitempty"`
	ActivityID   string  `json:"activity_id,omitempty"`
	SportType    string  `json:"sport_type,omitempty"`
	SubType      *string `json:"sub_type,omitempty"`
	IsElectric   *bool   `json:"is_electric,omitempty"`
	IsIndoor     *bool   `json:"is_indoor,omitempty"`
	IsVirtual    *bool   `json:"is_virtual,omitempty"`
	IsCommute    *bool   `json:"is_commute,omitempty"`
	Days         *int    `json:"days,omitempty"`
	Limit        *int    `json:"limit,omitempty"`
}

type planRouteArgs struct {
	Waypoints     [][]float64    `json:"waypoints"`
	Profile       string         `json:"profile,omitempty"`
	ProfileParams map[string]any `json:"profile_params,omitempty"`
}

type searchPOIsOnRouteArgs struct {
	RouteGeometry string `json:"route_geometry"`
	BufferM       *int   `json:"buffer_m,omitempty"`
	Category      string `json:"category,omitempty"`
	Limit         *int   `json:"limit,omitempty"`
}

type getIsochroneArgs struct {
	Lat         float64 `json:"lat"`
	Lng         float64 `json:"lng"`
	TimeMinutes *int    `json:"time_minutes,omitempty"`
}

type queryCoverageArgs struct {
	Lat    float64 `json:"lat"`
	Lon    float64 `json:"lon"`
	Layers string  `json:"layers"`
}

type listRoutingProfilesArgs struct {
	SportQuery string `json:"sport_query,omitempty"`
	Intent     string `json:"intent,omitempty"`
	IsElectric *bool  `json:"is_electric,omitempty"`
	IsIndoor   *bool  `json:"is_indoor,omitempty"`
	IsVirtual  *bool  `json:"is_virtual,omitempty"`
	IsCommute  *bool  `json:"is_commute,omitempty"`
}

type routingProfileToolItem struct {
	ID                  string          `json:"id,omitempty"`
	Key                 string          `json:"key"`
	Label               string          `json:"label"`
	Engine              string          `json:"engine"`
	BaseProfile         string          `json:"base_profile"`
	SportTypes          []string        `json:"sport_types,omitempty"`
	DefaultParams       map[string]any  `json:"default_params,omitempty"`
	Constraints         map[string]any  `json:"constraints"`
	Supports            map[string]bool `json:"supports"`
	SemanticPreferences []string        `json:"semantic_preferences"`
	ElevationSemantics  []string        `json:"elevation_semantics"`
	ProfileVersion      string          `json:"profile_version"`
	Source              string          `json:"source"`
}

func RegisterRouteTools(reg *Registry) {
	for _, tool := range RouteTools() {
		reg.Register(tool)
	}
}

func RouteTools() []*Tool {
	return []*Tool{
		{
			Name:        "search_routes",
			Description: "Search the user's historical routes by place, sport, modifiers, and date range.",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"location_name":{"type":"string"},"activity_id":{"type":"string","format":"uuid"},"sport_type":{"type":"string"},"sub_type":{"type":"string"},"is_electric":{"type":"boolean"},"is_indoor":{"type":"boolean"},"is_virtual":{"type":"boolean"},"is_commute":{"type":"boolean"},"days":{"type":"integer","minimum":1},"limit":{"type":"integer","minimum":1,"maximum":50}},"additionalProperties":false}`),
			Permission:  PermissionRead,
			IsUserOwned: true,
			ExecuteRead: executeSearchRoutes,
		},
		{
			Name:        "list_routing_profiles",
			Description: "List real Skadia routing profiles visible to the user, including profile keys for route preview selection and allowed override parameters.",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"sport_query":{"type":"string"},"intent":{"type":"string","enum":["direct","round_trip","goal","commute","unknown"]},"is_electric":{"type":"boolean"},"is_indoor":{"type":"boolean"},"is_virtual":{"type":"boolean"},"is_commute":{"type":"boolean"}},"additionalProperties":false}`),
			Permission:  PermissionRead,
			IsUserOwned: false,
			ExecuteRead: executeListRoutingProfiles,
		},
		{
			Name:        "plan_route",
			Description: "Preview a route between [lng,lat] waypoints via the configured routing service. The profile must be a key from list_routing_profiles. This is read-only and never saves a route.",
			InputSchema: json.RawMessage(`{"type":"object","required":["waypoints","profile"],"properties":{"waypoints":{"type":"array","minItems":2,"items":{"type":"array","minItems":2,"maxItems":2,"items":{"type":"number"}}},"profile":{"type":"string"},"profile_params":{"type":"object","additionalProperties":{"type":["string","number","boolean"]}}},"additionalProperties":false}`),
			Permission:  PermissionRead,
			IsUserOwned: false,
			ExecuteRead: executePlanRoute,
		},
		{
			Name:        "search_pois_on_route",
			Description: "Find shared OSM POIs along a provided GeoJSON route geometry.",
			InputSchema: json.RawMessage(`{"type":"object","required":["route_geometry"],"properties":{"route_geometry":{"type":"string","maxLength":500000},"buffer_m":{"type":"integer","minimum":1,"maximum":5000},"category":{"type":"string"},"limit":{"type":"integer","minimum":1,"maximum":50}},"additionalProperties":false}`),
			Permission:  PermissionRead,
			IsUserOwned: false,
			ExecuteRead: executeSearchPOIsOnRoute,
		},
		{
			Name:        "get_isochrone",
			Description: "Calculate a reachability area from a coordinate using the pgRouting topology.",
			InputSchema: json.RawMessage(`{"type":"object","required":["lat","lng"],"properties":{"lat":{"type":"number"},"lng":{"type":"number"},"time_minutes":{"type":"integer","minimum":1}},"additionalProperties":false}`),
			Permission:  PermissionRead,
			IsUserOwned: false,
			ExecuteRead: executeGetIsochrone,
		},
		{
			Name:        "query_coverage",
			Description: "Query coverage-grid layer values at a geographic point through the internal coverage endpoint.",
			InputSchema: json.RawMessage(`{"type":"object","required":["lat","lon","layers"],"properties":{"lat":{"type":"number"},"lon":{"type":"number"},"layers":{"type":"string"}},"additionalProperties":false}`),
			Permission:  PermissionRead,
			IsUserOwned: false,
			ExecuteRead: executeQueryCoverage,
		},
		ResolveUserLocationTool(),
		GeocodePlaceTool(),
		ResolveRoutePlaceTool(),
		ResolveRouteFeatureTool(),
		GetRoutingCoverageTool(),
		AnalyzeRoutePointsTool(),
		GetAgentServiceStatusTool(),
		PlanRouteBetweenTool(),
		SuggestRouteGoalTool(),
	}
}

func executeListRoutingProfiles(ctx context.Context, deps *Deps, userID string, raw json.RawMessage) (any, error) {
	var args listRoutingProfilesArgs
	if len(raw) != 0 {
		if err := json.Unmarshal(raw, &args); err != nil {
			return nil, fmt.Errorf("decode list_routing_profiles args: %w", err)
		}
	}
	profiles, err := listRoutingProfilesForTool(ctx, deps, userID)
	if err != nil {
		return nil, err
	}
	profiles = filterRoutingProfilesForTool(profiles, args)
	return map[string]any{"profiles": profiles, "count": len(profiles)}, nil
}

func executeSearchRoutes(ctx context.Context, deps *Deps, userID string, raw json.RawMessage) (any, error) {
	var args searchRoutesArgs
	if err := json.Unmarshal(raw, &args); err != nil {
		return nil, fmt.Errorf("decode search_routes args: %w", err)
	}
	sql, params, _, err := buildSearchRoutesSQL(userID, args, time.Now().UTC())
	if err != nil {
		return nil, err
	}
	rows, err := queryRows(ctx, deps, sql, params...)
	if err != nil {
		return nil, err
	}
	return map[string]any{"routes": rows}, nil
}

func executePlanRoute(ctx context.Context, deps *Deps, userID string, raw json.RawMessage) (any, error) {
	var args planRouteArgs
	if err := json.Unmarshal(raw, &args); err != nil {
		return nil, fmt.Errorf("decode plan_route args: %w", err)
	}
	if len(args.Waypoints) < 2 {
		return map[string]any{"error": "at least 2 waypoints required", "fallback": true}, nil
	}
	if err := validateWaypoints(args.Waypoints); err != nil {
		return map[string]any{"error": err.Error(), "fallback": true}, nil
	}
	resolved, err := resolveRoutingProfileForPreview(ctx, deps, userID, args.Profile)
	if err != nil {
		return map[string]any{"error": err.Error(), "fallback": true}, nil
	}
	mergedParams := routingProfileDefaultParams(resolved)
	for key, value := range args.ProfileParams {
		mergedParams[key] = value
	}
	params, err := stringifyProfileParams(mergedParams)
	if err != nil {
		return map[string]any{"error": err.Error(), "fallback": true}, nil
	}
	if err := routing.ValidateBRouterProfileParams(resolved.BaseProfile, params); err != nil {
		return map[string]any{"error": err.Error(), "fallback": true}, nil
	}
	args.Profile = resolved.BaseProfile
	args.ProfileParams = mapStringAny(params)

	baseURL := "http://brouter:17777"
	if deps != nil && deps.BRouterURL != "" {
		baseURL = deps.BRouterURL
	}
	requestURL, err := buildPlanRouteURL(baseURL, args)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := routeHTTPClient.Do(req)
	if err != nil {
		return map[string]any{"error": "Routing nicht verfügbar - BRouter-Segmente geladen?", "fallback": true}, nil
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return map[string]any{"error": fmt.Sprintf("BRouter returned HTTP %d", resp.StatusCode), "fallback": true}, nil
	}
	body, err := readCappedBody(resp.Body, maxPlanRouteResponseBytes)
	if err != nil {
		return map[string]any{"error": err.Error(), "fallback": true}, nil
	}
	var result map[string]any
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("decode BRouter response: %w", err)
	}
	result["preview"] = true
	result["profile_key"] = resolved.Key
	result["routing_profile"] = resolved.Label
	attachRoutePreviewToken(userID, result, resolved.Key, resolved.BaseProfile, resolved.ProfileVersion)
	return result, nil
}

func executeSearchPOIsOnRoute(ctx context.Context, deps *Deps, _ string, raw json.RawMessage) (any, error) {
	var args searchPOIsOnRouteArgs
	if err := json.Unmarshal(raw, &args); err != nil {
		return nil, fmt.Errorf("decode search_pois_on_route args: %w", err)
	}
	sql, params, _, err := buildSearchPOIsOnRouteSQL(args)
	if err != nil {
		return nil, err
	}
	rows, err := queryRows(ctx, deps, sql, params...)
	if err != nil {
		return nil, err
	}
	return map[string]any{"pois": rows, "count": len(rows)}, nil
}

func executeGetIsochrone(ctx context.Context, deps *Deps, _ string, raw json.RawMessage) (any, error) {
	var args getIsochroneArgs
	if err := json.Unmarshal(raw, &args); err != nil {
		return nil, fmt.Errorf("decode get_isochrone args: %w", err)
	}
	if !validLatLng(args.Lat, args.Lng) {
		return map[string]any{"error": "coordinates out of range"}, nil
	}
	nearestSQL, distanceSQL, hullSQL := buildIsochroneSQL()
	pool, err := readonlyPool(deps)
	if err != nil {
		return nil, err
	}
	var nearestID int64
	if err := pool.QueryRow(ctx, nearestSQL, args.Lng, args.Lat).Scan(&nearestID); err != nil {
		return map[string]any{"error": "Keine Topologie verfügbar - OSM-Import mit Trail-Topologie nötig"}, nil
	}
	timeMinutes := clampIntPtr(args.TimeMinutes, 30, 1, maxIsochroneMinutes)
	rows, err := queryRows(ctx, deps, distanceSQL, nearestID, float64(timeMinutes))
	if err != nil {
		return nil, err
	}
	points := make([]map[string]any, 0, len(rows))
	lngs := make([]float64, 0, len(rows))
	lats := make([]float64, 0, len(rows))
	for _, row := range rows {
		point := map[string]any{"lat": row["lat"], "lng": row["lng"], "minutes": row["agg_cost"]}
		points = append(points, point)
		lngs = append(lngs, numberValue(row["lng"]))
		lats = append(lats, numberValue(row["lat"]))
	}
	var hull any
	if len(points) >= 3 {
		var hullString *string
		if err := pool.QueryRow(ctx, hullSQL, lngs, lats).Scan(&hullString); err != nil {
			return nil, err
		}
		if hullString != nil {
			hull = *hullString
		}
	}
	return map[string]any{
		"points":       points,
		"hull_geojson": hull,
		"count":        len(points),
		"center":       map[string]any{"lat": args.Lat, "lng": args.Lng},
	}, nil
}

func executeQueryCoverage(ctx context.Context, deps *Deps, _ string, raw json.RawMessage) (any, error) {
	var args queryCoverageArgs
	if err := json.Unmarshal(raw, &args); err != nil {
		return nil, fmt.Errorf("decode query_coverage args: %w", err)
	}
	if !validLatLng(args.Lat, args.Lon) {
		return nil, fmt.Errorf("coordinates out of range")
	}
	layers, err := parseCoverageLayers(args.Layers)
	if err != nil {
		return nil, err
	}
	baseURL := routeAPIBaseURL
	u, err := url.Parse(strings.TrimRight(baseURL, "/") + "/internal/coverage/query")
	if err != nil {
		return nil, err
	}
	q := u.Query()
	q.Set("lat", strconvFloat(args.Lat))
	q.Set("lon", strconvFloat(args.Lon))
	q.Set("layers", strings.Join(layers, ","))
	u.RawQuery = q.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	if deps != nil && deps.InternalSecret != "" {
		req.Header.Set("X-ML-Callback-Secret", deps.InternalSecret)
	}
	resp, err := routeHTTPClient.Do(req)
	if err != nil {
		return map[string]any{"error": fmt.Sprintf("Coverage query nicht verfügbar: %v", err), "values": map[string]any{}}, nil
	}
	defer resp.Body.Close()
	body, err := readCappedBody(resp.Body, maxCoverageResponseBytes)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return map[string]any{"error": fmt.Sprintf("Coverage query returned HTTP %d", resp.StatusCode), "values": map[string]any{}}, nil
	}
	var coverageValues map[string]any
	if err := json.Unmarshal(body, &coverageValues); err != nil {
		return nil, fmt.Errorf("decode coverage response: %w", err)
	}
	return map[string]any{"lat": args.Lat, "lon": args.Lon, "layers": coverageValues}, nil
}

func buildSearchRoutesSQL(userID string, args searchRoutesArgs, now time.Time) (string, []any, int, error) {
	conditions := []string{"a.user_id = $1", "a.started_at >= $2"}
	params := []any{userID, sinceDate(now, clampIntPtr(args.Days, 365, 1, 36500))}
	if strings.TrimSpace(args.LocationName) != "" {
		params = append(params, "%"+args.LocationName+"%")
		conditions = append(conditions, fmt.Sprintf("(a.start_city ILIKE $%d OR a.start_state ILIKE $%d)", len(params), len(params)))
	}
	conditions, params = applyActivityFilters(conditions, params, ActivityFilters{
		TableAlias: "a",
		SportType:  args.SportType,
		SubType:    args.SubType,
		IsElectric: args.IsElectric,
		IsIndoor:   args.IsIndoor,
		IsVirtual:  args.IsVirtual,
		IsCommute:  args.IsCommute,
	})
	if strings.TrimSpace(args.ActivityID) != "" {
		activityID, err := uuid.Parse(args.ActivityID)
		if err != nil {
			return "", nil, 0, fmt.Errorf("invalid activity_id: %w", err)
		}
		params = append(params, activityID)
		conditions = append(conditions, fmt.Sprintf("a.id = $%d", len(params)))
	}
	limit := clampIntPtr(args.Limit, 10, 1, 50)
	sql := "SELECT a.id, a.title, a.sport_type, a.sub_type, a.is_electric, a.is_indoor, a.is_virtual, a.is_commute, a.started_at::date AS date, " +
		"ROUND((a.distance_m / 1000.0)::numeric, 1)::float8 AS distance_km, " +
		"a.elevation_gain_m, a.start_city " +
		"FROM activities a WHERE " + strings.Join(conditions, " AND ") + fmt.Sprintf(" ORDER BY a.started_at DESC LIMIT %d", limit)
	return sql, params, limit, nil
}

func buildPlanRouteURL(baseURL string, args planRouteArgs) (string, error) {
	if err := validateWaypoints(args.Waypoints); err != nil {
		return "", err
	}
	u, err := url.Parse(strings.TrimRight(baseURL, "/") + "/brouter")
	if err != nil {
		return "", err
	}
	if u.Scheme != "http" && u.Scheme != "https" || u.Host == "" {
		return "", fmt.Errorf("invalid routing service URL")
	}
	lonlats := make([]string, 0, len(args.Waypoints))
	for _, wp := range args.Waypoints {
		if len(wp) != 2 {
			return "", fmt.Errorf("waypoints must be [lng,lat] pairs")
		}
		lonlats = append(lonlats, fmt.Sprintf("%.6f,%.6f", wp[0], wp[1]))
	}
	profile := args.Profile
	if profile == "" {
		profile = "hiking-mountain"
	}
	q := u.Query()
	q.Set("lonlats", strings.Join(lonlats, "|"))
	q.Set("profile", profile)
	q.Set("format", "geojson")
	for k, v := range args.ProfileParams {
		q.Set("profile:"+k, fmt.Sprint(v))
	}
	u.RawQuery = q.Encode()
	if len(u.String()) > maxPlanRouteURLBytes {
		return "", fmt.Errorf("routing request URL too large")
	}
	return u.String(), nil
}

func validateWaypoints(waypoints [][]float64) error {
	if len(waypoints) < 2 {
		return fmt.Errorf("at least 2 waypoints required")
	}
	if len(waypoints) > maxPlanRouteWaypoints {
		return fmt.Errorf("too many waypoints (max %d)", maxPlanRouteWaypoints)
	}
	for i, wp := range waypoints {
		if len(wp) != 2 {
			return fmt.Errorf("waypoints must be [lng,lat] pairs")
		}
		lng, lat := wp[0], wp[1]
		if !validLatLng(lat, lng) {
			return fmt.Errorf("waypoint %d coordinates out of range", i)
		}
	}
	return nil
}

func validLatLng(lat, lng float64) bool {
	return !math.IsNaN(lat) && !math.IsInf(lat, 0) &&
		!math.IsNaN(lng) && !math.IsInf(lng, 0) &&
		lat >= -90 && lat <= 90 &&
		lng >= -180 && lng <= 180
}

func readCappedBody(r io.Reader, maxBytes int64) ([]byte, error) {
	body, err := io.ReadAll(io.LimitReader(r, maxBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(body)) > maxBytes {
		return nil, fmt.Errorf("response too large (max %d bytes)", maxBytes)
	}
	return body, nil
}

func parseCoverageLayers(raw string) ([]string, error) {
	allowed := map[string]bool{
		"forest": true, "landuse": true, "water": true, "protected": true,
		"urbanization": true, "river_proximity": true, "noise": true, "traffic": true,
		"populate": true, "industrial": true, "mw_density": true, "hw_density": true,
	}
	seen := map[string]bool{}
	out := make([]string, 0)
	for _, part := range strings.Split(raw, ",") {
		layer := strings.TrimSpace(part)
		if layer == "" {
			continue
		}
		if !allowed[layer] {
			return nil, fmt.Errorf("invalid coverage layer %q", layer)
		}
		if seen[layer] {
			continue
		}
		seen[layer] = true
		out = append(out, layer)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("at least one coverage layer is required")
	}
	if len(out) > maxCoverageLayers {
		return nil, fmt.Errorf("too many coverage layers (max %d)", maxCoverageLayers)
	}
	return out, nil
}

func buildSearchPOIsOnRouteSQL(args searchPOIsOnRouteArgs) (string, []any, int, error) {
	if err := validateRouteGeometry(args.RouteGeometry); err != nil {
		return "", nil, 0, err
	}
	buffer := clampIntPtr(args.BufferM, 500, 1, maxRoutePOIBufferM)
	limit := clampIntPtr(args.Limit, 50, 1, 50)
	params := []any{args.RouteGeometry, buffer}
	sql := `
SELECT name, category, subtype, ele,
       ST_Y(ST_Centroid(geom)) AS lat, ST_X(ST_Centroid(geom)) AS lng,
       ST_Distance(geom::geography, ST_GeomFromGeoJSON($1)::geography) AS dist_m
FROM osm_pois
WHERE ST_DWithin(geom::geography, ST_GeomFromGeoJSON($1)::geography, $2)`
	if args.Category != "" {
		params = append(params, args.Category)
		sql += fmt.Sprintf(" AND category = $%d", len(params))
	}
	params = append(params, limit)
	sql += fmt.Sprintf(" ORDER BY dist_m LIMIT $%d", len(params))
	return sql, params, limit, nil
}

func validateRouteGeometry(raw string) error {
	if strings.TrimSpace(raw) == "" {
		return fmt.Errorf("route geometry is required")
	}
	if len(raw) > maxRouteGeometryBytes {
		return fmt.Errorf("route geometry too large (max 500KB GeoJSON)")
	}
	var doc struct {
		Type        string          `json:"type"`
		Coordinates json.RawMessage `json:"coordinates"`
	}
	if err := json.Unmarshal([]byte(raw), &doc); err != nil {
		return fmt.Errorf("invalid route geometry GeoJSON: %w", err)
	}
	var count int
	var lengthM float64
	switch doc.Type {
	case "LineString":
		var line [][]float64
		if err := json.Unmarshal(doc.Coordinates, &line); err != nil {
			return fmt.Errorf("invalid LineString coordinates: %w", err)
		}
		n, meters, err := validateRouteLine(line)
		if err != nil {
			return err
		}
		count += n
		lengthM += meters
	case "MultiLineString":
		var lines [][][]float64
		if err := json.Unmarshal(doc.Coordinates, &lines); err != nil {
			return fmt.Errorf("invalid MultiLineString coordinates: %w", err)
		}
		for _, line := range lines {
			n, meters, err := validateRouteLine(line)
			if err != nil {
				return err
			}
			count += n
			lengthM += meters
		}
	default:
		return fmt.Errorf("route geometry must be LineString or MultiLineString")
	}
	if count < 2 {
		return fmt.Errorf("route geometry requires at least two coordinates")
	}
	if count > maxRouteGeometryVertices {
		return fmt.Errorf("route geometry has too many vertices (max %d)", maxRouteGeometryVertices)
	}
	if lengthM > maxRouteGeometryLengthM {
		return fmt.Errorf("route geometry too long (max %.0fm)", float64(maxRouteGeometryLengthM))
	}
	return nil
}

func validateRouteLine(line [][]float64) (int, float64, error) {
	if len(line) < 2 {
		return 0, 0, fmt.Errorf("route line requires at least two coordinates")
	}
	var lengthM float64
	var prev []float64
	for i, coord := range line {
		if len(coord) != 2 {
			return 0, 0, fmt.Errorf("route coordinate %d must be [lng,lat]", i)
		}
		lng, lat := coord[0], coord[1]
		if !validLatLng(lat, lng) {
			return 0, 0, fmt.Errorf("route coordinate %d out of range", i)
		}
		if prev != nil {
			lengthM += haversineMeters(prev[1], prev[0], lat, lng)
		}
		prev = coord
	}
	return len(line), lengthM, nil
}

func haversineMeters(lat1, lng1, lat2, lng2 float64) float64 {
	const earthRadiusM = 6371000.0
	phi1 := lat1 * math.Pi / 180
	phi2 := lat2 * math.Pi / 180
	dPhi := (lat2 - lat1) * math.Pi / 180
	dLambda := (lng2 - lng1) * math.Pi / 180
	a := math.Sin(dPhi/2)*math.Sin(dPhi/2) +
		math.Cos(phi1)*math.Cos(phi2)*math.Sin(dLambda/2)*math.Sin(dLambda/2)
	return earthRadiusM * 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
}

func buildIsochroneSQL() (string, string, string) {
	nearestSQL := `
SELECT id FROM osm_ways_vertices_pgr
ORDER BY the_geom <-> ST_SetSRID(ST_Point($1, $2), 4326)
LIMIT 1`
	distanceSQL := `
SELECT DISTINCT ON (node)
       node, agg_cost,
       ST_Y(v.the_geom) AS lat, ST_X(v.the_geom) AS lng
FROM pgr_drivingDistance(
    'SELECT id, source, target, cost, reverse_cost FROM osm_ways WHERE cost >= 0',
    $1, $2, false
) dd
JOIN osm_ways_vertices_pgr v ON dd.node = v.id
ORDER BY agg_cost
LIMIT ` + fmt.Sprint(maxIsochronePoints)
	hullSQL := `
SELECT ST_AsGeoJSON(ST_ConvexHull(ST_Collect(
    ST_SetSRID(ST_Point(lng, lat), 4326)
))) AS geojson
FROM unnest($1::float8[], $2::float8[]) AS t(lng, lat)`
	return nearestSQL, distanceSQL, hullSQL
}

func strconvFloat(v float64) string {
	return strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.12f", v), "0"), ".")
}

func listRoutingProfilesForTool(ctx context.Context, deps *Deps, userID string) ([]routingProfileToolItem, error) {
	profiles := make([]routingProfileToolItem, 0)
	if deps != nil && deps.DB != nil {
		dbProfiles, err := deps.DB.ListRoutingProfiles(ctx, userID)
		if err != nil {
			return nil, fmt.Errorf("list routing profiles: %w", err)
		}
		for _, p := range dbProfiles {
			profiles = append(profiles, routingProfileToolItemFromDB(p))
		}
	}
	for _, name := range routing.BRouterProfileNames() {
		profiles = append(profiles, routingProfileToolItem{
			Key:                 name,
			Label:               name,
			Engine:              "brouter",
			BaseProfile:         name,
			Constraints:         map[string]any{"profile_params": routing.BRouterProfileParamKeys(name)},
			Supports:            routingProfileSupports("brouter", routing.SourceFilesystem),
			SemanticPreferences: routingProfileSemanticPreferences(),
			ElevationSemantics:  routingProfileElevationSemantics(),
			ProfileVersion:      "filesystem:" + name,
			Source:              routing.SourceFilesystem,
		})
	}
	return profiles, nil
}

func routingProfileToolItemFromDB(p database.RoutingProfile) routingProfileToolItem {
	defaultParams := map[string]any{}
	if len(p.Config) > 0 {
		_ = json.Unmarshal(p.Config, &defaultParams)
	}
	previewProfile := mapBadRouterBaseProfileToBRouter(p.BaseProfile)
	previewParamKeys := []string{}
	if previewProfile != "" {
		previewParamKeys = routing.BRouterProfileParamKeys(previewProfile)
	}
	return routingProfileToolItem{
		ID:            p.ID,
		Key:           p.ID,
		Label:         p.Name,
		Engine:        "badrouter",
		BaseProfile:   p.BaseProfile,
		SportTypes:    p.SportTypes,
		DefaultParams: defaultParams,
		Constraints: map[string]any{
			"profile_params":          previewParamKeys,
			"native_profile_params":   routing.ProfileConfigOverrideParamKeys(),
			"route_preview_engine":    "brouter",
			"route_preview_profile":   previewProfile,
			"route_preview_no_save":   true,
			"route_preview_read_only": true,
		},
		Supports:            routingProfileSupports("badrouter", p.Source),
		SemanticPreferences: routingProfileSemanticPreferences(),
		ElevationSemantics:  routingProfileElevationSemantics(),
		ProfileVersion:      routingProfileVersion(p),
		Source:              p.Source,
	}
}

func routingProfileSupports(engine, source string) map[string]bool {
	return map[string]bool{
		"direct_route_preview": true,
		"route_suggestion":     false,
		"round_trip":           false,
		"imported_custom":      source != routing.SourceFilesystem && source != "system",
	}
}

func routingProfileSemanticPreferences() []string {
	return []string{
		"surface_groups",
		"highway_groups",
		"forest_share",
		"water_or_river_proximity",
		"protected_area",
		"traffic",
		"noise",
		"sac_scale",
		"mtb_scale",
	}
}

func routingProfileElevationSemantics() []string {
	return []string{
		"target_ascent_m",
		"ascent_tolerance_pct",
		"elevation_profile",
		"highest_point",
		"lowest_point",
		"max_grade",
		"ascent_per_km",
	}
}

func routingProfileVersion(p database.RoutingProfile) string {
	if strings.TrimSpace(p.ID) != "" {
		return p.Source + ":" + p.ID + ":" + p.UpdatedAt.UTC().Format(time.RFC3339Nano)
	}
	return p.Source + ":" + p.Name + ":" + p.BaseProfile
}

func filterRoutingProfilesForTool(profiles []routingProfileToolItem, args listRoutingProfilesArgs) []routingProfileToolItem {
	if strings.TrimSpace(args.SportQuery) == "" && strings.TrimSpace(args.Intent) == "" &&
		args.IsElectric == nil && args.IsIndoor == nil && args.IsVirtual == nil && args.IsCommute == nil {
		return profiles
	}
	sportType := ""
	if strings.TrimSpace(args.SportQuery) != "" {
		tuple, _, err := sport.NormalizeSportTuple(args.SportQuery, "")
		if err == nil {
			sportType = tuple.SportType
		}
	}
	out := make([]routingProfileToolItem, 0, len(profiles))
	for _, profile := range profiles {
		if sportType != "" && len(profile.SportTypes) > 0 && !routingProfileSupportsSport(profile, sportType) {
			continue
		}
		if strings.TrimSpace(args.Intent) == "goal" || strings.TrimSpace(args.Intent) == "round_trip" {
			if !profile.Supports["route_suggestion"] {
				profile.Constraints["unsupported_reason"] = "agent route-suggestion tool is not wired to the backend yet"
			}
		}
		out = append(out, profile)
	}
	return out
}

func routingProfileSupportsSport(profile routingProfileToolItem, sportType string) bool {
	for _, value := range profile.SportTypes {
		if value == sportType {
			return true
		}
	}
	return false
}

func resolveRoutingProfileForPreview(ctx context.Context, deps *Deps, userID, key string) (routingProfileToolItem, error) {
	key = strings.TrimSpace(key)
	if key == "" {
		return routingProfileToolItem{}, fmt.Errorf("profile is required; call list_routing_profiles first")
	}
	profiles, err := listRoutingProfilesForTool(ctx, deps, userID)
	if err != nil {
		return routingProfileToolItem{}, err
	}
	for _, profile := range profiles {
		if profile.Key == key || profile.ID == key || profile.Label == key {
			if profile.Engine == "brouter" {
				return profile, nil
			}
			mapped := mapBadRouterBaseProfileToBRouter(profile.BaseProfile)
			if mapped == "" {
				return routingProfileToolItem{}, fmt.Errorf("routing profile %q cannot be previewed through BRouter", key)
			}
			profile.BaseProfile = mapped
			return profile, nil
		}
	}
	return routingProfileToolItem{}, fmt.Errorf("unknown routing profile %q; call list_routing_profiles first", key)
}

func routingProfileDefaultParams(profile routingProfileToolItem) map[string]any {
	out := map[string]any{}
	if nested, ok := profile.DefaultParams["profile_params"].(map[string]any); ok {
		for key, value := range nested {
			out[key] = value
		}
		return out
	}
	allowed := map[string]bool{}
	for _, key := range routing.BRouterProfileParamKeys(profile.BaseProfile) {
		allowed[key] = true
	}
	for key, value := range profile.DefaultParams {
		if allowed[key] {
			out[key] = value
		}
	}
	return out
}

func mapBadRouterBaseProfileToBRouter(profile string) string {
	switch profile {
	case "hiking":
		return "hiking-mountain"
	case "mtb":
		return "cycling-mountain"
	case "gravel":
		return "cycling-gravel"
	case "road":
		return "cycling-road"
	case "trekking":
		return "trekking"
	default:
		return ""
	}
}

func stringifyProfileParams(params map[string]any) (map[string]string, error) {
	out := make(map[string]string, len(params))
	for key, value := range params {
		switch v := value.(type) {
		case string:
			out[key] = v
		case float64, bool:
			out[key] = fmt.Sprint(v)
		default:
			return nil, fmt.Errorf("profile_params.%s must be string, number, or boolean", key)
		}
	}
	return out, nil
}

func mapStringAny(values map[string]string) map[string]any {
	out := make(map[string]any, len(values))
	for key, value := range values {
		out[key] = value
	}
	return out
}
