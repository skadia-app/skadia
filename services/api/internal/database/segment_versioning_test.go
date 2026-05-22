package database

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/model"
	"github.com/google/uuid"
)

func TestUpdateSegmentVersionValidityWritesRevisionSnapshot(t *testing.T) {
	db, userID := segmentPRTestDB(t)
	ctx := context.Background()
	segmentID, versionID := insertVersioningTestSegment(t, db, userID, "Validity Snapshot")
	from := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)

	revision, err := db.UpdateSegmentVersionValidity(ctx, segmentID, versionID, userID, false, model.SegmentVersionValidityUpdate{
		OperationID: "validity-snapshot-" + uuid.NewString(),
		ValidFromAt: &from,
		Reason:      "test validity snapshot",
		Recalculate: true,
	})
	if err != nil {
		t.Fatalf("UpdateSegmentVersionValidity: %v", err)
	}
	if revision.Operation != "validity_update" {
		t.Fatalf("revision operation = %q, want validity_update", revision.Operation)
	}
	if revision.BeforeSnapshotSHA256 == "" || revision.AfterSnapshotSHA256 == "" {
		t.Fatalf("revision hashes missing: %#v", revision)
	}
	var after SegmentVersionSnapshot
	if err := json.Unmarshal(revision.AfterSnapshot, &after); err != nil {
		t.Fatalf("decode after snapshot: %v", err)
	}
	updated, ok := findSnapshotVersion(after, versionID)
	if !ok || updated.ValidFromAt == nil || !updated.ValidFromAt.Equal(from) {
		t.Fatalf("after snapshot version = %#v, want valid_from_at %v", updated, from)
	}
}

func TestUpdateSegmentVersionValidityRejectsOverlap(t *testing.T) {
	db, userID := segmentPRTestDB(t)
	ctx := context.Background()
	segmentID, versionID := insertVersioningTestSegment(t, db, userID, "Validity Overlap")
	future := time.Now().UTC().Add(24 * time.Hour).Truncate(time.Microsecond)
	if _, err := db.Pool.Exec(ctx, `
		UPDATE segment_versions
		SET valid_until_at = $2
		WHERE id = $1
	`, versionID, future); err != nil {
		t.Fatalf("bound v1: %v", err)
	}
	var secondVersionID string
	if err := db.Pool.QueryRow(ctx, `
		INSERT INTO segment_versions (
			segment_id, version_number, track, start_point, end_point, valid_from, created_by,
			valid_from_at, validity_basis, matching_contract_key, matching_contract_version, published_at
		)
		SELECT $1, 2,
		       ST_GeomFromText('LINESTRING(8 49, 8.02 49.02)', 4326),
		       ST_StartPoint(ST_GeomFromText('LINESTRING(8 49, 8.02 49.02)', 4326)),
		       ST_EndPoint(ST_GeomFromText('LINESTRING(8 49, 8.02 49.02)', 4326)),
		       CURRENT_DATE, $2, $3, 'editor_asserted', 'course_gps_v1', 1, NOW()
		RETURNING id
	`, segmentID, userID, future).Scan(&secondVersionID); err != nil {
		t.Fatalf("insert v2: %v", err)
	}
	overlapStart := future.Add(-time.Hour)
	_, err := db.UpdateSegmentVersionValidity(ctx, segmentID, secondVersionID, userID, false, model.SegmentVersionValidityUpdate{
		OperationID: "validity-overlap-" + uuid.NewString(),
		ValidFromAt: &overlapStart,
		Recalculate: true,
	})
	if !errors.Is(err, ErrSegmentValidation) {
		t.Fatalf("UpdateSegmentVersionValidity overlap error = %v, want ErrSegmentValidation", err)
	}
}

