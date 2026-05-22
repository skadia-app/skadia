package database

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/database/testdb"
	"github.com/google/uuid"
)

func TestGetSegmentPerformancesCollapsesRecordingGroupAndDuplicateEfforts(t *testing.T) {
	pool := testdb.Open(t)
	db := &DB{Pool: pool}
	ctx := context.Background()

	suffix := uuid.NewString()[:8]
	var userID string
	if err := pool.QueryRow(ctx,
		`INSERT INTO users (username, email, registration_source)
		 VALUES ($1, $2, 'test')
		 RETURNING id`,
		"segment-perf-"+suffix,
		"segment-perf-"+suffix+"@test.localhost",
	).Scan(&userID); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, userID)
	})

	segmentID := uuid.NewString()
	versionID := uuid.NewString()
	if _, err := pool.Exec(ctx,
		`INSERT INTO segments (id, created_by, name, sport_group, segment_type)
		 VALUES ($1, $2, 'Fruit Loops Test', 'bike', 'standard')`,
		segmentID, userID,
	); err != nil {
		t.Fatalf("insert segment: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO segment_versions (id, segment_id, version_number, track, valid_from, created_by)
		 VALUES ($1, $2, 1, ST_GeomFromText('LINESTRING(-179 -80, -178.99 -79.99)', 4326), CURRENT_DATE, $3)`,
		versionID, segmentID, userID,
	); err != nil {
		t.Fatalf("insert segment version: %v", err)
	}

	startedAt := time.Date(2024, 6, 20, 13, 49, 58, 0, time.UTC)
	primaryID := uuid.NewString()
	secondaryID := uuid.NewString()
	for _, row := range []struct {
		id          string
		title       string
		source      string
		isSecondary bool
	}{
		{primaryID, "Mueckenplage Neuburg", "Garmin", false},
		{secondaryID, "Mountainbike-Tour", "Komoot", true},
	} {
		if _, err := pool.Exec(ctx,
			`INSERT INTO activities
			 (id, user_id, title, sport_type, sub_type, started_at, moving_time_s, source, is_group_secondary, visibility)
			 VALUES ($1, $2, $3, 'cycling', 'mountain', $4, 9000, $5, $6, 'public')`,
			row.id, userID, row.title, startedAt, row.source, row.isSecondary,
		); err != nil {
			t.Fatalf("insert activity %s: %v", row.id, err)
		}
	}

	groupID := uuid.NewString()
	if _, err := pool.Exec(ctx,
		`INSERT INTO recording_groups (id, user_id, primary_activity_id, sport_type, sub_type)
		 VALUES ($1, $2, $3, 'cycling', 'mountain')`,
		groupID, userID, primaryID,
	); err != nil {
		t.Fatalf("insert recording group: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO recording_group_members (group_id, activity_id)
		 VALUES ($1, $2), ($1, $3)`,
		groupID, primaryID, secondaryID,
	); err != nil {
		t.Fatalf("insert recording group members: %v", err)
	}

	effortStarts := []time.Time{
		startedAt.Add(48*time.Minute + 24*time.Second),
		startedAt.Add(64*time.Minute + 59*time.Second),
		startedAt.Add(170*time.Minute + 42*time.Second),
	}
	for _, activityID := range []string{primaryID, secondaryID} {
		for _, effortStart := range effortStarts {
			activeEffortID := uuid.NewString()
			if _, err := pool.Exec(ctx,
				`INSERT INTO segment_efforts
				 (id, segment_id, segment_version_id, activity_id, user_id, started_at, elapsed_time_s, is_reverse, sport_type, sub_type)
				 VALUES ($1, $2, $3, $4, $5, $6, 60, false, 'cycling', 'mountain')`,
				activeEffortID, segmentID, versionID, activityID, userID, effortStart,
			); err != nil {
				t.Fatalf("insert active segment effort: %v", err)
			}
			for range 2 {
				if _, err := pool.Exec(ctx,
					`INSERT INTO segment_efforts
					 (id, segment_id, segment_version_id, activity_id, user_id, started_at, elapsed_time_s, is_reverse, sport_type, sub_type, superseded_by_effort_id, in_leaderboard)
					 VALUES ($1, $2, $3, $4, $5, $6, 60, false, 'cycling', 'mountain', $7, false)`,
					uuid.NewString(), segmentID, versionID, activityID, userID, effortStart, activeEffortID,
				); err != nil {
					t.Fatalf("insert superseded segment effort: %v", err)
				}
			}
		}
	}

	activityEfforts, err := db.GetActivitySegmentEfforts(ctx, primaryID)
	if err != nil {
		t.Fatalf("GetActivitySegmentEfforts: %v", err)
	}
	if len(activityEfforts) != 3 {
		t.Fatalf("activity efforts = %d, want 3", len(activityEfforts))
	}

	performances, err := db.GetSegmentPerformances(ctx, segmentID, userID, nil)
	if err != nil {
		t.Fatalf("GetSegmentPerformances: %v", err)
	}
	if len(performances) != 3 {
		t.Fatalf("performances = %d, want 3", len(performances))
	}
	for _, perf := range performances {
		if perf.ActivityID != primaryID {
			t.Fatalf("performance activity_id = %s, want primary %s", perf.ActivityID, primaryID)
		}
	}

	if err := db.UpdateSegmentEffortRanks(ctx, segmentID); err != nil {
		t.Fatalf("UpdateSegmentEffortRanks: %v", err)
	}
	leaderboard, err := db.GetSegmentLeaderboard(ctx, segmentID, "", nil, "", "")
	if err != nil {
		t.Fatalf("GetSegmentLeaderboard: %v", err)
	}
	if len(leaderboard) != 3 {
		t.Fatalf("leaderboard rows = %d, want 3", len(leaderboard))
	}
	for _, effort := range leaderboard {
		if effort.ActivityID != primaryID {
			t.Fatalf("leaderboard activity_id = %s, want primary %s", effort.ActivityID, primaryID)
		}
	}

	segment, err := db.GetSegment(ctx, segmentID, userID, false, false)
	if err != nil {
		t.Fatalf("GetSegment: %v", err)
	}
	if segment.EffortCount != 3 {
		t.Fatalf("segment effort_count = %d, want 3", segment.EffortCount)
	}
	if segment.RiddenCount != 3 {
		t.Fatalf("segment ridden_count = %d, want 3", segment.RiddenCount)
	}
}

