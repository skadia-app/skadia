package database

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/model"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func routeSubTypeTestDB(t *testing.T) (*DB, string) {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
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
	if err := db.UpdateRoute(ctx, loaded); err != nil {
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