func TestUpdateSegmentVersionValiditySchedulesBoundedRun(t *testing.T) {
	db, userID := segmentPRTestDB(t)
	ctx := context.Background()
	segmentID, versionID := insertVersioningTestSegment(t, db, userID, "Validity Recalc")
	activityAt := time.Date(2025, 4, 1, 8, 0, 0, 0, time.UTC)
	activityID := insertVersioningTestActivity(t, db, userID, activityAt)
	from := activityAt.Add(-time.Hour)
	until := activityAt.Add(time.Hour)

	revision, err := db.UpdateSegmentVersionValidity(ctx, segmentID, versionID, userID, false, model.SegmentVersionValidityUpdate{
		OperationID:  "validity-recalc-" + uuid.NewString(),
		ValidFromAt:  &from,
		ValidUntilAt: &until,
		Recalculate:  true,
	})
	if err != nil {
		t.Fatalf("UpdateSegmentVersionValidity: %v", err)
	}
	if revision.RecalculationRunID == nil {
		t.Fatal("revision missing recalculation run id")
	}
	var queued int
	if err := db.Pool.QueryRow(ctx, `
		SELECT count(*)
		FROM segment_recalculation_run_activities
		WHERE run_id = $1 AND activity_id = $2
	`, *revision.RecalculationRunID, activityID).Scan(&queued); err != nil {
		t.Fatalf("query queued recalculation activity: %v", err)
	}
	if queued != 1 {
		t.Fatalf("queued recalculation activities = %d, want 1", queued)
	}
}

func TestUpdateSegmentVersionValidityPublicSegmentQueuesAllUsers(t *testing.T) {
	db, userID := segmentPRTestDB(t)
	ctx := context.Background()
	otherUserID := insertTestUser(t, db)
	segmentID, versionID := insertVersioningTestSegment(t, db, userID, "Validity Public Recalc")
	activityAt := time.Date(2025, 4, 2, 8, 0, 0, 0, time.UTC)
	insertVersioningTestActivity(t, db, userID, activityAt)
	insertVersioningTestActivity(t, db, otherUserID, activityAt)

	revision, err := db.UpdateSegmentVersionValidity(ctx, segmentID, versionID, userID, false, model.SegmentVersionValidityUpdate{
		OperationID: "validity-public-recalc-" + uuid.NewString(),
		Recalculate: true,
	})
	if err != nil {
		t.Fatalf("UpdateSegmentVersionValidity: %v", err)
	}
	if revision.RecalculationRunID == nil {
		t.Fatal("revision missing recalculation run id")
	}
	var queuedUsers int
	if err := db.Pool.QueryRow(ctx, `
		SELECT count(DISTINCT user_id)
		FROM segment_recalculation_run_activities
		WHERE run_id = $1
	`, *revision.RecalculationRunID).Scan(&queuedUsers); err != nil {
		t.Fatalf("query queued recalculation users: %v", err)
	}
	if queuedUsers != 2 {
		t.Fatalf("queued users = %d, want owner and second user", queuedUsers)
	}
}

func TestUpdateSegmentVersionValidityTreatsNullBoundsAsUnbounded(t *testing.T) {
	db, userID := segmentPRTestDB(t)
	ctx := context.Background()
	segmentID, versionID := insertVersioningTestSegment(t, db, userID, "Validity Null Bounds")

	revision, err := db.UpdateSegmentVersionValidity(ctx, segmentID, versionID, userID, false, model.SegmentVersionValidityUpdate{
		OperationID: "validity-null-" + uuid.NewString(),
		Recalculate: true,
	})
	if err != nil {
		t.Fatalf("UpdateSegmentVersionValidity: %v", err)
	}
	if revision.AffectedFromAt != nil || revision.AffectedUntilAt != nil {
		t.Fatalf("affected bounds = %v/%v, want unbounded", revision.AffectedFromAt, revision.AffectedUntilAt)
	}
}

