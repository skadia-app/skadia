package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/geo"
	"github.com/skadia-app/skadia/services/api/internal/geosearch"
)

type resolveUserLocationArgs struct {
	Query    string      `json:"query,omitempty"`
	Category string      `json:"category,omitempty"`
	Near     *routePoint `json:"near,omitempty"`
	Limit    *int        `json:"limit,omitempty"`
}

type geocodePlaceArgs struct {
	Query   string      `json:"query"`
	Sources string      `json:"sources,omitempty"`
	Near    *routePoint `json:"near,omitempty"`
	BBox    *routeBBox  `json:"bbox,omitempty"`
	Limit   *int        `json:"limit,omitempty"`
}

var (
	defaultGeocodePlaceMu    sync.Mutex
	defaultGeocodePlaceCache *geosearch.Cache
	defaultGeocodePlaceQuota *geosearch.Limiter
)

func executeResolveUserLocation(ctx context.Context, deps *Deps, userID string, raw json.RawMessage) (any, error) {
	if strings.TrimSpace(userID) == "" {
		return nil, fmt.Errorf("resolve_user_location requires user_id")
	}
	var args resolveUserLocationArgs
	if err := json.Unmarshal(raw, &args); err != nil {
		return nil, fmt.Errorf("decode resolve_user_location args: %w", err)
	}
	category := strings.TrimSpace(args.Category)
	if category == "" {
		category = "standort"
	}
	limit := clampIntPtr(args.Limit, 5, 1, maxRouteResolverCandidates)
	query := strings.TrimSpace(args.Query)

	sql, params := buildResolveUserLocationSQL(userID, query, category, limit)
	rows, err := queryRows(ctx, deps, sql, params...)
	if err != nil {
		return nil, err
	}
	near, _ := resolveOptionalNearPointForUser(args.Near, userID)
	candidates := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		lat := numberValue(row["lat"])
		lng := numberValue(row["lng"])
		if near != nil {
			row["distance_m"] = geo.DistanceHaversine(near.Lat, near.Lng, lat, lng)
		}
		tagID := stringValue(row["source_id"])
		name := stringValue(row["name"])
		row["ref"] = encodeRouteResolverRef(routeResolverRef{
			Kind:      "location_tag",
			Source:    "user_location",
			SourceID:  tagID,
			UserID:    userID,
			Name:      name,
			Lat:       lat,
			Lng:       lng,
			CreatedAt: time.Now(),
		})
		row["confidence"] = routeResolverConfidence(1, numberValue(row["distance_m"]))
		row["source"] = "user_location"
		delete(row, "source_id")
		candidates = append(candidates, row)
	}
	sortRouteCandidateRows(candidates)
	if len(candidates) > limit {
		candidates = candidates[:limit]
	}

	missing, err := countUserLocationMatchesMissingCoordinates(ctx, deps, userID, query, category)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"query":                       query,
		"category":                    category,
		"candidates":                  candidates,
		"count":                       len(candidates),
		"ambiguous":                   len(candidates) > 1,
		"skipped_missing_coordinates": missing,
		"not_found_reason":            notFoundReason(len(candidates), "no matching user location with coordinates"),
	}, nil
}

func GeocodePlaceTool() *Tool {
	return &Tool{
		Name:        "geocode_place",
		Description: "Resolve user locations, imported map places, or optional external place names into route-point refs. Always check routing coverage before planning.",
		InputSchema: json.RawMessage(`{"type":"object","required":["query"],"properties":{"query":{"type":"string","minLength":2},"sources":{"type":"string","enum":["user_locations","local_map","external","all"],"default":"all"},"near":{"type":"object","properties":{"ref":{"type":"string"},"lat":{"type":"number"},"lng":{"type":"number"}},"additionalProperties":false},"bbox":{"type":"object","required":["min_lat","min_lng","max_lat","max_lng"],"properties":{"min_lat":{"type":"number"},"min_lng":{"type":"number"},"max_lat":{"type":"number"},"max_lng":{"type":"number"}},"additionalProperties":false},"limit":{"type":"integer","minimum":1,"maximum":10}},"additionalProperties":false}`),
		Permission:  PermissionRead,
		IsUserOwned: true,
		ExecuteRead: executeGeocodePlace,
	}
}

