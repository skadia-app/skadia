package database

import (
	"context"
	"testing"
	"time"

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
