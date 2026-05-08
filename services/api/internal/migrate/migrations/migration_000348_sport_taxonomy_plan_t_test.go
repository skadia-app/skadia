package migrations_test

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func readMigration000348PlanTSQL(t *testing.T) (upSQL, downSQL string) {
	t.Helper()
	upBytes, err := os.ReadFile("000348_sport_taxonomy_plan_t.up.sql")
	if err != nil {
		t.Fatalf("read 000348 up.sql: %v", err)
	}
	downBytes, err := os.ReadFile("000348_sport_taxonomy_plan_t.down.sql")
	if err != nil {
		t.Fatalf("read 000348 down.sql: %v", err)
	}
	return string(upBytes), string(downBytes)
}

func TestMigration000348PlanTDeclaresRequiredContracts(t *testing.T) {
	upSQL, downSQL := readMigration000348PlanTSQL(t)

	for _, needle := range []string{
		"SELECT pg_advisory_xact_lock(330348, 1)",
		"ALTER TABLE activities DROP CONSTRAINT IF EXISTS activities_sport_type_canonical",
		"wheelchair_push_walk",
		"wheelchair_push_run",
		"indoor_wheelchair_walk",
		"indoor_wheelchair_run",
		"strength_training",
		"('training','pilates')",
		"CREATE OR REPLACE FUNCTION fp_sport_taxonomy_normalize",
		"CREATE OR REPLACE FUNCTION fn_validate_predicates",
		"SET search_path = public",
		"fp_plan_t_rewrite_predicates",
		"fp_plan_t_rewrite_conditions",
		"fp_plan_t_rewrite_actions",
		"segments_allowed_predicates_valid",
		"routing_profiles_predicates_valid",
		"activities_sport_type_canonical",
		"Plan T sport taxonomy migration left removed sport values behind",
		"DROP FUNCTION IF EXISTS fp_sport_taxonomy_normalize(TEXT, TEXT)",
	} {
		if !strings.Contains(upSQL, needle) {
			t.Fatalf("up migration missing %q", needle)
		}
	}
	for _, needle := range []string{
		"intentionally conservative and lossy",
		"strength_training is not converted back to",
		"wheelchair_push_walk",
		"wheelchair_push_run",
		"SET search_path = public",
		"fp_plan_t_down_predicates",
		"DROP FUNCTION IF EXISTS fp_sport_taxonomy_normalize(TEXT, TEXT)",
	} {
		if !strings.Contains(downSQL, needle) {
			t.Fatalf("down migration missing %q", needle)
		}
	}
	for _, forbidden := range []string{
		"('wheelchair_push','run_speed')",
		"('wheelchair_push','walk_speed')",
		"('training','pilates')",
	} {
		if strings.Contains(downSQL, forbidden) {
			t.Fatalf("down migration still allows Plan T tuple %q", forbidden)
		}
	}
	if strings.Contains(upSQL, "CREATE INDEX CONCURRENTLY") {
		t.Fatal("Plan T migration must not use CREATE INDEX CONCURRENTLY")
	}
	for _, needle := range []string{
		"SELECT se.id, se.started_at, n.sport_type",
		"WHERE se.id = n.id",
		"AND se.started_at = n.started_at",
	} {
		if !strings.Contains(upSQL, needle) {
			t.Fatalf("segment_efforts update must use full hypertable key, missing %q", needle)
		}
	}
}

