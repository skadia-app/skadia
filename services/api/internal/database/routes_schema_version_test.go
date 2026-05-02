//go:build integration

package database

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/model"
)

// t0Profile returns a T0-compliant elevation profile JSON for use in tests.
func t0Profile() json.RawMessage {
	return json.RawMessage(`[{"dist_m":0,"ele_m":593},{"dist_m":100,"ele_m":650},{"dist_m":250,"ele_m":720}]`)
}

// t0Route returns a PlannedRoute with a T0-compliant elevation_profile (1 segment,
// profile starts at dist_m=0, monotonically increasing).
func t0Route(userID string) *model.PlannedRoute {
	r := minimalRoute(userID)
	r.Segments = []model.RouteSegment{
		{FromWaypoint: "wp0", ToWaypoint: "wp1", Mode: "routed", DistanceM: 250},
	}
	r.ElevationProfile = t0Profile()
	return r
}

// preT0Route returns a PlannedRoute without an elevation_profile (Pre-T0).
func preT0Route(userID string) *model.PlannedRoute {
	r := minimalRoute(userID)
	r.Segments = []model.RouteSegment{
		{FromWaypoint: "wp0", ToWaypoint: "wp1", Mode: "routed", DistanceM: 250},
	}
	// No ElevationProfile — will produce route_schema_version = 0.
	return r
}

// --- CreateRoute ---

// TestCreateRoute_T0Compliant_WritesV1 verifies that a route with a T0-compliant
// elevation_profile is stored with route_schema_version = 1.
func TestCreateRoute_T0Compliant_WritesV1(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()

	userID := fmt.Sprintf("test-schema-create-v1-%d", time.Now().UnixNano())
	t.Cleanup(func() { cleanupRoutes(t, db, userID) })

	id, err := db.CreateRoute(ctx, t0Route(userID))
	if err != nil {
		t.Fatalf("CreateRoute: %v", err)
	}

	loaded, err := db.GetRoute(ctx, id, userID)
	if err != nil {
		t.Fatalf("GetRoute: %v", err)
	}
	if loaded.RouteSchemaVersion != 1 {
		t.Errorf("expected RouteSchemaVersion = 1 for T0-compliant route, got %d", loaded.RouteSchemaVersion)
	}
}

// TestCreateRoute_PreT0Profile_WritesV0 verifies that a route without an
// elevation_profile (or with a non-compliant profile) is stored as version=0.
func TestCreateRoute_PreT0Profile_WritesV0(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()

	userID := fmt.Sprintf("test-schema-create-v0-%d", time.Now().UnixNano())
	t.Cleanup(func() { cleanupRoutes(t, db, userID) })

	id, err := db.CreateRoute(ctx, preT0Route(userID))
	if err != nil {
		t.Fatalf("CreateRoute: %v", err)
	}

	loaded, err := db.GetRoute(ctx, id, userID)
	if err != nil {
		t.Fatalf("GetRoute: %v", err)
	}
	if loaded.RouteSchemaVersion != 0 {
		t.Errorf("expected RouteSchemaVersion = 0 for Pre-T0 route (no profile), got %d", loaded.RouteSchemaVersion)
	}
}

// TestCreateRoute_NonZeroFirstDistM_WritesV0 verifies that a profile whose first
// dist_m is non-zero (approach-exclusive, Pre-T0 layout) yields version=0.
func TestCreateRoute_NonZeroFirstDistM_WritesV0(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()

	userID := fmt.Sprintf("test-schema-create-nozero-%d", time.Now().UnixNano())
	t.Cleanup(func() { cleanupRoutes(t, db, userID) })

	route := preT0Route(userID)
	// Profile that starts at dist_m > 0 — approach-exclusive Pre-T0 layout.
	route.ElevationProfile = json.RawMessage(`[{"dist_m":50,"ele_m":593},{"dist_m":150,"ele_m":650}]`)

	id, err := db.CreateRoute(ctx, route)
	if err != nil {
		t.Fatalf("CreateRoute: %v", err)
	}

	loaded, err := db.GetRoute(ctx, id, userID)
	if err != nil {
		t.Fatalf("GetRoute: %v", err)
	}
	if loaded.RouteSchemaVersion != 0 {
		t.Errorf("expected RouteSchemaVersion = 0 for non-zero first dist_m, got %d", loaded.RouteSchemaVersion)
	}
}

// --- UpdateRoute ---

