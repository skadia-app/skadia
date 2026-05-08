package database

import (
	"context"
	"testing"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/database/testdb"
	"github.com/google/uuid"

	"github.com/skadia-app/skadia/services/api/internal/bestefforts"
)

func bestEffortsCleanupDB(t *testing.T) (*DB, string, string) {
	t.Helper()
	pool := testdb.Open(t)
	ctx := context.Background()

	suffix := uuid.NewString()[:8]
	var userID string
	if err := pool.QueryRow(ctx,
		`INSERT INTO users (username, email, registration_source)
		 VALUES ($1, $2, 'test')
		 RETURNING id`,
		"best-efforts-cleanup-"+suffix,
		"best-efforts-cleanup-"+suffix+"@test.localhost",
	).Scan(&userID); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, userID)
	})

	activityID := uuid.NewString()
	if _, err := pool.Exec(ctx,
		`INSERT INTO activities (id, user_id, sport_type, title, started_at, file_hash, visibility, enrichment_status)
		 VALUES ($1, $2, 'running', 'Cleanup Test', $3, $4, 'private', 'complete')`,
		activityID, userID, time.Now().UTC(), "best-efforts-cleanup-"+suffix,
	); err != nil {
		t.Fatalf("insert activity: %v", err)
	}

	return &DB{Pool: pool}, userID, activityID
}

