package automation

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/database/testdb"
	"github.com/skadia-app/skadia/services/api/internal/model"
	"github.com/skadia-app/skadia/services/api/internal/rules"
)

func openAutomationTestDB(t *testing.T) (*database.DB, context.Context) {
	t.Helper()
	dsn := testdb.RequireSafeDSN(t)
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("pgxpool.New: %v", err)
	}
	t.Cleanup(func() { pool.Close() })
	return &database.DB{Pool: pool}, ctx
}

func insertAutomationTestUser(t *testing.T, db *database.DB, ctx context.Context) string {
	t.Helper()
	id := uuid.NewString()
	_, err := db.Pool.Exec(ctx,
		`INSERT INTO users (id, username, email) VALUES ($1, $2, $3)`,
		id, "u_"+id, id+"@example.test")
	if err != nil {
		t.Fatalf("insert user: %v", err)
	}
	return id
}

func insertAutomationTestRule(t *testing.T, db *database.DB, ctx context.Context, userID string, conditions string) model.AutomationRule {
	t.Helper()
	ruleID := uuid.NewString()
	if conditions == "" {
		conditions = `[]`
	}
	_, err := db.Pool.Exec(ctx, `
		INSERT INTO automation_rules (id, user_id, name, conditions, condition_groups, actions)
		VALUES ($1, $2, 'Gear rule', $3::jsonb, '[]'::jsonb, '[]'::jsonb)`,
		ruleID, userID, conditions)
	if err != nil {
		t.Fatalf("insert rule: %v", err)
	}
	return model.AutomationRule{ID: ruleID, UserID: userID, Name: "Gear rule", Conditions: json.RawMessage(conditions)}
}

func TestApplyChangesSkipsInvalidGearActionAndRecordsWarning(t *testing.T) {
	db, ctx := openAutomationTestDB(t)
	userID := insertAutomationTestUser(t, db, ctx)
	rule := insertAutomationTestRule(t, db, ctx, userID, "")
	activityID := uuid.NewString()
	gearID := uuid.NewString()

	if _, err := db.Pool.Exec(ctx, `
		INSERT INTO activities (id, user_id, sport_type, started_at)
		VALUES ($1, $2, 'cycling', $3)`,
		activityID, userID, time.Now().UTC()); err != nil {
		t.Fatalf("insert activity: %v", err)
	}
	if _, err := db.Pool.Exec(ctx, `
		INSERT INTO gear (id, user_id, name, type, scope, is_retired, retired_at)
		VALUES ($1, $2, 'Old Bike', 'bike', 'bike', true, NULL)`,
		gearID, userID); err != nil {
		t.Fatalf("insert gear: %v", err)
	}

	applyChanges(ctx, db, activityID, userID, rule, []rules.Action{{
		Action: "set_gear",
		Params: json.RawMessage(`{"gear_id":"` + gearID + `"}`),
	}}, map[string]any{})

	var assignmentCount int
	if err := db.Pool.QueryRow(ctx, `SELECT count(*) FROM activity_gear_assignments WHERE activity_id = $1 AND gear_id = $2`, activityID, gearID).Scan(&assignmentCount); err != nil {
		t.Fatalf("read activity gear assignments: %v", err)
	}
	if assignmentCount != 0 {
		t.Fatalf("gear assignment count = %d, want 0 (retired gear must not be assigned)", assignmentCount)
	}
	var warningCode string
	if err := db.Pool.QueryRow(ctx, `SELECT last_warning_code FROM automation_rules WHERE id = $1`, rule.ID).Scan(&warningCode); err != nil {
		t.Fatalf("read rule warning: %v", err)
	}
	if warningCode != ruleWarningGearReferenceInvalid {
		t.Fatalf("warning code = %q, want %q", warningCode, ruleWarningGearReferenceInvalid)
	}
}

func TestEvaluateGearLifecycleRuleMatchesElevationLoss(t *testing.T) {
	db, ctx := openAutomationTestDB(t)
	userID := insertAutomationTestUser(t, db, ctx)
	gearID := uuid.NewString()
	if _, err := db.Pool.Exec(ctx, `
		INSERT INTO gear (
			id, user_id, name, type, scope, total_distance_m, total_moving_time_s,
			total_time_s, total_elevation_gain_m, total_elevation_loss_m, total_activities
		)
		VALUES ($1, $2, 'Downhill Bike', 'bike', 'bike', 12000, 3600, 4200, 200, 2200, 3)`,
		gearID, userID); err != nil {
		t.Fatalf("insert gear: %v", err)
	}
	rule := insertAutomationTestRule(t, db, ctx, userID, `[]`)
	rule.ConditionGroups = json.RawMessage(`[
		{"logic":"and","conditions":[
			{"field":"gear_total_elevation_loss_m","op":"gte","value":2000}
		]}
	]`)

	matches, err := EvaluateGearLifecycleRule(ctx, db, userID, rule)
	if err != nil {
		t.Fatalf("EvaluateGearLifecycleRule: %v", err)
	}
	if len(matches) != 1 || matches[0] != gearID {
		t.Fatalf("matches = %#v, want [%s]", matches, gearID)
	}
}

