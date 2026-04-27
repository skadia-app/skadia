//go:build integration

package database

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/model"
)

// TestCreateRoute_SetsSchemaVersion1 verifies that CreateRoute writes
// route_schema_version = 1 regardless of the zero value in the struct.
func TestCreateRoute_SetsSchemaVersion1(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()

	userID := fmt.Sprintf("test-schema-create-%d", time.Now().UnixNano())
	t.Cleanup(func() { cleanupRoutes(t, db, userID) })

	route := minimalRoute(userID)
	// RouteSchemaVersion is 0 (zero value) — backend must override with 1.

	id, err := db.CreateRoute(ctx, route)
	if err != nil {
		t.Fatalf("CreateRoute: %v", err)
	}

	loaded, err := db.GetRoute(ctx, id, userID)
	if err != nil {
		t.Fatalf("GetRoute: %v", err)
	}

	if loaded.RouteSchemaVersion != 1 {
		t.Errorf("expected RouteSchemaVersion = 1 after CreateRoute, got %d", loaded.RouteSchemaVersion)
	}
}

// TestUpdateRoute_PromotesPreT0SchemaVersion verifies that UpdateRoute promotes
// a Pre-T0 route (version=0, inserted directly via SQL) to version=1.
func TestUpdateRoute_PromotesPreT0SchemaVersion(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()

	userID := fmt.Sprintf("test-schema-update-%d", time.Now().UnixNano())
	t.Cleanup(func() { cleanupRoutes(t, db, userID) })

	// Insert a Pre-T0 route with route_schema_version = 0 directly via SQL.
	var id string
	err := db.Pool.QueryRow(ctx, `
		INSERT INTO planned_routes (
			user_id, name, sport_type, profile,
			geometry, waypoints, segments,
			route_schema_version
		) VALUES (
			$1, 'Pre-T0 Route', 'hiking', 'outdoor',
			ST_Force2D(ST_GeomFromGeoJSON('{"type":"LineString","coordinates":[[11.39,47.26],[11.40,47.27]]}')),
			'[]', '[]',
			0
		) RETURNING id`,
		userID,
	).Scan(&id)
	if err != nil {
		t.Fatalf("insert pre-T0 route: %v", err)
	}

	// Load, mutate, and save via UpdateRoute.
	loaded, err := db.GetRoute(ctx, id, userID)
	if err != nil {
		t.Fatalf("GetRoute: %v", err)
	}
	if loaded.RouteSchemaVersion != 0 {
		t.Fatalf("precondition: expected RouteSchemaVersion = 0, got %d", loaded.RouteSchemaVersion)
	}

	if err := db.UpdateRoute(ctx, loaded); err != nil {
		t.Fatalf("UpdateRoute: %v", err)
	}

	updated, err := db.GetRoute(ctx, id, userID)
	if err != nil {
		t.Fatalf("GetRoute after update: %v", err)
	}
	if updated.RouteSchemaVersion != 1 {
		t.Errorf("expected RouteSchemaVersion = 1 after UpdateRoute, got %d", updated.RouteSchemaVersion)
	}
}

// TestDuplicateRoute_ResetsSchemaVersionTo1 verifies that DuplicateRoute writes
// route_schema_version = 1 even when the source route has version = 0.
func TestDuplicateRoute_ResetsSchemaVersionTo1(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()

	userID := fmt.Sprintf("test-schema-dup-%d", time.Now().UnixNano())
	t.Cleanup(func() { cleanupRoutes(t, db, userID) })

	// Insert a Pre-T0 source route with route_schema_version = 0.
	var srcID string
	err := db.Pool.QueryRow(ctx, `
		INSERT INTO planned_routes (
			user_id, name, sport_type, profile,
			geometry, waypoints, segments,
			route_schema_version
		) VALUES (
			$1, 'Source Pre-T0', 'hiking', 'outdoor',
			ST_Force2D(ST_GeomFromGeoJSON('{"type":"LineString","coordinates":[[11.39,47.26],[11.40,47.27]]}')),
			'[]', '[]',
			0
		) RETURNING id`,
		userID,
	).Scan(&srcID)
	if err != nil {
		t.Fatalf("insert source route: %v", err)
	}

	dupID, err := db.DuplicateRoute(ctx, srcID, userID)
	if err != nil {
		t.Fatalf("DuplicateRoute: %v", err)
	}

	dup, err := db.GetRoute(ctx, dupID, userID)
	if err != nil {
		t.Fatalf("GetRoute for duplicate: %v", err)
	}
	if dup.RouteSchemaVersion != 1 {
		t.Errorf("expected duplicate RouteSchemaVersion = 1, got %d (must not propagate source version=0)", dup.RouteSchemaVersion)
	}
}

