package database

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/model"
	"github.com/google/uuid"
)

func TestSeedSegmentRecalculationRunUsesActivityTimeVersion(t *testing.T) {
	db, userID := segmentPRTestDB(t)
	ctx := context.Background()
	segmentID, versionID := insertVersioningTestSegment(t, db, userID, "Seed Activity-Time Version")
	cutover := time.Date(2025, 2, 1, 0, 0, 0, 0, time.UTC)
	if _, err := db.Pool.Exec(ctx, `
		UPDATE segment_versions
		SET valid_until_at = $2
		WHERE id = $1
	`, versionID, cutover); err != nil {
		t.Fatalf("bound v1: %v", err)
	}
	if _, err := db.Pool.Exec(ctx, `
		INSERT INTO segment_versions (
			segment_id, version_number, track, start_point, end_point, valid_from, created_by,
			valid_from_at, validity_basis, matching_contract_key, matching_contract_version, published_at
		)
		VALUES (
			$1, 2,
			ST_GeomFromText('LINESTRING(20 50, 20.01 50.01)', 4326),
			ST_StartPoint(ST_GeomFromText('LINESTRING(20 50, 20.01 50.01)', 4326)),
			ST_EndPoint(ST_GeomFromText('LINESTRING(20 50, 20.01 50.01)', 4326)),
			CURRENT_DATE, $2, $3, 'editor_asserted', 'course_gps_v1', 1, NOW()
		)
	`, segmentID, userID, cutover); err != nil {
		t.Fatalf("insert v2: %v", err)
	}
	if err := db.RebuildSegmentCourseEligibilityProjection(ctx, segmentID); err != nil {
		t.Fatalf("rebuild eligibility projection: %v", err)
	}
	activityAt := time.Date(2025, 1, 15, 8, 0, 0, 0, time.UTC)
	insertSegmentRecalculationActivityTrack(t, db, userID, activityAt)

	runID, total, err := db.SeedSegmentRecalculationRun(ctx, userID, SegmentRecalculationScope{
		SegmentID:       segmentID,
		OwnerUserID:     userID,
		OperationID:     "activity-time-version-" + uuid.NewString(),
		IncludeAllUsers: false,
	}, "test activity-time active version", nil)
	if err != nil {
		t.Fatalf("SeedSegmentRecalculationRun: %v", err)
	}
	if runID == "" || total != 1 {
		t.Fatalf("runID/total = %q/%d, want one queued activity", runID, total)
	}
}

func TestSeedSegmentRecalculationRunRejectsPrivateAllUserScope(t *testing.T) {
	db, userID := segmentPRTestDB(t)
	ctx := context.Background()
	segmentID, _ := insertVersioningTestSegment(t, db, userID, "Seed Private Scope")
	if _, err := db.Pool.Exec(ctx, `UPDATE segments SET visibility = 'private' WHERE id = $1`, segmentID); err != nil {
		t.Fatalf("make segment private: %v", err)
	}

	_, _, err := db.SeedSegmentRecalculationRun(ctx, userID, SegmentRecalculationScope{
		SegmentID:       segmentID,
		OwnerUserID:     userID,
		OperationID:     "private-all-user-" + uuid.NewString(),
		IncludeAllUsers: true,
	}, "test private all-user rejection", nil)
	if !errors.Is(err, ErrSegmentNotAuthorized) {
		t.Fatalf("SeedSegmentRecalculationRun error = %v, want ErrSegmentNotAuthorized", err)
	}
}

