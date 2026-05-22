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

func TestCheckpointSequenceV1Constants(t *testing.T) {
	if CheckpointSequenceV1Key != "checkpoint_sequence_v1" {
		t.Fatalf("CheckpointSequenceV1Key = %q", CheckpointSequenceV1Key)
	}
	if CheckpointSequenceV1Version != 1 {
		t.Fatalf("CheckpointSequenceV1Version = %d", CheckpointSequenceV1Version)
	}
	if CheckpointSequencePolicyKey != "checkpoint_sequence_current_version" {
		t.Fatalf("CheckpointSequencePolicyKey = %q", CheckpointSequencePolicyKey)
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

func TestInsertSegmentEffortWithoutEligibilityProvenanceIsNonLeaderboard(t *testing.T) {
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
		InLeaderboard:    true,
	}
	PrepareCourseGPSV1EffortMetadata(effort, SegmentMatchActivityContext{IsElectric: true})
	if err := db.InsertSegmentEffort(ctx, effort); err != nil {
		t.Fatalf("InsertSegmentEffort: %v", err)
	}

	var inLeaderboard bool
	if err := db.Pool.QueryRow(ctx, `
		SELECT in_leaderboard
		FROM segment_efforts
		WHERE match_identity_key = $1
	`, effort.MatchIdentityKey).Scan(&inLeaderboard); err != nil {
		t.Fatalf("load effort leaderboard state: %v", err)
	}
	if inLeaderboard {
		t.Fatal("effort without eligibility provenance was rankable")
	}
}

func TestInsertSegmentEffortRejectsUntrustedEligibilityBypass(t *testing.T) {
	db, userID := segmentPRTestDB(t)
	ctx := context.Background()
	activityID, segmentID, versionID, startedAt := insertMatchContractFixture(t, db, userID)

	effort := &model.SegmentEffort{
		SegmentID:             segmentID,
		SegmentVersionID:      versionID,
		ActivityID:            activityID,
		UserID:                userID,
		StartedAt:             startedAt,
		ElapsedTimeS:          60,
		SportType:             "cycling",
		SubType:               "mountain",
		EligibilityPolicyKey:  "untrusted_bypass",
		EligibilitySourceHash: "caller-supplied",
		InLeaderboard:         true,
	}
	PrepareCourseGPSV1EffortMetadata(effort, SegmentMatchActivityContext{IsElectric: true})
	if err := db.InsertSegmentEffort(ctx, effort); err != nil {
		t.Fatalf("InsertSegmentEffort: %v", err)
	}

	var inLeaderboard bool
	if err := db.Pool.QueryRow(ctx, `
		SELECT in_leaderboard
		FROM segment_efforts
		WHERE match_identity_key = $1
	`, effort.MatchIdentityKey).Scan(&inLeaderboard); err != nil {
		t.Fatalf("load effort leaderboard state: %v", err)
	}
	if inLeaderboard {
		t.Fatal("effort with untrusted eligibility bypass was rankable")
	}
}

func TestInsertSegmentEffortAllowsExplicitLegacyLeaderboardBypass(t *testing.T) {
	db, userID := segmentPRTestDB(t)
	ctx := context.Background()
	activityID, segmentID, versionID, startedAt := insertMatchContractFixture(t, db, userID)

	effort := &model.SegmentEffort{
		SegmentID:             segmentID,
		SegmentVersionID:      versionID,
		ActivityID:            activityID,
		UserID:                userID,
		StartedAt:             startedAt,
		ElapsedTimeS:          60,
		SportType:             "cycling",
		SubType:               "mountain",
		EligibilityPolicyKey:  "legacy_leaderboard_bypass",
		EligibilitySourceHash: "legacy-test-bypass",
		InLeaderboard:         true,
	}
	PrepareCourseGPSV1EffortMetadata(effort, SegmentMatchActivityContext{IsElectric: true})
	if err := db.InsertSegmentEffort(ctx, effort); err != nil {
		t.Fatalf("InsertSegmentEffort: %v", err)
	}

	var inLeaderboard bool
	if err := db.Pool.QueryRow(ctx, `
		SELECT in_leaderboard
		FROM segment_efforts
		WHERE match_identity_key = $1
	`, effort.MatchIdentityKey).Scan(&inLeaderboard); err != nil {
		t.Fatalf("load effort leaderboard state: %v", err)
	}
	if !inLeaderboard {
		t.Fatal("explicit legacy leaderboard bypass was not rankable")
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
		InLeaderboard:    true,
	}
	PrepareCourseGPSV1EffortMetadata(effort, SegmentMatchActivityContext{IsElectric: true})
	effort.ActivityTraitBucketKey = "test-current-read"
	effort.EligibilityPolicyKey = courseEligibilityPolicyKey
	effort.EligibilityPolicyVersion = courseEligibilityPolicyVersion
	effort.EligibilitySourceHash = "test-current-read"
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
	history, err := db.GetActivitySegmentEffortHistory(ctx, activityID, segmentID)
	if err != nil {
		t.Fatalf("GetActivitySegmentEffortHistory: %v", err)
	}
	if len(history) != 2 || history[0].SupersededAt == nil && history[1].SupersededAt == nil {
		t.Fatalf("activity effort history = %#v, want active and superseded rows", history)
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

func TestSegmentLeaderboardAggregatesOnlyWithinEquivalenceGroup(t *testing.T) {
	db, userID := segmentPRTestDB(t)
	ctx := context.Background()
	segmentID, v1ID, v2ID := insertLeaderboardTwoVersionFixture(t, db, userID)
	v1GroupID := leaderboardGroupForVersion(t, db, v1ID)

	activity1ID, started1 := insertLeaderboardTestActivity(t, db, userID, "cycling", "mountain", false, time.Date(2025, 6, 1, 8, 0, 0, 0, time.UTC))
	activity2ID, started2 := insertLeaderboardTestActivity(t, db, userID, "cycling", "mountain", false, time.Date(2026, 6, 1, 8, 0, 0, 0, time.UTC))
	insertPreparedLeaderboardEffort(t, db, userID, segmentID, v1ID, activity1ID, started1, 80, false)
	insertPreparedLeaderboardEffort(t, db, userID, segmentID, v2ID, activity2ID, started2, 70, false)

	if err := db.UpdateSegmentEffortRanks(ctx, segmentID); err != nil {
		t.Fatalf("UpdateSegmentEffortRanks: %v", err)
	}

	current, err := db.GetSegmentLeaderboard(ctx, segmentID, "cycling", nil, "", "")
	if err != nil {
		t.Fatalf("GetSegmentLeaderboard current: %v", err)
	}
	if len(current) != 1 || current[0].SegmentVersionID != v2ID || current[0].VersionNumber != 2 {
		t.Fatalf("current leaderboard = %#v, want only version 2", current)
	}

	historical, err := db.GetSegmentLeaderboardWithQuery(ctx, model.SegmentLeaderboardQuery{
		SegmentID:                     segmentID,
		SportType:                     "cycling",
		Scope:                         "equivalence_group",
		LeaderboardEquivalenceGroupID: v1GroupID,
	})
	if err != nil {
		t.Fatalf("GetSegmentLeaderboardWithQuery historical: %v", err)
	}
	if len(historical) != 1 || historical[0].SegmentVersionID != v1ID || historical[0].VersionNumber != 1 {
		t.Fatalf("historical leaderboard = %#v, want only version 1", historical)
	}
}

func TestSegmentLeaderboardCanRequestSportOverallAcrossVersions(t *testing.T) {
	db, userID := segmentPRTestDB(t)
	ctx := context.Background()
	segmentID, v1ID, v2ID := insertLeaderboardTwoVersionFixture(t, db, userID)

	activity1ID, started1 := insertLeaderboardTestActivity(t, db, userID, "cycling", "mountain", false, time.Date(2025, 7, 1, 8, 0, 0, 0, time.UTC))
	activity2ID, started2 := insertLeaderboardTestActivity(t, db, userID, "cycling", "mountain", false, time.Date(2026, 7, 1, 8, 0, 0, 0, time.UTC))
	insertPreparedLeaderboardEffort(t, db, userID, segmentID, v1ID, activity1ID, started1, 80, false)
	insertPreparedLeaderboardEffort(t, db, userID, segmentID, v2ID, activity2ID, started2, 70, false)

	overall, err := db.GetSegmentLeaderboardWithQuery(ctx, model.SegmentLeaderboardQuery{
		SegmentID: segmentID,
		SportType: "cycling",
		Scope:     "sport_overall",
	})
	if err != nil {
		t.Fatalf("GetSegmentLeaderboardWithQuery sport_overall: %v", err)
	}
	if len(overall) != 2 {
		t.Fatalf("sport overall rows = %d, want 2: %#v", len(overall), overall)
	}
	if overall[0].SegmentVersionID != v2ID || overall[0].RankOverall == nil || *overall[0].RankOverall != 1 {
		t.Fatalf("sport overall first = %#v, want v2 rank 1", overall[0])
	}
	if overall[1].SegmentVersionID != v1ID || overall[1].VersionNumber != 1 || overall[1].LeaderboardEquivalenceGroupID == "" {
		t.Fatalf("sport overall second = %#v, want v1 with group context", overall[1])
	}
}

func TestSegmentLeaderboardElectricModeFiltersAndDerivesRanks(t *testing.T) {
	db, userID := segmentPRTestDB(t)
	ctx := context.Background()
	activityID, segmentID, versionID, startedAt := insertMatchContractFixture(t, db, userID)
	if _, err := db.Pool.Exec(ctx, `UPDATE activities SET is_electric = false WHERE id = $1`, activityID); err != nil {
		t.Fatalf("mark base activity non-electric: %v", err)
	}

	electricActivityID, electricStarted := insertLeaderboardTestActivity(t, db, userID, "cycling", "mountain", true, startedAt.Add(time.Hour))
	secondElectricID, secondElectricStarted := insertLeaderboardTestActivity(t, db, userID, "cycling", "mountain", true, startedAt.Add(2*time.Hour))
	insertPreparedLeaderboardEffort(t, db, userID, segmentID, versionID, activityID, startedAt, 60, false)
	insertPreparedLeaderboardEffort(t, db, userID, segmentID, versionID, electricActivityID, electricStarted, 50, true)
	insertPreparedLeaderboardEffort(t, db, userID, segmentID, versionID, secondElectricID, secondElectricStarted, 70, true)

	onlyElectric, err := db.GetSegmentLeaderboardWithQuery(ctx, model.SegmentLeaderboardQuery{
		SegmentID:    segmentID,
		SportType:    "cycling",
		ElectricMode: "only_electric",
	})
	if err != nil {
		t.Fatalf("GetSegmentLeaderboardWithQuery only_electric: %v", err)
	}
	if len(onlyElectric) != 2 || !onlyElectric[0].IsElectric || onlyElectric[0].RankOverall == nil || *onlyElectric[0].RankOverall != 1 || onlyElectric[1].RankOverall == nil || *onlyElectric[1].RankOverall != 2 {
		t.Fatalf("only electric leaderboard = %#v, want two electric rows ranked 1/2", onlyElectric)
	}

	excludeElectric, err := db.GetSegmentLeaderboardWithQuery(ctx, model.SegmentLeaderboardQuery{
		SegmentID:    segmentID,
		SportType:    "cycling",
		ElectricMode: "exclude_electric",
	})
	if err != nil {
		t.Fatalf("GetSegmentLeaderboardWithQuery exclude_electric: %v", err)
	}
	if len(excludeElectric) != 1 || excludeElectric[0].IsElectric || excludeElectric[0].RankOverall == nil || *excludeElectric[0].RankOverall != 1 {
		t.Fatalf("exclude electric leaderboard = %#v, want one non-electric row ranked 1", excludeElectric)
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

func insertLeaderboardTwoVersionFixture(t *testing.T, db *DB, userID string) (segmentID, v1ID, v2ID string) {
	t.Helper()
	ctx := context.Background()
	segmentID = uuid.NewString()
	v1ID = uuid.NewString()
	v2ID = uuid.NewString()
	boundary := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	if _, err := db.Pool.Exec(ctx,
		`INSERT INTO segments (id, created_by, name, sport_group, segment_type)
		 VALUES ($1, $2, $3, 'bike', 'standard')`,
		segmentID, userID, "Leaderboard Version Segment "+segmentID); err != nil {
		t.Fatalf("insert segment: %v", err)
	}
	if _, err := db.Pool.Exec(ctx,
		`INSERT INTO segment_versions (id, segment_id, version_number, track, valid_from, valid_until_at, created_by)
		 VALUES ($1, $2, 1, ST_GeomFromText('LINESTRING(8 49, 8.01 49.01)', 4326), CURRENT_DATE, $3, $4)`,
		v1ID, segmentID, boundary, userID); err != nil {
		t.Fatalf("insert v1: %v", err)
	}
	if _, err := db.Pool.Exec(ctx,
		`INSERT INTO segment_versions (id, segment_id, version_number, track, valid_from, valid_from_at, created_by)
		 VALUES ($1, $2, 2, ST_GeomFromText('LINESTRING(8 49, 8.02 49.02)', 4326), CURRENT_DATE, $3, $4)`,
		v2ID, segmentID, boundary, userID); err != nil {
		t.Fatalf("insert v2: %v", err)
	}
	return segmentID, v1ID, v2ID
}

func insertLeaderboardTestActivity(t *testing.T, db *DB, userID, sportType, subType string, isElectric bool, startedAt time.Time) (string, time.Time) {
	t.Helper()
	activityID := uuid.NewString()
	if _, err := db.Pool.Exec(context.Background(),
		`INSERT INTO activities (
			id, user_id, sport_type, sub_type, is_electric, is_indoor, is_virtual,
			title, started_at, moving_time_s, elapsed_time_s, file_hash, visibility, enrichment_status
		)
		 VALUES ($1, $2, $3, $4, $5, false, false,
		         'Leaderboard Query Test', $6, 60, 60, $7, 'public', 'complete')`,
		activityID, userID, sportType, subType, isElectric, startedAt, "leaderboard-query-"+activityID); err != nil {
		t.Fatalf("insert activity: %v", err)
	}
	return activityID, startedAt
}

func insertPreparedLeaderboardEffort(t *testing.T, db *DB, userID, segmentID, versionID, activityID string, startedAt time.Time, elapsed int, isElectric bool) {
	t.Helper()
	effort := &model.SegmentEffort{
		SegmentID:        segmentID,
		SegmentVersionID: versionID,
		ActivityID:       activityID,
		UserID:           userID,
		StartedAt:        startedAt,
		ElapsedTimeS:     elapsed,
		SportType:        "cycling",
		SubType:          "mountain",
		InLeaderboard:    true,
	}
	PrepareCourseGPSV1EffortMetadata(effort, SegmentMatchActivityContext{IsElectric: isElectric})
	effort.ActivityTraitBucketKey = "test-leaderboard"
	effort.EligibilityPolicyKey = courseEligibilityPolicyKey
	effort.EligibilityPolicyVersion = courseEligibilityPolicyVersion
	effort.EligibilitySourceHash = "test-leaderboard"
	if err := db.InsertSegmentEffort(context.Background(), effort); err != nil {
		t.Fatalf("insert segment effort: %v", err)
	}
}

func leaderboardGroupForVersion(t *testing.T, db *DB, versionID string) string {
	t.Helper()
	var groupID string
	if err := db.Pool.QueryRow(context.Background(), `
		SELECT group_id
		FROM segment_leaderboard_equivalence_group_versions
		WHERE segment_version_id = $1
		  AND leaderboard_surface = 'default'
		  AND retired_at IS NULL
	`, versionID).Scan(&groupID); err != nil {
		t.Fatalf("load group for version: %v", err)
	}
	return groupID
}
