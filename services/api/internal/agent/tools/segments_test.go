package tools

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/skadia-app/skadia/services/api/internal/database/testdb"
)

func TestSegmentsToolRegistersAsUserOwnedReadTool(t *testing.T) {
	reg := NewRegistry()
	RegisterSegmentTools(reg)

	tool := reg.tools["query_segments"]
	if tool == nil {
		t.Fatalf("missing query_segments")
	}
	if tool.Permission != PermissionRead {
		t.Fatalf("permission = %s, want read", tool.Permission)
	}
	if !tool.IsUserOwned {
		t.Fatalf("query_segments must be user-owned")
	}
	if tool.ExecuteRead == nil {
		t.Fatalf("query_segments has no read executor")
	}
	if err := reg.ValidateSchemas(); err != nil {
		t.Fatalf("ValidateSchemas: %v", err)
	}
}

func TestQuerySegmentsFiltersBySegmentEffortUserID(t *testing.T) {
	ctx := context.Background()
	pool := testdb.Open(t)
	ownerID := "00000000-0000-0000-0000-0000000bb101"
	otherID := "00000000-0000-0000-0000-0000000bb102"
	insertToolTestUser(t, pool, ownerID, "agent-segment-owner")
	insertToolTestUser(t, pool, otherID, "agent-segment-other")
	ids := insertToolTestSegmentFixture(t, pool, ownerID, otherID)
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM segment_efforts WHERE segment_id = $1::uuid`, ids.segmentID)
		_, _ = pool.Exec(ctx, `DELETE FROM activities WHERE id = ANY($1::uuid[])`, []string{ids.ownerActivityID, ids.otherActivityID})
		_, _ = pool.Exec(ctx, `DELETE FROM segments WHERE id = $1::uuid`, ids.segmentID)
	})

	got, err := querySegments(ctx, pool, ownerID, querySegmentsArgs{
		Query:          "Hill",
		IncludeEfforts: true,
		Days:           365,
		Limit:          5,
	})
	if err != nil {
		t.Fatalf("querySegments: %v", err)
	}
	segments := got["segments"].([]map[string]any)
	if len(segments) != 1 {
		t.Fatalf("segments len = %d, want 1: %#v", len(segments), segments)
	}
	seg := segments[0]
	if seg["effort_count"] != int64(1) {
		t.Fatalf("effort_count = %#v, want 1", seg["effort_count"])
	}
	if seg["pr_seconds"] != int32(300) && seg["pr_seconds"] != int64(300) && seg["pr_seconds"] != 300 {
		t.Fatalf("pr_seconds = %#v, want 300", seg["pr_seconds"])
	}
	efforts := seg["recent_efforts"].([]map[string]any)
	if len(efforts) != 1 {
		t.Fatalf("recent efforts len = %d, want 1: %#v", len(efforts), efforts)
	}
	if efforts[0]["time"] != "5min" {
		t.Fatalf("recent effort time = %#v, want 5min", efforts[0]["time"])
	}
}

func TestQuerySegmentsAppliesEffortFilters(t *testing.T) {
	ctx := context.Background()
	pool := testdb.Open(t)
	ownerID := "00000000-0000-0000-0000-0000000bb103"
	otherID := "00000000-0000-0000-0000-0000000bb104"
	insertToolTestUser(t, pool, ownerID, "agent-segment-filter-owner")
	insertToolTestUser(t, pool, otherID, "agent-segment-filter-other")
	ids := insertToolTestSegmentFixture(t, pool, ownerID, otherID)
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM segment_efforts WHERE segment_id = $1::uuid`, ids.segmentID)
		_, _ = pool.Exec(ctx, `DELETE FROM activities WHERE id = ANY($1::uuid[])`, []string{ids.ownerActivityID, ids.otherActivityID})
		_, _ = pool.Exec(ctx, `DELETE FROM segments WHERE id = $1::uuid`, ids.segmentID)
	})

	got, err := querySegments(ctx, pool, ownerID, querySegmentsArgs{
		Query:         "Hill",
		Days:          365,
		Limit:         5,
		SportType:     "running",
		InLeaderboard: segmentBoolPtr(false),
	})
	if err != nil {
		t.Fatalf("querySegments: %v", err)
	}
	if len(got["segments"].([]map[string]any)) != 0 {
		t.Fatalf("running/non-leaderboard filter returned segments: %#v", got)
	}
}