// TestUpdateRoute_PostFixWritesV1 verifies that updating a Pre-T0 route and
// supplying a compliant elevation_profile promotes it to version=1.
func TestUpdateRoute_PostFixWritesV1(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()

	userID := fmt.Sprintf("test-schema-update-v1-%d", time.Now().UnixNano())
	t.Cleanup(func() { cleanupRoutes(t, db, userID) })

	// Insert Pre-T0 route (no profile) directly.
	var id string
	err := db.Pool.QueryRow(ctx, `
		INSERT INTO planned_routes (
			user_id, name, sport_type, profile,
			geometry, waypoints, segments,
			route_schema_version
		) VALUES (
			$1, 'Pre-T0', 'hiking', 'outdoor',
			ST_Force2D(ST_GeomFromGeoJSON('{"type":"LineString","coordinates":[[11.39,47.26],[11.40,47.27]]}')),
			'[]', '[]',
			0
		) RETURNING id`,
		userID,
	).Scan(&id)
	if err != nil {
		t.Fatalf("insert pre-T0 route: %v", err)
	}

	loaded, err := db.GetRoute(ctx, id, userID)
	if err != nil {
		t.Fatalf("GetRoute: %v", err)
	}
	if loaded.RouteSchemaVersion != 0 {
		t.Fatalf("precondition: expected version=0, got %d", loaded.RouteSchemaVersion)
	}

	// Add a segment and a T0-compliant profile.
	loaded.Segments = []model.RouteSegment{
		{FromWaypoint: "wp0", ToWaypoint: "wp1", Mode: "routed", DistanceM: 250},
	}
	loaded.ElevationProfile = t0Profile()

	if err := db.UpdateRoute(ctx, loaded, 0); err != nil {
		t.Fatalf("UpdateRoute: %v", err)
	}

	updated, err := db.GetRoute(ctx, id, userID)
	if err != nil {
		t.Fatalf("GetRoute after update: %v", err)
	}
	if updated.RouteSchemaVersion != 1 {
		t.Errorf("expected RouteSchemaVersion = 1 after supplying T0 profile, got %d", updated.RouteSchemaVersion)
	}
}

// TestUpdateRoute_PostFixIncomplete_WritesV0 verifies that updating a Pre-T0 route
// without supplying a compliant elevation_profile keeps version=0.
func TestUpdateRoute_PostFixIncomplete_WritesV0(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()

	userID := fmt.Sprintf("test-schema-update-v0-%d", time.Now().UnixNano())
	t.Cleanup(func() { cleanupRoutes(t, db, userID) })

	// Insert Pre-T0 route directly.
	var id string
	err := db.Pool.QueryRow(ctx, `
		INSERT INTO planned_routes (
			user_id, name, sport_type, profile,
			geometry, waypoints, segments,
			route_schema_version
		) VALUES (
			$1, 'Pre-T0 Incomplete', 'hiking', 'outdoor',
			ST_Force2D(ST_GeomFromGeoJSON('{"type":"LineString","coordinates":[[11.39,47.26],[11.40,47.27]]}')),
			'[]', '[]',
			0
		) RETURNING id`,
		userID,
	).Scan(&id)
	if err != nil {
		t.Fatalf("insert pre-T0 route: %v", err)
	}

	loaded, err := db.GetRoute(ctx, id, userID)
	if err != nil {
		t.Fatalf("GetRoute: %v", err)
	}

	// Mutate name but do NOT add profile — still Pre-T0.
	loaded.Name = "Updated but still Pre-T0"
	if err := db.UpdateRoute(ctx, loaded, 0); err != nil {
		t.Fatalf("UpdateRoute: %v", err)
	}

	updated, err := db.GetRoute(ctx, id, userID)
	if err != nil {
		t.Fatalf("GetRoute after update: %v", err)
	}
	if updated.RouteSchemaVersion != 0 {
		t.Errorf("expected RouteSchemaVersion = 0 (no T0 profile supplied), got %d", updated.RouteSchemaVersion)
	}
}

// --- DuplicateRoute ---

// TestDuplicateRoute_PreT0_KeepsV0 verifies that duplicating a Pre-T0 route (v=0)
// produces a copy with version=0, not silently promoted to 1.
func TestDuplicateRoute_PreT0_KeepsV0(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()

	userID := fmt.Sprintf("test-schema-dup-v0-%d", time.Now().UnixNano())
	t.Cleanup(func() { cleanupRoutes(t, db, userID) })

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
	if dup.RouteSchemaVersion != 0 {
		t.Errorf("expected duplicate RouteSchemaVersion = 0 (copied from Pre-T0 source), got %d", dup.RouteSchemaVersion)
	}
}

// TestDuplicateRoute_T0_KeepsV1 verifies that duplicating a T0 route (v=1) preserves v=1.
func TestDuplicateRoute_T0_KeepsV1(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()

	userID := fmt.Sprintf("test-schema-dup-v1-%d", time.Now().UnixNano())
	t.Cleanup(func() { cleanupRoutes(t, db, userID) })

	srcID, err := db.CreateRoute(ctx, t0Route(userID))
	if err != nil {
		t.Fatalf("CreateRoute T0 source: %v", err)
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
		t.Errorf("expected duplicate RouteSchemaVersion = 1 (copied from T0 source), got %d", dup.RouteSchemaVersion)
	}
}

