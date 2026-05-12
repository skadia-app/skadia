package database

import (
	"context"
	"testing"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/database/testdb"
	"github.com/skadia-app/skadia/services/api/internal/model"
	"github.com/google/uuid"
)

func titleSurfaceTestDB(t *testing.T) (*DB, string) {
	t.Helper()
	ctx := context.Background()
	pool := testdb.Open(t)
	db := &DB{Pool: pool}

	var userID string
	username := "title-surface-" + uuid.NewString()
	if err := pool.QueryRow(ctx, `
		INSERT INTO users (username, email, registration_source)
		VALUES ($1, $2, 'test')
		RETURNING id`, username, username+"@example.test").Scan(&userID); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, userID)
	})

	return db, userID
}

func TestWorkoutActivitiesUseGeneratedTitleUnlessManual(t *testing.T) {
	ctx := context.Background()
	db, userID := titleSurfaceTestDB(t)
	workoutID, err := db.UpsertWorkout(ctx, userID, model.WorkoutRecord{SourceHash: uuid.NewString()}, nil)
	if err != nil {
		t.Fatalf("UpsertWorkout: %v", err)
	}

	if _, err := db.Pool.Exec(ctx, `
		INSERT INTO activities (user_id, sport_type, title, title_auto, title_is_manual, started_at, workout_id)
		VALUES
			($1, 'running', 'Source title', 'Generated title', false, $2, $3),
			($1, 'cycling', 'Manual title', 'Generated manual', true, $2 - interval '1 minute', $3)`,
		userID, time.Now().UTC(), workoutID); err != nil {
		t.Fatalf("insert activities: %v", err)
	}

	_, _, activities, err := db.GetWorkout(ctx, workoutID, userID)
	if err != nil {
		t.Fatalf("GetWorkout: %v", err)
	}
	if len(activities) != 2 {
		t.Fatalf("activities len = %d, want 2", len(activities))
	}
	if activities[0].Title == nil || *activities[0].Title != "Generated title" {
		t.Fatalf("first title = %#v, want generated title", activities[0].Title)
	}
	if activities[1].Title == nil || *activities[1].Title != "Manual title" {
		t.Fatalf("second title = %#v, want manual title", activities[1].Title)
	}
}

func TestListDeviceActivitiesUsesGeneratedTitleUnlessManual(t *testing.T) {
	ctx := context.Background()
	db, userID := titleSurfaceTestDB(t)

	var deviceID string
	if err := db.Pool.QueryRow(ctx, `
		INSERT INTO devices (user_id, type, manufacturer, product_name)
		VALUES ($1, 'recorder', 'test', 'unit')
		RETURNING id`, userID).Scan(&deviceID); err != nil {
		t.Fatalf("insert device: %v", err)
	}
	if _, err := db.Pool.Exec(ctx, `
		INSERT INTO activities (user_id, sport_type, title, title_auto, title_is_manual, started_at, recording_device_id)
		VALUES
			($1, 'running', 'Source title', 'Generated title', false, $2, $3),
			($1, 'cycling', 'Manual title', 'Generated manual', true, $2 - interval '1 minute', $3)`,
		userID, time.Now().UTC(), deviceID); err != nil {
		t.Fatalf("insert activities: %v", err)
	}

	activities, err := db.ListDeviceActivities(ctx, deviceID, 20, "")
	if err != nil {
		t.Fatalf("ListDeviceActivities: %v", err)
	}
	if len(activities) != 2 {
		t.Fatalf("activities len = %d, want 2", len(activities))
	}
	if activities[0].Title == nil || *activities[0].Title != "Generated title" {
		t.Fatalf("first title = %#v, want generated title", activities[0].Title)
	}
	if activities[1].Title == nil || *activities[1].Title != "Manual title" {
		t.Fatalf("second title = %#v, want manual title", activities[1].Title)
	}
}

func TestGetCachedFusionCandidatesUsesGeneratedTitleUnlessManual(t *testing.T) {
	ctx := context.Background()
	db, userID := titleSurfaceTestDB(t)

	var generatedID, manualID string
	if err := db.Pool.QueryRow(ctx, `
		INSERT INTO activities (user_id, sport_type, title, title_auto, title_is_manual, started_at)
		VALUES ($1, 'running', 'Source title', 'Generated title', false, $2)
		RETURNING id`, userID, time.Now().UTC()).Scan(&generatedID); err != nil {
		t.Fatalf("insert generated activity: %v", err)
	}
	if err := db.Pool.QueryRow(ctx, `
		INSERT INTO activities (user_id, sport_type, title, title_auto, title_is_manual, started_at)
		VALUES ($1, 'cycling', 'Manual title', 'Generated manual', true, $2)
		RETURNING id`, userID, time.Now().UTC().Add(time.Minute)).Scan(&manualID); err != nil {
		t.Fatalf("insert manual activity: %v", err)
	}
	if _, err := db.Pool.Exec(ctx, `
		INSERT INTO fusion_candidates (user_id, activity_id_a, activity_id_b, overlap_percent, field_richness_a, field_richness_b, time_diff_sec)
		VALUES ($1, $2, $3, 95, 4, 5, 30)`,
		userID, generatedID, manualID); err != nil {
		t.Fatalf("insert fusion candidate: %v", err)
	}

	rows, err := db.GetCachedFusionCandidates(ctx, userID, 10)
	if err != nil {
		t.Fatalf("GetCachedFusionCandidates: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("rows len = %d, want 1", len(rows))
	}
	if rows[0].TitleA == nil || *rows[0].TitleA != "Generated title" {
		t.Fatalf("TitleA = %#v, want generated title", rows[0].TitleA)
	}
	if rows[0].TitleB == nil || *rows[0].TitleB != "Manual title" {
		t.Fatalf("TitleB = %#v, want manual title", rows[0].TitleB)
	}
}