func insertToolTestUser(t *testing.T, pool *pgxpool.Pool, id, username string) {
	t.Helper()
	_, err := pool.Exec(context.Background(), `
		INSERT INTO users (id, username, email)
		VALUES ($1::uuid, $2, $2 || '@example.invalid')
		ON CONFLICT (id) DO UPDATE SET username = EXCLUDED.username, email = EXCLUDED.email
	`, id, username)
	if err != nil {
		t.Fatalf("insert user %s: %v", id, err)
	}
}

type toolTestSegmentIDs struct {
	segmentID       string
	ownerActivityID string
	otherActivityID string
}

func insertToolTestSegmentFixture(t *testing.T, pool *pgxpool.Pool, ownerID, otherID string) toolTestSegmentIDs {
	t.Helper()
	ctx := context.Background()
	ids := toolTestSegmentIDs{
		segmentID:       "00000000-0000-0000-0000-0000000bb201",
		ownerActivityID: "00000000-0000-0000-0000-0000000bb301",
		otherActivityID: "00000000-0000-0000-0000-0000000bb302",
	}
	versionID := "00000000-0000-0000-0000-0000000bb401"
	if _, err := pool.Exec(ctx, `DELETE FROM segment_efforts WHERE segment_id = $1::uuid`, ids.segmentID); err != nil {
		t.Fatalf("cleanup segment_efforts: %v", err)
	}
	if _, err := pool.Exec(ctx, `DELETE FROM activities WHERE id = ANY($1::uuid[])`, []string{ids.ownerActivityID, ids.otherActivityID}); err != nil {
		t.Fatalf("cleanup activities: %v", err)
	}
	if _, err := pool.Exec(ctx, `DELETE FROM segments WHERE id = $1::uuid`, ids.segmentID); err != nil {
		t.Fatalf("cleanup segments: %v", err)
	}
	now := time.Now().UTC()
	if _, err := pool.Exec(ctx, `
		INSERT INTO activities (id, user_id, title, sport_type, sub_type, is_indoor, is_virtual, is_electric, is_commute, started_at, elapsed_time_s)
		VALUES
			($1::uuid, $2::uuid, 'Owner activity', 'cycling', 'road', false, false, false, false, $5, 1800),
			($3::uuid, $4::uuid, 'Other activity', 'cycling', 'road', false, false, false, false, $5, 1700)
	`, ids.ownerActivityID, ownerID, ids.otherActivityID, otherID, now); err != nil {
		t.Fatalf("insert activities: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO segments (id, created_by, name, sport_group, allowed_predicates, distance_m, elevation_gain_m)
		VALUES ($1::uuid, $2::uuid, 'Hill Repeat', 'cycling', '[{"sport":"cycling","sub_type":"road"}]'::jsonb, 1234, 56)
	`, ids.segmentID, ownerID); err != nil {
		t.Fatalf("insert segment: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO segment_versions (id, segment_id, version_number, track, start_point, end_point, valid_from, created_by)
		VALUES (
			$1::uuid, $2::uuid, 1,
			ST_MakeLine(ST_MakePoint(7, 47), ST_MakePoint(7.01, 47.01))::geometry(LineString, 4326),
			ST_MakePoint(7, 47)::geometry(Point, 4326),
			ST_MakePoint(7.01, 47.01)::geometry(Point, 4326),
			CURRENT_DATE, $3::uuid
		)
	`, versionID, ids.segmentID, ownerID); err != nil {
		t.Fatalf("insert segment version: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO segment_efforts (id, segment_id, segment_version_id, activity_id, user_id, started_at, elapsed_time_s, sport_type, sub_type, avg_heart_rate, in_leaderboard)
		VALUES
			('00000000-0000-0000-0000-0000000bb501'::uuid, $1::uuid, $2::uuid, $3::uuid, $4::uuid, $6, 300, 'cycling', 'road', 145, true),
			('00000000-0000-0000-0000-0000000bb502'::uuid, $1::uuid, $2::uuid, $5::uuid, $7::uuid, $6, 200, 'cycling', 'road', 155, true)
	`, ids.segmentID, versionID, ids.ownerActivityID, ownerID, ids.otherActivityID, now, otherID); err != nil {
		t.Fatalf("insert segment efforts: %v", err)
	}
	return ids
}

func segmentBoolPtr(v bool) *bool {
	return &v
}

func TestQuerySegmentsExecuteReadUsesReadonlyDeps(t *testing.T) {
	reg := NewRegistry()
	RegisterSegmentTools(reg)
	tool := reg.tools["query_segments"]
	raw := json.RawMessage(`{"include_efforts": false}`)
	if _, err := tool.ExecuteRead(context.Background(), &Deps{}, "00000000-0000-0000-0000-000000000001", raw); err == nil {
		t.Fatalf("ExecuteRead without pool returned nil error")
	}
}
