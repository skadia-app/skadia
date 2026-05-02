package database

import (
	"context"
	"github.com/skadia-app/skadia/services/api/internal/database/testdb"
	"testing"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/model"
	"github.com/skadia-app/skadia/services/api/internal/sport"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func routeSubTypeTestDB(t *testing.T) (*DB, string) {
	t.Helper()
	dsn := testdb.RequireSafeDSN(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("pgxpool.New: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	var hasColumn bool
	if err := pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM information_schema.columns
			WHERE table_name = 'planned_routes' AND column_name = 'sub_type'
		)`,
	).Scan(&hasColumn); err != nil {
		t.Fatalf("check planned_routes.sub_type: %v", err)
	}
	if !hasColumn {
		t.Skip("planned_routes.sub_type not migrated")
	}

	userID := uuid.NewString()
	if _, err := pool.Exec(ctx, `
		INSERT INTO users (id, username, email, email_verified, registration_source)
		VALUES ($1, $2, $3, true, 'test')`,
		userID, "route-subtype-"+userID[:8], "route-subtype-"+userID[:8]+"@example.com"); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM planned_routes WHERE user_id = $1`, userID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, userID)
	})

	return &DB{Pool: pool}, userID
}

func TestPlannedRouteSubTypeRoundTrip(t *testing.T) {
	db, userID := routeSubTypeTestDB(t)
	ctx := context.Background()
	subType := "mountain"

	route := &model.PlannedRoute{
		UserID:          userID,
		Name:            "MTB route",
		SportType:       "cycling",
		SubType:         &subType,
		Profile:         "cycling-mountain",
		GeometryGeoJSON: `{"type":"LineString","coordinates":[[11.39,47.26],[11.40,47.27]]}`,
		Waypoints:       []model.RouteWaypoint{},
		Segments:        []model.RouteSegment{},
	}

	id, err := db.CreateRoute(ctx, route)
	if err != nil {
		t.Fatalf("CreateRoute: %v", err)
	}
	loaded, err := db.GetRoute(ctx, id, userID)
	if err != nil {
		t.Fatalf("GetRoute: %v", err)
	}
	if loaded.SubType == nil || *loaded.SubType != "mountain" {
		t.Fatalf("SubType = %v, want mountain", loaded.SubType)
	}

	loaded.SubType = nil
	if err := db.UpdateRoute(ctx, loaded, 0); err != nil {
		t.Fatalf("UpdateRoute clear subtype: %v", err)
	}
	cleared, err := db.GetRoute(ctx, id, userID)
	if err != nil {
		t.Fatalf("GetRoute after clear: %v", err)
	}
	if cleared.SubType != nil {
		t.Fatalf("SubType after clear = %q, want nil", *cleared.SubType)
	}
}

