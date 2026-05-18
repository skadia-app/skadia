package database

import (
	"context"
	"testing"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/database/testdb"
	"github.com/google/uuid"
)

func segmentPRTestDB(t *testing.T) (*DB, string) {
	t.Helper()
	pool := testdb.Open(t)
	ctx := context.Background()

	suffix := uuid.NewString()[:8]
	var userID string
	if err := pool.QueryRow(ctx,
		`INSERT INTO users (username, email, registration_source)
		 VALUES ($1, $2, 'test')
		 RETURNING id`,
		"segment-pr-"+suffix,
		"segment-pr-"+suffix+"@test.localhost",
	).Scan(&userID); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, userID)
	})
	return &DB{Pool: pool}, userID
}

func TestGetUserSegmentPRsIncludesActivityModifiers(t *testing.T) {
	db, userID := segmentPRTestDB(t)
	ctx := context.Background()

	activityID := uuid.NewString()
	if _, err := db.Pool.Exec(ctx,
		`INSERT INTO activities (id, user_id, sport_type, sub_type, is_electric, is_indoor, is_virtual, title, started_at, file_hash, visibility, enrichment_status)
		 VALUES ($1, $2, 'cycling', 'mountain', true, false, false, 'Segment PR Modifier Test', $3, $4, 'private', 'complete')`,
		activityID, userID, time.Now().UTC(), "segment-pr-modifier-"+activityID,
	); err != nil {
		t.Fatalf("insert activity: %v", err)
	}

	segmentID := uuid.NewString()
	versionID := uuid.NewString()
	if _, err := db.Pool.Exec(ctx,
		`INSERT INTO segments (id, created_by, name, sport_group, segment_type)
		 VALUES ($1, $2, 'Modifier Segment', 'bike', 'standard')`,
		segmentID, userID,
	); err != nil {
		t.Fatalf("insert segment: %v", err)
	}
	if _, err := db.Pool.Exec(ctx,
		`INSERT INTO segment_versions (id, segment_id, version_number, track, valid_from, created_by)
		 VALUES ($1, $2, 1, ST_GeomFromText('LINESTRING(8 49, 8.01 49.01)', 4326), CURRENT_DATE, $3)`,
		versionID, segmentID, userID,
	); err != nil {
		t.Fatalf("insert segment version: %v", err)
	}
	activeEffortID := uuid.NewString()
	supersededEffortID := uuid.NewString()
	startedAt := time.Now().UTC()
	if _, err := db.Pool.Exec(ctx,
		`INSERT INTO segment_efforts (id, segment_id, segment_version_id, activity_id, user_id, started_at, elapsed_time_s, sport_type, sub_type, personal_rank, in_leaderboard)
		 VALUES ($1, $2, $3, $4, $5, $6, 60, 'cycling', 'mountain', 2, true)`,
		activeEffortID, segmentID, versionID, activityID, userID, startedAt,
	); err != nil {
		t.Fatalf("insert active segment effort: %v", err)
	}
	if _, err := db.Pool.Exec(ctx,
		`INSERT INTO segment_efforts (id, segment_id, segment_version_id, activity_id, user_id, started_at, elapsed_time_s, sport_type, sub_type, personal_rank, in_leaderboard, superseded_by_effort_id)
		 VALUES ($1, $2, $3, $4, $5, $6, 55, 'cycling', 'mountain', 1, true, $7)`,
		supersededEffortID, segmentID, versionID, activityID, userID, startedAt.Add(-time.Second), activeEffortID,
	); err != nil {
		t.Fatalf("insert superseded segment effort: %v", err)
	}

	efforts, err := db.GetUserSegmentPRs(ctx, userID)
	if err != nil {
		t.Fatalf("GetUserSegmentPRs: %v", err)
	}
	if len(efforts) != 1 {
		t.Fatalf("efforts = %#v, want one", efforts)
	}
	if efforts[0].ID != activeEffortID {
		t.Fatalf("effort id = %s, want active effort %s", efforts[0].ID, activeEffortID)
	}
	if !efforts[0].IsElectric || efforts[0].IsIndoor || efforts[0].IsVirtual {
		t.Fatalf("segment PR modifiers = electric:%v indoor:%v virtual:%v, want true/false/false",
			efforts[0].IsElectric, efforts[0].IsIndoor, efforts[0].IsVirtual)
	}
}

