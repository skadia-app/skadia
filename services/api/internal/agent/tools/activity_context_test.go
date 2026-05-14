package tools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/database/testdb"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestActivityContextToolsMetadata(t *testing.T) {
	reg := NewRegistry()
	RegisterActivityContextTools(reg)

	for _, name := range []string{"query_activity_terrain_summary", "query_weather_at_point_time"} {
		tool := reg.tools[name]
		if tool == nil {
			t.Fatalf("tool %q was not registered", name)
		}
		if tool.Permission != PermissionRead {
			t.Fatalf("%s permission = %q, want read", name, tool.Permission)
		}
		if !tool.IsUserOwned {
			t.Fatalf("%s IsUserOwned = false, want true", name)
		}
		if tool.ExecuteRead == nil {
			t.Fatalf("%s ExecuteRead is nil", name)
		}
		if err := validateSchema(name, tool.InputSchema); err != nil {
			t.Fatalf("%s schema invalid: %v", name, err)
		}
	}
}

func TestBuildActivityTerrainSummarySQLScopesActivityAndEnrichments(t *testing.T) {
	args := queryActivityTerrainSummaryArgs{
		ActivityID: "00000000-0000-0000-0000-0000000000a1",
		Limit:      activityIntPtr(500),
	}
	sql, params, criteria, err := buildActivityTerrainSummarySQL("00000000-0000-0000-0000-000000000001", args, fixedAgentToolNow())
	if err != nil {
		t.Fatalf("buildActivityTerrainSummarySQL returned error: %v", err)
	}

	for _, want := range []string{
		"FROM activities a",
		"a.user_id = $1",
		"a.id::text = ANY($2)",
		"FROM activity_enrichments e WHERE e.activity_id = s.id AND e.category = 'surface'",
		"FROM activity_enrichments e WHERE e.activity_id = s.id AND e.category = 'highway'",
		"'ranges', e.ranges",
		"FROM activity_enrichment_meta m WHERE m.activity_id = s.id",
		"enrichment_categories",
		"geo_context",
		"e.category = 'forest_pct'",
		"e.category = 'protected_pct'",
		"activity_geo_tags gt WHERE gt.activity_id = s.id AND gt.category IN ('still_water','flowing_water')",
		"'noise', false",
		"'traffic', false",
		"LIMIT 50",
	} {
		if !strings.Contains(sql, want) {
			t.Fatalf("terrain SQL missing %q:\n%s", want, sql)
		}
	}
	if len(params) != 2 {
		t.Fatalf("params len = %d, want user and activity ids", len(params))
	}
	if got := criteria["activity_ids"]; got == nil {
		t.Fatalf("criteria missing activity_ids: %#v", criteria)
	}
}

