package database

import (
	"context"
	"testing"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/database/testdb"
	"github.com/google/uuid"
)

func summitTitleTestDB(t *testing.T) (*DB, string) {
	t.Helper()
	ctx := context.Background()
	pool := testdb.Open(t)
	db := &DB{Pool: pool}

	var userID string
	suffix := uuid.NewString()
	if err := pool.QueryRow(ctx, `
		INSERT INTO users (username, email, registration_source)
		VALUES ($1, $2, 'test')
		RETURNING id`, "summit-title-"+suffix, "summit-title-"+suffix+"@example.test").Scan(&userID); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, userID)
	})

	return db, userID
}

func TestGetSummitActivitiesUseAutoTitleForEmptySourceUnlessManual(t *testing.T) {
	ctx := context.Background()
	db, userID := summitTitleTestDB(t)
	now := time.Now().UTC()

	manualID := uuid.NewString()
	generatedID := uuid.NewString()
	sourceID := uuid.NewString()
	if _, err := db.Pool.Exec(ctx, `
		INSERT INTO activities (id, user_id, sport_type, title, title_auto, title_is_manual, started_at)
		VALUES
			($1, $4, 'hiking', 'Manual title', 'Generated manual', true, $5),
			($2, $4, 'hiking', '   ', 'Generated title', false, $5 - interval '1 minute'),
			($3, $4, 'hiking', 'Source fallback', NULL, false, $5 - interval '2 minutes')`,
		manualID, generatedID, sourceID, userID, now); err != nil {
		t.Fatalf("insert activities: %v", err)
	}
	if _, err := db.Pool.Exec(ctx, `
		INSERT INTO activity_geo_tags (activity_id, category, name, osm_id)
		VALUES
			($1, 'summit', 'Test Summit', 123456),
			($2, 'summit', 'Test Summit', 123456),
			($3, 'summit', 'Test Summit', 123456)`,
		manualID, generatedID, sourceID); err != nil {
		t.Fatalf("insert summit tags: %v", err)
	}

	osmID := int64(123456)
	activities, err := db.GetSummitActivities(ctx, userID, &osmID, "")
	if err != nil {
		t.Fatalf("GetSummitActivities: %v", err)
	}
	if len(activities) != 3 {
		t.Fatalf("activities len = %d, want 3", len(activities))
	}
	want := []string{"Manual title", "Generated title", "Source fallback"}
	for i, title := range want {
		if activities[i].Title != title {
			t.Fatalf("activities[%d].Title = %q, want %q", i, activities[i].Title, title)
		}
	}
}
