package database

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/skadia-app/skadia/services/api/internal/model"
	"github.com/skadia-app/skadia/services/api/internal/routing"
	"github.com/skadia-app/skadia/services/api/internal/sport"
)

// ErrVersionMismatch is returned by UpdateRoute when the caller-supplied
// expectedVersion does not match the row's current `version` column.
// Handlers translate this into HTTP 412 Precondition Failed (RFC 7232).
// Pass expectedVersion = 0 to skip the optimistic-concurrency check
// (backwards-compatible path for clients that never observed an ETag).
var ErrVersionMismatch = errors.New("planned route version mismatch")

type RouteListFilters struct {
	SportType       string
	SubType         string
	Predicates      []sport.Predicate
	ModifierFilters sport.ModifierFilters
	Search          string
	Sort            string
}

func normalizedRouteFilterPredicates(filter RouteListFilters) ([]sport.Predicate, error) {
	if filter.Predicates != nil {
		return sport.NormalizePredicates(filter.Predicates)
	}
	if filter.SportType == "" {
		return nil, nil
	}
	pred := sport.Predicate{Sport: filter.SportType}
	if filter.SubType != "" {
		pred.SubType = &filter.SubType
	}
	return sport.NormalizePredicates([]sport.Predicate{pred})
}

func appendRouteListWhere(where []string, args []any, filter RouteListFilters) ([]string, []any, error) {
	preds, err := normalizedRouteFilterPredicates(filter)
	if err != nil {
		return nil, nil, err
	}
	if preds != nil {
		raw, err := json.Marshal(preds)
		if err != nil {
			return nil, nil, err
		}
		idx := len(args) + 1
		where = append(where, fmt.Sprintf(`EXISTS (
			SELECT 1
			FROM jsonb_array_elements($%d::jsonb) pred
			WHERE pred->>'sport' = sport_type
			  AND (NOT pred ? 'sub_type' OR pred->>'sub_type' = COALESCE(sub_type, ''))
		)`, idx))
		args = append(args, raw)
	}
	appendBool := func(column string, value *bool) {
		if value == nil {
			return
		}
		idx := len(args) + 1
		where = append(where, fmt.Sprintf("%s = $%d", column, idx))
		args = append(args, *value)
	}
	appendBool("is_electric", filter.ModifierFilters.IsElectric)
	appendBool("is_indoor", filter.ModifierFilters.IsIndoor)
	appendBool("is_virtual", filter.ModifierFilters.IsVirtual)
	appendBool("is_commute", filter.ModifierFilters.IsCommute)

	if search := strings.TrimSpace(filter.Search); search != "" {
		idx := len(args) + 1
		where = append(where, fmt.Sprintf("(name ILIKE '%%' || $%d || '%%' OR COALESCE(description, '') ILIKE '%%' || $%d || '%%')", idx, idx))
		args = append(args, search)
	}
	return where, args, nil
}

// t0SchemaVersion returns 1 if the route data satisfies T0 form-compliance
// (non-empty elevation_profile starting at dist_m=0 with monotonically increasing
// distances), otherwise 0. This is the sole source of truth for
// route_schema_version at write time — the frontend's claim is overridden.
func t0SchemaVersion(segments []model.RouteSegment, elevationProfile json.RawMessage) int16 {
	if routing.IsT0FormCompliant(len(segments), elevationProfile) {
		return 1
	}
	return 0
}

func normalizePlannedRouteSportTuple(sportType string, subType *string) (string, *string, sport.ModifierPatch, error) {
	rawSubType := ""
	if subType != nil {
		rawSubType = *subType
	}
	tuple, patch, err := sport.NormalizeSelectableSportTuple(sportType, rawSubType)
	if err != nil {
		return "", nil, sport.ModifierPatch{}, err
	}
	if tuple.SubType == "" {
		return tuple.SportType, nil, patch, nil
	}
	return tuple.SportType, &tuple.SubType, patch, nil
}

func applyPlannedRouteModifierPatch(route *model.PlannedRoute, patch sport.ModifierPatch) {
	if patch.IsElectric != nil && *patch.IsElectric {
		route.IsElectric = true
	}
	if patch.IsIndoor != nil && *patch.IsIndoor {
		route.IsIndoor = true
	}
	if patch.IsVirtual != nil && *patch.IsVirtual {
		route.IsVirtual = true
	}
}