func TestBuildActivityTerrainSummarySQLUsesSelectionFilters(t *testing.T) {
	args := queryActivityTerrainSummaryArgs{
		TitleQuery: "training loop",
		SportQuery: "Gravel",
		DateFrom:   "2026-05-01",
		DateTo:     "2026-05-12",
	}
	sql, params, criteria, err := buildActivityTerrainSummarySQL("user-1", args, time.Date(2026, 5, 13, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("buildActivityTerrainSummarySQL returned error: %v", err)
	}
	for _, want := range []string{
		"a.started_at >= $2",
		"a.started_at < $3",
		"a.sport_type = $4",
		"a.sub_type = $5",
		"a.title ILIKE $6",
	} {
		if !strings.Contains(sql, want) {
			t.Fatalf("terrain SQL missing selection filter %q:\n%s", want, sql)
		}
	}
	if len(params) != 6 {
		t.Fatalf("params len = %d, want 6", len(params))
	}
	if criteria["title_search"] != "training loop" {
		t.Fatalf("criteria title_search = %#v, want training loop", criteria["title_search"])
	}
}

func TestBuildWeatherAtPointTimeSQLScopesThroughActivitiesAndRanksTargetTime(t *testing.T) {
	args := queryWeatherAtPointTimeArgs{
		ActivityID: "00000000-0000-0000-0000-0000000000a1",
		AtTime:     "2026-05-12T08:30:00Z",
		Limit:      activityIntPtr(100),
	}
	sql, params, err := buildWeatherAtPointTimeSQL("00000000-0000-0000-0000-000000000001", args)
	if err != nil {
		t.Fatalf("buildWeatherAtPointTimeSQL returned error: %v", err)
	}
	for _, want := range []string{
		"FROM activity_weather aw",
		"JOIN activities a ON a.id = aw.activity_id",
		"FROM activity_streams s",
		"WHERE a.user_id = $1 AND a.id::text = $2",
		"ABS(EXTRACT(EPOCH FROM (aw.recorded_at - $3::timestamptz)))",
		"distance_from_target_m ASC NULLS LAST",
		"seconds_from_target ASC NULLS LAST",
		"LIMIT 20",
	} {
		if !strings.Contains(sql, want) {
			t.Fatalf("weather SQL missing %q:\n%s", want, sql)
		}
	}
	if len(params) != 5 {
		t.Fatalf("params len = %d, want user, activity id, target time, target lng, target lat", len(params))
	}
}

func TestBuildWeatherAtPointTimeSQLSupportsActivityOffset(t *testing.T) {
	args := queryWeatherAtPointTimeArgs{
		ActivityID: "00000000-0000-0000-0000-0000000000a1",
		OffsetS:    activityFloatPtr(600),
	}
	sql, params, err := buildWeatherAtPointTimeSQL("user-1", args)
	if err != nil {
		t.Fatalf("buildWeatherAtPointTimeSQL returned error: %v", err)
	}
	if !strings.Contains(sql, "a.started_at + ($3::double precision * interval '1 second')") {
		t.Fatalf("weather SQL missing offset target expression:\n%s", sql)
	}
	if len(params) != 5 {
		t.Fatalf("params len = %d, want user, activity id, offset, target lng, target lat", len(params))
	}
}

func TestBuildWeatherAtPointTimeSQLUsesExplicitPointForDistance(t *testing.T) {
	lat, lng := 47.1, 11.2
	args := queryWeatherAtPointTimeArgs{
		ActivityID: "00000000-0000-0000-0000-0000000000a1",
		Lat:        &lat,
		Lng:        &lng,
	}
	sql, params, err := buildWeatherAtPointTimeSQL("user-1", args)
	if err != nil {
		t.Fatalf("buildWeatherAtPointTimeSQL returned error: %v", err)
	}
	if !strings.Contains(sql, "ST_SetSRID(ST_Point($3::double precision, $4::double precision), 4326)::geography") {
		t.Fatalf("weather SQL missing explicit target point expression:\n%s", sql)
	}
	if params[2] != lng || params[3] != lat {
		t.Fatalf("target coordinate params = %#v, want lng then lat", params[2:4])
	}
}

func TestWeatherAtPointTimeExplicitCoordinatesStoredOnlyDoesNotFetch(t *testing.T) {
	t.Setenv("SKADIA_AGENT_LIVE_WEATHER_POLICY", "")
	tool := QueryWeatherAtPointTimeTool()
	args := json.RawMessage(`{"lat":47.1,"lng":11.2,"time":"2026-05-12T08:30:00Z"}`)
	got, err := tool.ExecuteRead(context.Background(), nil, "user-1", args)
	if err != nil {
		t.Fatalf("ExecuteRead returned error: %v", err)
	}
	m, ok := got.(map[string]any)
	if !ok {
		t.Fatalf("result type = %T, want map", got)
	}
	if m["status"] != "no_stored_weather" {
		t.Fatalf("status = %#v, want no_stored_weather", m["status"])
	}
}

func TestQueryWeatherAtPointTimeStoredOnlyReturnsStoredWeatherAndDoesNotConsumeQuota(t *testing.T) {
	pool := testdb.Open(t)
	deps := &Deps{DB: &database.DB{Pool: pool}}
	userID := uuid.NewString()
	activityID := insertWeatherToolActivity(t, pool, userID, true)
	insertWeatherToolReading(t, pool, activityID, time.Date(2026, 5, 12, 8, 30, 0, 0, time.UTC))

	got, err := QueryWeatherAtPointTimeTool().ExecuteRead(context.Background(), deps, userID, json.RawMessage(`{"activity_id":"`+activityID+`","at_time":"2026-05-12T08:30:00Z"}`))
	if err != nil {
		t.Fatalf("ExecuteRead returned error: %v", err)
	}
	m := requireWeatherToolMap(t, got)
	if m["status"] != "ok" {
		t.Fatalf("status = %#v, want ok", m["status"])
	}
	if m["source"] != "stored_activity_weather" {
		t.Fatalf("source = %#v, want stored_activity_weather", m["source"])
	}
	if m["count"] != 1 {
		t.Fatalf("count = %#v, want 1", m["count"])
	}
	rl := requireWeatherToolNestedMap(t, m, "rate_limit")
	if rl["status"] != "not_consumed" || rl["consumed"] != false {
		t.Fatalf("rate_limit = %#v, want not_consumed with consumed=false", rl)
	}
}

func TestQueryWeatherAtPointTimeFetchOnlyDisabledForExplicitPoint(t *testing.T) {
	t.Setenv("SKADIA_AGENT_LIVE_WEATHER_POLICY", "")
	t.Setenv("SKADIA_AGENT_LIVE_WEATHER_PROVIDER_ENABLED", "")

	got, err := QueryWeatherAtPointTimeTool().ExecuteRead(context.Background(), nil, "user-1", json.RawMessage(`{"mode":"fetch_only","lat":47.1,"lng":11.2,"time":"2026-05-12T08:30:00Z"}`))
	if err != nil {
		t.Fatalf("ExecuteRead returned error: %v", err)
	}
	m := requireWeatherToolMap(t, got)
	if m["status"] != "live_weather_disabled" {
		t.Fatalf("status = %#v, want live_weather_disabled", m["status"])
	}
	rl := requireWeatherToolNestedMap(t, m, "rate_limit")
	if rl["status"] != "not_consumed" {
		t.Fatalf("rate_limit.status = %#v, want not_consumed", rl["status"])
	}
}

func TestQueryWeatherAtPointTimeFetchOnlyUnconfiguredForExplicitPoint(t *testing.T) {
	t.Setenv("SKADIA_AGENT_LIVE_WEATHER_POLICY", "allowed")
	t.Setenv("SKADIA_AGENT_LIVE_WEATHER_PROVIDER_ENABLED", "")

	got, err := QueryWeatherAtPointTimeTool().ExecuteRead(context.Background(), nil, "user-1", json.RawMessage(`{"mode":"fetch_only","lat":47.1,"lng":11.2,"time":"2026-05-12T08:30:00Z"}`))
	if err != nil {
		t.Fatalf("ExecuteRead returned error: %v", err)
	}
	m := requireWeatherToolMap(t, got)
	if m["status"] != "live_weather_unconfigured" {
		t.Fatalf("status = %#v, want live_weather_unconfigured", m["status"])
	}
}

func TestQueryWeatherAtPointTimeStoredOnlyDoesNotExposeOtherUsersActivity(t *testing.T) {
	pool := testdb.Open(t)
	deps := &Deps{DB: &database.DB{Pool: pool}}
	ownerID := uuid.NewString()
	otherUserID := uuid.NewString()
	activityID := insertWeatherToolActivity(t, pool, ownerID, true)
	insertWeatherToolReading(t, pool, activityID, time.Date(2026, 5, 12, 8, 30, 0, 0, time.UTC))

	got, err := QueryWeatherAtPointTimeTool().ExecuteRead(context.Background(), deps, otherUserID, json.RawMessage(`{"activity_id":"`+activityID+`","at_time":"2026-05-12T08:30:00Z"}`))
	if err != nil {
		t.Fatalf("ExecuteRead returned error: %v", err)
	}
	m := requireWeatherToolMap(t, got)
	if m["status"] != "no_stored_weather" {
		t.Fatalf("status = %#v, want no_stored_weather", m["status"])
	}
	if m["count"] != 0 {
		t.Fatalf("count = %#v, want 0", m["count"])
	}
}

func TestQueryWeatherAtPointTimeFetchOnlyActivityReturnsNoPointForTime(t *testing.T) {
	t.Setenv("SKADIA_AGENT_LIVE_WEATHER_POLICY", "allowed")
	t.Setenv("SKADIA_AGENT_LIVE_WEATHER_PROVIDER_ENABLED", "true")
	pool := testdb.Open(t)
	deps := &Deps{DB: &database.DB{Pool: pool}}
	userID := uuid.NewString()
	activityID := insertWeatherToolActivity(t, pool, userID, false)

	got, err := QueryWeatherAtPointTimeTool().ExecuteRead(context.Background(), deps, userID, json.RawMessage(`{"mode":"fetch_only","activity_id":"`+activityID+`","at_time":"2026-05-12T08:30:00Z"}`))
	if err != nil {
		t.Fatalf("ExecuteRead returned error: %v", err)
	}
	m := requireWeatherToolMap(t, got)
	if m["status"] != "no_point_for_time" {
		t.Fatalf("status = %#v, want no_point_for_time", m["status"])
	}
}

func TestQueryWeatherAtPointTimePolicyAndQuotaStatuses(t *testing.T) {
	rateLimit := liveWeatherQuotaNotConsumed()
	for _, tc := range []struct {
		name   string
		cfg    liveWeatherConfig
		status string
	}{
		{
			name:   "manual confirmation",
			cfg:    liveWeatherConfig{Policy: "manual_confirm"},
			status: "requires_user_confirmation",
		},
		{
			name:   "privacy blocked",
			cfg:    liveWeatherConfig{Policy: "blocked"},
			status: "skipped_by_privacy_policy",
		},
		{
			name:   "unconfigured provider",
			cfg:    liveWeatherConfig{Policy: "allowed"},
			status: "live_weather_unconfigured",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := liveWeatherPolicyBlockedResponse(tc.cfg, "fetch_only", rateLimit)
			if got["status"] != tc.status {
				t.Fatalf("status = %#v, want %s", got["status"], tc.status)
			}
		})
	}

	cfg := liveWeatherConfig{Policy: "allowed", ProviderEnabled: true, LimitPerHour: 1}
	now := time.Date(2026, 5, 12, 8, 30, 0, 0, time.UTC)
	userID := uuid.NewString()
	first := consumeLiveWeatherQuota(userID, cfg, now)
	if first["status"] != "available" || first["consumed"] != true || first["remaining"] != 0 {
		t.Fatalf("first quota = %#v, want consumed available with remaining 0", first)
	}
	second := consumeLiveWeatherQuota(userID, cfg, now)
	if second["status"] != "rate_limited" || second["consumed"] != false || second["blocking_reason"] != "rate_limited" {
		t.Fatalf("second quota = %#v, want rate_limited without consumption", second)
	}
}

