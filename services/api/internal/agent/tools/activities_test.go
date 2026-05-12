package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/database/testdb"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestActivityToolsMetadata(t *testing.T) {
	reg := NewRegistry()
	RegisterActivityTools(reg)

	names := []string{"aggregate_stats", "compare_periods", "get_activity_detail", "query_activities"}
	for _, name := range names {
		tool := reg.tools[name]
		if tool == nil {
			t.Fatalf("tool %q was not registered", name)
		}
		if tool.Permission != PermissionRead || !tool.IsUserOwned || tool.ExecuteRead == nil {
			t.Fatalf("unexpected metadata for %s: %#v", name, tool)
		}
	}
	if err := reg.ValidateSchemas(); err != nil {
		t.Fatalf("activity tool schemas must validate: %v", err)
	}
}

func TestBuildQueryActivitiesSQLFiltersByUserFirst(t *testing.T) {
	gearID := uuid.New().String()
	sql, params, err := buildQueryActivitiesSQL("user-1", queryActivitiesArgs{
		SportType:     "running",
		Days:          activityIntPtr(14),
		Limit:         activityIntPtr(99),
		SortBy:        "distance",
		MinDistanceKM: activityFloatPtr(5.5),
		TitleSearch:   "trail",
		GearID:        gearID,
		IsVirtual:     activityBoolPtr(false),
		IsCommute:     activityBoolPtr(true),
	}, time.Date(2026, 5, 12, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}

	for _, want := range []string{
		"FROM activities a WHERE a.user_id = $1",
		"a.started_at >= $2",
		"a.sport_type = $3",
		"a.is_virtual = $4",
		"a.is_commute = $5",
		"a.distance_m >= $6",
		"a.title ILIKE $7",
		"aga.activity_id = a.id AND aga.user_id = a.user_id AND aga.role_key = 'primary' AND aga.gear_id = $8",
		"ORDER BY a.distance_m DESC NULLS LAST LIMIT 50",
	} {
		if !strings.Contains(sql, want) {
			t.Fatalf("query SQL missing %q:\n%s", want, sql)
		}
	}
	if strings.Contains(sql, "gear_id = '"+gearID+"'") {
		t.Fatalf("gear_id must be parameterized, got SQL:\n%s", sql)
	}
	wantParams := []any{"user-1", time.Date(2026, 4, 28, 0, 0, 0, 0, time.UTC), "running", false, true, 5500.0, "%trail%"}
	if !reflect.DeepEqual(params[:7], wantParams) {
		t.Fatalf("params prefix mismatch:\nwant %#v\ngot  %#v", wantParams, params[:7])
	}
	if _, ok := params[7].(uuid.UUID); !ok {
		t.Fatalf("gear_id param must be parsed uuid.UUID, got %T", params[7])
	}
}

func TestBuildAggregateStatsSQLUsesUserOwnedWhereForEachGrouping(t *testing.T) {
	now := time.Date(2026, 5, 12, 12, 0, 0, 0, time.UTC)
	for _, groupBy := range []string{"none", "week", "month", "sport", "sport_tuple"} {
		sql, params := buildAggregateStatsSQL("user-1", aggregateStatsArgs{
			Metric:  "total_distance",
			Days:    activityIntPtr(30),
			GroupBy: groupBy,
		}, now)
		if !strings.Contains(sql, "FROM activities WHERE user_id = $1 AND started_at >= $2") {
			t.Fatalf("%s SQL must keep user_id first:\n%s", groupBy, sql)
		}
		if !reflect.DeepEqual(params, []any{"user-1", time.Date(2026, 4, 12, 0, 0, 0, 0, time.UTC)}) {
			t.Fatalf("%s params mismatch: %#v", groupBy, params)
		}
	}
}

func TestBuildComparePeriodsSQLFiltersUserFirst(t *testing.T) {
	sql, params, err := buildComparePeriodSQL("user-1", "2026-05-01", "2026-05-12", comparePeriodsArgs{
		SportType: "cycling",
		SubType:   activityStringPtr("road"),
		IsIndoor:  activityBoolPtr(false),
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"FROM activities WHERE user_id = $1",
		"started_at::date BETWEEN $2 AND $3",
		"sport_type = $4",
		"sub_type = $5",
		"is_indoor = $6",
	} {
		if !strings.Contains(sql, want) {
			t.Fatalf("compare SQL missing %q:\n%s", want, sql)
		}
	}
	wantParams := []any{"user-1", dateOnly(2026, 5, 1), dateOnly(2026, 5, 12), "cycling", "road", false}
	if !reflect.DeepEqual(params, wantParams) {
		t.Fatalf("params mismatch:\nwant %#v\ngot  %#v", wantParams, params)
	}
}

func TestBuildActivityDetailSQLFiltersUserFirst(t *testing.T) {
	activityID := uuid.New().String()
	sql, params, err := buildActivityDetailSQL("user-1", getActivityDetailArgs{ActivityID: activityID})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(sql, "WHERE a.user_id = $1 AND a.id = $2") {
		t.Fatalf("detail SQL must filter user first:\n%s", sql)
	}
	if !strings.Contains(sql, "ap.user_id = a.user_id") {
		t.Fatalf("photo count must be user-scoped:\n%s", sql)
	}
	if params[0] != "user-1" {
		t.Fatalf("user id must be first param: %#v", params)
	}
	if _, ok := params[1].(uuid.UUID); !ok {
		t.Fatalf("activity_id param must be parsed uuid.UUID, got %T", params[1])
	}
}

func TestActivityArgDefaults(t *testing.T) {
	var args queryActivitiesArgs
	if err := json.Unmarshal([]byte(`{}`), &args); err != nil {
		t.Fatal(err)
	}
	if got := args.days(); got != 30 {
		t.Fatalf("default days = %d, want 30", got)
	}
	if got := args.limit(); got != 10 {
		t.Fatalf("default limit = %d, want 10", got)
	}
}

func TestQueryActivitiesExecutesWithOwnershipIsolation(t *testing.T) {
	pool := testdb.Open(t)
	userA := insertActivityToolUser(t, pool)
	userB := insertActivityToolUser(t, pool)
	ownActivity := insertActivityToolActivity(t, pool, userA, "Eigene Runde")
	insertActivityToolActivity(t, pool, userB, "Fremde Runde")

	result, err := executeQueryActivities(context.Background(), &Deps{Readonly: pool}, userA, json.RawMessage(`{"days":30,"limit":10}`))
	if err != nil {
		t.Fatal(err)
	}
	activities := result.(map[string]any)["activities"].([]map[string]any)
	if len(activities) != 1 {
		t.Fatalf("activities len = %d, want 1: %#v", len(activities), activities)
	}
	if activities[0]["id"] != ownActivity || activities[0]["title"] != "Eigene Runde" {
		t.Fatalf("unexpected activity result: %#v", activities[0])
	}
}

func insertActivityToolUser(t *testing.T, pool *pgxpool.Pool) string {
	t.Helper()
	id := uuid.New().String()
	email := fmt.Sprintf("agent-tools-%s@example.com", id[:8])
	username := fmt.Sprintf("agenttools_%s", id[:8])
	_, err := pool.Exec(context.Background(), `
		INSERT INTO users (id, username, email, display_name, timezone, email_verified, registration_source)
		VALUES ($1, $2, $3, 'Agent Tools Test', 'UTC', true, 'test')
	`, id, username, email)
	if err != nil {
		t.Fatalf("insert user: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), "DELETE FROM users WHERE id = $1", id)
	})
	return id
}

func insertActivityToolActivity(t *testing.T, pool *pgxpool.Pool, userID, title string) string {
	t.Helper()
	id := uuid.New().String()
	_, err := pool.Exec(context.Background(), `
		INSERT INTO activities (
			id, user_id, title, sport_type, started_at, distance_m, elapsed_time_s,
			avg_speed_ms, elevation_gain_m, is_electric, is_indoor, is_virtual, is_commute
		)
		VALUES ($1, $2, $3, 'running', now() - interval '1 day', 5000, 1800, 2.77, 42, false, false, false, false)
	`, id, userID, title)
	if err != nil {
		t.Fatalf("insert activity: %v", err)
	}
	return id
}

func activityIntPtr(v int) *int { return &v }

func activityFloatPtr(v float64) *float64 { return &v }

func activityBoolPtr(v bool) *bool { return &v }

func activityStringPtr(v string) *string { return &v }