func TestUpdateSegmentVersionValidityForcesRecalculationForHistoricalChange(t *testing.T) {
	db, userID := segmentPRTestDB(t)
	ctx := context.Background()
	segmentID, versionID := insertVersioningTestSegment(t, db, userID, "Validity Recalc Required")
	from := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	_, err := db.UpdateSegmentVersionValidity(ctx, segmentID, versionID, userID, false, model.SegmentVersionValidityUpdate{
		OperationID: "validity-recalc-required-" + uuid.NewString(),
		ValidFromAt: &from,
		Recalculate: false,
	})
	if !errors.Is(err, ErrSegmentValidation) {
		t.Fatalf("UpdateSegmentVersionValidity error = %v, want ErrSegmentValidation", err)
	}
}

func TestUpdateSegmentGeometryWritesRevisionSnapshotAndSchedulesBoundedRun(t *testing.T) {
	db, userID := segmentPRTestDB(t)
	ctx := context.Background()
	segmentID, versionID := insertVersioningTestSegment(t, db, userID, "Geometry Snapshot")

	revision, err := db.UpdateSegmentVersionGeometry(ctx, segmentID, versionID, userID, false, model.SegmentGeometryUpdate{
		OperationID:         "geometry-snapshot-" + uuid.NewString(),
		TrackGeoJSON:        json.RawMessage(`{"type":"LineString","coordinates":[[8,49],[8.03,49.03]]}`),
		Reason:              "test geometry snapshot",
		Recalculate:         true,
		EquivalenceDecision: "new_group",
	})
	if err != nil {
		t.Fatalf("UpdateSegmentVersionGeometry: %v", err)
	}
	if revision.Operation != "geometry_update" || revision.RecalculationRunID == nil {
		t.Fatalf("revision = %#v, want geometry_update with recalculation", revision)
	}
	var versions int
	if err := db.Pool.QueryRow(ctx, `SELECT count(*) FROM segment_versions WHERE segment_id = $1`, segmentID).Scan(&versions); err != nil {
		t.Fatalf("count versions: %v", err)
	}
	if versions != 2 {
		t.Fatalf("versions = %d, want 2", versions)
	}
}

func TestUpdateSegmentGeometryContinueGroupPromotesVersionSpecificSource(t *testing.T) {
	db, userID := segmentPRTestDB(t)
	ctx := context.Background()
	segmentID, versionID := insertVersioningTestSegment(t, db, userID, "Geometry Continue Group")
	oldGroupID := activeTestLeaderboardEquivalenceGroup(t, db, segmentID, versionID)

	revision, err := db.UpdateSegmentVersionGeometry(ctx, segmentID, versionID, userID, false, model.SegmentGeometryUpdate{
		OperationID:         "geometry-continue-" + uuid.NewString(),
		TrackGeoJSON:        json.RawMessage(`{"type":"LineString","coordinates":[[8,49],[8.01,49.01],[8.02,49.02]]}`),
		EquivalenceDecision: "continue_group",
		EquivalenceEvidence: map[string]any{"reason": "minor correction"},
	})
	if err != nil {
		t.Fatalf("UpdateSegmentVersionGeometry: %v", err)
	}
	var after SegmentVersionSnapshot
	if err := json.Unmarshal(revision.AfterSnapshot, &after); err != nil {
		t.Fatalf("decode after snapshot: %v", err)
	}
	var newVersionID string
	for _, version := range after.Versions {
		if version.ID != versionID {
			newVersionID = version.ID
		}
	}
	if newVersionID == "" {
		t.Fatal("new version not found in snapshot")
	}
	newGroupID := activeTestLeaderboardEquivalenceGroup(t, db, segmentID, newVersionID)
	continuedOldGroupID := activeTestLeaderboardEquivalenceGroup(t, db, segmentID, versionID)
	if newGroupID == oldGroupID || continuedOldGroupID == oldGroupID || newGroupID != continuedOldGroupID {
		t.Fatalf("groups old/new/continued-old = %s/%s/%s, want promoted shared group", oldGroupID, newGroupID, continuedOldGroupID)
	}
}