func TestMigration000348PlanTAppliesAndRewritesRemovedValues(t *testing.T) {
	upSQL, downSQL := readMigration000348PlanTSQL(t)
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
		INSERT INTO users (id, username, email, email_verified, registration_source, is_admin)
		VALUES ($1, $2, $3, true, 'test', false)
	`, userID, "mig999_"+userID[:8], "mig999_"+userID[:8]+"@example.com"); err != nil {
		t.Fatalf("insert user: %v", err)
	}

	activityWalk := uuid.NewString()
	activityRun := uuid.NewString()
	parsedIndoorOnly := uuid.NewString()
	parsedElectricOnly := uuid.NewString()
	if _, err := tx.Exec(ctx, `
		INSERT INTO activities (id, user_id, title, sport_type, sub_type, parsed_sport_type, parsed_sub_type, is_indoor, started_at)
		VALUES
			($1, $3, 'wheel walk', 'wheelchair_push_walk', '', NULL, NULL, false, now()),
			($2, $3, 'wheel run', 'wheelchair_push_run', 'indoor_wheelchair_run', 'wheelchair_push_run', 'indoor_wheelchair_run', false, now()),
			($4, $3, 'parsed treadmill only', 'running', 'street', 'treadmill_running', '', false, now()),
			($5, $3, 'parsed ebike only', 'cycling', 'road', 'e_bike', '', false, now()),
			(gen_random_uuid(), $3, 'strength legacy', 'strength', NULL, NULL, NULL, false, now()),
			(gen_random_uuid(), $3, 'generic legacy subtype', 'running', 'generic', 'running', 'generic', false, now()),
			(gen_random_uuid(), $3, 'hiking legacy subtype', 'hiking', 'trail', 'hiking', 'trail', false, now()),
			(gen_random_uuid(), $3, 'cycling legacy subtype', 'cycling', 'commuting', NULL, NULL, false, now()),
			(gen_random_uuid(), $3, 'diving legacy subtype', 'diving', 'subsportinvalid(63)', NULL, NULL, false, now())
	`, activityWalk, activityRun, userID, parsedIndoorOnly, parsedElectricOnly); err != nil {
		t.Fatalf("insert activities: %v", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO heatmap_lines (user_id, activity_id, sport_type, sub_type, geom, activity_date)
		VALUES ($1, $2, 'wheelchair_push_walk', NULL, ST_GeomFromText('LINESTRING(8 49, 8.01 49.01)', 4326), CURRENT_DATE)
	`, userID, activityWalk); err != nil {
		t.Fatalf("insert heatmap line: %v", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO heatmap_points (user_id, activity_id, sport_type, sub_type, geom, activity_date)
		VALUES ($1, $2, 'wheelchair_push_run', NULL, ST_GeomFromText('POINT(8 49)', 4326), CURRENT_DATE)
	`, userID, activityRun); err != nil {
		t.Fatalf("insert heatmap point: %v", err)
	}

	segmentID := uuid.NewString()
	versionID := uuid.NewString()
	if _, err := tx.Exec(ctx, `
		INSERT INTO segments (id, created_by, name, sport_group, allowed_predicates)
		VALUES ($1, $2, 'wheel segment', 'foot', '[{"sport":"wheelchair_push_walk"},{"sport":"wheelchair_push_run"}]'::jsonb)
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
		VALUES ($1, $2, $3, $4, now(), 120, 'wheelchair_push_walk', 'generic')
	`, segmentID, versionID, activityWalk, userID); err != nil {
		t.Fatalf("insert segment effort: %v", err)
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO planned_routes (user_id, name, sport_type, sub_type, profile, geometry, waypoints, segments)
		VALUES ($1, 'wheel route', 'wheelchair_push_run', NULL, 'hiking', ST_GeomFromText('LINESTRING(8 49, 8.01 49.01)', 4326), '[]'::jsonb, '[]'::jsonb)
	`, userID); err != nil {
		t.Fatalf("insert planned route: %v", err)
	}
	groupID := uuid.NewString()
	if _, err := tx.Exec(ctx, `
		INSERT INTO recording_groups (id, user_id, sport_type)
		VALUES ($1, $2, 'wheelchair_push_walk')
	`, groupID, userID); err != nil {
		t.Fatalf("insert recording group: %v", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO routing_profiles (user_id, name, base_profile, sport_types, predicates, modifier_filters, config, source)
		VALUES ($1, 'wheel profile', 'hiking', ARRAY['wheelchair_push_walk','wheelchair_push_run','strength'], '[{"sport":"cycling","sub_type":"mountain"},{"sport":"wheelchair_push_walk"}]'::jsonb, '{}'::jsonb, '{}'::jsonb, 'user')
	`, userID); err != nil {
		t.Fatalf("insert routing profile: %v", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO automation_rules (user_id, name, conditions, condition_groups, actions, branches)
		VALUES (
			$1,
			'wheel rule',
			'[{"field":"sport_type","op":"eq","value":"wheelchair_push_walk"},{"field":"sport_type","op":"in","value":["running","strength"]}]'::jsonb,
			'[{"logic":"and","conditions":[{"field":"sport_type","op":"eq","value":"wheelchair_push_run"}]}]'::jsonb,
			'[{"action":"set_sport_type","params":{"sport_type":"strength"}}]'::jsonb,
			'[{"type":"if","condition_groups":[{"logic":"and","conditions":[{"field":"sport_type","op":"eq","value":"wheelchair_push_run"}]}],"actions":[{"action":"set_sport_type","params":{"sport_type":"wheelchair_push_walk","sub_type":"indoor_wheelchair_walk"}}]}]'::jsonb
		)
	`, userID); err != nil {
		t.Fatalf("insert automation rule: %v", err)
	}

	if _, err := tx.Exec(ctx, upSQL); err != nil {
		t.Fatalf("apply up.sql: %v", err)
	}

	var sportType, subType string
	var isIndoor bool
	if err := tx.QueryRow(ctx, `
		SELECT sport_type, COALESCE(sub_type, ''), is_indoor
		FROM activities WHERE id = $1
	`, activityRun).Scan(&sportType, &subType, &isIndoor); err != nil {
		t.Fatalf("query run activity: %v", err)
	}
	if sportType != "wheelchair_push" || subType != "run_speed" || !isIndoor {
		t.Fatalf("run activity tuple = (%s,%s), indoor=%v", sportType, subType, isIndoor)
	}
	var parsedSport, parsedSubType string
	var parsedOnlyIndoor, parsedOnlyElectric bool
	if err := tx.QueryRow(ctx, `
		SELECT COALESCE(parsed_sport_type, ''), COALESCE(parsed_sub_type, ''), is_indoor, is_electric
		FROM activities WHERE id = $1
	`, parsedIndoorOnly).Scan(&parsedSport, &parsedSubType, &parsedOnlyIndoor, &parsedOnlyElectric); err != nil {
		t.Fatalf("query parsed indoor-only activity: %v", err)
	}
	if parsedSport != "running" || parsedSubType != "treadmill" || parsedOnlyIndoor || parsedOnlyElectric {
		t.Fatalf("parsed-only treadmill rewrite = (%s,%s), indoor=%v electric=%v; want parsed tuple only", parsedSport, parsedSubType, parsedOnlyIndoor, parsedOnlyElectric)
	}
	if err := tx.QueryRow(ctx, `
		SELECT COALESCE(parsed_sport_type, ''), COALESCE(parsed_sub_type, ''), is_indoor, is_electric
		FROM activities WHERE id = $1
	`, parsedElectricOnly).Scan(&parsedSport, &parsedSubType, &parsedOnlyIndoor, &parsedOnlyElectric); err != nil {
		t.Fatalf("query parsed electric-only activity: %v", err)
	}
	if parsedSport != "cycling" || parsedSubType != "" || parsedOnlyIndoor || parsedOnlyElectric {
		t.Fatalf("parsed-only electric rewrite = (%s,%s), indoor=%v electric=%v; want parsed tuple only", parsedSport, parsedSubType, parsedOnlyIndoor, parsedOnlyElectric)
	}

	var routingSports []string
	if err := tx.QueryRow(ctx, `SELECT sport_types FROM routing_profiles WHERE user_id = $1 AND name = 'wheel profile'`, userID).Scan(&routingSports); err != nil {
		t.Fatalf("query routing sport types: %v", err)
	}
	if strings.Join(routingSports, ",") != "cycling,wheelchair_push" {
		t.Fatalf("routing sport_types = %v, want cycling,wheelchair_push", routingSports)
	}
	var routingPredicates string
	if err := tx.QueryRow(ctx, `SELECT predicates::text FROM routing_profiles WHERE user_id = $1 AND name = 'wheel profile'`, userID).Scan(&routingPredicates); err != nil {
		t.Fatalf("query routing predicates: %v", err)
	}
	for _, needle := range []string{`"sport": "cycling"`, `"sub_type": "mountain"`, `"sport": "wheelchair_push"`, `"sub_type": "walk_speed"`} {
		if !strings.Contains(routingPredicates, needle) {
			t.Fatalf("routing predicates missing %s: %s", needle, routingPredicates)
		}
	}

	var validatorOK, validatorOldOK bool
	if err := tx.QueryRow(ctx, `SELECT fn_validate_predicates('[{"sport":"wheelchair_push","sub_type":"walk_speed"}]'::jsonb)`).Scan(&validatorOK); err != nil {
		t.Fatalf("validate new predicate: %v", err)
	}
	if err := tx.QueryRow(ctx, `SELECT fn_validate_predicates('[{"sport":"wheelchair_push_walk"}]'::jsonb)`).Scan(&validatorOldOK); err != nil {
		t.Fatalf("validate old predicate: %v", err)
	}
	if !validatorOK || validatorOldOK {
		t.Fatalf("validator new=%v old=%v, want true/false", validatorOK, validatorOldOK)
	}

	var removedCount int
	if err := tx.QueryRow(ctx, `
		SELECT COUNT(*) FROM activities
		WHERE user_id = $1 AND sport_type IN ('wheelchair_push_walk', 'wheelchair_push_run', 'strength')
	`, userID).Scan(&removedCount); err != nil {
		t.Fatalf("query removed activities: %v", err)
	}
	if removedCount != 0 {
		t.Fatalf("removed activity sport values remain: %d", removedCount)
	}

	var invalidSubTypeCount int
	if err := tx.QueryRow(ctx, `
		SELECT COUNT(*) FROM activities
		WHERE user_id = $1
		  AND (
		    COALESCE(sub_type, '') = 'generic'
		    OR COALESCE(parsed_sub_type, '') = 'generic'
		    OR
		    (sport_type = 'hiking' AND COALESCE(sub_type, '') = 'trail')
		    OR (sport_type = 'cycling' AND COALESCE(sub_type, '') = 'commuting')
		    OR COALESCE(sub_type, '') LIKE 'subsportinvalid%'
		    OR (parsed_sport_type = 'hiking' AND COALESCE(parsed_sub_type, '') = 'trail')
		  )
	`, userID).Scan(&invalidSubTypeCount); err != nil {
		t.Fatalf("query invalid legacy subtypes: %v", err)
	}
	if invalidSubTypeCount != 0 {
		t.Fatalf("invalid legacy subtypes remain: %d", invalidSubTypeCount)
	}

	var automationJSON string
	if err := tx.QueryRow(ctx, `
		SELECT conditions::text || condition_groups::text || actions::text || branches::text
		FROM automation_rules WHERE user_id = $1 AND name = 'wheel rule'
	`, userID).Scan(&automationJSON); err != nil {
		t.Fatalf("query automation rule: %v", err)
	}
	for _, needle := range []string{`"value": "wheelchair_push"`, `"value": "walk_speed"`, `"value": "run_speed"`, `"value": ["running", "strength_training"]`} {
		if !strings.Contains(automationJSON, needle) {
			t.Fatalf("automation JSON missing %s: %s", needle, automationJSON)
		}
	}

	if _, err := tx.Exec(ctx, downSQL); err != nil {
		t.Fatalf("apply down.sql: %v", err)
	}
	var downOldOK, downNewOK, downTrainingPilatesOK bool
	if err := tx.QueryRow(ctx, `SELECT fn_validate_predicates('[{"sport":"wheelchair_push_walk"}]'::jsonb)`).Scan(&downOldOK); err != nil {
		t.Fatalf("validate old predicate after down: %v", err)
	}
	if err := tx.QueryRow(ctx, `SELECT fn_validate_predicates('[{"sport":"wheelchair_push"}]'::jsonb)`).Scan(&downNewOK); err != nil {
		t.Fatalf("validate new predicate after down: %v", err)
	}
	if err := tx.QueryRow(ctx, `SELECT fn_validate_predicates('[{"sport":"training","sub_type":"pilates"}]'::jsonb)`).Scan(&downTrainingPilatesOK); err != nil {
		t.Fatalf("validate training/pilates after down: %v", err)
	}
	if !downOldOK || downNewOK || downTrainingPilatesOK {
		t.Fatalf("down validator old=%v new=%v training/pilates=%v, want true/false/false", downOldOK, downNewOK, downTrainingPilatesOK)
	}
	var normalizeExists bool
	if err := tx.QueryRow(ctx, `SELECT to_regprocedure('fp_sport_taxonomy_normalize(text,text)') IS NOT NULL`).Scan(&normalizeExists); err != nil {
		t.Fatalf("query normalize function after down: %v", err)
	}
	if normalizeExists {
		t.Fatal("fp_sport_taxonomy_normalize still exists after down migration")
	}
}
