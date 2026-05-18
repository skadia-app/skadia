package database

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestSpatialCandidateSegmentsExcludeSpecializedEmptyPredicates(t *testing.T) {
	db, userID := segmentPRTestDB(t)
	ctx := context.Background()

	activityID := uuid.NewString()
	activityDate := time.Now().UTC().Add(24 * time.Hour)
	if _, err := db.Pool.Exec(ctx,
		`INSERT INTO activities (id, user_id, sport_type, sub_type, is_electric, is_indoor, is_virtual, title, started_at, file_hash, visibility, enrichment_status)
		 VALUES ($1, $2, 'running', '', false, false, false, 'Spatial Candidate Predicate Test', $3, $4, 'private', 'complete')`,
		activityID, userID, activityDate, "spatial-candidate-"+activityID,
	); err != nil {
		t.Fatalf("insert activity: %v", err)
	}
	if _, err := db.Pool.Exec(ctx,
		`INSERT INTO activity_tracks (activity_id, track_full, track_simplified, bbox, point_count)
		 VALUES (
		   $1,
		   ST_GeomFromText('LINESTRING(8 49, 8.01 49.01)', 4326),
		   ST_GeomFromText('LINESTRING(8 49, 8.01 49.01)', 4326),
		   ST_Envelope(ST_GeomFromText('LINESTRING(8 49, 8.01 49.01)', 4326)),
		   2
		 )`,
		activityID,
	); err != nil {
		t.Fatalf("insert activity track: %v", err)
	}

	nullWinterID := uuid.NewString()
	emptyWinterID := uuid.NewString()
	controlID := uuid.NewString()
	t.Cleanup(func() {
		_, _ = db.Pool.Exec(context.Background(), `DELETE FROM segments WHERE id = ANY($1)`, []string{nullWinterID, emptyWinterID, controlID})
	})

	insertCandidateSegment(t, db, userID, nullWinterID, "Winter Null", "generic", "wintersport", "")
	insertCandidateSegment(t, db, userID, emptyWinterID, "Winter Empty", "generic", "wintersport", "[]")
	insertCandidateSegment(t, db, userID, controlID, "Running Control", "foot", "standard", "")

	candidates, err := db.GetSpatialCandidateSegments(ctx, activityID, userID, "running", activityDate, "")
	if err != nil {
		t.Fatalf("GetSpatialCandidateSegments: %v", err)
	}
	seen := map[string]CandidateSegment{}
	for _, candidate := range candidates {
		seen[candidate.SegmentID] = candidate
	}
	if _, ok := seen[controlID]; !ok {
		t.Fatalf("control segment %s missing from candidates: %#v", controlID, candidates)
	}
	if _, ok := seen[nullWinterID]; ok {
		t.Fatalf("wintersport NULL-predicate segment matched running candidates: %#v", candidates)
	}
	if _, ok := seen[emptyWinterID]; ok {
		t.Fatalf("wintersport empty-predicate segment matched running candidates: %#v", candidates)
	}
}

func TestSpatialCandidateSegmentsUsesUnboundedVersionValidity(t *testing.T) {
	db, userID := segmentPRTestDB(t)
	ctx := context.Background()
	activityID, activityDate := insertSpatialCandidateActivity(t, db, userID, "cycling")
	segmentID := uuid.NewString()
	insertCandidateSegment(t, db, userID, segmentID, "Unbounded Validity", "bike", "standard", "")

	if _, err := db.Pool.Exec(ctx, `
		UPDATE segment_versions
		SET valid_from_at = NULL, valid_until_at = NULL
		WHERE segment_id = $1`, segmentID); err != nil {
		t.Fatalf("set unbounded validity: %v", err)
	}

	candidates, err := db.GetSpatialCandidateSegments(ctx, activityID, userID, "cycling", activityDate.Add(-365*24*time.Hour), "")
	if err != nil {
		t.Fatalf("GetSpatialCandidateSegments: %v", err)
	}
	if !candidateIDs(candidates)[segmentID] {
		t.Fatalf("unbounded segment missing from candidates: %#v", candidates)
	}
}