func TestRecomputeSegmentActivitySupersedesInsteadOfDeletes(t *testing.T) {
	db, userID := segmentPRTestDB(t)
	ctx := context.Background()
	segmentID, versionID := insertVersioningTestSegment(t, db, userID, "Supersede Replace")
	activityID := insertVersioningTestActivity(t, db, userID, time.Date(2025, 5, 1, 9, 0, 0, 0, time.UTC))
	runID := insertSegmentRecalculationRun(t, db, segmentID, userID)
	startedAt := time.Date(2025, 5, 1, 9, 10, 0, 0, time.UTC)
	oldIdentity := "test-replacement-" + uuid.NewString()
	oldID := insertSegmentRecalculationEffort(t, db, segmentID, versionID, activityID, userID, startedAt, oldIdentity, 60)

	newEffort := testSegmentRecalculationEffort(segmentID, versionID, activityID, userID, startedAt, oldIdentity, 55)
	result, err := db.SupersedeSegmentEffortsForActivitySegments(ctx, activityID, userID, []string{segmentID}, []*model.SegmentEffort{newEffort}, &runID, nil, nil)
	if err != nil {
		t.Fatalf("SupersedeSegmentEffortsForActivitySegments: %v", err)
	}
	if result.Superseded != 1 || result.Inserted != 1 {
		t.Fatalf("result = %#v, want one superseded and one inserted", result)
	}

	var activeCount int
	if err := db.Pool.QueryRow(ctx, `
		SELECT count(*)
		FROM segment_efforts
		WHERE segment_id = $1 AND activity_id = $2 AND superseded_at IS NULL
	`, segmentID, activityID).Scan(&activeCount); err != nil {
		t.Fatalf("count active efforts: %v", err)
	}
	if activeCount != 1 {
		t.Fatalf("active efforts = %d, want 1", activeCount)
	}

	var reason string
	var replacementID string
	var supersededRunID string
	if err := db.Pool.QueryRow(ctx, `
		SELECT superseded_reason, superseded_by_effort_id::text, superseded_by_recalculation_run_id::text
		FROM segment_efforts
		WHERE id = $1
	`, oldID).Scan(&reason, &replacementID, &supersededRunID); err != nil {
		t.Fatalf("load superseded effort: %v", err)
	}
	if reason != "recalculated_replaced" || replacementID == "" || supersededRunID != runID {
		t.Fatalf("old effort reason/replacement/run = %q/%q/%q", reason, replacementID, supersededRunID)
	}
}

func TestRecomputeSegmentActivityCanSupersedeToNoReplacement(t *testing.T) {
	db, userID := segmentPRTestDB(t)
	ctx := context.Background()
	segmentID, versionID := insertVersioningTestSegment(t, db, userID, "Supersede Removed")
	activityID := insertVersioningTestActivity(t, db, userID, time.Date(2025, 5, 2, 9, 0, 0, 0, time.UTC))
	runID := insertSegmentRecalculationRun(t, db, segmentID, userID)
	oldID := insertSegmentRecalculationEffort(t, db, segmentID, versionID, activityID, userID, time.Date(2025, 5, 2, 9, 10, 0, 0, time.UTC), "test-removed-"+uuid.NewString(), 60)

	result, err := db.SupersedeSegmentEffortsForActivitySegments(ctx, activityID, userID, []string{segmentID}, nil, &runID, nil, nil)
	if err != nil {
		t.Fatalf("SupersedeSegmentEffortsForActivitySegments: %v", err)
	}
	if result.Superseded != 1 || result.Inserted != 0 {
		t.Fatalf("result = %#v, want one superseded and zero inserted", result)
	}

	var activeCount int
	var reason string
	var replacementID *string
	if err := db.Pool.QueryRow(ctx, `
		SELECT
			(SELECT count(*) FROM segment_efforts WHERE segment_id = $1 AND activity_id = $2 AND superseded_at IS NULL),
			superseded_reason,
			superseded_by_effort_id::text
		FROM segment_efforts
		WHERE id = $3
	`, segmentID, activityID, oldID).Scan(&activeCount, &reason, &replacementID); err != nil {
		t.Fatalf("load removed effort: %v", err)
	}
	if activeCount != 0 || reason != "recalculated_removed" || replacementID != nil {
		t.Fatalf("active/reason/replacement = %d/%q/%v", activeCount, reason, replacementID)
	}
}

func TestSegmentRecalculationRunFailureLeavesOldEffortsActive(t *testing.T) {
	db, userID := segmentPRTestDB(t)
	ctx := context.Background()
	segmentID, versionID := insertVersioningTestSegment(t, db, userID, "Supersede Rollback")
	activityID := insertVersioningTestActivity(t, db, userID, time.Date(2025, 5, 3, 9, 0, 0, 0, time.UTC))
	runID := insertSegmentRecalculationRun(t, db, segmentID, userID)
	startedAt := time.Date(2025, 5, 3, 9, 10, 0, 0, time.UTC)
	identity := "test-rollback-" + uuid.NewString()
	oldID := insertSegmentRecalculationEffort(t, db, segmentID, versionID, activityID, userID, startedAt, identity, 60)

	badEffort := testSegmentRecalculationEffort(segmentID, uuid.NewString(), activityID, userID, startedAt, identity, 55)
	if _, err := db.SupersedeSegmentEffortsForActivitySegments(ctx, activityID, userID, []string{segmentID}, []*model.SegmentEffort{badEffort}, &runID, nil, nil); err == nil {
		t.Fatal("SupersedeSegmentEffortsForActivitySegments succeeded with invalid segment version, want error")
	}

	var active bool
	if err := db.Pool.QueryRow(ctx, `
		SELECT superseded_at IS NULL
		FROM segment_efforts
		WHERE id = $1
	`, oldID).Scan(&active); err != nil {
		t.Fatalf("load old effort active state: %v", err)
	}
	if !active {
		t.Fatal("old effort was superseded despite failed replacement insert")
	}
}

