package database

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/model"
)

func TestListSegmentsCompletedOnlyFiltersByUserEffort(t *testing.T) {
	db, userID := activitiesParentTestDB(t)
	ctx := context.Background()
	prefix := fmt.Sprintf("completed-filter-%d", time.Now().UnixNano())

	completed := createCompletedFilterSegment(t, db, userID, prefix+"-completed")
	incomplete := createCompletedFilterSegment(t, db, userID, prefix+"-incomplete")
	t.Cleanup(func() {
		_, _ = db.Pool.Exec(context.Background(), `DELETE FROM segments WHERE id = ANY($1::uuid[])`, []string{completed.ID, incomplete.ID})
	})

	act, err := db.InsertActivity(ctx, userID, &model.ActivityData{
		SportType: "cycling",
		StartedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("InsertActivity: %v", err)
	}

	versionID := latestSegmentVersionID(t, db, completed.ID)
	if _, err := db.Pool.Exec(ctx, `
		INSERT INTO segment_efforts (segment_id, segment_version_id, activity_id, user_id, started_at, elapsed_time_s, sport_type, sub_type)
		VALUES ($1, $2, $3, $4, $5, 60, 'cycling', 'generic')
	`, completed.ID, versionID, act.ID, userID, time.Now().UTC()); err != nil {
		t.Fatalf("insert segment effort: %v", err)
	}

	all, err := db.ListSegments(ctx, model.ListSegmentsParams{
		UserID:       userID,
		Name:         prefix,
		SegmentTypes: "standard",
		Limit:        -1,
	})
	if err != nil {
		t.Fatalf("ListSegments all: %v", err)
	}
	if len(all.Segments) != 2 {
		t.Fatalf("all segments count = %d, want 2", len(all.Segments))
	}

	onlyCompleted, err := db.ListSegments(ctx, model.ListSegmentsParams{
		UserID:        userID,
		Name:          prefix,
		SegmentTypes:  "standard",
		CompletedOnly: true,
		Limit:         -1,
	})
	if err != nil {
		t.Fatalf("ListSegments completed only: %v", err)
	}
	if len(onlyCompleted.Segments) != 1 {
		t.Fatalf("completed segments count = %d, want 1", len(onlyCompleted.Segments))
	}
	if onlyCompleted.Segments[0].ID != completed.ID {
		t.Fatalf("completed segment id = %s, want %s", onlyCompleted.Segments[0].ID, completed.ID)
	}
	if onlyCompleted.Segments[0].RiddenCount != 1 {
		t.Fatalf("completed segment ridden_count = %d, want 1", onlyCompleted.Segments[0].RiddenCount)
	}
}

func TestListSegmentsCompletedOnlyFiltersLiftSegments(t *testing.T) {
	db, userID := activitiesParentTestDB(t)
	ctx := context.Background()
	prefix := fmt.Sprintf("completed-lift-filter-%d", time.Now().UnixNano())

	completed := createCompletedFilterSegmentOfType(t, db, userID, prefix+"-completed", "lift")
	incomplete := createCompletedFilterSegmentOfType(t, db, userID, prefix+"-incomplete", "lift")
	t.Cleanup(func() {
		_, _ = db.Pool.Exec(context.Background(), `DELETE FROM segments WHERE id = ANY($1::uuid[])`, []string{completed.ID, incomplete.ID})
	})

	act, err := db.InsertActivity(ctx, userID, &model.ActivityData{
		SportType: "cycling",
		StartedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("InsertActivity: %v", err)
	}

	versionID := latestSegmentVersionID(t, db, completed.ID)
	if _, err := db.Pool.Exec(ctx, `
		INSERT INTO segment_efforts (segment_id, segment_version_id, activity_id, user_id, started_at, elapsed_time_s, sport_type, sub_type)
		VALUES ($1, $2, $3, $4, $5, 60, 'cycling', 'generic')
	`, completed.ID, versionID, act.ID, userID, time.Now().UTC()); err != nil {
		t.Fatalf("insert segment effort: %v", err)
	}

	onlyCompleted, err := db.ListSegments(ctx, model.ListSegmentsParams{
		UserID:        userID,
		Name:          prefix,
		SegmentTypes:  "lift",
		CompletedOnly: true,
		Limit:         -1,
	})
	if err != nil {
		t.Fatalf("ListSegments completed lift only: %v", err)
	}
	if len(onlyCompleted.Segments) != 1 {
		t.Fatalf("completed lift segments count = %d, want 1", len(onlyCompleted.Segments))
	}
	if onlyCompleted.Segments[0].ID != completed.ID {
		t.Fatalf("completed lift segment id = %s, want %s", onlyCompleted.Segments[0].ID, completed.ID)
	}
}

func createCompletedFilterSegment(t *testing.T, db *DB, userID, name string) *model.Segment {
	return createCompletedFilterSegmentOfType(t, db, userID, name, "standard")
}

func createCompletedFilterSegmentOfType(t *testing.T, db *DB, userID, name, segmentType string) *model.Segment {
	t.Helper()
	segment, err := db.CreateSegment(context.Background(), userID, &model.SegmentCreate{
		Name:         name,
		SportGroup:   "bike",
		SegmentType:  segmentType,
		GeoJSONTrack: `{"type":"LineString","coordinates":[[7,47],[7.001,47.001]]}`,
		Visibility:   "public",
	})
	if err != nil {
		t.Fatalf("CreateSegment %s: %v", name, err)
	}
	return segment
}

func latestSegmentVersionID(t *testing.T, db *DB, segmentID string) string {
	t.Helper()
	var versionID string
	if err := db.Pool.QueryRow(context.Background(), `
		SELECT id FROM segment_versions
		WHERE segment_id = $1
		ORDER BY version_number DESC
		LIMIT 1
	`, segmentID).Scan(&versionID); err != nil {
		t.Fatalf("latest segment version: %v", err)
	}
	return versionID
}