func TestGetUserGlobalTopSegmentEffortsIgnoresSupersededRows(t *testing.T) {
	db, userID := segmentPRTestDB(t)
	ctx := context.Background()

	activityID := uuid.NewString()
	if _, err := db.Pool.Exec(ctx,
		`INSERT INTO activities (id, user_id, sport_type, sub_type, title, started_at, file_hash, visibility, enrichment_status)
		 VALUES ($1, $2, 'cycling', 'road', 'Segment Global Top Superseded Test', $3, $4, 'private', 'complete')`,
		activityID, userID, time.Now().UTC(), "segment-global-top-"+activityID,
	); err != nil {
		t.Fatalf("insert activity: %v", err)
	}

	segmentID := uuid.NewString()
	versionID := uuid.NewString()
	if _, err := db.Pool.Exec(ctx,
		`INSERT INTO segments (id, created_by, name, sport_group, segment_type, athlete_count)
		 VALUES ($1, $2, 'Global Top Segment', 'bike', 'standard', 7)`,
		segmentID, userID,
	); err != nil {
		t.Fatalf("insert segment: %v", err)
	}
	if _, err := db.Pool.Exec(ctx,
		`INSERT INTO segment_versions (id, segment_id, version_number, track, valid_from, created_by)
		 VALUES ($1, $2, 1, ST_GeomFromText('LINESTRING(8 49, 8.01 49.01)', 4326), CURRENT_DATE, $3)`,
		versionID, segmentID, userID,
	); err != nil {
		t.Fatalf("insert segment version: %v", err)
	}

	activeEffortID := uuid.NewString()
	supersededEffortID := uuid.NewString()
	startedAt := time.Now().UTC()
	if _, err := db.Pool.Exec(ctx,
		`INSERT INTO segment_efforts (id, segment_id, segment_version_id, activity_id, user_id, started_at, elapsed_time_s, sport_type, sub_type, rank_overall, in_leaderboard)
		 VALUES ($1, $2, $3, $4, $5, $6, 60, 'cycling', 'road', 2, true)`,
		activeEffortID, segmentID, versionID, activityID, userID, startedAt,
	); err != nil {
		t.Fatalf("insert active segment effort: %v", err)
	}
	if _, err := db.Pool.Exec(ctx,
		`INSERT INTO segment_efforts (id, segment_id, segment_version_id, activity_id, user_id, started_at, elapsed_time_s, sport_type, sub_type, rank_overall, in_leaderboard, superseded_by_effort_id)
		 VALUES ($1, $2, $3, $4, $5, $6, 55, 'cycling', 'road', 1, true, $7)`,
		supersededEffortID, segmentID, versionID, activityID, userID, startedAt.Add(-time.Second), activeEffortID,
	); err != nil {
		t.Fatalf("insert superseded segment effort: %v", err)
	}

	efforts, err := db.GetUserGlobalTopSegmentEfforts(ctx, userID)
	if err != nil {
		t.Fatalf("GetUserGlobalTopSegmentEfforts: %v", err)
	}
	if len(efforts) != 1 {
		t.Fatalf("efforts = %#v, want one", efforts)
	}
	if efforts[0].ID != activeEffortID {
		t.Fatalf("effort id = %s, want active effort %s", efforts[0].ID, activeEffortID)
	}
	if efforts[0].AthleteCount != 7 {
		t.Fatalf("athlete_count = %d, want 7", efforts[0].AthleteCount)
	}
}