func applyRouteImportModifierPatch(route *model.RouteImportData, patch sport.ModifierPatch) {
	if patch.IsElectric != nil && *patch.IsElectric {
		route.IsElectric = true
	}
	if patch.IsIndoor != nil && *patch.IsIndoor {
		route.IsIndoor = true
	}
	if patch.IsVirtual != nil && *patch.IsVirtual {
		route.IsVirtual = true
	}
}

// CreateRoute inserts a new planned route and returns its UUID.
func (db *DB) CreateRoute(ctx context.Context, route *model.PlannedRoute) (string, error) {
	return db.createRouteOn(ctx, db.Pool, route)
}

// CreateRouteTx inserts a planned route using an existing transaction.
func (db *DB) CreateRouteTx(ctx context.Context, tx pgx.Tx, route *model.PlannedRoute) (string, error) {
	return db.createRouteOn(ctx, tx, route)
}

func (db *DB) createRouteOn(ctx context.Context, q Querier, route *model.PlannedRoute) (string, error) {
	sportType, subType, patch, err := normalizePlannedRouteSportTuple(route.SportType, route.SubType)
	if err != nil {
		return "", fmt.Errorf("normalize route sport tuple: %w", err)
	}
	route.SportType = sportType
	route.SubType = subType
	applyPlannedRouteModifierPatch(route, patch)

	waypointsJSON, err := json.Marshal(route.Waypoints)
	if err != nil {
		return "", fmt.Errorf("marshal waypoints: %w", err)
	}
	segmentsJSON, err := json.Marshal(route.Segments)
	if err != nil {
		return "", fmt.Errorf("marshal segments: %w", err)
	}

	schemaVersion := t0SchemaVersion(route.Segments, route.ElevationProfile)

	var id string
	err = q.QueryRow(ctx, `
		INSERT INTO planned_routes (
			user_id, name, description, sport_type, sub_type,
			is_electric, is_indoor, is_virtual, is_commute,
			difficulty, profile,
			geometry, waypoints, segments,
			total_distance_m, total_elevation_gain, total_elevation_loss,
			estimated_time_min, time_override, auto_tag,
			elevation_profile, enrichment, routing_engine,
			route_schema_version
		) VALUES (
			$1, $2, $3, $4, $5,
			$6, $7, $8, $9,
			$10, $11,
			ST_Force2D(ST_GeomFromGeoJSON($12)), $13, $14,
			$15, $16, $17,
			$18, $19, $20,
			$21, $22, $23,
			$24
		)
		RETURNING id, version
	`,
		route.UserID, route.Name, route.Description, route.SportType, route.SubType,
		route.IsElectric, route.IsIndoor, route.IsVirtual, route.IsCommute,
		route.Difficulty, route.Profile,
		route.GeometryGeoJSON, waypointsJSON, segmentsJSON,
		route.TotalDistanceM, route.TotalElevationGain, route.TotalElevationLoss,
		route.EstimatedTimeMin, route.TimeOverride, route.AutoTag,
		route.ElevationProfile, route.Enrichment, route.RoutingEngine,
		schemaVersion,
	).Scan(&id, &route.Version)
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
		SELECT id, user_id, name, description, sport_type, sub_type,
			is_electric, is_indoor, is_virtual, is_commute,
			difficulty, profile,
			ST_AsGeoJSON(geometry) AS geometry_geojson,
			waypoints, segments,
			total_distance_m, total_elevation_gain, total_elevation_loss,
			estimated_time_min, time_override, auto_tag,
			created_at, updated_at,
			elevation_profile, enrichment, routing_engine,
			route_schema_version, version
		FROM planned_routes
		WHERE id = $1 AND user_id = $2
	`, id, userID).Scan(
		&route.ID, &route.UserID, &route.Name, &route.Description,
		&route.SportType, &route.SubType,
		&route.IsElectric, &route.IsIndoor, &route.IsVirtual, &route.IsCommute,
		&route.Difficulty, &route.Profile,
		&route.GeometryGeoJSON,
		&waypointsJSON, &segmentsJSON,
		&route.TotalDistanceM, &route.TotalElevationGain, &route.TotalElevationLoss,
		&route.EstimatedTimeMin, &route.TimeOverride, &route.AutoTag,
		&route.CreatedAt, &route.UpdatedAt,
		&route.ElevationProfile, &route.Enrichment, &route.RoutingEngine,
		&route.RouteSchemaVersion, &route.Version,
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
	photos, err := db.GetRoutePhotos(ctx, route.ID, userID)
	if err != nil {
		return nil, fmt.Errorf("get route photos: %w", err)
	}
	route.Photos = photos
	return route, nil
}

// ListRoutes returns paginated routes for a user, with simplified geometry for mini-map display.
// E13: geometry_simplified uses ST_Simplify(geometry, 0.001) to reduce payload size.
func (db *DB) ListRoutes(ctx context.Context, userID string, limit, offset int, filters ...RouteListFilters) ([]model.PlannedRoute, error) {
	if limit <= 0 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	filter := RouteListFilters{}
	if len(filters) > 0 {
		filter = filters[0]
	}
	orderBy := "updated_at DESC"
	switch filter.Sort {
	case "created_at":
		orderBy = "created_at DESC"
	case "name":
		orderBy = "name ASC, updated_at DESC"
	case "distance":
		orderBy = "total_distance_m DESC NULLS LAST, updated_at DESC"
	case "elevation":
		orderBy = "total_elevation_gain DESC NULLS LAST, updated_at DESC"
	}

	where := []string{"user_id = $1"}
	args := []any{userID, limit, offset}
	where, args, err := appendRouteListWhere(where, args, filter)
	if err != nil {
		return nil, fmt.Errorf("normalize route filters: %w", err)
	}

	query := fmt.Sprintf(`
		SELECT id, user_id, name, description, sport_type, sub_type, difficulty, profile,
			is_electric, is_indoor, is_virtual, is_commute,
			ST_AsGeoJSON(ST_Simplify(geometry, 0.001)) AS geometry_simplified,
			total_distance_m, total_elevation_gain, total_elevation_loss,
			estimated_time_min, time_override, auto_tag,
			created_at, updated_at,
			route_schema_version, version
		FROM planned_routes
		WHERE %s
		ORDER BY %s
		LIMIT $2 OFFSET $3
	`, strings.Join(where, " AND "), orderBy)
	rows, err := db.Pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list routes: %w", err)
	}
	defer rows.Close()

	var routes []model.PlannedRoute
	for rows.Next() {
		var r model.PlannedRoute
		if err := rows.Scan(
			&r.ID, &r.UserID, &r.Name, &r.Description,
			&r.SportType, &r.SubType, &r.Difficulty, &r.Profile,
			&r.IsElectric, &r.IsIndoor, &r.IsVirtual, &r.IsCommute,
			&r.GeometryGeoJSON,
			&r.TotalDistanceM, &r.TotalElevationGain, &r.TotalElevationLoss,
			&r.EstimatedTimeMin, &r.TimeOverride, &r.AutoTag,
			&r.CreatedAt, &r.UpdatedAt,
			&r.RouteSchemaVersion, &r.Version,
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
//
// Optimistic concurrency: when expectedVersion > 0 the WHERE clause adds
// `version = expectedVersion`. A RowsAffected = 0 result then has two
// possible causes — wrong owner / missing row, or version mismatch — which
// the function disambiguates with a follow-up ownership probe so the
// handler can return 404 vs. 412 correctly.
//
// Pass expectedVersion = 0 to skip the version check (legacy callers that
// never observed an ETag) — the row is updated unconditionally.
//
// On success, route.Version is updated to the new (incremented) value.
func (db *DB) UpdateRoute(ctx context.Context, route *model.PlannedRoute, expectedVersion int64) error {
	sportType, subType, patch, err := normalizePlannedRouteSportTuple(route.SportType, route.SubType)
	if err != nil {
		return fmt.Errorf("normalize route sport tuple: %w", err)
	}
	route.SportType = sportType
	route.SubType = subType
	applyPlannedRouteModifierPatch(route, patch)

	waypointsJSON, err := json.Marshal(route.Waypoints)
	if err != nil {
		return fmt.Errorf("marshal waypoints: %w", err)
	}
	segmentsJSON, err := json.Marshal(route.Segments)
	if err != nil {
		return fmt.Errorf("marshal segments: %w", err)
	}

	schemaVersion := t0SchemaVersion(route.Segments, route.ElevationProfile)

	// Build the WHERE-clause tail conditionally so the legacy path (no
	// ETag) keeps the previous semantics exactly.
	whereTail := ""
	args := []any{
		route.ID, route.UserID,
		route.Name, route.Description, route.SportType, route.SubType,
		route.IsElectric, route.IsIndoor, route.IsVirtual, route.IsCommute,
		route.Difficulty, route.Profile,
		route.GeometryGeoJSON, waypointsJSON, segmentsJSON,
		route.TotalDistanceM, route.TotalElevationGain, route.TotalElevationLoss,
		route.EstimatedTimeMin, route.TimeOverride, route.AutoTag,
		route.ElevationProfile, route.Enrichment, route.RoutingEngine,
		schemaVersion,
	}
	if expectedVersion > 0 {
		whereTail = " AND version = $26"
		args = append(args, expectedVersion)
	}

	query := `
		UPDATE planned_routes SET
			name               = $3,
			description        = $4,
			sport_type         = $5,
			sub_type           = $6,
			is_electric        = $7,
			is_indoor          = $8,
			is_virtual         = $9,
			is_commute         = $10,
			difficulty         = $11,
			profile            = $12,
			geometry           = ST_Force2D(ST_GeomFromGeoJSON($13)),
			waypoints          = $14,
			segments           = $15,
			total_distance_m   = $16,
			total_elevation_gain = $17,
			total_elevation_loss = $18,
			estimated_time_min = $19,
			time_override      = $20,
			auto_tag           = $21,
			elevation_profile  = $22,
			enrichment         = $23,
			routing_engine     = $24,
			route_schema_version = $25,
			version            = version + 1
		WHERE id = $1 AND user_id = $2` + whereTail + `
		RETURNING version
	`

	var newVersion int64
	err = db.Pool.QueryRow(ctx, query, args...).Scan(&newVersion)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// Disambiguate version-mismatch from missing/foreign row when
			// the optimistic path is in use. Without the probe a stale ETag
			// would surface as 404 and confuse the client.
			if expectedVersion > 0 {
				var dummy string
				probeErr := db.Pool.QueryRow(ctx,
					`SELECT id FROM planned_routes WHERE id = $1 AND user_id = $2`,
					route.ID, route.UserID,
				).Scan(&dummy)
				if probeErr == nil {
					return ErrVersionMismatch
				}
				if errors.Is(probeErr, pgx.ErrNoRows) {
					return pgx.ErrNoRows
				}
				return fmt.Errorf("update route ownership probe: %w", probeErr)
			}
			return pgx.ErrNoRows
		}
		return fmt.Errorf("update route: %w", err)
	}
	route.Version = newVersion
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
// route_schema_version is copied from the source row: a Pre-T0 duplicate remains
// Pre-T0 (triggering migration on next load) rather than being silently promoted.
func (db *DB) DuplicateRoute(ctx context.Context, id, userID string) (string, error) {
	var newID string
	err := db.Pool.QueryRow(ctx, `
		INSERT INTO planned_routes (
			user_id, name, description, sport_type, sub_type,
			is_electric, is_indoor, is_virtual, is_commute, difficulty,
			profile, geometry, waypoints, segments,
			total_distance_m, total_elevation_gain, total_elevation_loss,
			estimated_time_min, time_override, auto_tag,
			elevation_profile, enrichment, routing_engine,
			route_schema_version
		)
		SELECT
			user_id, name || ' (Kopie)', description, sport_type, sub_type,
			is_electric, is_indoor, is_virtual, is_commute, difficulty,
			profile, geometry, waypoints, segments,
			total_distance_m, total_elevation_gain, total_elevation_loss,
			estimated_time_min, time_override, auto_tag,
			elevation_profile, enrichment, routing_engine,
			route_schema_version
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
func (db *DB) CountRoutes(ctx context.Context, userID string, filters ...RouteListFilters) (int, error) {
	filter := RouteListFilters{}
	if len(filters) > 0 {
		filter = filters[0]
	}
	var count int
	where := []string{"user_id = $1"}
	args := []any{userID}
	var err error
	where, args, err = appendRouteListWhere(where, args, filter)
	if err != nil {
		return 0, fmt.Errorf("normalize route filters: %w", err)
	}
	query := fmt.Sprintf(`SELECT COUNT(*) FROM planned_routes WHERE %s`, strings.Join(where, " AND "))
	err = db.Pool.QueryRow(ctx, query, args...).Scan(&count)
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
			t.lat, t.lng
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
			&t.Lat, &t.Lng,
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
	if err != nil {
		return nil, err
	}
	return &id, nil
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
	if err != nil {
		return nil, err
	}
	return &id, nil
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
		       geometry
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
// Imported routes rarely carry a T0 elevation_profile, so route_schema_version is
// validated dynamically — importers that do supply a compliant profile get v=1.
func (db *DB) CreateRouteFromImport(ctx context.Context, userID string, r *model.RouteImportData) (string, error) {
	sportType, subType, patch, err := normalizePlannedRouteSportTuple(r.SportType, r.SubType)
	if err != nil {
		return "", fmt.Errorf("normalize route import sport tuple: %w", err)
	}
	r.SportType = sportType
	r.SubType = subType
	applyRouteImportModifierPatch(r, patch)

	// Decode the segments JSON to count segments for T0 validation.
	var segs []model.RouteSegment
	if r.SegmentsJSON != "" && r.SegmentsJSON != "[]" {
		_ = json.Unmarshal([]byte(r.SegmentsJSON), &segs) // best-effort; invalid JSON → empty slice → v=0
	}
	schemaVersion := t0SchemaVersion(segs, r.ElevationProfile)

	var id string
	err = db.Pool.QueryRow(ctx, `
		INSERT INTO planned_routes (
			user_id, name, description, sport_type, sub_type,
			is_electric, is_indoor, is_virtual, is_commute,
			profile,
			geometry, waypoints, segments,
			total_distance_m, total_elevation_gain, total_elevation_loss,
			source_platform, source_id, source_url,
			geometry_hash,
			route_schema_version
		) VALUES (
			$1, $2, $3, $4, $5,
			$6, $7, $8, $9,
			$10,
			ST_Force2D(ST_GeomFromGeoJSON($11)), $12, $13,
			$14, $15, $16,
			$17, $18, $19,
			encode(digest(ST_AsEWKB(ST_SnapToGrid(ST_Force2D(ST_GeomFromGeoJSON($11)), 0.00001)), 'sha256'), 'hex'),
			$20
		) RETURNING id`,
		userID, r.Name, r.Description, r.SportType, r.SubType,
		r.IsElectric, r.IsIndoor, r.IsVirtual, r.IsCommute,
		r.Profile,
		r.GeometryGeoJSON, r.WaypointsJSON, r.SegmentsJSON,
		r.TotalDistanceM, r.TotalElevationGain, r.TotalElevationLoss,
		r.SourcePlatform, r.SourceID, r.SourceURL,
		schemaVersion,
	).Scan(&id)
	if err != nil {
		return "", fmt.Errorf("create route from import: %w", err)
	}
	return id, nil
}

// GetKnownKomootRouteIDs returns the set of Komoot tour IDs already imported as
// planned routes for a user. Used to enable incremental sync (skip already-known tours).
func (db *DB) GetKnownKomootRouteIDs(ctx context.Context, userID string) (map[int64]bool, error) {
	rows, err := db.Pool.Query(ctx,
		`SELECT source_id FROM planned_routes
		 WHERE user_id = $1 AND source_platform = 'komoot' AND source_id IS NOT NULL`,
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := map[int64]bool{}
	for rows.Next() {
		var sid string
		if err := rows.Scan(&sid); err != nil {
			continue
		}
		id, err := strconv.ParseInt(sid, 10, 64)
		if err == nil {
			result[id] = true
		}
	}
	return result, rows.Err()
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