func TestUpdateSegmentGeometryContinueGroupPreservesFrozenEffortPartition(t *testing.T) {
	db, userID := segmentPRTestDB(t)
	ctx := context.Background()
	segmentID, versionID := insertVersioningTestSegment(t, db, userID, "Geometry Continue Efforts")
	activityID := insertVersioningTestActivity(t, db, userID, time.Date(2025, 8, 1, 9, 0, 0, 0, time.UTC))
	effortID := insertSegmentRecalculationEffort(t, db, segmentID, versionID, activityID, userID, time.Date(2025, 8, 1, 9, 5, 0, 0, time.UTC), "continue-effort-"+uuid.NewString(), 60)
	oldGroupID := activeTestLeaderboardEquivalenceGroup(t, db, segmentID, versionID)

	revision, err := db.UpdateSegmentVersionGeometry(ctx, segmentID, versionID, userID, false, model.SegmentGeometryUpdate{
		OperationID:         "geometry-continue-efforts-" + uuid.NewString(),
		TrackGeoJSON:        json.RawMessage(`{"type":"LineString","coordinates":[[8,49],[8.012,49.012],[8.024,49.024]]}`),
		EquivalenceDecision: "continue_group",
		EquivalenceEvidence: map[string]any{"trusted": true, "length_delta_ratio": 999},
	})
	if err != nil {
		t.Fatalf("UpdateSegmentVersionGeometry: %v", err)
	}
	var after SegmentVersionSnapshot
	if err := json.Unmarshal(revision.AfterSnapshot, &after); err != nil {
		t.Fatalf("decode after snapshot: %v", err)
	}
	var newVersionID string
	for _, version := range after.Versions {
		if version.ID != versionID {
			newVersionID = version.ID
		}
	}
	newGroupID := activeTestLeaderboardEquivalenceGroup(t, db, segmentID, newVersionID)

	var effortGroupID string
	if err := db.Pool.QueryRow(ctx, `
		SELECT leaderboard_equivalence_group_id
		FROM segment_efforts
		WHERE id = $1
	`, effortID).Scan(&effortGroupID); err != nil {
		t.Fatalf("query effort group: %v", err)
	}
	if effortGroupID != oldGroupID {
		t.Fatalf("frozen effort group = %s, want original group %s", effortGroupID, oldGroupID)
	}
	if oldGroupID == newGroupID {
		t.Fatalf("old and new group both %s, want regroup projection without rewriting effort", oldGroupID)
	}

	efforts, err := db.GetSegmentLeaderboardWithQuery(ctx, model.SegmentLeaderboardQuery{
		SegmentID:  segmentID,
		Scope:      "current_version",
		SportType:  "cycling",
		OnlyUserID: userID,
	})
	if err != nil {
		t.Fatalf("GetSegmentLeaderboardWithQuery: %v", err)
	}
	if len(efforts) != 1 || efforts[0].ID != effortID {
		t.Fatalf("current leaderboard efforts = %#v, want frozen effort %s projected into current group", efforts, effortID)
	}
}

