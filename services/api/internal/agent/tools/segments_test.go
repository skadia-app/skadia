package tools

import (
	"context"
	"encoding/json"
	"strings"
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
	if seg["pr_date"] == "" {
		t.Fatalf("pr_date missing in segment result: %#v", seg)
	}
	if seg["pr_activity_title"] != "Owner activity" {
		t.Fatalf("pr_activity_title = %#v, want Owner activity", seg["pr_activity_title"])
	}
	efforts := seg["recent_efforts"].([]map[string]any)
	if len(efforts) != 1 {
		t.Fatalf("recent efforts len = %d, want 1: %#v", len(efforts), efforts)
	}
	if efforts[0]["time"] != "5min" {
		t.Fatalf("recent effort time = %#v, want 5min", efforts[0]["time"])
	}
}

func TestQuerySegmentsDefaultWindowExcludesOldPR(t *testing.T) {
	ctx := context.Background()
	pool := testdb.Open(t)
	ownerID := "00000000-0000-0000-0000-0000000bb105"
	otherID := "00000000-0000-0000-0000-0000000bb106"
	insertToolTestUser(t, pool, ownerID, "agent-segment-pr-owner")
	insertToolTestUser(t, pool, otherID, "agent-segment-pr-other")
	ids := insertToolTestSegmentFixture(t, pool, ownerID, otherID)
	oldActivityID := "00000000-0000-0000-0000-0000000bb303"
	oldEffortID := "00000000-0000-0000-0000-0000000bb503"
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM segment_efforts WHERE segment_id = $1::uuid`, ids.segmentID)
		_, _ = pool.Exec(ctx, `DELETE FROM activities WHERE id = ANY($1::uuid[])`, []string{ids.ownerActivityID, ids.otherActivityID, oldActivityID})
		_, _ = pool.Exec(ctx, `DELETE FROM segments WHERE id = $1::uuid`, ids.segmentID)
	})

	oldDate := time.Now().UTC().AddDate(0, 0, -500)
	if _, err := pool.Exec(ctx, `
		INSERT INTO activities (id, user_id, title, sport_type, sub_type, is_indoor, is_virtual, is_electric, is_commute, started_at, elapsed_time_s)
		VALUES ($1::uuid, $2::uuid, 'Historic PR activity', 'cycling', 'road', false, false, false, false, $3, 1200)
	`, oldActivityID, ownerID, oldDate); err != nil {
		t.Fatalf("insert old activity: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO segment_efforts (id, segment_id, segment_version_id, activity_id, user_id, started_at, elapsed_time_s, sport_type, sub_type, avg_heart_rate, in_leaderboard)
		SELECT $1::uuid, $2::uuid, sv.id, $3::uuid, $4::uuid, $5, 250, 'cycling', 'road', 150, true
		FROM segment_versions sv
		WHERE sv.segment_id = $2::uuid
		ORDER BY sv.version_number DESC
		LIMIT 1
	`, oldEffortID, ids.segmentID, oldActivityID, ownerID, oldDate); err != nil {
		t.Fatalf("insert old effort: %v", err)
	}

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
	if seg["pr_seconds"] != int32(300) && seg["pr_seconds"] != int64(300) && seg["pr_seconds"] != 300 {
		t.Fatalf("pr_seconds = %#v, want 300", seg["pr_seconds"])
	}
	if seg["pr_activity_title"] != "Owner activity" {
		t.Fatalf("pr_activity_title = %#v, want Owner activity", seg["pr_activity_title"])
	}
	efforts := seg["recent_efforts"].([]map[string]any)
	if len(efforts) != 1 || efforts[0]["time"] != "5min" {
		t.Fatalf("recent efforts = %#v, want only current 5min effort", efforts)
	}
}

