package database

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/skadia-app/skadia/services/api/internal/model"
)

// CreateRoute inserts a new planned route and returns its UUID.
func (db *DB) CreateRoute(ctx context.Context, route *model.PlannedRoute) (string, error) {
	waypointsJSON, err := json.Marshal(route.Waypoints)
	if err != nil {
		return "", fmt.Errorf("marshal waypoints: %w", err)
	}
	segmentsJSON, err := json.Marshal(route.Segments)
	if err != nil {
		return "", fmt.Errorf("marshal segments: %w", err)
	}

	var id string
	err = db.Pool.QueryRow(ctx, `
		INSERT INTO planned_routes (
			user_id, name, description, sport_type, difficulty, profile,
			geometry, waypoints, segments,
			total_distance_m, total_elevation_gain, total_elevation_loss,
			estimated_time_min, time_override, auto_tag
		) VALUES (
			$1, $2, $3, $4, $5, $6,
			ST_GeomFromGeoJSON($7), $8, $9,
			$10, $11, $12,
			$13, $14, $15
		)
		RETURNING id
	`,
		route.UserID, route.Name, route.Description, route.SportType, route.Difficulty, route.Profile,
		route.GeometryGeoJSON, waypointsJSON, segmentsJSON,
		route.TotalDistanceM, route.TotalElevationGain, route.TotalElevationLoss,
		route.EstimatedTimeMin, route.TimeOverride, route.AutoTag,
	).Scan(&id)
	if err != nil {
		return "", fmt.Errorf("create route: %w", err)
	}
	return id, nil
}

// GetRoute fetches a single route by ID. Returns pgx.ErrNoRows if not found or ownership check fails.
func (db *DB) GetRoute(ctx context.Context, id, userID string) (*model.PlannedRoute, error) {
	route := &model.PlannedRoute{}
	var waypointsJSON, segmentsJSON []byte

	err := db.Pool.QueryRow(ctx, `
		SELECT id, user_id, name, description, sport_type, difficulty, profile,
			ST_AsGeoJSON(geometry) AS geometry_geojson,
			waypoints, segments,
			total_distance_m, total_elevation_gain, total_elevation_loss,
			estimated_time_min, time_override, auto_tag,
			created_at, updated_at
		FROM planned_routes
		WHERE id = $1 AND user_id = $2
	`, id, userID).Scan(
		&route.ID, &route.UserID, &route.Name, &route.Description,
		&route.SportType, &route.Difficulty, &route.Profile,
		&route.GeometryGeoJSON,
		&waypointsJSON, &segmentsJSON,
		&route.TotalDistanceM, &route.TotalElevationGain, &route.TotalElevationLoss,
		&route.EstimatedTimeMin, &route.TimeOverride, &route.AutoTag,
		&route.CreatedAt, &route.UpdatedAt,
	)
	if err != nil {
		return nil, err // caller checks pgx.ErrNoRows
	}

	if err := json.Unmarshal(waypointsJSON, &route.Waypoints); err != nil {
		return nil, fmt.Errorf("unmarshal waypoints: %w", err)
	}
	if err := json.Unmarshal(segmentsJSON, &route.Segments); err != nil {
		return nil, fmt.Errorf("unmarshal segments: %w", err)
	}
	return route, nil
}

// ListRoutes returns paginated routes for a user, with simplified geometry for mini-map display.
// E13: geometry_simplified uses ST_Simplify(geometry, 0.001) to reduce payload size.
func (db *DB) ListRoutes(ctx context.Context, userID string, limit, offset int) ([]model.PlannedRoute, error) {
	if limit <= 0 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}

	rows, err := db.Pool.Query(ctx, `
		SELECT id, user_id, name, description, sport_type, difficulty, profile,
			ST_AsGeoJSON(ST_Simplify(geometry, 0.001)) AS geometry_simplified,
			total_distance_m, total_elevation_gain, total_elevation_loss,
			estimated_time_min, time_override, auto_tag,
			created_at, updated_at
		FROM planned_routes
		WHERE user_id = $1
		ORDER BY updated_at DESC
		LIMIT $2 OFFSET $3
	`, userID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("list routes: %w", err)
	}
	defer rows.Close()

	var routes []model.PlannedRoute
	for rows.Next() {
		var r model.PlannedRoute
		if err := rows.Scan(
			&r.ID, &r.UserID, &r.Name, &r.Description,
			&r.SportType, &r.Difficulty, &r.Profile,
			&r.GeometryGeoJSON,
			&r.TotalDistanceM, &r.TotalElevationGain, &r.TotalElevationLoss,
			&r.EstimatedTimeMin, &r.TimeOverride, &r.AutoTag,
			&r.CreatedAt, &r.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan route: %w", err)
		}
		routes = append(routes, r)
	}
	// E15: check for iteration errors
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list routes: %w", err)
	}

	if routes == nil {
		routes = []model.PlannedRoute{}
	}
	return routes, nil
}