func TestUpdateSegmentGeometryContinueGroupBuildsTrustedEvidence(t *testing.T) {
	db, userID := segmentPRTestDB(t)
	ctx := context.Background()
	segmentID, versionID := insertVersioningTestSegment(t, db, userID, "Geometry Trusted Evidence")

	revision, err := db.UpdateSegmentVersionGeometry(ctx, segmentID, versionID, userID, false, model.SegmentGeometryUpdate{
		OperationID:         "geometry-trusted-" + uuid.NewString(),
		TrackGeoJSON:        json.RawMessage(`{"type":"LineString","coordinates":[[8,49],[8.01,49.01],[8.02,49.02]]}`),
		EquivalenceDecision: "continue_group",
		EquivalenceEvidence: map[string]any{"trusted": true, "length_delta_ratio": 999},
	})
	if err != nil {
		t.Fatalf("UpdateSegmentVersionGeometry: %v", err)
	}
	var after SegmentVersionSnapshot
	if err := json.Unmarshal(revision.AfterSnapshot, &after); err != nil {
		t.Fatalf("decode after snapshot: %v", err)
	}
	var newVersionID string
	for _, version := range after.Versions {
		if version.ID != versionID {
			newVersionID = version.ID
		}
	}
	if newVersionID == "" {
		t.Fatal("new version not found in snapshot")
	}
	newGroupID := activeTestLeaderboardEquivalenceGroup(t, db, segmentID, newVersionID)
	var metadata json.RawMessage
	if err := db.Pool.QueryRow(ctx, `
		SELECT metadata
		FROM segment_leaderboard_equivalence_groups
		WHERE id = $1
	`, newGroupID).Scan(&metadata); err != nil {
		t.Fatalf("load group metadata: %v", err)
	}
	var payload struct {
		Evidence map[string]any `json:"evidence"`
	}
	if err := json.Unmarshal(metadata, &payload); err != nil {
		t.Fatalf("decode group metadata: %v", err)
	}
	if payload.Evidence["trusted"] == true || payload.Evidence["length_delta_ratio"] == float64(999) {
		t.Fatalf("stored caller-supplied evidence in group metadata: %s", metadata)
	}
	for _, key := range []string{
		"source", "actor_user_id", "target_version_id", "new_version_id",
		"matching_contract_key", "matching_contract_version", "length_m",
		"target_length_m", "length_delta_ratio", "start_distance_m",
		"end_distance_m", "direction",
	} {
		if _, ok := payload.Evidence[key]; !ok {
			t.Fatalf("trusted evidence missing %q: %#v", key, payload.Evidence)
		}
	}
}

func TestUpdateSegmentGeometryValidatesProvidedEndpoints(t *testing.T) {
	db, userID := segmentPRTestDB(t)
	ctx := context.Background()
	segmentID, versionID := insertVersioningTestSegment(t, db, userID, "Geometry Endpoint Validation")

	_, err := db.UpdateSegmentVersionGeometry(ctx, segmentID, versionID, userID, false, model.SegmentGeometryUpdate{
		OperationID:       "geometry-endpoint-" + uuid.NewString(),
		TrackGeoJSON:      json.RawMessage(`{"type":"LineString","coordinates":[[8,49],[8.01,49.01]]}`),
		StartPointGeoJSON: json.RawMessage(`{"type":"Point","coordinates":[8.5,49.5]}`),
	})
	if !errors.Is(err, ErrSegmentValidation) {
		t.Fatalf("UpdateSegmentVersionGeometry endpoint error = %v, want ErrSegmentValidation", err)
	}
}

func TestRollbackSegmentVersionRevisionRestoresBoundsGeometryAndSchedulesRun(t *testing.T) {
	db, userID := segmentPRTestDB(t)
	ctx := context.Background()
	segmentID, versionID := insertVersioningTestSegment(t, db, userID, "Rollback Bounds")
	from := time.Date(2025, 7, 1, 0, 0, 0, 0, time.UTC)
	revision, err := db.UpdateSegmentVersionValidity(ctx, segmentID, versionID, userID, false, model.SegmentVersionValidityUpdate{
		OperationID: "rollback-source-" + uuid.NewString(),
		ValidFromAt: &from,
		Recalculate: true,
	})
	if err != nil {
		t.Fatalf("UpdateSegmentVersionValidity: %v", err)
	}

	rollback, err := db.RollbackSegmentVersionRevision(ctx, segmentID, revision.ID, userID, false, "test rollback")
	if err != nil {
		t.Fatalf("RollbackSegmentVersionRevision: %v", err)
	}
	if rollback.Operation != "rollback_revision" || rollback.RollbackOfRevisionID == nil || *rollback.RollbackOfRevisionID != revision.ID {
		t.Fatalf("rollback revision = %#v, want rollback of %s", rollback, revision.ID)
	}
	var restoredFrom *time.Time
	if err := db.Pool.QueryRow(ctx, `SELECT valid_from_at FROM segment_versions WHERE id = $1`, versionID).Scan(&restoredFrom); err != nil {
		t.Fatalf("query restored bound: %v", err)
	}
	if restoredFrom != nil {
		t.Fatalf("restored valid_from_at = %v, want nil", restoredFrom)
	}
}