func TestSegmentRecalculationRunResumeSkipsCompletedActivities(t *testing.T) {
	db, userID := segmentPRTestDB(t)
	ctx := context.Background()
	clearPendingSegmentRecalculationRuns(t, db)
	segmentID, _ := insertVersioningTestSegment(t, db, userID, "Claim Skip Completed")
	runID := insertSegmentRecalculationRun(t, db, segmentID, userID)
	completedActivityID := insertVersioningTestActivity(t, db, userID, time.Date(2025, 5, 4, 9, 0, 0, 0, time.UTC))
	queuedActivityID := insertVersioningTestActivity(t, db, userID, time.Date(2025, 5, 4, 10, 0, 0, 0, time.UTC))
	insertSegmentRecalculationRunActivity(t, db, runID, completedActivityID, userID, "completed", nil)
	insertSegmentRecalculationRunActivity(t, db, runID, queuedActivityID, userID, "queued", nil)

	claim, err := db.ClaimNextSegmentRecalculationActivity(ctx, "test-worker", time.Minute)
	if err != nil {
		t.Fatalf("ClaimNextSegmentRecalculationActivity: %v", err)
	}
	if claim == nil || claim.ActivityID != queuedActivityID {
		t.Fatalf("claim = %#v, want queued activity %s", claim, queuedActivityID)
	}
}

func TestSegmentRecalculationRunClaimsOnlyQueuedOrStaleRunning(t *testing.T) {
	db, userID := segmentPRTestDB(t)
	ctx := context.Background()
	clearPendingSegmentRecalculationRuns(t, db)
	segmentID, _ := insertVersioningTestSegment(t, db, userID, "Claim Stale Running")
	runID := insertSegmentRecalculationRun(t, db, segmentID, userID)
	freshActivityID := insertVersioningTestActivity(t, db, userID, time.Date(2025, 5, 5, 9, 0, 0, 0, time.UTC))
	staleActivityID := insertVersioningTestActivity(t, db, userID, time.Date(2025, 5, 5, 10, 0, 0, 0, time.UTC))
	futureClaimUntil := time.Now().UTC().Add(time.Hour)
	pastClaimUntil := time.Now().UTC().Add(-time.Hour)
	insertSegmentRecalculationRunActivity(t, db, runID, freshActivityID, userID, "running", &futureClaimUntil)
	insertSegmentRecalculationRunActivity(t, db, runID, staleActivityID, userID, "running", &pastClaimUntil)

	claim, err := db.ClaimNextSegmentRecalculationActivity(ctx, "test-worker", time.Minute)
	if err != nil {
		t.Fatalf("ClaimNextSegmentRecalculationActivity: %v", err)
	}
	if claim == nil || claim.ActivityID != staleActivityID {
		t.Fatalf("claim = %#v, want stale running activity %s", claim, staleActivityID)
	}
}

