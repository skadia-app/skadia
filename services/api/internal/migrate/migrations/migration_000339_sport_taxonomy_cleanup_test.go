package migrations_test

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/skadia-app/skadia/services/api/internal/sport"
)

func readMigration000339SQL(t *testing.T) (upSQL, downSQL, preflightSQL string) {
	t.Helper()
	upBytes, err := os.ReadFile("000339_sport_taxonomy_cleanup.up.sql")
	if err != nil {
		t.Fatalf("read 000339 up.sql: %v", err)
	}
	downBytes, err := os.ReadFile("000339_sport_taxonomy_cleanup.down.sql")
	if err != nil {
		t.Fatalf("read 000339 down.sql: %v", err)
	}
	preflightBytes, err := os.ReadFile("../preflight_000339_sport_taxonomy_rules.sql")
	if err != nil {
		t.Fatalf("read 000339 rule preflight sql: %v", err)
	}
	return string(upBytes), string(downBytes), string(preflightBytes)
}

func TestMigration000339SQLFilesDeclareRequiredContracts(t *testing.T) {
	upSQL, downSQL, preflightSQL := readMigration000339SQL(t)

	requiredUp := []string{
		"CREATE OR REPLACE FUNCTION fn_validate_predicates",
		"ALTER TABLE routing_profiles",
		"ADD COLUMN IF NOT EXISTS predicates JSONB NOT NULL DEFAULT '[]'::jsonb",
		"ADD COLUMN IF NOT EXISTS modifier_filters JSONB NOT NULL DEFAULT '{}'::jsonb",
		"routing_profiles_predicates_valid",
		"routing_profiles_modifier_filters_valid",
		"activities_sport_type_canonical",
		"CREATE TABLE IF NOT EXISTS segments_legacy_sub_type_hints",
		"original_sport_types TEXT[] NOT NULL",
		"original_sub_types TEXT[]",
		"ALTER TABLE planned_routes",
		"ADD COLUMN IF NOT EXISTS sub_type TEXT",
		"ALTER TABLE recording_groups",
		"ADD COLUMN IF NOT EXISTS sub_type TEXT",
		"ADD COLUMN IF NOT EXISTS is_electric BOOLEAN NOT NULL DEFAULT FALSE",
		"ADD COLUMN IF NOT EXISTS is_indoor BOOLEAN NOT NULL DEFAULT FALSE",
		"ADD COLUMN IF NOT EXISTS is_virtual BOOLEAN NOT NULL DEFAULT FALSE",
		"mountain_biking",
		"gravel_cycling",
		"indoor_cycling",
		"trail_running",
		"treadmill_running",
		"lap_swimming",
		"open_water_swimming",
		"road_cycling",
		"bike",
		"e_mountain_biking",
		"e_biking",
		"e_bike",
	}
	for _, needle := range requiredUp {
		if !strings.Contains(upSQL, needle) {
			t.Errorf("up migration missing %q", needle)
		}
	}

	requiredDown := []string{
		"WARNING: This down migration is destructive for post-cleanup sub_types.",
		"DROP TABLE IF EXISTS segments_legacy_sub_type_hints",
		"DROP FUNCTION IF EXISTS fn_validate_predicates",
		"DROP COLUMN IF EXISTS predicates",
		"DROP COLUMN IF EXISTS modifier_filters",
		"DROP COLUMN IF EXISTS sub_type",
		"DROP COLUMN IF EXISTS is_electric",
		"DROP COLUMN IF EXISTS is_indoor",
		"DROP COLUMN IF EXISTS is_virtual",
		"parsed_sport_type",
		"parsed_sub_type",
	}
	for _, needle := range requiredDown {
		if !strings.Contains(downSQL, needle) {
			t.Errorf("down migration missing %q", needle)
		}
	}

	if !strings.Contains(preflightSQL, "condition_groups") ||
		!strings.Contains(preflightSQL, "branches") ||
		!strings.Contains(preflightSQL, "logic") ||
		!strings.Contains(preflightSQL, "or") {
		t.Errorf("preflight SQL must scan OR condition groups in top-level and branch rule JSON")
	}
}