func TestQuerySegmentsFindsAccessibleSegmentWithoutEfforts(t *testing.T) {
	ctx := context.Background()
	pool := testdb.Open(t)
	ownerID := "00000000-0000-0000-0000-0000000bb120"
	insertToolTestUser(t, pool, ownerID, "agent-segment-name-owner")
	segmentID := "00000000-0000-0000-0000-0000000bb620"
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM segments WHERE id = $1::uuid`, segmentID)
	})
	if _, err := pool.Exec(ctx, `
		INSERT INTO segments (id, created_by, name, sport_group, segment_type, visibility, distance_m)
		VALUES ($1::uuid, $2::uuid, 'fixture segment', 'cycling', 'trail', 'private', 1200)
	`, segmentID, ownerID); err != nil {
		t.Fatalf("insert segment: %v", err)
	}

	got, err := querySegments(ctx, pool, ownerID, querySegmentsArgs{
		Query: "fixture segment", IncludeEfforts: false, Days: 365, Limit: 5,
	})
	if err != nil {
		t.Fatalf("querySegments: %v", err)
	}
	if got["status"] != "ok" || got["mode"] != "existence" {
		t.Fatalf("status/mode = %#v/%#v, want ok/existence: %#v", got["status"], got["mode"], got)
	}
	segments := got["segments"].([]map[string]any)
	if len(segments) != 1 {
		t.Fatalf("segments len = %d, want 1: %#v", len(segments), segments)
	}
	if segments[0]["name"] != "fixture segment" {
		t.Fatalf("segment = %#v", segments[0])
	}
	if segments[0]["effort_count"] != int64(0) {
		t.Fatalf("effort_count = %#v, want 0", segments[0]["effort_count"])
	}
	if segments[0]["effort_status"] != "no_matching_efforts" {
		t.Fatalf("effort_status = %#v, want no_matching_efforts", segments[0]["effort_status"])
	}

	got, err = querySegments(ctx, pool, ownerID, querySegmentsArgs{
		Query: "fixture segment", IncludeEfforts: true, Days: 365, Limit: 5,
	})
	if err != nil {
		t.Fatalf("querySegments with efforts: %v", err)
	}
	if got["status"] != "not_found" || got["mode"] != "efforts" {
		t.Fatalf("status/mode = %#v/%#v, want not_found/efforts for no-effort segment: %#v", got["status"], got["mode"], got)
	}
}

func TestQuerySegmentsCanUseWiderWindowForHistoricalPR(t *testing.T) {
	ctx := context.Background()
	pool := testdb.Open(t)
	ownerID := "00000000-0000-0000-0000-0000000bb107"
	otherID := "00000000-0000-0000-0000-0000000bb108"
	insertToolTestUser(t, pool, ownerID, "agent-segment-wide-owner")
	insertToolTestUser(t, pool, otherID, "agent-segment-wide-other")
	ids := insertToolTestSegmentFixture(t, pool, ownerID, otherID)
	oldActivityID := "00000000-0000-0000-0000-0000000bb304"
	oldEffortID := "00000000-0000-0000-0000-0000000bb504"
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM segment_efforts WHERE segment_id = $1::uuid`, ids.segmentID)
		_, _ = pool.Exec(ctx, `DELETE FROM activities WHERE id = ANY($1::uuid[])`, []string{ids.ownerActivityID, ids.otherActivityID, oldActivityID})
		_, _ = pool.Exec(ctx, `DELETE FROM segments WHERE id = $1::uuid`, ids.segmentID)
	})

	oldDate := time.Now().UTC().AddDate(0, 0, -500)
	if _, err := pool.Exec(ctx, `
		INSERT INTO activities (id, user_id, title, sport_type, sub_type, is_indoor, is_virtual, is_electric, is_commute, started_at, elapsed_time_s)
		VALUES ($1::uuid, $2::uuid, 'Historic PR activity', 'cycling', 'road', false, false, false, false, $3, 1200)
	`, oldActivityID, ownerID, oldDate); err != nil {
		t.Fatalf("insert old activity: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO segment_efforts (id, segment_id, segment_version_id, activity_id, user_id, started_at, elapsed_time_s, sport_type, sub_type, avg_heart_rate, in_leaderboard)
		SELECT $1::uuid, $2::uuid, sv.id, $3::uuid, $4::uuid, $5, 250, 'cycling', 'road', 150, true
		FROM segment_versions sv
		WHERE sv.segment_id = $2::uuid
		ORDER BY sv.version_number DESC
		LIMIT 1
	`, oldEffortID, ids.segmentID, oldActivityID, ownerID, oldDate); err != nil {
		t.Fatalf("insert old effort: %v", err)
	}

	got, err := querySegments(ctx, pool, ownerID, querySegmentsArgs{
		Query:          "Hill",
		IncludeEfforts: true,
		Days:           1000,
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
	if seg["pr_seconds"] != int32(250) && seg["pr_seconds"] != int64(250) && seg["pr_seconds"] != 250 {
		t.Fatalf("pr_seconds = %#v, want 250", seg["pr_seconds"])
	}
	if seg["pr_activity_title"] != "Historic PR activity" {
		t.Fatalf("pr_activity_title = %#v, want Historic PR activity", seg["pr_activity_title"])
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
	segments := got["segments"].([]map[string]any)
	if len(segments) != 1 {
		t.Fatalf("segments len = %d, want accessible segment with no matching efforts: %#v", len(segments), got)
	}
	seg := segments[0]
	if seg["effort_count"] != int64(0) || seg["effort_status"] != "no_matching_efforts" {
		t.Fatalf("effort coverage = %#v/%#v, want zero no_matching_efforts", seg["effort_count"], seg["effort_status"])
	}
}

func TestQuerySegmentsDefaultsToLeaderboardEfforts(t *testing.T) {
	ctx := context.Background()
	pool := testdb.Open(t)
	ownerID := "00000000-0000-0000-0000-0000000bb109"
	otherID := "00000000-0000-0000-0000-0000000bb110"
	insertToolTestUser(t, pool, ownerID, "agent-segment-board-owner")
	insertToolTestUser(t, pool, otherID, "agent-segment-board-other")
	ids := insertToolTestSegmentFixture(t, pool, ownerID, otherID)
	fastActivityID := "00000000-0000-0000-0000-0000000bb305"
	fastEffortID := "00000000-0000-0000-0000-0000000bb505"
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM segment_efforts WHERE segment_id = $1::uuid`, ids.segmentID)
		_, _ = pool.Exec(ctx, `DELETE FROM activities WHERE id = ANY($1::uuid[])`, []string{ids.ownerActivityID, ids.otherActivityID, fastActivityID})
		_, _ = pool.Exec(ctx, `DELETE FROM segments WHERE id = $1::uuid`, ids.segmentID)
	})

	if _, err := pool.Exec(ctx, `
		INSERT INTO activities (id, user_id, title, sport_type, sub_type, is_indoor, is_virtual, is_electric, is_commute, started_at, elapsed_time_s)
		VALUES ($1::uuid, $2::uuid, 'Excluded effort activity', 'cycling', 'road', false, false, false, false, now(), 900)
	`, fastActivityID, ownerID); err != nil {
		t.Fatalf("insert fast activity: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO segment_efforts (id, segment_id, segment_version_id, activity_id, user_id, started_at, elapsed_time_s, sport_type, sub_type, avg_heart_rate, in_leaderboard)
		SELECT $1::uuid, $2::uuid, sv.id, $3::uuid, $4::uuid, now(), 100, 'cycling', 'road', 150, false
		FROM segment_versions sv
		WHERE sv.segment_id = $2::uuid
		ORDER BY sv.version_number DESC
		LIMIT 1
	`, fastEffortID, ids.segmentID, fastActivityID, ownerID); err != nil {
		t.Fatalf("insert excluded effort: %v", err)
	}

	got, err := querySegments(ctx, pool, ownerID, querySegmentsArgs{Query: "Hill", IncludeEfforts: true, Limit: 5})
	if err != nil {
		t.Fatalf("querySegments: %v", err)
	}
	seg := got["segments"].([]map[string]any)[0]
	if seg["pr_seconds"] != int32(300) && seg["pr_seconds"] != int64(300) && seg["pr_seconds"] != 300 {
		t.Fatalf("pr_seconds = %#v, want leaderboard 300 not excluded 100", seg["pr_seconds"])
	}
}

func TestQuerySegmentsResolvesSportQuery(t *testing.T) {
	sql, params, err := buildQuerySegmentsSQL("00000000-0000-0000-0000-0000000bb103", querySegmentsArgs{
		SportQuery: "Gravel",
		Limit:      5,
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"se.sport_type = $3", "se.sub_type = $4"} {
		if !strings.Contains(sql, want) {
			t.Fatalf("segment SQL missing %q:\n%s", want, sql)
		}
	}
	if params[2] != "cycling" || params[3] != "gravel_cycling" {
		t.Fatalf("sport params = %#v, want cycling/gravel_cycling", params[2:])
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