func TestFailSegmentRecalculationActivityFinalizesRunAndRedactsError(t *testing.T) {
	db, userID := segmentPRTestDB(t)
	ctx := context.Background()
	clearPendingSegmentRecalculationRuns(t, db)
	segmentID, _ := insertVersioningTestSegment(t, db, userID, "Fail Finalize")
	runID := insertSegmentRecalculationRun(t, db, segmentID, userID)
	activityID := insertVersioningTestActivity(t, db, userID, time.Date(2025, 5, 6, 9, 0, 0, 0, time.UTC))
	insertSegmentRecalculationRunActivity(t, db, runID, activityID, userID, "queued", nil)

	claim, err := db.ClaimNextSegmentRecalculationActivity(ctx, "test-worker", time.Minute)
	if err != nil {
		t.Fatalf("ClaimNextSegmentRecalculationActivity: %v", err)
	}
	if claim == nil {
		t.Fatal("claim = nil, want queued activity")
	}
	if err := db.FailSegmentRecalculationActivity(ctx, *claim, "raw sql /tmp/private/path failed"); err != nil {
		t.Fatalf("FailSegmentRecalculationActivity: %v", err)
	}

	var runStatus, errorCode, errorMessage string
	var failedActivities int
	if err := db.Pool.QueryRow(ctx, `
		SELECT r.status, r.failed_activities, ra.error_code, ra.error_message
		FROM segment_recalculation_runs r
		JOIN segment_recalculation_run_activities ra ON ra.run_id = r.id
		WHERE r.id = $1 AND ra.activity_id = $2
	`, runID, activityID).Scan(&runStatus, &failedActivities, &errorCode, &errorMessage); err != nil {
		t.Fatalf("load failed run: %v", err)
	}
	if runStatus != "failed" || failedActivities != 1 || errorCode != "activity_failed" {
		t.Fatalf("run status/failed/code = %q/%d/%q", runStatus, failedActivities, errorCode)
	}
	if errorMessage != "activity recalculation failed" {
		t.Fatalf("error message = %q, want redacted generic message", errorMessage)
	}
}

func TestValidateSegmentRecalculationClaimRejectsObsoleteGeneration(t *testing.T) {
	db, userID := segmentPRTestDB(t)
	ctx := context.Background()
	clearPendingSegmentRecalculationRuns(t, db)
	segmentID, _ := insertVersioningTestSegment(t, db, userID, "Claim Obsolete Generation")
	runID := insertSegmentRecalculationRun(t, db, segmentID, userID)
	activityID := insertVersioningTestActivity(t, db, userID, time.Date(2025, 5, 7, 9, 0, 0, 0, time.UTC))
	insertSegmentRecalculationRunActivity(t, db, runID, activityID, userID, "queued", nil)
	claim, err := db.ClaimNextSegmentRecalculationActivity(ctx, "test-worker", time.Minute)
	if err != nil {
		t.Fatalf("ClaimNextSegmentRecalculationActivity: %v", err)
	}
	if claim == nil {
		t.Fatal("claim = nil, want queued activity")
	}
	if _, err := db.Pool.Exec(ctx, `
		UPDATE segment_recalculation_segment_state
		SET generation = generation + 1
		WHERE segment_id = $1
	`, segmentID); err != nil {
		t.Fatalf("advance generation: %v", err)
	}
	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin validation tx: %v", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	err = ValidateSegmentRecalculationActivityClaimTx(ctx, tx, *claim)
	if !errors.Is(err, ErrSegmentValidation) {
		t.Fatalf("ValidateSegmentRecalculationActivityClaimTx error = %v, want ErrSegmentValidation", err)
	}
}

func insertSegmentRecalculationRun(t *testing.T, db *DB, segmentID, userID string) string {
	t.Helper()
	runID := uuid.NewString()
	if _, err := db.Pool.Exec(context.Background(), `
		INSERT INTO segment_recalculation_segment_state (segment_id, generation)
		VALUES ($1, 1)
		ON CONFLICT (segment_id) DO UPDATE
			SET generation = 1,
			    updated_at = NOW()
	`, segmentID); err != nil {
		t.Fatalf("insert recalculation state: %v", err)
	}
	if _, err := db.Pool.Exec(context.Background(), `
		INSERT INTO segment_recalculation_runs (
			id, segment_id, actor_user_id, operation_key_hash, idempotency_key,
			generation, reason
		)
		VALUES ($1, $2, $3, $4, $5, 1, 'test recalculation')
	`, runID, segmentID, userID, "test-operation-"+runID, "test-idempotency-"+runID); err != nil {
		t.Fatalf("insert recalculation run: %v", err)
	}
	return runID
}

func clearPendingSegmentRecalculationRuns(t *testing.T, db *DB) {
	t.Helper()
	if _, err := db.Pool.Exec(context.Background(), `
		UPDATE segment_recalculation_run_activities
		SET status = 'completed'
		WHERE status IN ('queued', 'running')
	`); err != nil {
		t.Fatalf("clear pending recalculation activities: %v", err)
	}
	if _, err := db.Pool.Exec(context.Background(), `
		UPDATE segment_recalculation_runs
		SET status = 'completed',
		    completed_at = NOW()
		WHERE status IN ('queued', 'running')
	`); err != nil {
		t.Fatalf("clear pending recalculation runs: %v", err)
	}
}

