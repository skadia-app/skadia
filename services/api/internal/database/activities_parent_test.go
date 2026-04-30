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

func activitiesParentTestDB(t *testing.T) (*DB, string) {
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

	userID := uuid.NewString()
	if _, err := pool.Exec(ctx, `
		INSERT INTO users (id, username, email, email_verified, registration_source)
		VALUES ($1, $2, $3, true, 'test')`,
		userID, "activity-parent-"+userID[:8], "activity-parent-"+userID[:8]+"@example.com"); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM activities WHERE user_id = $1`, userID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, userID)
	})

	return &DB{Pool: pool}, userID
}

func TestInsertActivityWithParentPersistsVirtualModifier(t *testing.T) {
	db, userID := activitiesParentTestDB(t)
	ctx := context.Background()
	parentID := uuid.NewString()

	if err := db.InsertMultisportParent(ctx, &model.Activity{
		ID:        parentID,
		UserID:    userID,
		SportType: "multisport",
		StartedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("InsertMultisportParent: %v", err)
	}

	child, err := db.InsertActivityWithParent(ctx, userID, &model.ActivityData{
		SportType: "cycling",
		StartedAt: time.Now().UTC(),
		IsVirtual: true,
	}, parentID)
	if err != nil {
		t.Fatalf("InsertActivityWithParent: %v", err)
	}

	if !child.IsVirtual || !child.ExcludeFromHeatmap {
		t.Fatalf("returned child virtual=%v exclude_from_heatmap=%v, want true/true", child.IsVirtual, child.ExcludeFromHeatmap)
	}
	children, err := db.GetChildActivities(ctx, parentID)
	if err != nil {
		t.Fatalf("GetChildActivities: %v", err)
	}
	if len(children) != 1 || !children[0].IsVirtual || !children[0].ExcludeFromHeatmap {
		t.Fatalf("stored child = %+v, want virtual child excluded from heatmap", children)
	}
}

func TestAggregateParentMetricsRollsUpModifiers(t *testing.T) {
	db, userID := activitiesParentTestDB(t)
	ctx := context.Background()
	parentID := uuid.NewString()

	if err := db.InsertMultisportParent(ctx, &model.Activity{
		ID:        parentID,
		UserID:    userID,
		SportType: "multisport",
		StartedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("InsertMultisportParent: %v", err)
	}

	for _, data := range []*model.ActivityData{
		{SportType: "cycling", StartedAt: time.Now().UTC(), IsIndoor: true, IsElectric: true, IsVirtual: true, HasGPS: false},
		{SportType: "running", StartedAt: time.Now().UTC(), IsIndoor: true, IsElectric: false, IsVirtual: false, HasGPS: true},
	} {
		if _, err := db.InsertActivityWithParent(ctx, userID, data, parentID); err != nil {
			t.Fatalf("InsertActivityWithParent: %v", err)
		}
	}

	if err := db.AggregateParentMetrics(ctx, parentID); err != nil {
		t.Fatalf("AggregateParentMetrics: %v", err)
	}
	parent, err := db.GetActivity(ctx, parentID)
	if err != nil {
		t.Fatalf("GetActivity parent: %v", err)
	}
	if !parent.IsIndoor || !parent.IsElectric || !parent.IsVirtual || !parent.ExcludeFromHeatmap || !parent.HasGPS {
		t.Fatalf("parent modifiers indoor/electric/virtual/heatmap/gps = %v/%v/%v/%v/%v, want all true",
			parent.IsIndoor, parent.IsElectric, parent.IsVirtual, parent.ExcludeFromHeatmap, parent.HasGPS)
	}
}