func executeGeocodePlace(ctx context.Context, deps *Deps, userID string, raw json.RawMessage) (any, error) {
	var args geocodePlaceArgs
	if err := json.Unmarshal(raw, &args); err != nil {
		return nil, fmt.Errorf("decode geocode_place args: %w", err)
	}
	query := strings.TrimSpace(args.Query)
	if len(query) < 2 {
		return nil, fmt.Errorf("geocode_place query must be at least 2 characters")
	}
	if args.BBox != nil {
		return nil, fmt.Errorf("geocode_place bbox is not supported yet; resolve a nearby point first")
	}
	sources := normalizeGeocodePlaceSources(args.Sources)
	limit := clampIntPtr(args.Limit, 5, 1, 10)
	near, err := resolveOptionalNearPointForUser(args.Near, userID)
	if err != nil {
		return nil, fmt.Errorf("invalid near point: %w", err)
	}

	candidates := []map[string]any{}
	if sources["user_locations"] && strings.TrimSpace(userID) != "" {
		userRaw, _ := json.Marshal(resolveUserLocationArgs{Query: query, Limit: &limit})
		userResult, err := executeResolveUserLocation(ctx, deps, userID, userRaw)
		if err != nil {
			return nil, err
		}
		if userMap, ok := userResult.(map[string]any); ok {
			if rows, ok := userMap["candidates"].([]map[string]any); ok {
				candidates = append(candidates, rows...)
			}
		}
	}

	searchResult := geosearch.SearchResult{
		Results:        []geosearch.Result{},
		LocalStatus:    geosearch.LocalStatusUnavailable,
		ExternalStatus: geosearch.ExternalStatusSkipped,
	}
	if sources["local_map"] || sources["external"] {
		var err error
		searchResult, err = geocodePlaceService(deps).Search(ctx, geosearch.SearchOptions{
			Query:           query,
			Type:            geosearchPlaceType(sources),
			Limit:           limit,
			Near:            geosearchPoint(near),
			UserID:          userID,
			IncludeExternal: sources["external"],
			PrivateInput:    sources["user_locations"] && len(candidates) > 0,
		})
		if err != nil {
			return nil, err
		}
		for _, result := range searchResult.Results {
			candidates = append(candidates, geocodePlaceCandidate(result))
		}
	}
	sortRouteCandidateRows(candidates)
	if len(candidates) > limit {
		candidates = candidates[:limit]
	}

	return map[string]any{
		"query":             query,
		"sources":           sourcesForGeocodeResult(sources),
		"candidates":        candidates,
		"count":             len(candidates),
		"ambiguous":         len(candidates) > 1,
		"not_found_reason":  notFoundReason(len(candidates), "no matching place"),
		"local_status":      searchResult.LocalStatus,
		"external_status":   searchResult.ExternalStatus,
		"rate_limit":        searchResult.Quota,
		"consent_request":   searchResult.ConsentRequest,
		"coverage_required": true,
	}, nil
}

func buildResolveUserLocationSQL(userID, query, category string, limit int) (string, []any) {
	params := []any{userID, category}
	where := []string{"user_id = $1", "category = $2", "lat IS NOT NULL", "lng IS NOT NULL"}
	if strings.TrimSpace(query) != "" {
		params = append(params, "%"+query+"%")
		where = append(where, fmt.Sprintf("name ILIKE $%d", len(params)))
	}
	params = append(params, limit)
	sql := fmt.Sprintf(`
SELECT id::text AS source_id, name, category, lat, lng
FROM tags
WHERE %s
ORDER BY name ASC
LIMIT $%d`, strings.Join(where, " AND "), len(params))
	return sql, params
}

func countUserLocationMatchesMissingCoordinates(ctx context.Context, deps *Deps, userID, query, category string) (int, error) {
	params := []any{userID, category}
	where := []string{"user_id = $1", "category = $2", "(lat IS NULL OR lng IS NULL)"}
	if strings.TrimSpace(query) != "" {
		params = append(params, "%"+query+"%")
		where = append(where, fmt.Sprintf("name ILIKE $%d", len(params)))
	}
	rows, err := queryRows(ctx, deps, "SELECT count(*) AS count FROM tags WHERE "+strings.Join(where, " AND "), params...)
	if err != nil {
		return 0, err
	}
	if len(rows) == 0 {
		return 0, nil
	}
	return int(numberValue(rows[0]["count"])), nil
}

func resolveOptionalNearPointForUser(point *routePoint, userID string) (*routeResolverRef, error) {
	if point == nil {
		return nil, nil
	}
	resolved, err := resolveRoutePointForUser(*point, userID)
	if err != nil {
		return nil, err
	}
	return &resolved, nil
}

func sortRouteCandidateRows(rows []map[string]any) {
	for i := 1; i < len(rows); i++ {
		for j := i; j > 0 && routeCandidateLess(rows[j], rows[j-1]); j-- {
			rows[j], rows[j-1] = rows[j-1], rows[j]
		}
	}
}