func insertSegmentRecalculationRunActivity(t *testing.T, db *DB, runID, activityID, userID, status string, claimedUntil *time.Time) {
	t.Helper()
	if _, err := db.Pool.Exec(context.Background(), `
		INSERT INTO segment_recalculation_run_activities (
			run_id, activity_id, user_id, status, claim_token, claimed_until
		)
		VALUES ($1, $2, $3, $4, CASE WHEN $5::timestamptz IS NULL THEN NULL ELSE gen_random_uuid() END, $5)
	`, runID, activityID, userID, status, claimedUntil); err != nil {
		t.Fatalf("insert recalculation run activity: %v", err)
	}
}

func insertSegmentRecalculationEffort(t *testing.T, db *DB, segmentID, versionID, activityID, userID string, startedAt time.Time, identity string, elapsed int) string {
	t.Helper()
	effort := testSegmentRecalculationEffort(segmentID, versionID, activityID, userID, startedAt, identity, elapsed)
	if err := db.InsertSegmentEffort(context.Background(), effort); err != nil {
		t.Fatalf("insert segment effort: %v", err)
	}
	var id string
	if err := db.Pool.QueryRow(context.Background(), `
		SELECT id
		FROM segment_efforts
		WHERE segment_id = $1 AND activity_id = $2 AND match_identity_key = $3 AND superseded_at IS NULL
	`, segmentID, activityID, identity).Scan(&id); err != nil {
		t.Fatalf("lookup segment effort: %v", err)
	}
	return id
}

func insertSegmentRecalculationActivityTrack(t *testing.T, db *DB, userID string, startedAt time.Time) string {
	t.Helper()
	ctx := context.Background()
	activityID := uuid.NewString()
	if _, err := db.Pool.Exec(ctx, `
		INSERT INTO activities (id, user_id, sport_type, sub_type, is_electric, is_indoor, is_virtual, title, started_at, file_hash, visibility, enrichment_status)
		VALUES ($1, $2, 'cycling', '', false, false, false, 'Segment Recalculation Activity', $3, $4, 'private', 'complete')
	`, activityID, userID, startedAt, "segment-recalculation-track-"+activityID); err != nil {
		t.Fatalf("insert recalculation activity: %v", err)
	}
	if _, err := db.Pool.Exec(ctx, `
		INSERT INTO activity_tracks (activity_id, track_full, track_simplified, bbox, point_count)
		VALUES (
			$1,
			ST_GeomFromText('LINESTRING(8 49, 8.01 49.01)', 4326),
			ST_GeomFromText('LINESTRING(8 49, 8.01 49.01)', 4326),
			ST_Envelope(ST_GeomFromText('LINESTRING(8 49, 8.01 49.01)', 4326)),
			2
		)
	`, activityID); err != nil {
		t.Fatalf("insert recalculation activity track: %v", err)
	}
	return activityID
}

func testSegmentRecalculationEffort(segmentID, versionID, activityID, userID string, startedAt time.Time, identity string, elapsed int) *model.SegmentEffort {
	return &model.SegmentEffort{
		SegmentID:                      segmentID,
		SegmentVersionID:               versionID,
		ActivityID:                     activityID,
		UserID:                         userID,
		StartedAt:                      startedAt,
		ElapsedTimeS:                   elapsed,
		SportType:                      "cycling",
		SubType:                        "mountain",
		MatchingContractKey:            "course_gps_v1",
		MatchingContractVersion:        1,
		MatchSourceType:                "platform_activity",
		MatchSourceID:                  activityID,
		MatchIdentityKey:               identity,
		LeaderboardSportType:           "cycling",
		LeaderboardSubType:             "mountain",
		LeaderboardEquipmentClass:      "unspecified",
		LeaderboardAssistClass:         "none",
		LeaderboardIndoorPolicyState:   "outdoor",
		LeaderboardVirtualPolicyState:  "real",
		LeaderboardElectricPolicyState: "none",
		LeaderboardDirection:           "forward",
		LeaderboardVersionPolicyKey:    "legacy_segment_wide",
		LeaderboardContractKey:         "course_gps_v1",
		LeaderboardContractVersion:     1,
		ActivityTraitBucketKey:         "cycling:mountain",
		InLeaderboard:                  true,
	}
}
