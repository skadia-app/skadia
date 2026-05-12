package database

import (
	"context"
	"testing"

	"github.com/skadia-app/skadia/services/api/internal/database/testdb"
	"github.com/google/uuid"
)

func TestGetTagActivitiesReturnsTitleMetadata(t *testing.T) {
	ctx := context.Background()
	pool := testdb.Open(t)
	db := &DB{Pool: pool}

	var userID string
	username := "tag-activity-title-" + uuid.NewString()
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
		VALUES ($1, 'auto_first')`, userID); err != nil {
		t.Fatalf("insert settings: %v", err)
	}

	var activityID string
	if err := pool.QueryRow(ctx, `
		INSERT INTO activities (user_id, sport_type, title, title_auto, title_is_manual, started_at)
		VALUES ($1, 'hiking', 'Source', 'Generated', false, now())
		RETURNING id`, userID).Scan(&activityID); err != nil {
		t.Fatalf("insert activity: %v", err)
	}

	var tagID string
	tagName := "Tag Title " + uuid.NewString()
	if err := pool.QueryRow(ctx, `
		INSERT INTO tags (user_id, name, color, text_color, category)
		VALUES ($1, $2, '#f97316', '#ffffff', 'custom')
		RETURNING id`, userID, tagName).Scan(&tagID); err != nil {
		t.Fatalf("insert tag: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO activity_tags (activity_id, tag_id, assigned_by)
		VALUES ($1, $2, 'test')`, activityID, tagID); err != nil {
		t.Fatalf("insert activity tag: %v", err)
	}

	activities, err := db.GetTagActivities(ctx, tagID, userID)
	if err != nil {
		t.Fatalf("GetTagActivities: %v", err)
	}
	if len(activities) != 1 {
		t.Fatalf("activities len = %d, want 1", len(activities))
	}
	got := activities[0]
	if got.TitleAuto == nil || *got.TitleAuto != "Generated" {
		t.Fatalf("TitleAuto = %#v, want Generated", got.TitleAuto)
	}
	if got.DisplayTitle == nil || *got.DisplayTitle != "Generated" {
		t.Fatalf("DisplayTitle = %#v, want Generated", got.DisplayTitle)
	}
	if got.Title == nil || *got.Title != "Source" {
		t.Fatalf("Title = %#v, want raw source title", got.Title)
	}
	if got.TitleIsManual {
		t.Fatal("TitleIsManual = true, want false")
	}
}
