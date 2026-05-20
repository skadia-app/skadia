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

func TestSegmentUpdateRequiresVersionedPathForMatchingRelevantMetadata(t *testing.T) {
	db, userID := segmentPRTestDB(t)
	ctx := context.Background()
	segmentID, _ := insertVersioningTestSegment(t, db, userID, "Metadata Guard")
	terrain := "gravel"

	_, err := db.UpdateSegment(ctx, segmentID, userID, false, &model.SegmentUpdate{TerrainType: &terrain})
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
	return activityID
}