// TestCreateRouteFromImport_SetsSchemaVersion1 verifies that CreateRouteFromImport
// writes route_schema_version = 1.
func TestCreateRouteFromImport_SetsSchemaVersion1(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()

	userID := fmt.Sprintf("test-schema-import-%d", time.Now().UnixNano())
	t.Cleanup(func() { cleanupRoutes(t, db, userID) })

	platform := "komoot"
	sourceID := fmt.Sprintf("k-%d", time.Now().UnixNano())
	importData := &model.RouteImportData{
		Name:            "Imported Route",
		SportType:       "hiking",
		Profile:         "outdoor",
		GeometryGeoJSON: `{"type":"LineString","coordinates":[[11.39,47.26],[11.40,47.27]]}`,
		WaypointsJSON:   "[]",
		SegmentsJSON:    "[]",
		SourcePlatform:  &platform,
		SourceID:        &sourceID,
	}

	id, err := db.CreateRouteFromImport(ctx, userID, importData)
	if err != nil {
		t.Fatalf("CreateRouteFromImport: %v", err)
	}

	loaded, err := db.GetRoute(ctx, id, userID)
	if err != nil {
		t.Fatalf("GetRoute: %v", err)
	}
	if loaded.RouteSchemaVersion != 1 {
		t.Errorf("expected RouteSchemaVersion = 1 after CreateRouteFromImport, got %d", loaded.RouteSchemaVersion)
	}
}

// TestListRoutes_ReturnsSchemaVersion verifies that ListRoutes passes through
// route_schema_version correctly for mixed pre-T0 (v=0) and T0 (v=1) routes.
func TestListRoutes_ReturnsSchemaVersion(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()

	userID := fmt.Sprintf("test-schema-list-%d", time.Now().UnixNano())
	t.Cleanup(func() { cleanupRoutes(t, db, userID) })

	// Insert pre-T0 route with version=0 directly via SQL.
	err := db.Pool.QueryRow(ctx, `
		INSERT INTO planned_routes (
			user_id, name, sport_type, profile,
			geometry, waypoints, segments,
			route_schema_version
		) VALUES (
			$1, 'Pre-T0 List', 'hiking', 'outdoor',
			ST_Force2D(ST_GeomFromGeoJSON('{"type":"LineString","coordinates":[[11.39,47.26],[11.40,47.27]]}')),
			'[]', '[]',
			0
		) RETURNING id`,
		userID,
	).Scan(new(string))
	if err != nil {
		t.Fatalf("insert v=0 route: %v", err)
	}

	// Insert T0 route (version=1) via CreateRoute.
	v1Route := minimalRoute(userID)
	v1Route.Name = "T0 List"
	if _, err := db.CreateRoute(ctx, v1Route); err != nil {
		t.Fatalf("CreateRoute v=1: %v", err)
	}

	routes, err := db.ListRoutes(ctx, userID, 10, 0)
	if err != nil {
		t.Fatalf("ListRoutes: %v", err)
	}
	if len(routes) < 2 {
		t.Fatalf("expected at least 2 routes, got %d", len(routes))
	}

	versionByName := make(map[string]int)
	for _, r := range routes {
		versionByName[r.Name] = r.RouteSchemaVersion
	}
	if v, ok := versionByName["Pre-T0 List"]; !ok || v != 0 {
		t.Errorf("Pre-T0 List: want RouteSchemaVersion=0, got %d (found=%v)", v, ok)
	}
	if v, ok := versionByName["T0 List"]; !ok || v != 1 {
		t.Errorf("T0 List: want RouteSchemaVersion=1, got %d (found=%v)", v, ok)
	}
}

// TestGetPlannedRoute_ReturnsSchemaVersion verifies that GetRoute correctly reads
// and returns route_schema_version from the database row.
func TestGetPlannedRoute_ReturnsSchemaVersion(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()

	userID := fmt.Sprintf("test-schema-get-%d", time.Now().UnixNano())
	t.Cleanup(func() { cleanupRoutes(t, db, userID) })

	// Insert with explicit version=0 (Pre-T0 simulation) directly.
	var id string
	err := db.Pool.QueryRow(ctx, `
		INSERT INTO planned_routes (
			user_id, name, sport_type, profile,
			geometry, waypoints, segments,
			route_schema_version
		) VALUES (
			$1, 'Version Check', 'hiking', 'outdoor',
			ST_Force2D(ST_GeomFromGeoJSON('{"type":"LineString","coordinates":[[11.39,47.26],[11.40,47.27]]}')),
			'[]', '[]',
			0
		) RETURNING id`,
		userID,
	).Scan(&id)
	if err != nil {
		t.Fatalf("insert route: %v", err)
	}

	loaded, err := db.GetRoute(ctx, id, userID)
	if err != nil {
		t.Fatalf("GetRoute: %v", err)
	}
	if loaded.RouteSchemaVersion != 0 {
		t.Errorf("expected RouteSchemaVersion = 0 for pre-T0 row, got %d", loaded.RouteSchemaVersion)
	}
}