func TestDeleteOrphanedBestEffortsRemovesInvalidSportGroups(t *testing.T) {
	db, userID, activityID := bestEffortsCleanupDB(t)
	ctx := context.Background()

	for _, sportGroup := range []string{"running", "trail_running"} {
		if _, err := db.Pool.Exec(ctx,
			`INSERT INTO best_efforts (user_id, activity_id, category, sport_group, elapsed_time_s, achieved_at)
			 VALUES ($1, $2, 'dist_5km', $3, 1200, $4)`,
			userID, activityID, sportGroup, time.Now().UTC(),
		); err != nil {
			t.Fatalf("insert best_effort %s: %v", sportGroup, err)
		}
	}

	if err := db.DeleteOrphanedBestEfforts(ctx, userID, []string{activityID}); err != nil {
		t.Fatalf("DeleteOrphanedBestEfforts: %v", err)
	}

	var sportGroups []string
	rows, err := db.Pool.Query(ctx, `SELECT sport_group FROM best_efforts WHERE user_id = $1 ORDER BY sport_group`, userID)
	if err != nil {
		t.Fatalf("query best_efforts: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var sportGroup string
		if err := rows.Scan(&sportGroup); err != nil {
			t.Fatalf("scan sport_group: %v", err)
		}
		sportGroups = append(sportGroups, sportGroup)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows: %v", err)
	}
	if len(sportGroups) != 1 || sportGroups[0] != "running" {
		t.Fatalf("sport_groups after cleanup = %#v, want [running]", sportGroups)
	}
}

func TestReplaceBestEffortsForActivitiesRemovesStaleValidRows(t *testing.T) {
	db, userID, activityID := bestEffortsCleanupDB(t)
	ctx := context.Background()
	now := time.Now().UTC()

	for _, row := range []struct {
		category   string
		sportGroup string
		elapsedS   int
	}{
		{category: "dist_5km", sportGroup: "running", elapsedS: 1200},
		{category: "dist_10km", sportGroup: "running", elapsedS: 2500},
	} {
		if _, err := db.Pool.Exec(ctx,
			`INSERT INTO best_efforts (user_id, activity_id, category, sport_group, elapsed_time_s, achieved_at)
			 VALUES ($1, $2, $3, $4, $5, $6)`,
			userID, activityID, row.category, row.sportGroup, row.elapsedS, now,
		); err != nil {
			t.Fatalf("insert stale best_effort %s: %v", row.category, err)
		}
	}

	replacementElapsed := 1300
	if err := db.ReplaceBestEffortsForActivities(ctx, userID, []string{activityID}, []bestefforts.BestEffortRecord{
		{
			ID:           uuid.NewString(),
			UserID:       userID,
			ActivityID:   activityID,
			Category:     "dist_5km",
			SportGroup:   "running",
			ElapsedTimeS: &replacementElapsed,
			AchievedAt:   now.Add(time.Minute),
		},
	}); err != nil {
		t.Fatalf("ReplaceBestEffortsForActivities: %v", err)
	}

	rows, err := db.Pool.Query(ctx,
		`SELECT category, elapsed_time_s
		 FROM best_efforts
		 WHERE activity_id = $1
		 ORDER BY category`,
		activityID,
	)
	if err != nil {
		t.Fatalf("query best_efforts: %v", err)
	}
	defer rows.Close()

	var got []struct {
		category string
		elapsedS int
	}
	for rows.Next() {
		var row struct {
			category string
			elapsedS int
		}
		if err := rows.Scan(&row.category, &row.elapsedS); err != nil {
			t.Fatalf("scan replacement row: %v", err)
		}
		got = append(got, row)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows: %v", err)
	}
	if len(got) != 1 || got[0].category != "dist_5km" || got[0].elapsedS != replacementElapsed {
		t.Fatalf("remaining rows = %#v, want one replacement dist_5km row", got)
	}
}

func TestReplaceBestEffortsForActivitiesDeletesOnlyRequestedUserRows(t *testing.T) {
	db, userID, activityID := bestEffortsCleanupDB(t)
	ctx := context.Background()
	otherDB, otherUserID, otherActivityID := bestEffortsCleanupDB(t)
	_ = otherDB
	now := time.Now().UTC()

	for _, row := range []struct {
		userID     string
		activityID string
		category   string
	}{
		{userID: userID, activityID: activityID, category: "dist_5km"},
		{userID: otherUserID, activityID: otherActivityID, category: "dist_10km"},
	} {
		if _, err := db.Pool.Exec(ctx,
			`INSERT INTO best_efforts (user_id, activity_id, category, sport_group, elapsed_time_s, achieved_at)
			 VALUES ($1, $2, $3, 'running', 1200, $4)`,
			row.userID, row.activityID, row.category, now,
		); err != nil {
			t.Fatalf("insert stale best_effort %s: %v", row.category, err)
		}
	}

	if err := db.ReplaceBestEffortsForActivities(ctx, userID, []string{activityID, otherActivityID}, nil); err != nil {
		t.Fatalf("ReplaceBestEffortsForActivities: %v", err)
	}

	var ownCount, otherCount int
	if err := db.Pool.QueryRow(ctx, `SELECT COUNT(*) FROM best_efforts WHERE activity_id = $1`, activityID).Scan(&ownCount); err != nil {
		t.Fatalf("count own best_efforts: %v", err)
	}
	if err := db.Pool.QueryRow(ctx, `SELECT COUNT(*) FROM best_efforts WHERE activity_id = $1`, otherActivityID).Scan(&otherCount); err != nil {
		t.Fatalf("count other best_efforts: %v", err)
	}
	if ownCount != 0 {
		t.Fatalf("own activity best_efforts = %d, want 0", ownCount)
	}
	if otherCount != 1 {
		t.Fatalf("other user best_efforts = %d, want untouched 1", otherCount)
	}
}

func TestReplaceBestEffortsForActivitiesRejectsRecordsOutsideDeleteScope(t *testing.T) {
	db, userID, activityID := bestEffortsCleanupDB(t)
	_, _, otherActivityID := bestEffortsCleanupDB(t)
	ctx := context.Background()

	err := db.ReplaceBestEffortsForActivities(ctx, userID, []string{activityID}, []bestefforts.BestEffortRecord{
		{
			ID:         uuid.NewString(),
			UserID:     userID,
			ActivityID: otherActivityID,
			Category:   "dist_5km",
			SportGroup: "running",
			AchievedAt: time.Now().UTC(),
		},
	})
	if err == nil {
		t.Fatal("ReplaceBestEffortsForActivities returned nil, want scope error")
	}
}

func TestReplaceBestEffortsForActivitiesEmptyRecordsDeletesStaleRows(t *testing.T) {
	db, userID, activityID := bestEffortsCleanupDB(t)
	ctx := context.Background()

	if _, err := db.Pool.Exec(ctx,
		`INSERT INTO best_efforts (user_id, activity_id, category, sport_group, elapsed_time_s, achieved_at)
		 VALUES ($1, $2, 'dist_5km', 'running', 1200, $3)`,
		userID, activityID, time.Now().UTC(),
	); err != nil {
		t.Fatalf("insert stale best_effort: %v", err)
	}

	if err := db.ReplaceBestEffortsForActivities(ctx, userID, []string{activityID}, nil); err != nil {
		t.Fatalf("ReplaceBestEffortsForActivities: %v", err)
	}

	var count int
	if err := db.Pool.QueryRow(ctx, `SELECT COUNT(*) FROM best_efforts WHERE activity_id = $1`, activityID).Scan(&count); err != nil {
		t.Fatalf("count best_efforts: %v", err)
	}
	if count != 0 {
		t.Fatalf("best_efforts count = %d, want 0", count)
	}
}

func insertBestEffortEligibilityActivity(t *testing.T, db *DB, userID, suffix string, overrides string) string {
	t.Helper()
	activityID := uuid.NewString()
	query := `INSERT INTO activities (id, user_id, sport_type, title, started_at, file_hash, visibility, enrichment_status` + overrides + `)
		VALUES ($1, $2, 'running', $3, $4, $5, 'private', 'complete'`
	args := []any{
		activityID,
		userID,
		"Eligibility Test " + suffix,
		time.Now().UTC(),
		"best-efforts-eligibility-" + suffix + "-" + activityID,
	}
	switch overrides {
	case ", num_sessions":
		query += `, 2`
	case "":
	default:
		t.Fatalf("unsupported activity override %q", overrides)
	}
	query += `)`
	if _, err := db.Pool.Exec(context.Background(), query, args...); err != nil {
		t.Fatalf("insert activity %s: %v", suffix, err)
	}
	return activityID
}

func insertBestEffortEligibilityRow(t *testing.T, db *DB, userID, activityID string, elapsedS int) {
	t.Helper()
	if _, err := db.Pool.Exec(context.Background(),
		`INSERT INTO best_efforts (user_id, activity_id, category, sport_group, elapsed_time_s, achieved_at)
		 VALUES ($1, $2, 'dist_5km', 'running', $3, $4)`,
		userID, activityID, elapsedS, time.Now().UTC(),
	); err != nil {
		t.Fatalf("insert best_effort: %v", err)
	}
}

func TestBestEffortRankingExcludesMultiSessionAndFusionSourceActivities(t *testing.T) {
	db, userID, _ := bestEffortsCleanupDB(t)
	ctx := context.Background()

	eligibleID := insertBestEffortEligibilityActivity(t, db, userID, "eligible", "")
	multiSessionID := insertBestEffortEligibilityActivity(t, db, userID, "multi", ", num_sessions")
	fusionSourceID := insertBestEffortEligibilityActivity(t, db, userID, "source", "")
	mergedID := insertBestEffortEligibilityActivity(t, db, userID, "merged", "")

	if _, err := db.Pool.Exec(ctx,
		`INSERT INTO activity_fusion_memberships (merged_id, source_id, field_key, user_id)
		 VALUES ($1, $2, 'gps', $3)`,
		mergedID, fusionSourceID, userID,
	); err != nil {
		t.Fatalf("insert fusion membership: %v", err)
	}

	insertBestEffortEligibilityRow(t, db, userID, eligibleID, 1200)
	insertBestEffortEligibilityRow(t, db, userID, multiSessionID, 600)
	insertBestEffortEligibilityRow(t, db, userID, fusionSourceID, 700)

	records, err := db.GetBestEffortPersonalRecords(ctx, userID, "running", 3, nil, nil)
	if err != nil {
		t.Fatalf("GetBestEffortPersonalRecords: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("records = %#v, want only eligible activity", records)
	}
	if records[0].ActivityID != eligibleID {
		t.Fatalf("record activity = %s, want %s", records[0].ActivityID, eligibleID)
	}
}

func TestGetBestEffortActivityInfoExcludesMultiSessionActivities(t *testing.T) {
	db, userID, _ := bestEffortsCleanupDB(t)
	activityID := insertBestEffortEligibilityActivity(t, db, userID, "single-info-multi", ", num_sessions")

	info, err := db.GetBestEffortActivityInfo(context.Background(), activityID)
	if err != nil {
		t.Fatalf("GetBestEffortActivityInfo: %v", err)
	}
	if info != nil {
		t.Fatalf("GetBestEffortActivityInfo returned %#v, want nil for multi-session activity", info)
	}
}
