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

	names := []string{"aggregate_stats", "aggregate_time_series", "compare_periods", "get_activity_detail", "query_activities", "query_records"}
	for _, name := range names {
		tool := reg.tools[name]
		if tool == nil {
			t.Fatalf("tool %q was not registered", name)
		}
		if tool.Permission != PermissionRead || !tool.IsUserOwned || tool.ExecuteRead == nil {
			t.Fatalf("unexpected metadata for %s: %#v", name, tool)
		}
	}
	if tool := reg.tools["preview_activity_selection"]; tool == nil {
		t.Fatal("tool \"preview_activity_selection\" was not registered")
	} else if tool.Permission != PermissionRead || tool.AuditMode != AuditPreview || tool.ExecutePreview == nil {
		t.Fatalf("unexpected preview metadata: %#v", tool)
	}
	for _, name := range []string{"update_activity_metadata", "rename_activity", "tag_activity", "bulk_tag_activities"} {
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
		Days:          new(14),
		Limit:         new(99),
		SortBy:        "distance",
		MinDistanceKM: new(5.5),
		TitleSearch:   "trail",
		GearID:        gearID,
		IsVirtual:     new(false),
		IsCommute:     new(true),
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

func TestBuildQueryActivitiesSQLFiltersByVisitedCountryCodes(t *testing.T) {
	sql, params, err := buildQueryActivitiesSQL("user-1", queryActivitiesArgs{
		CountryCodes: []string{"ch", "IT", "bad"},
	}, time.Date(2026, 5, 12, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(sql, "a.visited_country_codes && $3::text[]") {
		t.Fatalf("query SQL missing visited-country filter:\n%s", sql)
	}
	if !reflect.DeepEqual(params[2], []string{"CH", "IT"}) {
		t.Fatalf("country params = %#v, want CH/IT", params[2])
	}
}

func TestBuildQueryActivitiesSQLSupportsDateMaxDistanceAndTag(t *testing.T) {
	sql, params, err := buildQueryActivitiesSQL("user-1", queryActivitiesArgs{
		DateFrom:      "2026-01-01",
		DateTo:        "2026-01-31",
		MaxDistanceKM: new(12.5),
		TagName:       "Intervalle",
	}, time.Date(2026, 5, 12, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}

	for _, want := range []string{
		"FROM activities a WHERE a.user_id = $1",
		"a.started_at >= $2",
		"a.started_at < $3",
		"a.distance_m <= $4",
		"at.activity_id = a.id AND t.user_id = a.user_id AND lower(t.name) = lower($5)",
	} {
		if !strings.Contains(sql, want) {
			t.Fatalf("query SQL missing %q:\n%s", want, sql)
		}
	}
	wantParams := []any{
		"user-1",
		dateOnly(2026, 1, 1),
		dateOnly(2026, 2, 1),
		12500.0,
		"Intervalle",
	}
	if !reflect.DeepEqual(params, wantParams) {
		t.Fatalf("params mismatch:\nwant %#v\ngot  %#v", wantParams, params)
	}
}

func TestActivitySelectionDefaultsToBoundedRecentWindow(t *testing.T) {
	plan, err := buildActivitySelectionPlan("user-1", activitySelectionCriteria{SportType: "running"}, time.Date(2026, 5, 12, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	where := strings.Join(plan.Conditions, " AND ")
	if !strings.Contains(where, "a.user_id = $1") || !strings.Contains(where, "a.started_at >= $2") {
		t.Fatalf("selection did not apply default date bound:\n%s", where)
	}
	if got := plan.CriteriaForResponse["default_days_applied"]; got != true {
		t.Fatalf("default_days_applied = %#v, want true", got)
	}
	if got := plan.CriteriaForResponse["days"]; got != defaultActivitySelectionDays {
		t.Fatalf("days = %#v, want %d", got, defaultActivitySelectionDays)
	}
	if !reflect.DeepEqual(plan.Params[:2], []any{"user-1", dateOnly(2026, 4, 12)}) {
		t.Fatalf("params prefix = %#v", plan.Params[:2])
	}
}

func TestBuildAggregateStatsSQLUsesUserOwnedWhereForEachGrouping(t *testing.T) {
	now := time.Date(2026, 5, 12, 12, 0, 0, 0, time.UTC)
	for _, groupBy := range []string{"none", "week", "month", "sport", "sport_tuple"} {
		sql, params, err := buildAggregateStatsSQL("user-1", aggregateStatsArgs{
			Metric:  "total_distance",
			Days:    new(30),
			GroupBy: groupBy,
		}, now)
		if err != nil {
			t.Fatalf("%s buildAggregateStatsSQL: %v", groupBy, err)
		}
		if !strings.Contains(sql, "FROM activities a WHERE a.user_id = $1 AND a.started_at >= $2") {
			t.Fatalf("%s SQL must keep user_id first:\n%s", groupBy, sql)
		}
		if !reflect.DeepEqual(params, []any{"user-1", time.Date(2026, 4, 12, 0, 0, 0, 0, time.UTC)}) {
			t.Fatalf("%s params mismatch: %#v", groupBy, params)
		}
	}
}

func TestActivitySelectionResolvesNaturalSportQuery(t *testing.T) {
	plan, err := buildActivitySelectionPlan("user-1", activitySelectionCriteria{
		SportQuery: "last swim",
		Days:       new(365),
	}, time.Date(2026, 5, 12, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	where := strings.Join(plan.Conditions, " AND ")
	if !strings.Contains(where, "a.sport_type = $3") {
		t.Fatalf("selection did not add sport_type filter:\n%s", where)
	}
	if got := plan.Params[2]; got != "swimming" {
		t.Fatalf("sport_type param = %#v, want swimming", got)
	}
	if got := plan.CriteriaForResponse["sport_query"]; got != "last swim" {
		t.Fatalf("sport_query response = %#v, want original query", got)
	}
}

func TestAggregateStatsResolvesNaturalSportQuery(t *testing.T) {
	sql, params, err := buildAggregateStatsSQL("user-1", aggregateStatsArgs{
		Metric:     "count",
		SportQuery: "Freiwasserschwimmen",
		Days:       new(365),
	}, time.Date(2026, 5, 12, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"sport_type = $3", "sub_type = $4"} {
		if !strings.Contains(sql, want) {
			t.Fatalf("aggregate SQL missing %q:\n%s", want, sql)
		}
	}
	if !reflect.DeepEqual(params[2:], []any{"swimming", "open_water"}) {
		t.Fatalf("sport params = %#v, want swimming/open_water", params[2:])
	}
}

func TestActivitySelectionRejectsConflictingNaturalSportQuery(t *testing.T) {
	_, err := buildActivitySelectionPlan("user-1", activitySelectionCriteria{
		SportQuery: "last swim",
		SportType:  "running",
		Days:       new(365),
	}, time.Date(2026, 5, 12, 12, 0, 0, 0, time.UTC))
	if err == nil || !strings.Contains(err.Error(), "resolves to sport_type") {
		t.Fatalf("err = %v, want sport_query conflict", err)
	}
}

func TestBuildAggregateTimeSeriesSQLUsesSelectionPlanAndDefaults(t *testing.T) {
	sql, params, criteria, err := buildAggregateTimeSeriesSQL("user-1", aggregateTimeSeriesArgs{
		Metric:     "total_distance",
		Bucket:     "year",
		SportQuery: "Gravel",
	}, time.Date(2026, 5, 12, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"date_trunc('year', a.started_at)::date AS period",
		"FROM activities a WHERE a.user_id = $1",
		"a.started_at >= $2",
		"a.sport_type = $3",
		"a.sub_type = $4",
		"GROUP BY period ORDER BY period",
	} {
		if !strings.Contains(sql, want) {
			t.Fatalf("time series SQL missing %q:\n%s", want, sql)
		}
	}
	if !reflect.DeepEqual(params[2:], []any{"cycling", "gravel_cycling"}) {
		t.Fatalf("sport params = %#v, want cycling/gravel_cycling", params[2:])
	}
	if criteria["days"] != 365 {
		t.Fatalf("criteria days = %#v, want 365 default", criteria["days"])
	}
}

func TestBuildQueryRecordsSQLUsesSelectionPlanAndCapsLimit(t *testing.T) {
	sql, params, criteria, err := buildQueryRecordsSQL("user-1", queryRecordsArgs{
		RecordType:    "most_elevation",
		SportQuery:    "MTB",
		AllHistory:    true,
		Limit:         new(99),
		MinDistanceKM: activityFloatPtr(10),
	}, time.Date(2026, 5, 12, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"FROM activities a WHERE a.user_id = $1",
		"a.sport_type = $2",
		"a.sub_type = $3",
		"a.distance_m >= $4",
		"ORDER BY a.elevation_gain_m DESC NULLS LAST",
		"LIMIT 20",
	} {
		if !strings.Contains(sql, want) {
			t.Fatalf("records SQL missing %q:\n%s", want, sql)
		}
	}
	if !reflect.DeepEqual(params, []any{"user-1", "cycling", "mountain", 10000.0}) {
		t.Fatalf("params = %#v", params)
	}
	if criteria["all_history"] != true {
		t.Fatalf("all_history criteria = %#v, want true", criteria["all_history"])
	}
}

func TestBuildComparePeriodsSQLFiltersUserFirst(t *testing.T) {
	sql, params, err := buildComparePeriodSQL("user-1", "2026-05-01", "2026-05-12", comparePeriodsArgs{
		SportType: "cycling",
		SubType:   new("road"),
		IsIndoor:  new(false),
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

func TestBulkTagActivitiesPrepareAndExecuteUsesPreparedIDsOnly(t *testing.T) {
	ctx := context.Background()
	pool := testdb.Open(t)
	userA := insertActivityToolUser(t, pool)
	userB := insertActivityToolUser(t, pool)
	ownMatch := insertActivityToolActivity(t, pool, userA, "Langer Lauf")
	ownNonMatch := insertActivityToolActivity(t, pool, userA, "Kurzer Lauf")
	otherUserActivity := insertActivityToolActivity(t, pool, userB, "Fremder Lauf")
	tagID := insertActivityToolTag(t, pool, userA, "Bulk")

	if _, err := pool.Exec(ctx, `UPDATE activities SET distance_m = 15000 WHERE id = $1`, ownMatch); err != nil {
		t.Fatalf("update own match: %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE activities SET distance_m = 3000 WHERE id = $1`, ownNonMatch); err != nil {
		t.Fatalf("update own non-match: %v", err)
	}

	db := &database.DB{Pool: pool}
	prepared, err := prepareBulkTagActivities(ctx, &Deps{DB: db, Readonly: pool}, userA, json.RawMessage(`{
		"tag_name":"Bulk",
		"mode":"add",
		"selection":{"sport_type":"running","min_distance_km":10,"days":30}
	}`))
	if err != nil {
		t.Fatalf("prepareBulkTagActivities: %v", err)
	}
	var preparedArgs bulkTagActivitiesArgs
	if err := json.Unmarshal(prepared.Args, &preparedArgs); err != nil {
		t.Fatalf("decode prepared args: %v", err)
	}
	if !reflect.DeepEqual(preparedArgs.PreparedActivityIDs, []string{ownMatch}) {
		t.Fatalf("prepared ids = %#v, want only %s", preparedArgs.PreparedActivityIDs, ownMatch)
	}

	preparedArgs.Selection = activitySelectionCriteria{ActivityIDs: []string{otherUserActivity, ownNonMatch}, AllHistory: true}
	executeRaw, err := json.Marshal(preparedArgs)
	if err != nil {
		t.Fatalf("marshal execute args: %v", err)
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	result, execErr := executeBulkTagActivities(ctx, &WriteExecutionDeps{Deps: &Deps{DB: db}, Tx: tx}, userA, executeRaw)
	if execErr != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("executeBulkTagActivities: %v", execErr)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit: %v", err)
	}
	if result.(map[string]any)["affected"] != 1 {
		t.Fatalf("result = %#v", result)
	}

	assertActivityTagLinked(t, pool, ownMatch, tagID, true)
	assertActivityTagLinked(t, pool, ownNonMatch, tagID, false)
	assertActivityTagLinked(t, pool, otherUserActivity, tagID, false)
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

func assertActivityTagLinked(t *testing.T, pool *pgxpool.Pool, activityID, tagID string, want bool) {
	t.Helper()
	var linked bool
	if err := pool.QueryRow(context.Background(),
		`SELECT EXISTS (SELECT 1 FROM activity_tags WHERE activity_id = $1 AND tag_id = $2)`,
		activityID, tagID,
	).Scan(&linked); err != nil {
		t.Fatalf("select activity tag link: %v", err)
	}
	if linked != want {
		t.Fatalf("activity %s tag link = %v, want %v", activityID, linked, want)
	}
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

//go:fix inline
func activityIntPtr(v int) *int { return new(v) }

//go:fix inline
func activityFloatPtr(v float64) *float64 { return new(v) }

//go:fix inline
func activityBoolPtr(v bool) *bool { return new(v) }

//go:fix inline
func activityStringPtr(v string) *string { return new(v) }