func TestRollbackSegmentVersionRevisionRetiresVersionsAbsentFromSnapshot(t *testing.T) {
	db, userID := segmentPRTestDB(t)
	ctx := context.Background()
	segmentID, versionID := insertVersioningTestSegment(t, db, userID, "Rollback Retire Absent")

	revision, err := db.UpdateSegmentVersionGeometry(ctx, segmentID, versionID, userID, false, model.SegmentGeometryUpdate{
		OperationID:         "rollback-retire-source-" + uuid.NewString(),
		TrackGeoJSON:        json.RawMessage(`{"type":"LineString","coordinates":[[8,49],[8.03,49.03]]}`),
		EquivalenceDecision: "new_group",
	})
	if err != nil {
		t.Fatalf("UpdateSegmentVersionGeometry: %v", err)
	}
	var after SegmentVersionSnapshot
	if err := json.Unmarshal(revision.AfterSnapshot, &after); err != nil {
		t.Fatalf("decode geometry snapshot: %v", err)
	}
	var newVersionID string
	for _, version := range after.Versions {
		if version.ID != versionID {
			newVersionID = version.ID
		}
	}
	if newVersionID == "" {
		t.Fatal("new version not found")
	}

	if _, err := db.RollbackSegmentVersionRevision(ctx, segmentID, revision.ID, userID, false, "test rollback"); err != nil {
		t.Fatalf("RollbackSegmentVersionRevision: %v", err)
	}
	var restoredFrom *time.Time
	var retiredBasis string
	if err := db.Pool.QueryRow(ctx, `SELECT valid_from_at FROM segment_versions WHERE id = $1`, versionID).Scan(&restoredFrom); err != nil {
		t.Fatalf("query restored version: %v", err)
	}
	if err := db.Pool.QueryRow(ctx, `SELECT validity_basis FROM segment_versions WHERE id = $1`, newVersionID).Scan(&retiredBasis); err != nil {
		t.Fatalf("query retired version: %v", err)
	}
	if restoredFrom != nil || retiredBasis != "rollback_retired" {
		t.Fatalf("restored_from/retired_basis = %v/%q, want nil/rollback_retired", restoredFrom, retiredBasis)
	}
}

func TestSegmentVersionOperationIDRejectsDifferentReplayPayload(t *testing.T) {
	db, userID := segmentPRTestDB(t)
	ctx := context.Background()
	segmentID, versionID := insertVersioningTestSegment(t, db, userID, "Operation Replay")
	operationID := "validity-replay-" + uuid.NewString()
	firstFrom := time.Date(2025, 9, 1, 0, 0, 0, 0, time.UTC)
	secondFrom := firstFrom.Add(24 * time.Hour)

	if _, err := db.UpdateSegmentVersionValidity(ctx, segmentID, versionID, userID, false, model.SegmentVersionValidityUpdate{
		OperationID: operationID,
		ValidFromAt: &firstFrom,
		Recalculate: true,
	}); err != nil {
		t.Fatalf("UpdateSegmentVersionValidity first call: %v", err)
	}
	_, err := db.UpdateSegmentVersionValidity(ctx, segmentID, versionID, userID, false, model.SegmentVersionValidityUpdate{
		OperationID: operationID,
		ValidFromAt: &secondFrom,
		Recalculate: true,
	})
	if !errors.Is(err, ErrSegmentValidation) {
		t.Fatalf("UpdateSegmentVersionValidity replay error = %v, want ErrSegmentValidation", err)
	}
}

func TestSegmentUpdateRequiresVersionedPathForMatchingRelevantMetadata(t *testing.T) {
	db, userID := segmentPRTestDB(t)
	ctx := context.Background()
	segmentID, _ := insertVersioningTestSegment(t, db, userID, "Metadata Guard")
	bidirectional := false

	_, err := db.UpdateSegment(ctx, segmentID, userID, false, &model.SegmentUpdate{Bidirectional: &bidirectional})
	if !errors.Is(err, ErrSegmentValidation) {
		t.Fatalf("UpdateSegment metadata error = %v, want ErrSegmentValidation", err)
	}
}

