package database

import (
	"context"
	"fmt"
	"strings"
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

func TestListSegmentsLimitMinusOneKeepsAllRowsAndGeoJSON(t *testing.T) {
	db, userID := activitiesParentTestDB(t)
	ctx := context.Background()
	prefix := fmt.Sprintf("limit-minus-one-%d", time.Now().UnixNano())

	first := createCompletedFilterSegment(t, db, userID, prefix+"-a")
	second := createCompletedFilterSegment(t, db, userID, prefix+"-b")
	t.Cleanup(func() {
		_, _ = db.Pool.Exec(context.Background(), `DELETE FROM segments WHERE id = ANY($1::uuid[])`, []string{first.ID, second.ID})
	})

	result, err := db.ListSegments(ctx, model.ListSegmentsParams{
		UserID:         userID,
		Name:           prefix,
		SegmentTypes:   "standard",
		IncludeGeoJSON: true,
		Limit:          -1,
	})
	if err != nil {
		t.Fatalf("ListSegments: %v", err)
	}
	if len(result.Segments) != 2 {
		t.Fatalf("segments count = %d, want 2", len(result.Segments))
	}
	if result.HasMore {
		t.Fatal("has_more = true, want false for limit=-1")
	}
	for _, segment := range result.Segments {
		if segment.TrackGeoJSON == nil || !strings.Contains(*segment.TrackGeoJSON, "LineString") {
			t.Fatalf("segment %s missing LineString GeoJSON: %#v", segment.ID, segment.TrackGeoJSON)
		}
	}
}

func TestListSegmentsElevationKindFiltersProjection(t *testing.T) {
	db, userID := activitiesParentTestDB(t)
	ctx := context.Background()
	prefix := fmt.Sprintf("elevation-kind-%d", time.Now().UnixNano())

	flat := createCompletedFilterSegmentWithElevation(t, db, userID, prefix+"-flat", "standard", "standard", nil)
	uphill := createCompletedFilterSegmentWithElevation(t, db, userID, prefix+"-uphill", "standard", "bergauf", nil)
	hillyKind := "hilly"
	hilly := createCompletedFilterSegmentWithElevation(t, db, userID, prefix+"-hilly", "standard", "standard", &hillyKind)
	downhill := createCompletedFilterSegmentWithElevation(t, db, userID, prefix+"-downhill", "standard", "bergab", nil)
	t.Cleanup(func() {
		_, _ = db.Pool.Exec(context.Background(), `DELETE FROM segments WHERE id = ANY($1::uuid[])`, []string{flat.ID, uphill.ID, hilly.ID, downhill.ID})
	})

	for _, tc := range []struct {
		kind string
		want string
	}{
		{"flat", flat.ID},
		{"uphill", uphill.ID},
		{"hilly", hilly.ID},
		{"downhill", downhill.ID},
	} {
		t.Run(tc.kind, func(t *testing.T) {
			result, err := db.ListSegments(ctx, model.ListSegmentsParams{
				UserID:        userID,
				Name:          prefix,
				SegmentTypes:  "standard",
				ElevationKind: tc.kind,
				Limit:         -1,
			})
			if err != nil {
				t.Fatalf("ListSegments: %v", err)
			}
			if len(result.Segments) != 1 {
				t.Fatalf("segments count = %d, want 1 (%s)", len(result.Segments), tc.want)
			}
			if result.Segments[0].ID != tc.want {
				t.Fatalf("segment id = %s, want %s", result.Segments[0].ID, tc.want)
			}
			if result.Segments[0].ElevationKind != tc.kind {
				t.Fatalf("elevation_kind = %s, want %s", result.Segments[0].ElevationKind, tc.kind)
			}
		})
	}
}

func TestListSegmentsElevationKindDoesNotIncludeLiftsByAccident(t *testing.T) {
	db, userID := activitiesParentTestDB(t)
	ctx := context.Background()
	prefix := fmt.Sprintf("elevation-lift-gate-%d", time.Now().UnixNano())

	standard := createCompletedFilterSegmentWithElevation(t, db, userID, prefix+"-standard", "standard", "standard", nil)
	lift := createCompletedFilterSegmentWithElevation(t, db, userID, prefix+"-lift", "lift", "standard", nil)
	t.Cleanup(func() {
		_, _ = db.Pool.Exec(context.Background(), `DELETE FROM segments WHERE id = ANY($1::uuid[])`, []string{standard.ID, lift.ID})
	})

	result, err := db.ListSegments(ctx, model.ListSegmentsParams{
		UserID:        userID,
		Name:          prefix,
		ElevationKind: "flat",
		Limit:         -1,
	})
	if err != nil {
		t.Fatalf("ListSegments without lift type: %v", err)
	}
	if len(result.Segments) != 1 || result.Segments[0].ID != standard.ID {
		t.Fatalf("segments without explicit lift = %#v, want only %s", result.Segments, standard.ID)
	}

	withLift, err := db.ListSegments(ctx, model.ListSegmentsParams{
		UserID:        userID,
		Name:          prefix,
		SegmentTypes:  "standard,lift",
		ElevationKind: "flat",
		Limit:         -1,
	})
	if err != nil {
		t.Fatalf("ListSegments with lift type: %v", err)
	}
	if len(withLift.Segments) != 2 {
		t.Fatalf("segments with explicit lift = %d, want 2", len(withLift.Segments))
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

func createCompletedFilterSegmentWithElevation(t *testing.T, db *DB, userID, name, segmentType, climbType string, elevationKind *string) *model.Segment {
	t.Helper()
	segment, err := db.CreateSegment(context.Background(), userID, &model.SegmentCreate{
		Name:          name,
		SportGroup:    "bike",
		SegmentType:   segmentType,
		GeoJSONTrack:  `{"type":"LineString","coordinates":[[7,47],[7.001,47.001]]}`,
		Visibility:    "public",
		ClimbType:     climbType,
		ElevationKind: elevationKind,
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