func TestQueryWeatherAtPointTimeExplicitPointAcceptsAtTime(t *testing.T) {
	lat, lng := 47.1, 11.2
	point, err := explicitLiveWeatherPoint(queryWeatherAtPointTimeArgs{
		Lat:    &lat,
		Lng:    &lng,
		AtTime: "2026-05-12T08:30:00Z",
	})
	if err != nil {
		t.Fatalf("explicitLiveWeatherPoint returned error: %v", err)
	}
	if point == nil {
		t.Fatal("point is nil, want explicit point")
	}
	if !point.At.Equal(time.Date(2026, 5, 12, 8, 30, 0, 0, time.UTC)) {
		t.Fatalf("point.At = %s, want requested at_time", point.At)
	}
}

func insertWeatherToolActivity(t *testing.T, pool *pgxpool.Pool, userID string, withStream bool) string {
	t.Helper()
	ctx := context.Background()
	insertWeatherToolUser(t, pool, userID)
	activityID := uuid.NewString()
	startedAt := time.Date(2026, 5, 12, 8, 0, 0, 0, time.UTC)
	_, err := pool.Exec(ctx, `
		INSERT INTO activities (id, user_id, title, sport_type, started_at, ended_at, file_hash, has_gps)
		VALUES ($1, $2, 'weather tool test', 'cycling', $3, $4, $5, $6)
	`, activityID, userID, startedAt, startedAt.Add(time.Hour), uuid.NewString(), withStream)
	if err != nil {
		t.Fatalf("insert activity: %v", err)
	}
	if withStream {
		_, err = pool.Exec(ctx, `
			INSERT INTO activity_streams (activity_id, time, lat, lng, altitude_m)
			VALUES ($1, $2, 47.1, 11.2, 650)
		`, activityID, startedAt.Add(30*time.Minute))
		if err != nil {
			t.Fatalf("insert stream point: %v", err)
		}
	}
	return activityID
}