func TestPlannedRouteMetadataAndModifiersRoundTrip(t *testing.T) {
	db, userID := routeSubTypeTestDB(t)
	ctx := context.Background()
	subType := "mountain"
	difficulty := "MTB S2"

	route := &model.PlannedRoute{
		UserID:          userID,
		Name:            "metadata route",
		SportType:       "cycling",
		SubType:         &subType,
		IsElectric:      true,
		IsIndoor:        false,
		IsVirtual:       true,
		IsCommute:       true,
		Difficulty:      &difficulty,
		Profile:         "mtb",
		GeometryGeoJSON: `{"type":"LineString","coordinates":[[11.39,47.26],[11.40,47.27]]}`,
		Waypoints:       []model.RouteWaypoint{},
		Segments:        []model.RouteSegment{},
	}

	id, err := db.CreateRoute(ctx, route)
	if err != nil {
		t.Fatalf("CreateRoute: %v", err)
	}

	assertMetadata := func(t *testing.T, got *model.PlannedRoute, wantDifficulty, wantProfile string, wantElectric, wantIndoor, wantVirtual, wantCommute bool) {
		t.Helper()
		if got.Difficulty == nil || *got.Difficulty != wantDifficulty {
			t.Fatalf("Difficulty = %v, want %q", got.Difficulty, wantDifficulty)
		}
		if got.Profile != wantProfile {
			t.Fatalf("Profile = %q, want %q", got.Profile, wantProfile)
		}
		if got.IsElectric != wantElectric || got.IsIndoor != wantIndoor || got.IsVirtual != wantVirtual || got.IsCommute != wantCommute {
			t.Fatalf("modifiers = electric:%v indoor:%v virtual:%v commute:%v, want electric:%v indoor:%v virtual:%v commute:%v",
				got.IsElectric, got.IsIndoor, got.IsVirtual, got.IsCommute,
				wantElectric, wantIndoor, wantVirtual, wantCommute)
		}
	}

	loaded, err := db.GetRoute(ctx, id, userID)
	if err != nil {
		t.Fatalf("GetRoute: %v", err)
	}
	assertMetadata(t, loaded, "MTB S2", "mtb", true, false, true, true)

	listed, err := db.ListRoutes(ctx, userID, 10, 0, RouteListFilters{SportType: "cycling", SubType: "mountain"})
	if err != nil {
		t.Fatalf("ListRoutes: %v", err)
	}
	if len(listed) != 1 {
		t.Fatalf("ListRoutes len = %d, want 1", len(listed))
	}
	assertMetadata(t, &listed[0], "MTB S2", "mtb", true, false, true, true)

	updatedDifficulty := "SAC T3"
	loaded.Difficulty = &updatedDifficulty
	loaded.Profile = "hiking-sac2"
	loaded.IsElectric = false
	loaded.IsIndoor = true
	loaded.IsVirtual = false
	loaded.IsCommute = false
	if err := db.UpdateRoute(ctx, loaded, 0); err != nil {
		t.Fatalf("UpdateRoute: %v", err)
	}
	updated, err := db.GetRoute(ctx, id, userID)
	if err != nil {
		t.Fatalf("GetRoute after update: %v", err)
	}
	assertMetadata(t, updated, "SAC T3", "hiking-sac2", false, true, false, false)

	dupID, err := db.DuplicateRoute(ctx, id, userID)
	if err != nil {
		t.Fatalf("DuplicateRoute: %v", err)
	}
	duplicated, err := db.GetRoute(ctx, dupID, userID)
	if err != nil {
		t.Fatalf("GetRoute duplicate: %v", err)
	}
	assertMetadata(t, duplicated, "SAC T3", "hiking-sac2", false, true, false, false)
}

func TestPlannedRouteCreateNormalizesLegacySportTuple(t *testing.T) {
	db, userID := routeSubTypeTestDB(t)
	ctx := context.Background()

	route := &model.PlannedRoute{
		UserID:          userID,
		Name:            "legacy route",
		SportType:       "mountain_biking",
		Profile:         "mtb",
		GeometryGeoJSON: `{"type":"LineString","coordinates":[[11.39,47.26],[11.40,47.27]]}`,
		Waypoints:       []model.RouteWaypoint{},
		Segments:        []model.RouteSegment{},
	}

	id, err := db.CreateRoute(ctx, route)
	if err != nil {
		t.Fatalf("CreateRoute: %v", err)
	}
	loaded, err := db.GetRoute(ctx, id, userID)
	if err != nil {
		t.Fatalf("GetRoute: %v", err)
	}
	if loaded.SportType != "cycling" || loaded.SubType == nil || *loaded.SubType != "mountain" {
		t.Fatalf("route tuple = (%s,%v), want (cycling,mountain)", loaded.SportType, loaded.SubType)
	}
}

func TestNormalizePlannedRouteSportTupleReturnsModifierPatch(t *testing.T) {
	sportType, subType, patch, err := normalizePlannedRouteSportTuple("e_mountain_biking", nil)
	if err != nil {
		t.Fatalf("normalizePlannedRouteSportTuple: %v", err)
	}
	if sportType != "cycling" || subType == nil || *subType != "mountain" {
		t.Fatalf("tuple = (%s,%v), want (cycling,mountain)", sportType, subType)
	}
	if patch.IsElectric == nil || !*patch.IsElectric {
		t.Fatalf("IsElectric patch = %#v, want true", patch.IsElectric)
	}
}