func TestInsertSegmentEffortRequiresActiveEquivalenceMembership(t *testing.T) {
	db, userID := segmentPRTestDB(t)
	ctx := context.Background()
	segmentID, versionID := insertVersioningTestSegment(t, db, userID, "Effort Group")
	activityID := insertVersioningTestActivity(t, db, userID, time.Now().UTC())

	if _, err := db.Pool.Exec(ctx, `
		INSERT INTO segment_efforts (
			segment_id, segment_version_id, activity_id, user_id,
			started_at, elapsed_time_s, sport_type, sub_type
		)
		VALUES ($1, $2, $3, $4, NOW(), 60, 'cycling', 'mountain')
	`, segmentID, versionID, activityID, userID); err != nil {
		t.Fatalf("insert effort without explicit group: %v", err)
	}
	var groupID string
	if err := db.Pool.QueryRow(ctx, `
		SELECT leaderboard_equivalence_group_id
		FROM segment_efforts
		WHERE activity_id = $1
	`, activityID).Scan(&groupID); err != nil {
		t.Fatalf("query effort group: %v", err)
	}
	if groupID == "" {
		t.Fatal("leaderboard_equivalence_group_id was not filled")
	}
}

func insertVersioningTestSegment(t *testing.T, db *DB, userID, name string) (string, string) {
	t.Helper()
	segmentID := uuid.NewString()
	insertCandidateSegment(t, db, userID, segmentID, name, "bike", "standard", "")
	var versionID string
	if err := db.Pool.QueryRow(context.Background(), `
		SELECT id
		FROM segment_versions
		WHERE segment_id = $1
		ORDER BY version_number DESC
		LIMIT 1
	`, segmentID).Scan(&versionID); err != nil {
		t.Fatalf("lookup version: %v", err)
	}
	return segmentID, versionID
}

func insertVersioningTestActivity(t *testing.T, db *DB, userID string, startedAt time.Time) string {
	t.Helper()
	activityID := uuid.NewString()
	if _, err := db.Pool.Exec(context.Background(), `
		INSERT INTO activities (id, user_id, sport_type, sub_type, title, started_at, file_hash, visibility, enrichment_status)
		VALUES ($1, $2, 'cycling', 'mountain', 'Versioning Test Activity', $3, $4, 'private', 'complete')
	`, activityID, userID, startedAt, "segment-versioning-"+activityID); err != nil {
		t.Fatalf("insert activity: %v", err)
	}
	if _, err := db.Pool.Exec(context.Background(), `
		INSERT INTO activity_tracks (activity_id, track_full, track_simplified, bbox, point_count)
		VALUES (
			$1,
			ST_GeomFromText('LINESTRING(8 49, 8.01 49.01)', 4326),
			ST_GeomFromText('LINESTRING(8 49, 8.01 49.01)', 4326),
			ST_Envelope(ST_GeomFromText('LINESTRING(8 49, 8.01 49.01)', 4326)),
			2
		)
	`, activityID); err != nil {
		t.Fatalf("insert activity track: %v", err)
	}
	return activityID
}

func activeTestLeaderboardEquivalenceGroup(t *testing.T, db *DB, segmentID, versionID string) string {
	t.Helper()
	var groupID string
	if err := db.Pool.QueryRow(context.Background(), `
		SELECT gv.group_id
		FROM segment_leaderboard_equivalence_group_versions gv
		JOIN segment_leaderboard_equivalence_groups g ON g.id = gv.group_id
		WHERE gv.segment_version_id = $1
		  AND gv.leaderboard_surface = 'default'
		  AND gv.retired_at IS NULL
		  AND g.segment_id = $2
	`, versionID, segmentID).Scan(&groupID); err != nil {
		t.Fatalf("load active equivalence group for version %s: %v", versionID, err)
	}
	return groupID
}
