package database

import (
	"context"
	"testing"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/model"
	"github.com/google/uuid"
)

func TestFormatCourseGPSV1InstantUsesCanonicalMicroseconds(t *testing.T) {
	tests := []struct {
		name string
		in   time.Time
		want string
	}{
		{
			name: "exact second keeps six fractional digits",
			in:   time.Date(2026, 5, 14, 9, 30, 0, 0, time.UTC),
			want: "2026-05-14T09:30:00.000000Z",
		},
		{
			name: "microsecond value keeps six fractional digits",
			in:   time.Date(2026, 5, 14, 9, 30, 0, 123456789, time.FixedZone("offset", 2*60*60)),
			want: "2026-05-14T07:30:00.123457Z",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := FormatCourseGPSV1Instant(tc.in); got != tc.want {
				t.Fatalf("FormatCourseGPSV1Instant() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestBuildCourseGPSV1MatchIdentityIsStable(t *testing.T) {
	startedAt := time.Date(2026, 5, 14, 9, 30, 0, 123456000, time.UTC)
	got := BuildCourseGPSV1MatchIdentity("act-1", "seg-1", "ver-1", startedAt, true, 2)
	want := "platform_activity:act-1:segment:seg-1:version:ver-1:contract:course_gps_v1:1:direction:reverse:start:2026-05-14T09:30:00.123456Z:occurrence:2"
	if got != want {
		t.Fatalf("identity = %q, want %q", got, want)
	}
}

func TestPrepareCourseGPSV1EffortMetadataFreezesPartition(t *testing.T) {
	effort := &model.SegmentEffort{
		SegmentID:        "seg-1",
		SegmentVersionID: "ver-1",
		ActivityID:       "act-1",
		StartedAt:        time.Date(2026, 5, 14, 9, 30, 0, 0, time.UTC),
		IsReverse:        false,
		SportType:        "cycling",
		SubType:          "mountain",
	}
	ctx := SegmentMatchActivityContext{
		IsIndoor:   true,
		IsVirtual:  false,
		IsElectric: true,
	}

	PrepareCourseGPSV1EffortMetadata(effort, ctx)

	if effort.MatchingContractKey != CourseGPSV1Key || effort.MatchingContractVersion != CourseGPSV1Version {
		t.Fatalf("contract = %s/%d", effort.MatchingContractKey, effort.MatchingContractVersion)
	}
	if effort.MatchSourceType != "platform_activity" || effort.MatchSourceID != "act-1" {
		t.Fatalf("source = %s/%s", effort.MatchSourceType, effort.MatchSourceID)
	}
	if effort.LeaderboardSportType != "cycling" || effort.LeaderboardSubType != "mountain" {
		t.Fatalf("leaderboard sport tuple = %s/%s", effort.LeaderboardSportType, effort.LeaderboardSubType)
	}
	if effort.LeaderboardAssistClass != "electric" {
		t.Fatalf("assist class = %s, want electric", effort.LeaderboardAssistClass)
	}
	if effort.LeaderboardIndoorPolicyState != "indoor" {
		t.Fatalf("indoor state = %s, want indoor", effort.LeaderboardIndoorPolicyState)
	}
	if effort.LeaderboardVirtualPolicyState != "real" {
		t.Fatalf("virtual state = %s, want real", effort.LeaderboardVirtualPolicyState)
	}
	if effort.LeaderboardDirection != "forward" {
		t.Fatalf("direction = %s, want forward", effort.LeaderboardDirection)
	}
	if effort.LeaderboardVersionPolicyKey != "legacy_segment_wide" {
		t.Fatalf("version policy = %s, want legacy_segment_wide", effort.LeaderboardVersionPolicyKey)
	}
}

func TestInsertSegmentEffortsBatchIsIdempotentByMatchIdentity(t *testing.T) {
	db, userID := segmentPRTestDB(t)
	ctx := context.Background()
	activityID, segmentID, versionID, startedAt := insertMatchContractFixture(t, db, userID)

	effort := &model.SegmentEffort{
		SegmentID:        segmentID,
		SegmentVersionID: versionID,
		ActivityID:       activityID,
		UserID:           userID,
		StartedAt:        startedAt,
		ElapsedTimeS:     60,
		SportType:        "cycling",
		SubType:          "mountain",
	}
	PrepareCourseGPSV1EffortMetadata(effort, SegmentMatchActivityContext{IsElectric: true})

	if err := db.InsertSegmentEffortsBatch(ctx, []*model.SegmentEffort{effort}); err != nil {
		t.Fatalf("first insert: %v", err)
	}
	if err := db.InsertSegmentEffortsBatch(ctx, []*model.SegmentEffort{effort}); err != nil {
		t.Fatalf("second insert: %v", err)
	}

	var count int
	if err := db.Pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM segment_efforts WHERE match_identity_key = $1`,
		effort.MatchIdentityKey).Scan(&count); err != nil {
		t.Fatalf("count efforts: %v", err)
	}
	if count != 1 {
		t.Fatalf("effort count = %d, want 1", count)
	}
}

func TestInsertSegmentEffortIsIdempotentByMatchIdentity(t *testing.T) {
	db, userID := segmentPRTestDB(t)
	ctx := context.Background()
	activityID, segmentID, versionID, startedAt := insertMatchContractFixture(t, db, userID)

	effort := &model.SegmentEffort{
		SegmentID:        segmentID,
		SegmentVersionID: versionID,
		ActivityID:       activityID,
		UserID:           userID,
		StartedAt:        startedAt,
		ElapsedTimeS:     60,
		SportType:        "cycling",
		SubType:          "mountain",
	}
	PrepareCourseGPSV1EffortMetadata(effort, SegmentMatchActivityContext{IsElectric: true})

	if err := db.InsertSegmentEffort(ctx, effort); err != nil {
		t.Fatalf("first insert: %v", err)
	}
	if err := db.InsertSegmentEffort(ctx, effort); err != nil {
		t.Fatalf("second insert: %v", err)
	}

	var count int
	if err := db.Pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM segment_efforts WHERE match_identity_key = $1`,
		effort.MatchIdentityKey).Scan(&count); err != nil {
		t.Fatalf("count efforts: %v", err)
	}
	if count != 1 {
		t.Fatalf("effort count = %d, want 1", count)
	}
}

func TestSegmentEffortCurrentReadsIgnoreSupersededRows(t *testing.T) {
	db, userID := segmentPRTestDB(t)
	ctx := context.Background()
	activityID, segmentID, versionID, startedAt := insertMatchContractFixture(t, db, userID)

	effort := &model.SegmentEffort{
		SegmentID:        segmentID,
		SegmentVersionID: versionID,
		ActivityID:       activityID,
		UserID:           userID,
		StartedAt:        startedAt,
		ElapsedTimeS:     60,
		SportType:        "cycling",
		SubType:          "mountain",
	}
	PrepareCourseGPSV1EffortMetadata(effort, SegmentMatchActivityContext{IsElectric: true})
	if err := db.InsertSegmentEffort(ctx, effort); err != nil {
		t.Fatalf("insert active effort: %v", err)
	}

	var activeID string
	if err := db.Pool.QueryRow(ctx,
		`SELECT id FROM segment_efforts WHERE match_identity_key = $1`,
		effort.MatchIdentityKey).Scan(&activeID); err != nil {
		t.Fatalf("query active effort id: %v", err)
	}
	if _, err := db.Pool.Exec(ctx, `
		INSERT INTO segment_efforts (
			id, segment_id, segment_version_id, activity_id, user_id,
			started_at, elapsed_time_s, is_reverse, sport_type, sub_type,
			match_identity_key, superseded_by_effort_id, in_leaderboard
		)
		VALUES (gen_random_uuid(), $1, $2, $3, $4, $5, 60, false, 'cycling', 'mountain', $6, $7, false)
	`, segmentID, versionID, activityID, userID, startedAt, effort.MatchIdentityKey, activeID); err != nil {
		t.Fatalf("insert superseded effort: %v", err)
	}

	activityEfforts, err := db.GetActivitySegmentEfforts(ctx, activityID)
	if err != nil {
		t.Fatalf("GetActivitySegmentEfforts: %v", err)
	}
	if len(activityEfforts) != 1 {
		t.Fatalf("activity efforts = %d, want 1", len(activityEfforts))
	}
	leaderboard, err := db.GetSegmentLeaderboard(ctx, segmentID, "cycling", nil, "", "")
	if err != nil {
		t.Fatalf("GetSegmentLeaderboard: %v", err)
	}
	if len(leaderboard) != 1 {
		t.Fatalf("leaderboard efforts = %d, want 1", len(leaderboard))
	}
	performances, err := db.GetSegmentPerformances(ctx, segmentID, userID, nil)
	if err != nil {
		t.Fatalf("GetSegmentPerformances: %v", err)
	}
	if len(performances) != 1 {
		t.Fatalf("performances = %d, want 1", len(performances))
	}
	if err := db.UpdateSegmentEffortRanks(ctx, segmentID); err != nil {
		t.Fatalf("UpdateSegmentEffortRanks: %v", err)
	}
	var effortCount, athleteCount int
	if err := db.Pool.QueryRow(ctx,
		`SELECT effort_count, athlete_count FROM segments WHERE id = $1`,
		segmentID).Scan(&effortCount, &athleteCount); err != nil {
		t.Fatalf("query segment counts: %v", err)
	}
	if effortCount != 1 || athleteCount != 1 {
		t.Fatalf("segment counts = %d/%d, want 1/1", effortCount, athleteCount)
	}
	result, err := db.ListSegments(ctx, model.ListSegmentsParams{
		UserID:       userID,
		Name:         "Match Contract Segment " + segmentID,
		SegmentTypes: "standard",
		Limit:        -1,
	})
	if err != nil {
		t.Fatalf("ListSegments: %v", err)
	}
	if len(result.Segments) != 1 || result.Segments[0].RiddenCount != 1 {
		t.Fatalf("list segments = %#v, want one row with ridden_count=1", result.Segments)
	}
}

func TestSegmentEffortCountsIncludeCompletedNonLeaderboardRows(t *testing.T) {
	db, userID := segmentPRTestDB(t)
	ctx := context.Background()
	activityID, segmentID, versionID, startedAt := insertMatchContractFixture(t, db, userID)

	effort := &model.SegmentEffort{
		SegmentID:                segmentID,
		SegmentVersionID:         versionID,
		ActivityID:               activityID,
		UserID:                   userID,
		StartedAt:                startedAt,
		ElapsedTimeS:             60,
		SportType:                "cycling",
		SubType:                  "mountain",
		ActivityTraitBucketKey:   "cycling:mountain:real:outdoor:acoustic",
		EligibilityPolicyKey:     courseEligibilityPolicyKey,
		EligibilityPolicyVersion: courseEligibilityPolicyVersion,
		EligibilitySourceHash:    "test-non-rankable",
		InLeaderboard:            false,
	}
	PrepareCourseGPSV1EffortMetadata(effort, SegmentMatchActivityContext{IsElectric: true})
	if err := db.InsertSegmentEffort(ctx, effort); err != nil {
		t.Fatalf("insert non-leaderboard effort: %v", err)
	}
	if err := db.UpdateSegmentEffortRanks(ctx, segmentID); err != nil {
		t.Fatalf("UpdateSegmentEffortRanks: %v", err)
	}

	var effortCount, athleteCount int
	if err := db.Pool.QueryRow(ctx,
		`SELECT effort_count, athlete_count FROM segments WHERE id = $1`,
		segmentID).Scan(&effortCount, &athleteCount); err != nil {
		t.Fatalf("query segment counts: %v", err)
	}
	if effortCount != 1 || athleteCount != 1 {
		t.Fatalf("segment counts = %d/%d, want completed traversal counts 1/1", effortCount, athleteCount)
	}

	var rankOverall *int
	var isPersonalBest, isSegmentRecord bool
	if err := db.Pool.QueryRow(ctx, `
		SELECT rank_overall, is_personal_best, is_segment_record
		FROM segment_efforts
		WHERE segment_id = $1 AND activity_id = $2
	`, segmentID, activityID).Scan(&rankOverall, &isPersonalBest, &isSegmentRecord); err != nil {
		t.Fatalf("query effort rank flags: %v", err)
	}
	if rankOverall != nil || isPersonalBest || isSegmentRecord {
		t.Fatalf("non-leaderboard effort was ranked: rank=%v pb=%t record=%t", rankOverall, isPersonalBest, isSegmentRecord)
	}
}

func insertMatchContractFixture(t *testing.T, db *DB, userID string) (activityID, segmentID, versionID string, startedAt time.Time) {
	t.Helper()
	ctx := context.Background()
	activityID = uuid.NewString()
	startedAt = time.Date(2026, 5, 14, 9, 30, 0, 0, time.UTC)
	if _, err := db.Pool.Exec(ctx,
		`INSERT INTO activities (
			id, user_id, sport_type, sub_type, is_electric, is_indoor, is_virtual,
			title, started_at, moving_time_s, elapsed_time_s, file_hash, visibility, enrichment_status
		)
		 VALUES ($1, $2, 'cycling', 'mountain', true, false, false,
		         'Idempotent Segment Effort Test', $3, 60, 60, $4, 'public', 'complete')`,
		activityID, userID, startedAt, "idempotent-effort-"+activityID); err != nil {
		t.Fatalf("insert activity: %v", err)
	}

	segmentID = uuid.NewString()
	versionID = uuid.NewString()
	if _, err := db.Pool.Exec(ctx,
		`INSERT INTO segments (id, created_by, name, sport_group, segment_type)
		 VALUES ($1, $2, $3, 'bike', 'standard')`,
		segmentID, userID, "Match Contract Segment "+segmentID); err != nil {
		t.Fatalf("insert segment: %v", err)
	}
	if _, err := db.Pool.Exec(ctx,
		`INSERT INTO segment_versions (id, segment_id, version_number, track, valid_from, valid_from_at, created_by)
		 VALUES ($1, $2, 1, ST_GeomFromText('LINESTRING(8 49, 8.01 49.01)', 4326), CURRENT_DATE, NULL, $3)`,
		versionID, segmentID, userID); err != nil {
		t.Fatalf("insert version: %v", err)
	}
	return activityID, segmentID, versionID, startedAt
}