func TestEvaluateGearLifecycleRuleDoesNotWarnForValidNonMatchingGear(t *testing.T) {
	db, ctx := openAutomationTestDB(t)
	userID := insertAutomationTestUser(t, db, ctx)
	gearID := uuid.NewString()
	if _, err := db.Pool.Exec(ctx, `
		INSERT INTO gear (
			id, user_id, name, type, scope, total_distance_m, total_moving_time_s,
			total_time_s, total_elevation_gain_m, total_elevation_loss_m, total_activities
		)
		VALUES ($1, $2, 'Road Bike', 'bike', 'bike', 50000, 3600, 4200, 200, 300, 3)`,
		gearID, userID); err != nil {
		t.Fatalf("insert gear: %v", err)
	}
	rule := insertAutomationTestRule(t, db, ctx, userID, `[
		{"field":"gear_id","op":"eq","value":"`+gearID+`"},
		{"field":"gear_total_distance_m","op":"gte","value":100000}
	]`)

	matches, err := EvaluateGearLifecycleRule(ctx, db, userID, rule)
	if err != nil {
		t.Fatalf("EvaluateGearLifecycleRule: %v", err)
	}
	if len(matches) != 0 {
		t.Fatalf("matches = %#v, want none", matches)
	}
	var warningCode *string
	if err := db.Pool.QueryRow(ctx, `SELECT last_warning_code FROM automation_rules WHERE id = $1`, rule.ID).Scan(&warningCode); err != nil {
		t.Fatalf("read warning: %v", err)
	}
	if warningCode != nil {
		t.Fatalf("warning = %q, want nil", *warningCode)
	}
}

func TestEvaluateGearLifecycleRuleClearsStaleReferenceWarning(t *testing.T) {
	db, ctx := openAutomationTestDB(t)
	userID := insertAutomationTestUser(t, db, ctx)
	gearID := uuid.NewString()
	if _, err := db.Pool.Exec(ctx, `
		INSERT INTO gear (
			id, user_id, name, type, scope, total_distance_m, total_moving_time_s,
			total_time_s, total_elevation_gain_m, total_elevation_loss_m, total_activities
		)
		VALUES ($1, $2, 'Road Bike', 'bike', 'bike', 50000, 3600, 4200, 200, 300, 3)`,
		gearID, userID); err != nil {
		t.Fatalf("insert gear: %v", err)
	}
	rule := insertAutomationTestRule(t, db, ctx, userID, `[
		{"field":"gear_id","op":"eq","value":"`+gearID+`"}
	]`)
	if err := db.SetRuleWarning(ctx, rule.ID, userID, ruleWarningGearReferenceInvalid, "stale"); err != nil {
		t.Fatalf("set stale warning: %v", err)
	}

	if _, err := EvaluateGearLifecycleRule(ctx, db, userID, rule); err != nil {
		t.Fatalf("EvaluateGearLifecycleRule: %v", err)
	}
	var warningCode *string
	if err := db.Pool.QueryRow(ctx, `SELECT last_warning_code FROM automation_rules WHERE id = $1`, rule.ID).Scan(&warningCode); err != nil {
		t.Fatalf("read warning: %v", err)
	}
	if warningCode != nil {
		t.Fatalf("warning = %q, want nil", *warningCode)
	}
}

func TestEvaluateGearLifecycleRuleWarnsForRetiredGearInConditionGroup(t *testing.T) {
	db, ctx := openAutomationTestDB(t)
	userID := insertAutomationTestUser(t, db, ctx)
	gearID := uuid.NewString()
	if _, err := db.Pool.Exec(ctx, `
		INSERT INTO gear (id, user_id, name, type, scope, status, is_retired)
		VALUES ($1, $2, 'Old Bike', 'bike', 'bike', 'retired', true)`,
		gearID, userID); err != nil {
		t.Fatalf("insert gear: %v", err)
	}
	rule := insertAutomationTestRule(t, db, ctx, userID, `[]`)
	rule.ConditionGroups = json.RawMessage(`[
		{"logic":"and","conditions":[
			{"field":"gear_id","op":"in","value":["` + gearID + `"]}
		]}
	]`)

	if _, err := EvaluateGearLifecycleRule(ctx, db, userID, rule); err != nil {
		t.Fatalf("EvaluateGearLifecycleRule: %v", err)
	}
	var warningCode string
	if err := db.Pool.QueryRow(ctx, `SELECT last_warning_code FROM automation_rules WHERE id = $1`, rule.ID).Scan(&warningCode); err != nil {
		t.Fatalf("read warning: %v", err)
	}
	if warningCode != ruleWarningGearReferenceInvalid {
		t.Fatalf("warning code = %q, want %q", warningCode, ruleWarningGearReferenceInvalid)
	}
}