// UpdateRoute updates all mutable fields of a planned route.
// Ownership is enforced via WHERE user_id = $2.
func (db *DB) UpdateRoute(ctx context.Context, route *model.PlannedRoute) error {
	waypointsJSON, err := json.Marshal(route.Waypoints)
	if err != nil {
		return fmt.Errorf("marshal waypoints: %w", err)
	}
	segmentsJSON, err := json.Marshal(route.Segments)
	if err != nil {
		return fmt.Errorf("marshal segments: %w", err)
	}

	tag, err := db.Pool.Exec(ctx, `
		UPDATE planned_routes SET
			name               = $3,
			description        = $4,
			sport_type         = $5,
			difficulty         = $6,
			profile            = $7,
			geometry           = ST_GeomFromGeoJSON($8),
			waypoints          = $9,
			segments           = $10,
			total_distance_m   = $11,
			total_elevation_gain = $12,
			total_elevation_loss = $13,
			estimated_time_min = $14,
			time_override      = $15,
			auto_tag           = $16,
			updated_at         = NOW()
		WHERE id = $1 AND user_id = $2
	`,
		route.ID, route.UserID,
		route.Name, route.Description, route.SportType, route.Difficulty, route.Profile,
		route.GeometryGeoJSON, waypointsJSON, segmentsJSON,
		route.TotalDistanceM, route.TotalElevationGain, route.TotalElevationLoss,
		route.EstimatedTimeMin, route.TimeOverride, route.AutoTag,
	)
	if err != nil {
		return fmt.Errorf("update route: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

// DeleteRoute removes a route. Returns pgx.ErrNoRows if the route does not exist or belongs to another user.
func (db *DB) DeleteRoute(ctx context.Context, id, userID string) error {
	tag, err := db.Pool.Exec(ctx,
		"DELETE FROM planned_routes WHERE id = $1 AND user_id = $2",
		id, userID,
	)
	if err != nil {
		return fmt.Errorf("delete route: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

// DuplicateRoute creates a copy of an existing route owned by the same user.
// Returns the new route's UUID.
func (db *DB) DuplicateRoute(ctx context.Context, id, userID string) (string, error) {
	var newID string
	err := db.Pool.QueryRow(ctx, `
		INSERT INTO planned_routes (
			user_id, name, description, sport_type, difficulty,
			profile, geometry, waypoints, segments,
			total_distance_m, total_elevation_gain, total_elevation_loss,
			estimated_time_min, time_override, auto_tag
		)
		SELECT
			user_id, name || ' (Kopie)', description, sport_type, difficulty,
			profile, geometry, waypoints, segments,
			total_distance_m, total_elevation_gain, total_elevation_loss,
			estimated_time_min, time_override, auto_tag
		FROM planned_routes
		WHERE id = $1 AND user_id = $2
		RETURNING id
	`, id, userID).Scan(&newID)
	if err != nil {
		return "", fmt.Errorf("duplicate route: %w", err)
	}
	return newID, nil
}

// CountRoutes returns the total number of routes for a user (for pagination metadata).
func (db *DB) CountRoutes(ctx context.Context, userID string) (int, error) {
	var count int
	err := db.Pool.QueryRow(ctx,
		"SELECT COUNT(*) FROM planned_routes WHERE user_id = $1", userID,
	).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count routes: %w", err)
	}
	return count, nil
}

// GetRouteTags returns all tags assigned to the given route.
// Ownership is enforced implicitly: only the route owner can call handlers that
// call this function (ownership checked in the handler layer via GetRoute).
func (db *DB) GetRouteTags(ctx context.Context, routeID string) ([]model.Tag, error) {
	rows, err := db.Pool.Query(ctx, `
		SELECT t.id, t.user_id, t.name, t.color, t.text_color, t.category, t.is_system,
			t.is_auto, t.conditions, t.condition_groups, t.conditions_logic, t.description, t.activity_count, t.created_at,
			t.lat, t.lng, t.cover_image_url
		FROM tags t
		JOIN route_tags rt ON rt.tag_id = t.id
		WHERE rt.route_id = $1
		ORDER BY t.name ASC
	`, routeID)
	if err != nil {
		return nil, fmt.Errorf("get route tags: %w", err)
	}
	defer rows.Close()

	var tags []model.Tag
	for rows.Next() {
		var t model.Tag
		if err := rows.Scan(
			&t.ID, &t.UserID, &t.Name, &t.Color, &t.TextColor,
			&t.Category, &t.IsSystem, &t.IsAuto, &t.Conditions,
			&t.ConditionGroups, &t.ConditionsLogic,
			&t.Description, &t.ActivityCount, &t.CreatedAt,
			&t.Lat, &t.Lng, &t.CoverImageURL,
		); err != nil {
			return nil, fmt.Errorf("scan route tag: %w", err)
		}
		tags = append(tags, t)
	}
	// E15: check for iteration errors
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("get route tags: %w", err)
	}
	if tags == nil {
		tags = []model.Tag{}
	}
	return tags, nil
}

// SetRouteTags replaces all tags on a route with the given tag IDs (PUT semantics).
// Ownership of the route must be verified by the caller before invoking this function.
// Tags not owned by userID are silently ignored (they won't appear in the join).
func (db *DB) SetRouteTags(ctx context.Context, routeID, userID string, tagIDs []string) error {
	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("set route tags begin tx: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	// Delete all existing route_tags entries for this route.
	if _, err := tx.Exec(ctx,
		`DELETE FROM route_tags WHERE route_id = $1`, routeID,
	); err != nil {
		return fmt.Errorf("set route tags delete: %w", err)
	}

	// Insert the new set — only tags belonging to the user are accepted.
	for _, tagID := range tagIDs {
		if _, err := tx.Exec(ctx, `
			INSERT INTO route_tags (route_id, tag_id)
			SELECT $1, id FROM tags WHERE id = $2 AND user_id = $3
			ON CONFLICT DO NOTHING
		`, routeID, tagID, userID); err != nil {
			return fmt.Errorf("set route tags insert %s: %w", tagID, err)
		}
	}

	return tx.Commit(ctx)
}

// AssignRouteTag adds a single tag to a route.
// Silently succeeds if the tag is already present (ON CONFLICT DO NOTHING).
// Only tags owned by userID are accepted.
func (db *DB) AssignRouteTag(ctx context.Context, routeID, tagID, userID string) error {
	_, err := db.Pool.Exec(ctx, `
		INSERT INTO route_tags (route_id, tag_id)
		SELECT $1, id FROM tags WHERE id = $2 AND user_id = $3
		ON CONFLICT DO NOTHING
	`, routeID, tagID, userID)
	if err != nil {
		return fmt.Errorf("assign route tag: %w", err)
	}
	return nil
}

// RemoveRouteTag removes a single tag from a route.
// Returns nil if the tag was not present (idempotent).
func (db *DB) RemoveRouteTag(ctx context.Context, routeID, tagID string) error {
	_, err := db.Pool.Exec(ctx,
		`DELETE FROM route_tags WHERE route_id = $1 AND tag_id = $2`,
		routeID, tagID,
	)
	if err != nil {
		return fmt.Errorf("remove route tag: %w", err)
	}
	return nil
}

// PlannedRouteCandidate is used for Hausdorff-based route deduplication.
type PlannedRouteCandidate struct {
	ID             string
	Name           string
	TotalDistanceM *float64
	GeoJSON        string
}

// CheckRouteBySource checks if a route from this platform+id already exists.
// Returns the existing route's ID, or nil if not found.
func (db *DB) CheckRouteBySource(ctx context.Context, userID, platform, sourceID string) (*string, error) {
	var id string
	err := db.Pool.QueryRow(ctx,
		`SELECT id FROM planned_routes
		 WHERE user_id = $1 AND source_platform = $2 AND source_id = $3`,
		userID, platform, sourceID,
	).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return &id, err
}

// CheckRouteByGeometryHashFromGeoJSON computes the geometry hash in DB from GeoJSON and checks for duplicates.
// Hash is always computed in DB to avoid Go/DB float rounding inconsistency.
func (db *DB) CheckRouteByGeometryHashFromGeoJSON(ctx context.Context, userID, geojson string) (*string, error) {
	var id string
	err := db.Pool.QueryRow(ctx,
		`SELECT id FROM planned_routes
		 WHERE user_id = $1 AND geometry_hash = encode(
		     digest(ST_AsEWKB(ST_SnapToGrid(ST_GeomFromGeoJSON($2), 0.00001)), 'sha256'), 'hex')`,
		userID, geojson,
	).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return &id, err
}

// FindRouteCandidatesByBBox finds routes in the bounding box of a given geometry for Hausdorff comparison.
func (db *DB) FindRouteCandidatesByBBox(ctx context.Context, userID string, geojson string) ([]PlannedRouteCandidate, error) {
	rows, err := db.Pool.Query(ctx,
		`SELECT id, name, total_distance_m,
		        ST_AsGeoJSON(geometry) AS geojson
		 FROM planned_routes
		 WHERE user_id = $1
		   AND ST_Intersects(
		       ST_Expand(ST_GeomFromGeoJSON($2)::geometry, 0.01),
		       ST_GeomFromGeoJSON($2)
		   )`,
		userID, geojson,
	)
	if err != nil {
		return nil, fmt.Errorf("find route candidates: %w", err)
	}
	defer rows.Close()

	var candidates []PlannedRouteCandidate
	for rows.Next() {
		var c PlannedRouteCandidate
		if err := rows.Scan(&c.ID, &c.Name, &c.TotalDistanceM, &c.GeoJSON); err != nil {
			return nil, fmt.Errorf("scan route candidate: %w", err)
		}
		candidates = append(candidates, c)
	}
	return candidates, rows.Err()
}

// CreateRouteFromImport inserts a route with source tracking and DB-computed geometry hash.
func (db *DB) CreateRouteFromImport(ctx context.Context, userID string, r *model.RouteImportData) (string, error) {
	var id string
	err := db.Pool.QueryRow(ctx, `
		INSERT INTO planned_routes (
			user_id, name, description, sport_type, profile,
			geometry, waypoints, segments,
			total_distance_m, total_elevation_gain, total_elevation_loss,
			source_platform, source_id, source_url,
			geometry_hash
		) VALUES (
			$1, $2, $3, $4, $5,
			ST_GeomFromGeoJSON($6), $7, $8,
			$9, $10, $11,
			$12, $13, $14,
			encode(digest(ST_AsEWKB(ST_SnapToGrid(ST_GeomFromGeoJSON($6), 0.00001)), 'sha256'), 'hex')
		) RETURNING id`,
		userID, r.Name, r.Description, r.SportType, r.Profile,
		r.GeometryGeoJSON, r.WaypointsJSON, r.SegmentsJSON,
		r.TotalDistanceM, r.TotalElevationGain, r.TotalElevationLoss,
		r.SourcePlatform, r.SourceID, r.SourceURL,
	).Scan(&id)
	if err != nil {
		return "", fmt.Errorf("create route from import: %w", err)
	}
	return id, nil
}

// GetUserSpeedFactor computes the ratio between the user's actual moving speed
// and the Tobler baseline speed for the given sport type, averaged over the
// user's 20 most recent activities.
//
// A value of 1.0 means the user matches the Tobler hiking-function baseline
// exactly. Values > 1.0 indicate the user is slower than baseline (takes more
// time per km); values < 1.0 indicate a faster athlete. If no qualifying
// activities are found, or if the
// computed factor is non-positive, 1.0 is returned (safe fallback).
//
// The Tobler speed used here treats the average slope of each activity
// (net elevation change / distance) as a single representative gradient.
// This is an intentional simplification — segment-level slopes are applied
// during route-time estimation in the routing package.
func (db *DB) GetUserSpeedFactor(ctx context.Context, userID string, sportType string) (float64, error) {
	var factor float64
	err := db.Pool.QueryRow(ctx, `
		SELECT COALESCE(AVG(
			(moving_time_seconds / 60.0) /
			NULLIF(distance_m / (6.0 * EXP(-3.5 * ABS(
				(total_elevation_gain - total_elevation_loss) / NULLIF(distance_m, 0) + 0.05
			)) * 1000.0 / 60.0), 0)
		), 1.0) AS speed_factor
		FROM (
			SELECT moving_time_seconds, distance_m, total_elevation_gain, total_elevation_loss
			FROM activities
			WHERE user_id = $1 AND sport_type = $2 AND moving_time_seconds > 600
			ORDER BY started_at DESC LIMIT 20
		) recent
	`, userID, sportType).Scan(&factor)
	if err != nil || factor <= 0 {
		return 1.0, nil
	}
	return factor, nil
}