func insertWeatherToolUser(t *testing.T, pool *pgxpool.Pool, userID string) {
	t.Helper()
	_, err := pool.Exec(context.Background(), `
		INSERT INTO users (id, username, email, email_verified, registration_source)
		VALUES ($1, $2, $3, true, 'test')
		ON CONFLICT (id) DO NOTHING
	`, userID, "weather-tool-"+userID[:8], "weather-tool-"+userID[:8]+"@example.test")
	if err != nil {
		t.Fatalf("insert user: %v", err)
	}
}

func insertWeatherToolReading(t *testing.T, pool *pgxpool.Pool, activityID string, recordedAt time.Time) {
	t.Helper()
	_, err := pool.Exec(context.Background(), `
		INSERT INTO activity_weather (
			activity_id, reading_type, recorded_at, lat, lng, elevation_m,
			temp_c, apparent_temp_c, precipitation_mm, wind_speed_ms, wind_gusts_ms,
			humidity, uv_index, freezing_level_m, condition, tags, model
		) VALUES ($1, 'single', $2, 47.1, 11.2, 650, 12.3, 11.7, 0.2, 3.4, 5.6, 72, 2.1, 1800, 'klar', ARRAY['sonnig'], 'test')
	`, activityID, recordedAt)
	if err != nil {
		t.Fatalf("insert weather reading: %v", err)
	}
}

func requireWeatherToolMap(t *testing.T, got any) map[string]any {
	t.Helper()
	m, ok := got.(map[string]any)
	if !ok {
		t.Fatalf("result type = %T, want map", got)
	}
	return m
}

func requireWeatherToolNestedMap(t *testing.T, parent map[string]any, key string) map[string]any {
	t.Helper()
	m, ok := parent[key].(map[string]any)
	if !ok {
		t.Fatalf("%s type = %T, want map", key, parent[key])
	}
	return m
}