func TestMigration000339DoesNotMutateTagTablesOrTriggers(t *testing.T) {
	upSQL, _, _ := readMigration000339SQL(t)
	sql := stripSQLComments(upSQL)
	forbidden := []string{
		"activity_tags",
		"sync_system_flags_and_tags",
		"sync_virtual_tag",
		"prevent_virtual_tag_removal",
		"CREATE INDEX CONCURRENTLY",
	}
	for _, needle := range forbidden {
		if strings.Contains(sql, needle) {
			t.Fatalf("up migration must not contain %q outside comments", needle)
		}
	}
}

func TestMigration000339AppliesAndNormalizesFixtures(t *testing.T) {
	upSQL, _, _ := readMigration000339SQL(t)
	pool := migrationTestPool(t)
	ctx := context.Background()

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	t.Cleanup(func() {
		if err := tx.Rollback(ctx); err != nil && err.Error() != "tx is closed" {
			t.Logf("rollback: %v", err)
		}
	})

	userID := uuid.NewString()
	if _, err := tx.Exec(ctx, `
		INSERT INTO users (id, username, email, is_admin)
		VALUES ($1, $2, $3, false)
	`, userID, "mig339_"+userID[:8], "mig339_"+userID[:8]+"@example.com"); err != nil {
		t.Fatalf("insert user: %v", err)
	}

	activityID := uuid.NewString()
	if _, err := tx.Exec(ctx, `
		INSERT INTO activities (id, user_id, title, sport_type, sub_type, parsed_sport_type, parsed_sub_type, is_indoor, is_electric, started_at)
		VALUES ($1, $2, 'legacy activity', 'e_mountain_biking', NULL, 'treadmill_running', NULL, false, false, now())
	`, activityID, userID); err != nil {
		t.Fatalf("insert activity: %v", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO heatmap_lines (user_id, activity_id, sport_type, sub_type, geom, activity_date)
		VALUES ($1, $2, 'mountain_biking', NULL, ST_GeomFromText('LINESTRING(8 49, 8.01 49.01)', 4326), CURRENT_DATE)
	`, userID, activityID); err != nil {
		t.Fatalf("insert heatmap line: %v", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO heatmap_points (user_id, activity_id, sport_type, sub_type, geom, activity_date)
		VALUES ($1, $2, 'trail_running', NULL, ST_GeomFromText('POINT(8 49)', 4326), CURRENT_DATE)
	`, userID, activityID); err != nil {
		t.Fatalf("insert heatmap point: %v", err)
	}

	segmentID := uuid.NewString()
	versionID := uuid.NewString()
	if _, err := tx.Exec(ctx, `
		INSERT INTO segments (id, created_by, name, allowed_sport_types, allowed_sub_types)
		VALUES ($1, $2, 'legacy segment', ARRAY['mountain_biking','trail_running'], ARRAY[]::text[])
	`, segmentID, userID); err != nil {
		t.Fatalf("insert segment: %v", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO segment_versions (id, segment_id, version_number, track, valid_from, created_by)
		VALUES ($1, $2, 1, ST_GeomFromText('LINESTRING(8 49, 8.01 49.01)', 4326), CURRENT_DATE, $3)
	`, versionID, segmentID, userID); err != nil {
		t.Fatalf("insert segment version: %v", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO segment_efforts (segment_id, segment_version_id, activity_id, user_id, started_at, elapsed_time_s, sport_type, sub_type)
		VALUES ($1, $2, $3, $4, now(), 120, 'mountain_biking', 'generic')
	`, segmentID, versionID, activityID, userID); err != nil {
		t.Fatalf("insert segment effort: %v", err)
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO planned_routes (user_id, name, sport_type, profile, geometry, waypoints, segments)
		VALUES ($1, 'legacy route', 'mountain_biking', 'mtb', ST_GeomFromText('LINESTRING(8 49, 8.01 49.01)', 4326), '[]'::jsonb, '[]'::jsonb)
	`, userID); err != nil {
		t.Fatalf("insert planned route: %v", err)
	}
	groupID := uuid.NewString()
	if _, err := tx.Exec(ctx, `
		INSERT INTO recording_groups (id, user_id, sport_type)
		VALUES ($1, $2, 'trail_running')
	`, groupID, userID); err != nil {
		t.Fatalf("insert recording group: %v", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO routing_profiles (user_id, name, base_profile, sport_types, config, source)
		VALUES
			($1, 'legacy mtb', 'mtb', ARRAY['mountain_biking'], '{}'::jsonb, 'user'),
			($1, 'empty road', 'road', ARRAY[]::text[], '{}'::jsonb, 'user')
	`, userID); err != nil {
		t.Fatalf("insert routing profiles: %v", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO automation_rules (user_id, name, conditions, condition_groups, actions, branches)
		VALUES (
			$1,
			'legacy rule',
			'[{"field":"sport_type","op":"eq","value":"mountain_biking"}]'::jsonb,
			'[{"logic":"and","conditions":[{"field":"sport_type","op":"eq","value":"treadmill_running"}]}]'::jsonb,
			'[{"action":"set_sport_type","params":{"sport_type":"e_mountain_biking"}}]'::jsonb,
			'[{"type":"if","condition_groups":[{"logic":"and","conditions":[{"field":"sport_type","op":"eq","value":"gravel_cycling"}]}],"actions":[{"action":"set_sport_type","params":{"sport_type":"indoor_cycling"}}]}]'::jsonb
		)
	`, userID); err != nil {
		t.Fatalf("insert automation rule: %v", err)
	}

	if _, err := tx.Exec(ctx, `SET LOCAL skadia.allow_segment_sport_broadening = 'on'`); err != nil {
		t.Fatalf("set segment broadening gate: %v", err)
	}
	if _, err := tx.Exec(ctx, upSQL); err != nil {
		t.Fatalf("apply up.sql: %v", err)
	}

	var sportType, subType, parsedSportType, parsedSubType string
	var indoor, electric bool
	if err := tx.QueryRow(ctx, `
		SELECT sport_type, COALESCE(sub_type, ''), parsed_sport_type, COALESCE(parsed_sub_type, ''), is_indoor, is_electric
		FROM activities WHERE id = $1
	`, activityID).Scan(&sportType, &subType, &parsedSportType, &parsedSubType, &indoor, &electric); err != nil {
		t.Fatalf("query activity: %v", err)
	}
	if sportType != "cycling" || subType != "mountain" || parsedSportType != "running" || parsedSubType != "indoor_running" || !indoor || !electric {
		t.Fatalf("activity tuple = (%s,%s), parsed=(%s,%s), indoor=%v electric=%v", sportType, subType, parsedSportType, parsedSubType, indoor, electric)
	}

	var groupSportType, groupSubType string
	if err := tx.QueryRow(ctx, `
		SELECT sport_type, COALESCE(sub_type, '')
		FROM recording_groups WHERE id = $1
	`, groupID).Scan(&groupSportType, &groupSubType); err != nil {
		t.Fatalf("query recording group: %v", err)
	}
	if groupSportType != "running" || groupSubType != "trail" {
		t.Fatalf("recording group tuple = (%s,%s), want (running,trail)", groupSportType, groupSubType)
	}

	var hints int
	if err := tx.QueryRow(ctx, `SELECT COUNT(*) FROM segments_legacy_sub_type_hints WHERE segment_id = $1 AND original_sport_types @> ARRAY['mountain_biking']::text[]`, segmentID).Scan(&hints); err != nil {
		t.Fatalf("query segment hints: %v", err)
	}
	if hints != 1 {
		t.Fatalf("segment hints count = %d, want 1", hints)
	}

	var ruleActions string
	if err := tx.QueryRow(ctx, `SELECT actions::text FROM automation_rules WHERE name = 'legacy rule' AND user_id = $1`, userID).Scan(&ruleActions); err != nil {
		t.Fatalf("query rule actions: %v", err)
	}
	for _, needle := range []string{`"sport_type": "cycling"`, `"sub_type": "mountain"`, `"action": "set_electric"`} {
		if !strings.Contains(ruleActions, needle) {
			t.Fatalf("rewritten actions missing %q in %s", needle, ruleActions)
		}
	}
}

func TestMigration000339BlocksSegmentBroadeningWithoutGate(t *testing.T) {
	upSQL, _, _ := readMigration000339SQL(t)
	pool := migrationTestPool(t)
	ctx := context.Background()

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	t.Cleanup(func() {
		if err := tx.Rollback(ctx); err != nil && err.Error() != "tx is closed" {
			t.Logf("rollback: %v", err)
		}
	})

	userID := uuid.NewString()
	if _, err := tx.Exec(ctx, `
		INSERT INTO users (id, username, email, is_admin)
		VALUES ($1, $2, $3, false)
	`, userID, "mig339_segment_gate_"+userID[:8], "mig339_segment_gate_"+userID[:8]+"@example.com"); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO segments (id, created_by, name, allowed_sport_types, allowed_sub_types)
		VALUES ($1, $2, 'blocking legacy segment', ARRAY['mountain_biking'], ARRAY[]::text[])
	`, uuid.NewString(), userID); err != nil {
		t.Fatalf("insert segment: %v", err)
	}

	_, err = tx.Exec(ctx, upSQL)
	if err == nil || !strings.Contains(err.Error(), "segment sport collapse would broaden segment matching") {
		t.Fatalf("apply up.sql error = %v, want segment broadening block", err)
	}
}

func TestMigration000339DoesNotBlockSegmentWhenCanonicalParentAlreadyAllowed(t *testing.T) {
	upSQL, _, _ := readMigration000339SQL(t)
	pool := migrationTestPool(t)
	ctx := context.Background()

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	t.Cleanup(func() {
		if err := tx.Rollback(ctx); err != nil && err.Error() != "tx is closed" {
			t.Logf("rollback: %v", err)
		}
	})

	userID := uuid.NewString()
	if _, err := tx.Exec(ctx, `
		INSERT INTO users (id, username, email, is_admin)
		VALUES ($1, $2, $3, false)
	`, userID, "mig339_segment_parent_"+userID[:8], "mig339_segment_parent_"+userID[:8]+"@example.com"); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	segmentID := uuid.NewString()
	if _, err := tx.Exec(ctx, `
		INSERT INTO segments (id, created_by, name, allowed_sport_types, allowed_sub_types)
		VALUES ($1, $2, 'parent already allowed segment', ARRAY['cycling','mountain_biking','gravel_cycling','indoor_cycling'], ARRAY[]::text[])
	`, segmentID, userID); err != nil {
		t.Fatalf("insert segment: %v", err)
	}

	if _, err := tx.Exec(ctx, upSQL); err != nil {
		t.Fatalf("apply up.sql without broadening gate: %v", err)
	}

	var allowedSportTypes []string
	if err := tx.QueryRow(ctx, `
		SELECT allowed_sport_types
		FROM segments
		WHERE id = $1
	`, segmentID).Scan(&allowedSportTypes); err != nil {
		t.Fatalf("query segment: %v", err)
	}
	if len(allowedSportTypes) != 1 || allowedSportTypes[0] != "cycling" {
		t.Fatalf("allowed_sport_types = %v, want [cycling]", allowedSportTypes)
	}
}

func TestMigration000339PredicateValidatorMatchesGoFixtures(t *testing.T) {
	upSQL, _, _ := readMigration000339SQL(t)
	pool := migrationTestPool(t)
	ctx := context.Background()

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	t.Cleanup(func() {
		if err := tx.Rollback(ctx); err != nil && err.Error() != "tx is closed" {
			t.Logf("rollback: %v", err)
		}
	})
	if _, err := tx.Exec(ctx, `SET LOCAL skadia.allow_segment_sport_broadening = 'on'`); err != nil {
		t.Fatalf("set segment broadening gate: %v", err)
	}
	if _, err := tx.Exec(ctx, upSQL); err != nil {
		t.Fatalf("apply up.sql: %v", err)
	}

	empty := ""
	badSubType := "does_not_exist"
	legacyMountain := "mountain_biking"
	fixtures := []struct {
		name  string
		preds []sport.Predicate
	}{
		{name: "cycling mountain", preds: []sport.Predicate{{Sport: "cycling", SubType: strPtr000339("mountain")}}},
		{name: "road explicit none", preds: []sport.Predicate{{Sport: "cycling", SubType: &empty}}},
		{name: "hiking open", preds: []sport.Predicate{{Sport: "hiking"}}},
		{name: "invalid subtype", preds: []sport.Predicate{{Sport: "cycling", SubType: &badSubType}}},
		{name: "legacy sport", preds: []sport.Predicate{{Sport: legacyMountain}}},
		{name: "unknown sport", preds: []sport.Predicate{{Sport: "not_a_sport"}}},
	}

	for _, tc := range fixtures {
		t.Run(tc.name, func(t *testing.T) {
			goOK := sport.ValidatePredicates(tc.preds, false) == nil
			raw, err := json.Marshal(tc.preds)
			if err != nil {
				t.Fatalf("marshal fixture: %v", err)
			}
			var sqlOK bool
			if err := tx.QueryRow(ctx, `SELECT fn_validate_predicates($1::jsonb)`, string(raw)).Scan(&sqlOK); err != nil {
				t.Fatalf("SQL validator: %v", err)
			}
			if sqlOK != goOK {
				t.Fatalf("validator drift for %s: SQL=%v Go=%v fixture=%s", tc.name, sqlOK, goOK, string(raw))
			}
		})
	}
}

func TestMigration000339PreflightReportsORGroupLegacySport(t *testing.T) {
	_, _, preflightSQL := readMigration000339SQL(t)
	pool := migrationTestPool(t)
	ctx := context.Background()

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	t.Cleanup(func() {
		if err := tx.Rollback(ctx); err != nil && err.Error() != "tx is closed" {
			t.Logf("rollback: %v", err)
		}
	})

	userID := uuid.NewString()
	if _, err := tx.Exec(ctx, `
		INSERT INTO users (id, username, email, is_admin)
		VALUES ($1, $2, $3, false)
	`, userID, "mig339_preflight_"+userID[:8], "mig339_preflight_"+userID[:8]+"@example.com"); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO automation_rules (user_id, name, conditions, condition_groups, actions, branches)
		VALUES (
			$1,
			'or legacy rule',
			'[]'::jsonb,
			'[{"logic":"or","conditions":[{"field":"sport_type","op":"eq","value":"mountain_biking"}]}]'::jsonb,
			'[]'::jsonb,
			'[{"type":"if","condition_groups":[{"logic":"or","conditions":[{"field":"sport_type","op":"eq","value":"gravel_cycling"}]}],"actions":[]}]'::jsonb
		)
	`, userID); err != nil {
		t.Fatalf("insert automation rule: %v", err)
	}

	rows, err := tx.Query(ctx, preflightSQL)
	if err != nil {
		t.Fatalf("run preflight: %v", err)
	}
	defer rows.Close()

	var count int
	for rows.Next() {
		count++
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("preflight rows: %v", err)
	}
	if count < 2 {
		t.Fatalf("preflight rows = %d, want top-level and branch OR findings", count)
	}
}

func stripSQLComments(sql string) string {
	var out []string
	for _, line := range strings.Split(sql, "\n") {
		if idx := strings.Index(line, "--"); idx >= 0 {
			line = line[:idx]
		}
		out = append(out, line)
	}
	return strings.Join(out, "\n")
}

func strPtr000339(v string) *string {
	return &v
}
