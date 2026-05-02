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

// cleanupRoutes deletes all planned_routes for a test user.
func cleanupRoutes(t *testing.T, db *DB, userID string) {
	t.Helper()
	_, err := db.Pool.Exec(context.Background(),
		`DELETE FROM planned_routes WHERE user_id = $1`, userID)
	if err != nil {
		t.Fatalf("cleanup routes: %v", err)
	}
}

// minimalRoute returns a PlannedRoute with all required fields set for a unique test user.
func minimalRoute(userID string) *model.PlannedRoute {
	return &model.PlannedRoute{
		UserID:    userID,
		Name:      "Test Route",
		SportType: "hiking",
		Profile:   "outdoor",
		Waypoints: []model.RouteWaypoint{},
		Segments:  []model.RouteSegment{},
		// Geometry: a trivial linestring in Innsbruck
		GeometryGeoJSON: `{"type":"LineString","coordinates":[[11.39,47.26],[11.40,47.27]]}`,
	}
}

// TestCreateRoute_PersistElevationProfile verifies that a non-nil ElevationProfile
// is stored and retrieved correctly via CreateRoute + GetRoute.
func TestCreateRoute_PersistElevationProfile(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()

	userID := fmt.Sprintf("test-elev-profile-%d", time.Now().UnixNano())
	t.Cleanup(func() { cleanupRoutes(t, db, userID) })

	profile := json.RawMessage(`[{"dist_m":0,"ele_m":593},{"dist_m":100,"ele_m":650}]`)
	route := minimalRoute(userID)
	route.ElevationProfile = profile

	id, err := db.CreateRoute(ctx, route)
	if err != nil {
		t.Fatalf("CreateRoute: %v", err)
	}

	loaded, err := db.GetRoute(ctx, id, userID)
	if err != nil {
		t.Fatalf("GetRoute: %v", err)
	}

	var got, want any
	if err := json.Unmarshal(loaded.ElevationProfile, &got); err != nil {
		t.Fatalf("unmarshal loaded ElevationProfile: %v", err)
	}
	if err := json.Unmarshal(profile, &want); err != nil {
		t.Fatalf("unmarshal expected ElevationProfile: %v", err)
	}
	gotB, _ := json.Marshal(got)
	wantB, _ := json.Marshal(want)
	if string(gotB) != string(wantB) {
		t.Errorf("ElevationProfile mismatch:\n got  %s\n want %s", gotB, wantB)
	}
}

// TestCreateRoute_NullElevationProfile verifies that a nil ElevationProfile
// is stored as NULL and returns empty/nil after round-trip.
func TestCreateRoute_NullElevationProfile(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()

	userID := fmt.Sprintf("test-elev-null-%d", time.Now().UnixNano())
	t.Cleanup(func() { cleanupRoutes(t, db, userID) })

	route := minimalRoute(userID)
	// ElevationProfile is nil (zero value)

	id, err := db.CreateRoute(ctx, route)
	if err != nil {
		t.Fatalf("CreateRoute: %v", err)
	}

	loaded, err := db.GetRoute(ctx, id, userID)
	if err != nil {
		t.Fatalf("GetRoute: %v", err)
	}

	if len(loaded.ElevationProfile) != 0 {
		t.Errorf("expected nil/empty ElevationProfile, got %s", loaded.ElevationProfile)
	}
}

// TestCreateRoute_PersistEnrichment verifies that a non-nil Enrichment
// is stored and retrieved correctly via CreateRoute + GetRoute.
func TestCreateRoute_PersistEnrichment(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()

	userID := fmt.Sprintf("test-enrichment-%d", time.Now().UnixNano())
	t.Cleanup(func() { cleanupRoutes(t, db, userID) })

	enrich := json.RawMessage(`{"enrichment_version":1,"forest_pct":0.42}`)
	route := minimalRoute(userID)
	route.Enrichment = enrich

	id, err := db.CreateRoute(ctx, route)
	if err != nil {
		t.Fatalf("CreateRoute: %v", err)
	}

	loaded, err := db.GetRoute(ctx, id, userID)
	if err != nil {
		t.Fatalf("GetRoute: %v", err)
	}

	var got, want any
	if err := json.Unmarshal(loaded.Enrichment, &got); err != nil {
		t.Fatalf("unmarshal loaded Enrichment: %v", err)
	}
	if err := json.Unmarshal(enrich, &want); err != nil {
		t.Fatalf("unmarshal expected Enrichment: %v", err)
	}
	gotB, _ := json.Marshal(got)
	wantB, _ := json.Marshal(want)
	if string(gotB) != string(wantB) {
		t.Errorf("Enrichment mismatch:\n got  %s\n want %s", gotB, wantB)
	}
}

