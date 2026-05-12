package database

import (
	"context"
	"testing"

	"github.com/skadia-app/skadia/services/api/internal/database/testdb"
	"github.com/skadia-app/skadia/services/api/internal/model"
	"github.com/google/uuid"
)

func TestListActivitiesReturnsTitleAutoAndManualFlag(t *testing.T) {
	ctx := context.Background()
	pool := testdb.Open(t)
	db := &DB{Pool: pool}

	var userID string
	username := "list-autotitle-" + uuid.NewString()
	if err := pool.QueryRow(ctx, `
		INSERT INTO users (username, email, registration_source)
		VALUES ($1, $2, 'test')
		RETURNING id`, username, username+"@example.test").Scan(&userID); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, userID)
	})

	if _, err := pool.Exec(ctx, `
		INSERT INTO activities (user_id, sport_type, title, title_auto, title_is_manual, started_at)
		VALUES ($1, 'hiking', 'Source title', 'Hochplatte ab Marquartstein', false, now())`, userID); err != nil {
		t.Fatalf("insert activity: %v", err)
	}

	list, err := db.ListActivities(ctx, userID, "", 20, "started_at", "desc", model.ActivityFilters{})
	if err != nil {
		t.Fatalf("ListActivities: %v", err)
	}
	if len(list.Activities) != 1 {
		t.Fatalf("activities len = %d, want 1", len(list.Activities))
	}
	got := list.Activities[0]
	if got.TitleAuto == nil || *got.TitleAuto != "Hochplatte ab Marquartstein" {
		t.Fatalf("TitleAuto = %#v, want generated title", got.TitleAuto)
	}
	if got.TitleIsManual {
		t.Fatal("TitleIsManual = true, want false")
	}
}

func TestActivityReadSurfacesUseAutoTitleDisplayPolicy(t *testing.T) {
	ctx := context.Background()
	pool := testdb.Open(t)
	db := &DB{Pool: pool}

	var userID string
	username := "list-title-policy-" + uuid.NewString()
	if err := pool.QueryRow(ctx, `
		INSERT INTO users (username, email, registration_source)
		VALUES ($1, $2, 'test')
		RETURNING id`, username, username+"@example.test").Scan(&userID); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, userID)
	})

	if _, err := pool.Exec(ctx, `
		INSERT INTO user_enrichment_settings (user_id, auto_title_display_policy)
		VALUES ($1, 'source_unless_generic')`, userID); err != nil {
		t.Fatalf("insert settings: %v", err)
	}

	var activityID string
	if err := pool.QueryRow(ctx, `
		INSERT INTO activities (user_id, sport_type, title, title_auto, title_is_manual, started_at)
		VALUES ($1, 'cycling', '   ', 'Hochplatte ab Marquartstein', false, now())
		RETURNING id`, userID).Scan(&activityID); err != nil {
		t.Fatalf("insert activity: %v", err)
	}

	detail, err := db.GetActivity(ctx, activityID)
	if err != nil {
		t.Fatalf("GetActivity: %v", err)
	}
	if detail == nil || detail.DisplayTitle == nil || *detail.DisplayTitle != "Hochplatte ab Marquartstein" {
		t.Fatalf("GetActivity display title = %#v, want generated title", detail)
	}
	if detail.Title == nil || *detail.Title != "   " {
		t.Fatalf("GetActivity raw title = %#v, want source title", detail.Title)
	}

	list, err := db.ListActivities(ctx, userID, "", 20, "started_at", "desc", model.ActivityFilters{})
	if err != nil {
		t.Fatalf("ListActivities: %v", err)
	}
	if len(list.Activities) != 1 {
		t.Fatalf("activities len = %d, want 1", len(list.Activities))
	}
	got := list.Activities[0]
	if got.Title == nil || *got.Title != "Hochplatte ab Marquartstein" {
		t.Fatalf("ListActivities title = %#v, want generated title", got.Title)
	}
}
