package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/agent"
	"github.com/skadia-app/skadia/services/api/internal/database"
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
	for _, name := range []string{"update_activity_metadata", "rename_activity", "tag_activity"} {
		tool := reg.tools[name]
		if tool == nil {
			t.Fatalf("write tool %q was not registered", name)
		}
		if tool.Permission != PermissionWriteSafe || !tool.RequiresConfirmation || !tool.IsUserOwned {
			t.Fatalf("unexpected write metadata for %s: %#v", name, tool)
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

func TestRenameActivityWriteToolUpdatesTitle(t *testing.T) {
	pool := testdb.Open(t)
	userID := insertActivityToolUser(t, pool)
	activityID := insertActivityToolActivity(t, pool, userID, "Alter Name")
	envelope := activityWriteEnvelope(t, pool, userID, "rename_activity")
	sessionID := uuid.NewString()
	args := json.RawMessage(`{"activity_id":"` + activityID + `","title":"Neuer Name"}`)

	if _, err := envelope.PrepareConfirmation(context.Background(), ToolExecutionRequest{
		UserID: userID, SessionID: sessionID, ToolCallID: "call-rename", ToolName: "rename_activity", Args: args,
	}); err != nil {
		t.Fatalf("PrepareConfirmation: %v", err)
	}
	confirmEnvelopeTool(t, envelope, sessionID, "call-rename")
	if _, err := envelope.ExecuteConfirmed(context.Background(), ToolExecutionRequest{
		UserID: userID, SessionID: sessionID, ToolCallID: "call-rename", ToolName: "rename_activity", Args: args,
	}); err != nil {
		t.Fatalf("ExecuteConfirmed: %v", err)
	}
	var title string
	if err := pool.QueryRow(context.Background(), `SELECT title FROM activities WHERE id = $1`, activityID).Scan(&title); err != nil {
		t.Fatalf("select title: %v", err)
	}
	if title != "Neuer Name" {
		t.Fatalf("title = %q, want Neuer Name", title)
	}
}

func TestTagActivityWriteToolMissingTagAsksUser(t *testing.T) {
	pool := testdb.Open(t)
	userID := insertActivityToolUser(t, pool)
	activityID := insertActivityToolActivity(t, pool, userID, "Tagrunde")
	envelope := activityWriteEnvelope(t, pool, userID, "tag_activity")
	args := json.RawMessage(`{"activity_id":"` + activityID + `","tag_name":"NochNichtDa","mode":"add"}`)

	_, err := envelope.PrepareConfirmation(context.Background(), ToolExecutionRequest{
		UserID: userID, SessionID: uuid.NewString(), ToolCallID: "call-missing-tag", ToolName: "tag_activity", Args: args,
	})
	if err == nil || !strings.Contains(err.Error(), "TAG_NOT_FOUND") {
		t.Fatalf("PrepareConfirmation err = %v, want TAG_NOT_FOUND", err)
	}
}

func TestTagActivityWriteToolAddsExistingTag(t *testing.T) {
	pool := testdb.Open(t)
	userID := insertActivityToolUser(t, pool)
	activityID := insertActivityToolActivity(t, pool, userID, "Tagrunde")
	tagID := insertActivityToolTag(t, pool, userID, "Liebling")
	envelope := activityWriteEnvelope(t, pool, userID, "tag_activity")
	sessionID := uuid.NewString()
	args := json.RawMessage(`{"activity_id":"` + activityID + `","tag_name":"Liebling","mode":"add"}`)

	if _, err := envelope.PrepareConfirmation(context.Background(), ToolExecutionRequest{
		UserID: userID, SessionID: sessionID, ToolCallID: "call-tag", ToolName: "tag_activity", Args: args,
	}); err != nil {
		t.Fatalf("PrepareConfirmation: %v", err)
	}
	confirmEnvelopeTool(t, envelope, sessionID, "call-tag")
	if _, err := envelope.ExecuteConfirmed(context.Background(), ToolExecutionRequest{
		UserID: userID, SessionID: sessionID, ToolCallID: "call-tag", ToolName: "tag_activity", Args: args,
	}); err != nil {
		t.Fatalf("ExecuteConfirmed: %v", err)
	}
	var linked bool
	if err := pool.QueryRow(context.Background(),
		`SELECT EXISTS (SELECT 1 FROM activity_tags WHERE activity_id = $1 AND tag_id = $2)`,
		activityID, tagID,
	).Scan(&linked); err != nil {
		t.Fatalf("select link: %v", err)
	}
	if !linked {
		t.Fatal("activity_tags link missing")
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

func insertActivityToolTag(t *testing.T, pool *pgxpool.Pool, userID, name string) string {
	t.Helper()
	id := uuid.NewString()
	_, err := pool.Exec(context.Background(),
		`INSERT INTO tags (id, user_id, name, color, text_color, category) VALUES ($1, $2, $3, '#6366F1', '#FFFFFF', 'custom')`,
		id, userID, name,
	)
	if err != nil {
		t.Fatalf("insert tag: %v", err)
	}
	return id
}

func activityWriteEnvelope(t *testing.T, pool *pgxpool.Pool, userID string, allowedTools ...string) *ExecutionEnvelope {
	t.Helper()
	db := &database.DB{Pool: pool}
	if _, err := db.UpsertChatPrefs(context.Background(), userID, database.ChatUserPreferences{
		ChatWriteToolsEnabled: true,
		ChatAllowedWriteTools: allowedTools,
	}); err != nil {
		t.Fatalf("upsert chat prefs: %v", err)
	}
	reg := NewRegistry()
	for _, tool := range WriteActivityTools() {
		reg.Register(tool)
	}
	return NewExecutionEnvelope(reg, &Deps{
		DB:      db,
		Audit:   agent.NewAuditWriter(db),
		Confirm: agent.NewConfirmationManager(5 * time.Minute),
		Preferences: &agent.PreferenceLoader{LoadFunc: func(ctx context.Context, userID string) (*agent.ChatPreferences, error) {
			prefs, err := db.GetChatPrefs(ctx, userID)
			if err != nil {
				return nil, err
			}
			return &agent.ChatPreferences{
				ChatWriteToolsEnabled: prefs.ChatWriteToolsEnabled,
				ChatAllowedWriteTools: prefs.ChatAllowedWriteTools,
			}, nil
		}},
	})
}

func activityIntPtr(v int) *int { return &v }

func activityFloatPtr(v float64) *float64 { return &v }

func activityBoolPtr(v bool) *bool { return &v }

func activityStringPtr(v string) *string { return &v }