func routeCandidateLess(a, b map[string]any) bool {
	ad := numberValue(a["distance_m"])
	bd := numberValue(b["distance_m"])
	if ad > 0 || bd > 0 {
		if ad == 0 {
			return false
		}
		if bd == 0 {
			return true
		}
		return ad < bd
	}
	return strings.ToLower(stringValue(a["name"])) < strings.ToLower(stringValue(b["name"]))
}

func normalizeGeocodePlaceSources(raw string) map[string]bool {
	switch strings.TrimSpace(raw) {
	case "user_locations":
		return map[string]bool{"user_locations": true}
	case "local_map":
		return map[string]bool{"local_map": true}
	case "external":
		return map[string]bool{"external": true}
	default:
		return map[string]bool{"user_locations": true, "local_map": true, "external": true}
	}
}

func geosearchPoint(ref *routeResolverRef) *geosearch.Point {
	if ref == nil {
		return nil
	}
	return &geosearch.Point{Lat: ref.Lat, Lng: ref.Lng}
}

func geocodePlaceService(deps *Deps) *geosearch.Service {
	var local geosearch.LocalSearcher
	if queryer := geocodePlaceQueryer(deps); queryer != nil {
		local = geosearch.NewLocalDBSearcher(queryer)
	}
	return geosearch.NewService(geosearch.ServiceConfig{
		Local:    local,
		External: geocodePlaceExternal(deps),
		Policy:   geocodePlacePolicy(deps),
		Cache:    geocodePlaceCache(deps),
		Quota:    geocodePlaceQuota(deps),
	})
}

func geocodePlaceQueryer(deps *Deps) geosearch.Queryer {
	if deps == nil {
		return nil
	}
	if deps.Readonly != nil {
		return deps.Readonly
	}
	if deps.DB != nil && deps.DB.Pool != nil {
		return deps.DB.Pool
	}
	return nil
}

func geocodePlaceExternal(deps *Deps) geosearch.ForwardSearcher {
	if deps == nil {
		return nil
	}
	return deps.GeoSearchExternal
}

func geocodePlacePolicy(deps *Deps) geosearch.ExternalPolicy {
	if deps == nil || deps.GeoSearchPolicy == "" {
		return geosearch.ExternalPolicyDisabled
	}
	return deps.GeoSearchPolicy
}

func geocodePlaceCache(deps *Deps) *geosearch.Cache {
	if deps != nil && deps.GeoSearchCache != nil {
		return deps.GeoSearchCache
	}
	defaultGeocodePlaceMu.Lock()
	defer defaultGeocodePlaceMu.Unlock()
	if defaultGeocodePlaceCache == nil {
		defaultGeocodePlaceCache = geosearch.NewCache(10 * time.Minute)
	}
	return defaultGeocodePlaceCache
}

func geocodePlaceQuota(deps *Deps) *geosearch.Limiter {
	if deps != nil && deps.GeoSearchQuota != nil {
		return deps.GeoSearchQuota
	}
	defaultGeocodePlaceMu.Lock()
	defer defaultGeocodePlaceMu.Unlock()
	if defaultGeocodePlaceQuota == nil {
		defaultGeocodePlaceQuota = geosearch.NewLimiter(60, time.Hour, time.Now)
	}
	return defaultGeocodePlaceQuota
}

func geosearchPlaceType(sources map[string]bool) string {
	if sources["external"] && !sources["local_map"] {
		return "external"
	}
	return "all"
}

func geocodePlaceCandidate(result geosearch.Result) map[string]any {
	row := map[string]any{
		"ref": encodeRouteResolverRef(routeResolverRef{
			Kind:      result.Type,
			Source:    string(result.Source),
			Name:      result.Name,
			Lat:       result.Lat,
			Lng:       result.Lng,
			CreatedAt: time.Now(),
		}),
		"name":            result.Name,
		"kind":            result.Type,
		"subtype":         result.Subtype,
		"lat":             result.Lat,
		"lng":             result.Lng,
		"source":          string(result.Source),
		"confidence":      routeResolverConfidence(result.Similarity, optionalDistance(result.DistanceM)),
		"coverage_hint":   "check_get_routing_coverage",
		"external_result": result.Source == geosearch.SourceExternal,
	}
	if result.DistanceM != nil {
		row["distance_m"] = *result.DistanceM
	}
	return row
}

func optionalDistance(value *float64) float64 {
	if value == nil {
		return 0
	}
	return *value
}

func sourcesForGeocodeResult(sources map[string]bool) []string {
	out := []string{}
	for _, source := range []string{"user_locations", "local_map", "external"} {
		if sources[source] {
			out = append(out, source)
		}
	}
	return out
}
