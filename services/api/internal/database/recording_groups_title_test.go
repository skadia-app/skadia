package database

import (
	"context"
	"testing"

	"github.com/skadia-app/skadia/services/api/internal/database/testdb"
	"github.com/google/uuid"
)

func TestRecordingGroupMembersReturnTitleMetadata(t *testing.T) {
	ctx := context.Background()
	pool := testdb.Open(t)
	db := &DB{Pool: pool}

	var userID string
	username := "recording-title-" + uuid.NewString()
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

	var primaryID, secondaryID string
	if err := pool.QueryRow(ctx, `
		INSERT INTO activities (user_id, sport_type, title, title_auto, title_is_manual, started_at)
		VALUES ($1, 'hiking', 'Source', 'Generated', false, now())
		RETURNING id`, userID).Scan(&primaryID); err != nil {
		t.Fatalf("insert primary activity: %v", err)
	}
	if err := pool.QueryRow(ctx, `
		INSERT INTO activities (user_id, sport_type, title, title_auto, title_is_manual, started_at)
		VALUES ($1, 'hiking', 'Manual', 'Other generated', true, now() + interval '1 minute')
		RETURNING id`, userID).Scan(&secondaryID); err != nil {
		t.Fatalf("insert secondary activity: %v", err)
	}

	group, err := db.CreateRecordingGroup(ctx, userID, []string{primaryID, secondaryID}, primaryID, "")
	if err != nil {
		t.Fatalf("CreateRecordingGroup: %v", err)
	}

	members, err := db.GetRecordingGroupMembers(ctx, group.ID)
	if err != nil {
		t.Fatalf("GetRecordingGroupMembers: %v", err)
	}
	if len(members) != 2 {
		t.Fatalf("members len = %d, want 2", len(members))
	}

	var got *RecordingGroupMemberDetail
	for i := range members {
		if members[i].ActivityID == primaryID {
			got = &members[i]
			break
		}
	}
	if got == nil {
		t.Fatalf("primary member %s not returned", primaryID)
	}
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

	groups, err := db.GetRecordingGroupsForUser(ctx, userID, 20, 0)
	if err != nil {
		t.Fatalf("GetRecordingGroupsForUser: %v", err)
	}
	if len(groups) != 1 {
		t.Fatalf("groups len = %d, want 1", len(groups))
	}
	got = nil
	for i := range groups[0].Members {
		if groups[0].Members[i].ActivityID == primaryID {
			got = &groups[0].Members[i]
			break
		}
	}
	if got == nil {
		t.Fatalf("primary member %s not returned from group list", primaryID)
	}
	if got.TitleAuto == nil || *got.TitleAuto != "Generated" {
		t.Fatalf("group list TitleAuto = %#v, want Generated", got.TitleAuto)
	}
	if got.DisplayTitle == nil || *got.DisplayTitle != "Generated" {
		t.Fatalf("group list DisplayTitle = %#v, want Generated", got.DisplayTitle)
	}
	if got.Title == nil || *got.Title != "Source" {
		t.Fatalf("group list Title = %#v, want raw source title", got.Title)
	}
	if got.TitleIsManual {
		t.Fatal("group list TitleIsManual = true, want false")
	}
}
