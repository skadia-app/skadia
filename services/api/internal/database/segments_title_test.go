package database

import (
	"context"
	"testing"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/database/testdb"
	"github.com/google/uuid"
)

func segmentTitleTestDB(t *testing.T) (*DB, string) {
	t.Helper()
	ctx := context.Background()
	pool := testdb.Open(t)
	db := &DB{Pool: pool}

	var userID string
	suffix := uuid.NewString()
	if err := pool.QueryRow(ctx, `
		INSERT INTO users (username, email, registration_source)
		VALUES ($1, $2, 'test')
		RETURNING id`, "segment-title-"+suffix, "segment-title-"+suffix+"@example.test").Scan(&userID); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, userID)
	})

	return db, userID
}

func insertSegmentTitleFixture(t *testing.T, db *DB, userID string) string {
	t.Helper()
	ctx := context.Background()

	segmentID := uuid.NewString()
	versionID := uuid.NewString()
	if _, err := db.Pool.Exec(ctx, `
		INSERT INTO segments (id, created_by, name, sport_group, segment_type)
		VALUES ($1, $2, 'Title Segment', 'hike', 'standard')`,
		segmentID, userID); err != nil {
		t.Fatalf("insert segment: %v", err)
	}
	if _, err := db.Pool.Exec(ctx, `
		INSERT INTO segment_versions (id, segment_id, version_number, track, valid_from, created_by)
		VALUES ($1, $2, 1, ST_GeomFromText('LINESTRING(8 49, 8.01 49.01)', 4326), CURRENT_DATE, $3)`,
		versionID, segmentID, userID); err != nil {
		t.Fatalf("insert segment version: %v", err)
	}

	now := time.Now().UTC()
	rows := []struct {
		id        string
		title     string
		autoTitle *string
		manual    bool
		startedAt time.Time
	}{
		{uuid.NewString(), "Manual title", segmentTitleStringPtr("Generated manual"), true, now},
		{uuid.NewString(), "Source title", segmentTitleStringPtr("Generated title"), false, now.Add(-time.Minute)},
		{uuid.NewString(), "Source fallback", nil, false, now.Add(-2 * time.Minute)},
	}
	for _, row := range rows {
		if _, err := db.Pool.Exec(ctx, `
			INSERT INTO activities (id, user_id, sport_type, title, title_auto, title_is_manual, started_at, moving_time_s)
			VALUES ($1, $2, 'hiking', $3, $4, $5, $6, 120)`,
			row.id, userID, row.title, row.autoTitle, row.manual, row.startedAt); err != nil {
			t.Fatalf("insert activity: %v", err)
		}
		if _, err := db.Pool.Exec(ctx, `
			INSERT INTO segment_efforts (segment_id, segment_version_id, activity_id, user_id, started_at, elapsed_time_s, sport_type, sub_type, personal_rank, rank_overall, in_leaderboard)
			VALUES ($1, $2, $3, $4, $5, 60, 'hiking', '', 1, 1, true)`,
			segmentID, versionID, row.id, userID, row.startedAt); err != nil {
			t.Fatalf("insert segment effort: %v", err)
		}
	}

	return segmentID
}

func TestGetSegmentPerformancesUsesGeneratedTitleUnlessManual(t *testing.T) {
	ctx := context.Background()
	db, userID := segmentTitleTestDB(t)
	segmentID := insertSegmentTitleFixture(t, db, userID)

	performances, err := db.GetSegmentPerformances(ctx, segmentID, userID, nil)
	if err != nil {
		t.Fatalf("GetSegmentPerformances: %v", err)
	}
	if len(performances) != 3 {
		t.Fatalf("performances len = %d, want 3", len(performances))
	}
	want := []string{"Manual title", "Generated title", "Source fallback"}
	for i, title := range want {
		if performances[i].Title != title {
			t.Fatalf("performances[%d].Title = %q, want %q", i, performances[i].Title, title)
		}
	}
}

func segmentTitleStringPtr(s string) *string {
	return &s
}
