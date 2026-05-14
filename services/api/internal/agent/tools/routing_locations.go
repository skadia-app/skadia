package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/geo"
)

type resolveUserLocationArgs struct {
	Query    string      `json:"query,omitempty"`
	Category string      `json:"category,omitempty"`
	Near     *routePoint `json:"near,omitempty"`
	Limit    *int        `json:"limit,omitempty"`
}

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