func TestRouteModifierPatchesApplyVirtual(t *testing.T) {
	virtual := true
	patch := sport.ModifierPatch{IsVirtual: &virtual}

	planned := &model.PlannedRoute{}
	applyPlannedRouteModifierPatch(planned, patch)
	if !planned.IsVirtual {
		t.Fatal("planned route IsVirtual = false, want true")
	}

	importData := &model.RouteImportData{}
	applyRouteImportModifierPatch(importData, patch)
	if !importData.IsVirtual {
		t.Fatal("route import IsVirtual = false, want true")
	}
}

func TestPlannedRouteCreateRejectsInvalidSubType(t *testing.T) {
	db, userID := routeSubTypeTestDB(t)
	ctx := context.Background()

	badSubType := "trail"
	route := &model.PlannedRoute{
		UserID:          userID,
		Name:            "bad route",
		SportType:       "cycling",
		SubType:         &badSubType,
		Profile:         "mtb",
		GeometryGeoJSON: `{"type":"LineString","coordinates":[[11.39,47.26],[11.40,47.27]]}`,
		Waypoints:       []model.RouteWaypoint{},
		Segments:        []model.RouteSegment{},
	}

	if _, err := db.CreateRoute(ctx, route); err == nil {
		t.Fatal("CreateRoute succeeded for invalid cycling/trail tuple")
	}
}

func TestListRoutesFiltersBySportTuple(t *testing.T) {
	db, userID := routeSubTypeTestDB(t)
	ctx := context.Background()
	mountain := "mountain"
	road := "road"

	for _, route := range []*model.PlannedRoute{
		{
			UserID:          userID,
			Name:            "MTB route",
			SportType:       "cycling",
			SubType:         &mountain,
			IsElectric:      true,
			Profile:         "mtb",
			GeometryGeoJSON: `{"type":"LineString","coordinates":[[11.39,47.26],[11.40,47.27]]}`,
			Waypoints:       []model.RouteWaypoint{},
			Segments:        []model.RouteSegment{},
		},
		{
			UserID:          userID,
			Name:            "Road route",
			SportType:       "cycling",
			SubType:         &road,
			Profile:         "road",
			GeometryGeoJSON: `{"type":"LineString","coordinates":[[11.39,47.26],[11.40,47.27]]}`,
			Waypoints:       []model.RouteWaypoint{},
			Segments:        []model.RouteSegment{},
		},
		{
			UserID:          userID,
			Name:            "Hike route",
			SportType:       "hiking",
			Profile:         "hiking",
			GeometryGeoJSON: `{"type":"LineString","coordinates":[[11.39,47.26],[11.40,47.27]]}`,
			Waypoints:       []model.RouteWaypoint{},
			Segments:        []model.RouteSegment{},
		},
	} {
		if _, err := db.CreateRoute(ctx, route); err != nil {
			t.Fatalf("CreateRoute %q: %v", route.Name, err)
		}
	}

	got, err := db.ListRoutes(ctx, userID, 10, 0, RouteListFilters{SportType: "cycling", SubType: "mountain"})
	if err != nil {
		t.Fatalf("ListRoutes cycling/mountain: %v", err)
	}
	if len(got) != 1 || got[0].Name != "MTB route" {
		t.Fatalf("cycling/mountain routes = %#v, want only MTB route", got)
	}

	got, err = db.ListRoutes(ctx, userID, 10, 0, RouteListFilters{SportType: "cycling"})
	if err != nil {
		t.Fatalf("ListRoutes cycling: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("cycling routes len = %d, want 2", len(got))
	}

	got, err = db.ListRoutes(ctx, userID, 10, 0, RouteListFilters{
		Predicates: []sport.Predicate{
			{Sport: "cycling", SubType: &mountain},
			{Sport: "hiking"},
		},
	})
	if err != nil {
		t.Fatalf("ListRoutes predicate OR: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("predicate OR routes len = %d, want 2", len(got))
	}

	wantTrue := true
	got, err = db.ListRoutes(ctx, userID, 10, 0, RouteListFilters{
		Predicates:      []sport.Predicate{{Sport: "cycling"}},
		ModifierFilters: sport.ModifierFilters{IsElectric: &wantTrue},
	})
	if err != nil {
		t.Fatalf("ListRoutes electric cycling: %v", err)
	}
	if len(got) != 1 || got[0].Name != "MTB route" {
		t.Fatalf("electric cycling routes = %#v, want only MTB route", got)
	}
}