// --- CreateRouteFromImport ---

// TestCreateRouteFromImport_T0Compliant_WritesV1 verifies that an importer that
// supplies a T0-compliant elevation_profile gets version=1.
func TestCreateRouteFromImport_T0Compliant_WritesV1(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()

	userID := fmt.Sprintf("test-schema-import-v1-%d", time.Now().UnixNano())
	t.Cleanup(func() { cleanupRoutes(t, db, userID) })

	platform := "komoot"
	sourceID := fmt.Sprintf("k-%d", time.Now().UnixNano())
	importData := &model.RouteImportData{
		Name:            "Imported T0 Route",
		SportType:       "hiking",
		Profile:         "outdoor",
		GeometryGeoJSON: `{"type":"LineString","coordinates":[[11.39,47.26],[11.40,47.27]]}`,
		WaypointsJSON:   "[]",
		SegmentsJSON:    `[{"from_waypoint":"wp0","to_waypoint":"wp1","mode":"routed","distance_m":250}]`,
		SourcePlatform:  &platform,
		SourceID:        &sourceID,
		ElevationProfile: t0Profile(),
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
		t.Errorf("expected RouteSchemaVersion = 1 for T0-compliant import, got %d", loaded.RouteSchemaVersion)
	}
}

// TestCreateRouteFromImport_NoProfile_WritesV0 verifies that imports without
// an elevation_profile (the common case) are stored as version=0.
func TestCreateRouteFromImport_NoProfile_WritesV0(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()

	userID := fmt.Sprintf("test-schema-import-v0-%d", time.Now().UnixNano())
	t.Cleanup(func() { cleanupRoutes(t, db, userID) })

	platform := "garmin"
	sourceID := fmt.Sprintf("g-%d", time.Now().UnixNano())
	importData := &model.RouteImportData{
		Name:            "Garmin Import (no profile)",
		SportType:       "cycling",
		Profile:         "outdoor",
		GeometryGeoJSON: `{"type":"LineString","coordinates":[[11.39,47.26],[11.40,47.27]]}`,
		WaypointsJSON:   "[]",
		SegmentsJSON:    "[]",
		SourcePlatform:  &platform,
		SourceID:        &sourceID,
		// No ElevationProfile — matches all real importers today.
	}

	id, err := db.CreateRouteFromImport(ctx, userID, importData)
	if err != nil {
		t.Fatalf("CreateRouteFromImport: %v", err)
	}

	loaded, err := db.GetRoute(ctx, id, userID)
	if err != nil {
		t.Fatalf("GetRoute: %v", err)
	}
	if loaded.RouteSchemaVersion != 0 {
		t.Errorf("expected RouteSchemaVersion = 0 for import without profile, got %d", loaded.RouteSchemaVersion)
	}
}

// --- Read-path pass-through ---

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
	v1Route := t0Route(userID)
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

	versionByName := make(map[string]int16)
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

// --- Self-tracking DB probe (Finding #4) ---

// TestMigrationPathStillReachable_DBProbe queries the real database to count
// planned_routes with route_schema_version = 0 (Pre-T0). When that count reaches
// zero, the test fails with explicit removal instructions for the migration code
// path. This is the backend counterpart to the frontend needsMigration predicate
// test — it is the *real* self-tracking signal tied to actual production data.
//
// Run with:
//
//	go test -tags=integration ./internal/database/ -run TestMigrationPathStillReachable_DBProbe
func TestMigrationPathStillReachable_DBProbe(t *testing.T) {
	db := newTestDB(t)

	var preT0Count int
	err := db.Pool.QueryRow(t.Context(),
		`SELECT COUNT(*) FROM planned_routes WHERE route_schema_version = 0`,
	).Scan(&preT0Count)
	if err != nil {
		t.Fatalf("count query: %v", err)
	}

	if preT0Count == 0 {
		t.Fatalf(`No Pre-T0 routes remain in the database (count=0).
This means the migration code path is no longer reachable in production.
ACTION REQUIRED:
  1. Remove migrateRoute.ts and migrateRouteCached in frontend/src/hooks/migrateRoute.ts
  2. Remove the migration branch in useRoutePlanner.ts loadRoute
  3. Remove fillTokens / lazyFillToken / lazyFillPendingSave fields from planner state
  4. Remove this test (TestMigrationPathStillReachable_DBProbe) and the frontend
     TestMigrationPathStillReachable predicate test
  5. Update docs/_internal/STATE.md self-tracking entry to "Removed"
The branch state in DB confirms the frontend migration path cleanup is safe.`)
	}
	t.Logf("Migration path still needed (Pre-T0 route count = %d).", preT0Count)
}
