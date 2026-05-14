package tools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"
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
	if m["status"] != "unsupported_no_stored_activity" {
		t.Fatalf("status = %#v, want unsupported_no_stored_activity", m["status"])
	}
}