// TestMigration000325_UpDown verifies the column lifecycle for migration 000325
// (routes_persist_profile_enrichment): down removes elevation_profile + enrichment,
// up adds them, and the cycle is idempotent when repeated.
//
// Test order: down (idempotent) → up → assert columns present → down → assert
// columns absent → up → assert columns present (idempotent re-add).
//
// Critic finding #18 (NIT).
func TestMigration000325_UpDown(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()

	hasColumn := func(col string) bool {
		var exists bool
		err := db.Pool.QueryRow(ctx, `
			SELECT EXISTS(
				SELECT 1 FROM information_schema.columns
				WHERE table_name = 'planned_routes' AND column_name = $1
			)`, col).Scan(&exists)
		if err != nil {
			t.Fatalf("hasColumn(%q): %v", col, err)
		}
		return exists
	}

	runSQL := func(label, sql string) {
		t.Helper()
		if _, err := db.Pool.Exec(ctx, sql); err != nil {
			t.Fatalf("%s: %v", label, err)
		}
	}

	// Step 1: down (idempotent — IF EXISTS handles already-absent columns).
	runSQL("down", `ALTER TABLE planned_routes
		DROP COLUMN IF EXISTS elevation_profile,
		DROP COLUMN IF EXISTS enrichment`)

	if hasColumn("elevation_profile") {
		t.Error("elevation_profile still present after down — IF EXISTS did not work")
	}
	if hasColumn("enrichment") {
		t.Error("enrichment still present after down — IF EXISTS did not work")
	}

	// Step 2: up — must add both columns.
	runSQL("up", `ALTER TABLE planned_routes
		ADD COLUMN elevation_profile JSONB,
		ADD COLUMN enrichment        JSONB`)

	if !hasColumn("elevation_profile") {
		t.Error("elevation_profile absent after up")
	}
	if !hasColumn("enrichment") {
		t.Error("enrichment absent after up")
	}

	// Step 3: down again — must remove columns.
	runSQL("down again", `ALTER TABLE planned_routes
		DROP COLUMN IF EXISTS elevation_profile,
		DROP COLUMN IF EXISTS enrichment`)

	if hasColumn("elevation_profile") {
		t.Error("elevation_profile still present after second down")
	}
	if hasColumn("enrichment") {
		t.Error("enrichment still present after second down")
	}

	// Step 4: up again — idempotent re-add.
	runSQL("up again", `ALTER TABLE planned_routes
		ADD COLUMN elevation_profile JSONB,
		ADD COLUMN enrichment        JSONB`)

	if !hasColumn("elevation_profile") {
		t.Error("elevation_profile absent after second up — idempotency broken")
	}
	if !hasColumn("enrichment") {
		t.Error("enrichment absent after second up — idempotency broken")
	}
}

// TestUpdateRoute_PersistElevationProfile verifies that UpdateRoute persists
// ElevationProfile and Enrichment changes correctly.
func TestUpdateRoute_PersistElevationProfile(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()

	userID := fmt.Sprintf("test-update-elev-%d", time.Now().UnixNano())
	t.Cleanup(func() { cleanupRoutes(t, db, userID) })

	route := minimalRoute(userID)
	id, err := db.CreateRoute(ctx, route)
	if err != nil {
		t.Fatalf("CreateRoute: %v", err)
	}

	// Update with new elevation_profile and enrichment.
	profile := json.RawMessage(`[{"dist_m":0,"ele_m":700}]`)
	enrich := json.RawMessage(`{"enrichment_version":2,"forest_pct":0.1}`)

	route.ID = id
	route.ElevationProfile = profile
	route.Enrichment = enrich

	if err := db.UpdateRoute(ctx, route, 0); err != nil {
		t.Fatalf("UpdateRoute: %v", err)
	}

	loaded, err := db.GetRoute(ctx, id, userID)
	if err != nil {
		t.Fatalf("GetRoute after update: %v", err)
	}

	var gotP, wantP any
	if err := json.Unmarshal(loaded.ElevationProfile, &gotP); err != nil {
		t.Fatalf("unmarshal loaded ElevationProfile: %v", err)
	}
	if err := json.Unmarshal(profile, &wantP); err != nil {
		t.Fatalf("unmarshal expected ElevationProfile: %v", err)
	}
	gotB, _ := json.Marshal(gotP)
	wantB, _ := json.Marshal(wantP)
	if string(gotB) != string(wantB) {
		t.Errorf("ElevationProfile after update mismatch:\n got  %s\n want %s", gotB, wantB)
	}

	var gotE, wantE any
	if err := json.Unmarshal(loaded.Enrichment, &gotE); err != nil {
		t.Fatalf("unmarshal loaded Enrichment: %v", err)
	}
	if err := json.Unmarshal(enrich, &wantE); err != nil {
		t.Fatalf("unmarshal expected Enrichment: %v", err)
	}
	gotEB, _ := json.Marshal(gotE)
	wantEB, _ := json.Marshal(wantE)
	if string(gotEB) != string(wantEB) {
		t.Errorf("Enrichment after update mismatch:\n got  %s\n want %s", gotEB, wantEB)
	}
}