func TestListActivityIDsForSegmentRecomputeIncludesExistingEfforts(t *testing.T) {
	pool := testdb.Open(t)
	db := &DB{Pool: pool}
	ctx := context.Background()

	suffix := uuid.NewString()[:8]
	var userID string
	if err := pool.QueryRow(ctx,
		`INSERT INTO users (username, email, registration_source)
		 VALUES ($1, $2, 'test')
		 RETURNING id`,
		"segment-recompute-"+suffix,
		"segment-recompute-"+suffix+"@test.localhost",
	).Scan(&userID); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, userID)
	})

	segmentID := uuid.NewString()
	versionID := uuid.NewString()
	if _, err := pool.Exec(ctx,
		`INSERT INTO segments (id, created_by, name, sport_group, segment_type)
		 VALUES ($1, $2, 'stale effort segment', 'bike', 'standard')`,
		segmentID, userID,
	); err != nil {
		t.Fatalf("insert segment: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO segment_versions (id, segment_id, version_number, track, valid_from, created_by)
		 VALUES ($1, $2, 1, ST_GeomFromText('LINESTRING(-179 -80, -178.99 -79.99)', 4326), CURRENT_DATE, $3)`,
		versionID, segmentID, userID,
	); err != nil {
		t.Fatalf("insert segment version: %v", err)
	}

	activityID := uuid.NewString()
	startedAt := time.Date(2024, 6, 20, 13, 49, 58, 0, time.UTC)
	if _, err := pool.Exec(ctx,
		`INSERT INTO activities
		 (id, user_id, title, sport_type, sub_type, started_at, moving_time_s, visibility)
		 VALUES ($1, $2, 'stale effort activity', 'cycling', 'mountain', $3, 9000, 'public')`,
		activityID, userID, startedAt,
	); err != nil {
		t.Fatalf("insert activity: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO segment_efforts
		 (segment_id, segment_version_id, activity_id, user_id, started_at, elapsed_time_s, sport_type, sub_type)
		 VALUES ($1, $2, $3, $4, $5, 60, 'cycling', 'mountain')`,
		segmentID, versionID, activityID, userID, startedAt.Add(5*time.Minute),
	); err != nil {
		t.Fatalf("insert stale effort: %v", err)
	}

	ids, err := db.ListActivityIDsForSegmentRecompute(ctx, userID, segmentID)
	if err != nil {
		t.Fatalf("ListActivityIDsForSegmentRecompute: %v", err)
	}
	if len(ids) != 1 || ids[0] != activityID {
		t.Fatalf("recompute ids = %v, want [%s]", ids, activityID)
	}

	allIDs, err := db.ListAllActivityIDsForSegmentRecompute(ctx, segmentID)
	if err != nil {
		t.Fatalf("ListAllActivityIDsForSegmentRecompute: %v", err)
	}
	if len(allIDs) != 1 || allIDs[0] != activityID {
		t.Fatalf("all recompute ids = %v, want [%s]", allIDs, activityID)
	}
}

func TestListSegmentsUsesSegmentScopedEffortDedupe(t *testing.T) {
	src, err := os.ReadFile("segments.go")
	if err != nil {
		t.Fatalf("read segments.go: %v", err)
	}
	body := string(src)
	if strings.Contains(body, "WITH user_efforts_ranked AS") {
		t.Fatalf("ListSegments still uses a global user_efforts_ranked CTE")
	}
	if !strings.Contains(body, "useInlineRiddenCounts := p.CompletedOnly || p.LiftsOnlyRidden") {
		t.Fatalf("ListSegments does not restrict inline viewer aggregation to filters that need it")
	}
	if !strings.Contains(body, "attachListSegmentRiddenCounts") {
		t.Fatalf("ListSegments does not batch viewer ridden counts after page selection")
	}
}