func TestSpatialCandidateSegmentsRejectsFutureOnlyVersionWithoutFallback(t *testing.T) {
	db, userID := segmentPRTestDB(t)
	ctx := context.Background()
	activityID, activityDate := insertSpatialCandidateActivity(t, db, userID, "cycling")
	segmentID := uuid.NewString()
	insertCandidateSegment(t, db, userID, segmentID, "Future Only", "bike", "standard", "")

	if _, err := db.Pool.Exec(ctx, `
		UPDATE segment_versions
		SET valid_from_at = $2, valid_until_at = NULL
		WHERE segment_id = $1`, segmentID, activityDate.Add(24*time.Hour)); err != nil {
		t.Fatalf("set future validity: %v", err)
	}

	candidates, err := db.GetSpatialCandidateSegments(ctx, activityID, userID, "cycling", activityDate, "")
	if err != nil {
		t.Fatalf("GetSpatialCandidateSegments: %v", err)
	}
	if candidateIDs(candidates)[segmentID] {
		t.Fatalf("future-only segment matched via fallback: %#v", candidates)
	}
}

func insertCandidateSegment(t *testing.T, db *DB, userID, segmentID, name, sportGroup, segmentType, allowedPredicatesJSON string) {
	t.Helper()
	ctx := context.Background()
	versionID := uuid.NewString()
	if _, err := db.Pool.Exec(ctx,
		`INSERT INTO segments (id, created_by, name, sport_group, segment_type, allowed_predicates)
		 VALUES ($1, $2, $3, $4, $5, NULLIF($6, '')::jsonb)`,
		segmentID, userID, name, sportGroup, segmentType, allowedPredicatesJSON,
	); err != nil {
		t.Fatalf("insert segment %s: %v", name, err)
	}
	if _, err := db.Pool.Exec(ctx,
		`INSERT INTO segment_versions (id, segment_id, version_number, track, valid_from, created_by)
		 VALUES ($1, $2, 1, ST_GeomFromText('LINESTRING(8 49, 8.01 49.01)', 4326), CURRENT_DATE, $3)`,
		versionID, segmentID, userID,
	); err != nil {
		t.Fatalf("insert segment version %s: %v", name, err)
	}
}

func insertSpatialCandidateActivity(t *testing.T, db *DB, userID, sportType string) (string, time.Time) {
	t.Helper()
	ctx := context.Background()
	activityID := uuid.NewString()
	activityDate := time.Now().UTC()
	if _, err := db.Pool.Exec(ctx,
		`INSERT INTO activities (id, user_id, sport_type, sub_type, is_electric, is_indoor, is_virtual, title, started_at, file_hash, visibility, enrichment_status)
		 VALUES ($1, $2, $3, '', false, false, false, 'Spatial Candidate Validity Test', $4, $5, 'private', 'complete')`,
		activityID, userID, sportType, activityDate, "spatial-candidate-validity-"+activityID); err != nil {
		t.Fatalf("insert activity: %v", err)
	}
	if _, err := db.Pool.Exec(ctx,
		`INSERT INTO activity_tracks (activity_id, track_full, track_simplified, bbox, point_count)
		 VALUES (
		   $1,
		   ST_GeomFromText('LINESTRING(8 49, 8.01 49.01)', 4326),
		   ST_GeomFromText('LINESTRING(8 49, 8.01 49.01)', 4326),
		   ST_Envelope(ST_GeomFromText('LINESTRING(8 49, 8.01 49.01)', 4326)),
		   2
		 )`,
		activityID); err != nil {
		t.Fatalf("insert activity track: %v", err)
	}
	return activityID, activityDate
}

func candidateIDs(candidates []CandidateSegment) map[string]bool {
	ids := make(map[string]bool, len(candidates))
	for _, candidate := range candidates {
		ids[candidate.SegmentID] = true
	}
	return ids
}
