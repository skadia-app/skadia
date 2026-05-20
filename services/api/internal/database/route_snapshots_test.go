package database

import (
	"context"
	"errors"
	"testing"

	"github.com/skadia-app/skadia/services/api/internal/model"
	"github.com/jackc/pgx/v5"
)

func TestCreatePlannedRouteSnapshotIsIdempotentAndVersionChecked(t *testing.T) {
	db, userID := routeSubTypeTestDB(t)
	ctx := context.Background()
	route := insertRouteSnapshotTestRoute(t, db, userID, "snapshot route")

	first, err := db.CreatePlannedRouteSnapshot(ctx, model.CreatePlannedRouteSnapshotInput{
		RouteID:            route.ID,
		UserID:             userID,
		SourceRouteVersion: route.Version,
		OperationID:        "snapshot-op-1",
		Reason:             "publish",
	})
	if err != nil {
		t.Fatalf("CreatePlannedRouteSnapshot first: %v", err)
	}
	if first.ID == "" || first.SnapshotHash == "" || first.GeometryHash == "" {
		t.Fatalf("snapshot missing stable ids/hashes: %#v", first)
	}

	second, err := db.CreatePlannedRouteSnapshot(ctx, model.CreatePlannedRouteSnapshotInput{
		RouteID:            route.ID,
		UserID:             userID,
		SourceRouteVersion: route.Version,
		OperationID:        "snapshot-op-1",
		Reason:             "publish",
	})
	if err != nil {
		t.Fatalf("CreatePlannedRouteSnapshot second: %v", err)
	}
	if second.ID != first.ID {
		t.Fatalf("idempotent snapshot id = %s, want %s", second.ID, first.ID)
	}

	loaded, err := db.GetRoute(ctx, route.ID, userID)
	if err != nil {
		t.Fatalf("GetRoute: %v", err)
	}
	loaded.Name = "snapshot route edited"
	if err := db.UpdateRoute(ctx, loaded, route.Version); err != nil {
		t.Fatalf("UpdateRoute: %v", err)
	}

	_, err = db.CreatePlannedRouteSnapshot(ctx, model.CreatePlannedRouteSnapshotInput{
		RouteID:            route.ID,
		UserID:             userID,
		SourceRouteVersion: route.Version,
		OperationID:        "snapshot-op-stale",
		Reason:             "publish",
	})
	if !errors.Is(err, ErrVersionMismatch) {
		t.Fatalf("stale snapshot error = %v, want ErrVersionMismatch", err)
	}

	current, err := db.GetRoute(ctx, route.ID, userID)
	if err != nil {
		t.Fatalf("GetRoute current: %v", err)
	}
	changed, err := db.CreatePlannedRouteSnapshot(ctx, model.CreatePlannedRouteSnapshotInput{
		RouteID:            route.ID,
		UserID:             userID,
		SourceRouteVersion: current.Version,
		OperationID:        "snapshot-op-2",
		Reason:             "publish",
	})
	if err != nil {
		t.Fatalf("CreatePlannedRouteSnapshot changed: %v", err)
	}
	if changed.ID == first.ID || changed.SnapshotHash == first.SnapshotHash {
		t.Fatalf("changed route reused snapshot: first=%#v changed=%#v", first, changed)
	}
}

func TestCreatePlannedRouteSnapshotRequiresOwnership(t *testing.T) {
	db, userID := routeSubTypeTestDB(t)
	otherUserID := insertObjectAccessTestUser(t, db)
	route := insertRouteSnapshotTestRoute(t, db, userID, "owned route")

	_, err := db.CreatePlannedRouteSnapshot(context.Background(), model.CreatePlannedRouteSnapshotInput{
		RouteID:            route.ID,
		UserID:             otherUserID,
		SourceRouteVersion: route.Version,
		OperationID:        "snapshot-op-other",
	})
	if !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("ownership error = %v, want pgx.ErrNoRows", err)
	}
}

func TestCreatePlannedRouteSnapshotSurvivesSourceRouteDeletion(t *testing.T) {
	db, userID := routeSubTypeTestDB(t)
	ctx := context.Background()
	route := insertRouteSnapshotTestRoute(t, db, userID, "deleted source route")

	snapshot, err := db.CreatePlannedRouteSnapshot(ctx, model.CreatePlannedRouteSnapshotInput{
		RouteID:            route.ID,
		UserID:             userID,
		SourceRouteVersion: route.Version,
		OperationID:        "snapshot-op-delete-source",
	})
	if err != nil {
		t.Fatalf("CreatePlannedRouteSnapshot: %v", err)
	}
	if err := db.DeleteRoute(ctx, route.ID, userID); err != nil {
		t.Fatalf("DeleteRoute: %v", err)
	}

	loaded, err := db.GetPlannedRouteSnapshot(ctx, snapshot.ID, userID)
	if err != nil {
		t.Fatalf("GetPlannedRouteSnapshot after route delete: %v", err)
	}
	if loaded.RouteID != nil {
		t.Fatalf("snapshot RouteID after route delete = %v, want nil", *loaded.RouteID)
	}
	if loaded.RouteName != route.Name || loaded.SnapshotHash != snapshot.SnapshotHash {
		t.Fatalf("snapshot changed after route delete: %#v", loaded)
	}
}

func insertRouteSnapshotTestRoute(t *testing.T, db *DB, userID, name string) *model.PlannedRoute {
	t.Helper()
	subType := "mountain"
	route := &model.PlannedRoute{
		UserID:          userID,
		Name:            name,
		SportType:       "cycling",
		SubType:         &subType,
		Profile:         "cycling-mountain",
		GeometryGeoJSON: `{"type":"LineString","coordinates":[[11.39,47.26],[11.40,47.27]]}`,
		Waypoints:       []model.RouteWaypoint{},
		Segments:        []model.RouteSegment{},
	}
	id, err := db.CreateRoute(context.Background(), route)
	if err != nil {
		t.Fatalf("CreateRoute: %v", err)
	}
	loaded, err := db.GetRoute(context.Background(), id, userID)
	if err != nil {
		t.Fatalf("GetRoute created route: %v", err)
	}
	return loaded
}
