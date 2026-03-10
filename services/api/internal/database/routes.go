package database

import (
	"context"
	"encoding/json"
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

// GetUserSpeedFactor computes the ratio between the user's actual moving speed
// and the Tobler baseline speed for the given sport type, averaged over the
// user's 20 most recent activities.
//
// A value of 1.0 means the user matches the Tobler hiking-function baseline
// exactly. Values > 1.0 indicate a faster-than-baseline athlete; values < 1.0
// indicate a slower one. If no qualifying activities are found, or if the
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
