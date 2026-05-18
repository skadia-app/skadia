package database

import (
	"context"
	"testing"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/model"
	"github.com/google/uuid"
)

func TestCreateNewSegmentVersionClosesPreviousValidity(t *testing.T) {
	db, userID := segmentPRTestDB(t)
	ctx := context.Background()
	segmentID := uuid.NewString()
	insertCandidateSegment(t, db, userID, segmentID, "Version Closure", "bike", "standard", "")

	if _, err := db.Pool.Exec(ctx, `ALTER TABLE segment_versions DISABLE TRIGGER trg_segment_versions_validity_defaults`); err != nil {
		t.Fatalf("disable validity trigger: %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.Pool.Exec(context.Background(), `ALTER TABLE segment_versions ENABLE TRIGGER trg_segment_versions_validity_defaults`)
	})

	if err := db.createNewSegmentVersion(ctx, segmentID); err != nil {
		t.Fatalf("createNewSegmentVersion: %v", err)
	}

	rows, err := db.Pool.Query(ctx, `
		SELECT version_number, valid_from_at, valid_until_at
		FROM segment_versions
		WHERE segment_id = $1
		ORDER BY version_number`, segmentID)
	if err != nil {
		t.Fatalf("query versions: %v", err)
	}
	defer rows.Close()

	var v1Until, v2From *time.Time
	for rows.Next() {
		var version int
		var from, until *time.Time
		if err := rows.Scan(&version, &from, &until); err != nil {
			t.Fatalf("scan version: %v", err)
		}
		if version == 1 {
			v1Until = until
		}
		if version == 2 {
			v2From = from
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("versions rows: %v", err)
	}
	if v1Until == nil || v2From == nil || !v1Until.Equal(*v2From) {
		t.Fatalf("v1 until = %v, v2 from = %v, want same non-nil boundary", v1Until, v2From)
	}
}

func TestCreateNewSegmentVersionUsesActiveVersionBeforeFutureVersion(t *testing.T) {
	db, userID := segmentPRTestDB(t)
	ctx := context.Background()
	segmentID := uuid.NewString()
	insertCandidateSegment(t, db, userID, segmentID, "Future Scheduled Version", "bike", "standard", "")

	futureAt := time.Now().UTC().Add(24 * time.Hour).Truncate(time.Microsecond)
	if _, err := db.Pool.Exec(ctx, `
		INSERT INTO segment_versions (
			id, segment_id, version_number, track, start_point, end_point, valid_from, created_by,
			valid_from_at, validity_basis, matching_contract_key, matching_contract_version, published_at
		)
		SELECT gen_random_uuid(), $1, 2,
		       ST_GeomFromText('LINESTRING(9 49, 9.01 49.01)', 4326),
		       ST_StartPoint(ST_GeomFromText('LINESTRING(9 49, 9.01 49.01)', 4326)),
		       ST_EndPoint(ST_GeomFromText('LINESTRING(9 49, 9.01 49.01)', 4326)),
		       CURRENT_DATE, $2, $3, 'editor_asserted', 'course_gps_v1', 1, NOW()`,
		segmentID, userID, futureAt); err != nil {
		t.Fatalf("insert future version: %v", err)
	}

	segment, err := db.UpdateSegment(ctx, segmentID, userID, false, &model.SegmentUpdate{NewVersion: true})
	if err != nil {
		t.Fatalf("UpdateSegment(new_version): %v", err)
	}
	if segment == nil {
		t.Fatal("UpdateSegment(new_version) returned nil segment")
	}

	var versionNumber int
	var validUntil time.Time
	var track string
	if err := db.Pool.QueryRow(ctx, `
		SELECT version_number, valid_until_at, ST_AsText(track)
		FROM segment_versions
		WHERE segment_id = $1
		ORDER BY version_number DESC
		LIMIT 1`, segmentID).Scan(&versionNumber, &validUntil, &track); err != nil {
		t.Fatalf("query new version: %v", err)
	}
	if versionNumber != 3 {
		t.Fatalf("new version_number = %d, want 3", versionNumber)
	}
	if !validUntil.Equal(futureAt) {
		t.Fatalf("new valid_until_at = %v, want future boundary %v", validUntil, futureAt)
	}
	if track != "LINESTRING(8 49,8.01 49.01)" {
		t.Fatalf("new track = %q, want copy of currently active v1 track", track)
	}
}
