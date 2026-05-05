package migrations_test

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func readMigration000349GearDefaultPredicatesSQL(t *testing.T) (upSQL, downSQL string) {
	t.Helper()
	upBytes, err := os.ReadFile("000349_gear_default_predicates.up.sql")
	if err != nil {
		t.Fatalf("read 000349 up.sql: %v", err)
	}
	downBytes, err := os.ReadFile("000349_gear_default_predicates.down.sql")
	if err != nil {
		t.Fatalf("read 000349 down.sql: %v", err)
	}
	return string(upBytes), string(downBytes)
}

func TestMigration000349GearDefaultPredicatesDeclaresRequiredContracts(t *testing.T) {
	upSQL, downSQL := readMigration000349GearDefaultPredicatesSQL(t)
	for _, needle := range []string{
		"ADD COLUMN IF NOT EXISTS default_sport_predicates JSONB NOT NULL DEFAULT '[]'::jsonb",
		"jsonb_agg(predicate ORDER BY ord)",
		"e_mountain_biking",
		"e_biking",
		"'is_electric', true",
		"mountain_biking",
		"WHERE predicate IS NOT NULL",
		"WITH ORDINALITY",
		"WHERE default_sport_predicates = '[]'::jsonb",
	} {
		if !strings.Contains(upSQL, needle) {
			t.Fatalf("up migration missing %q", needle)
		}
	}
	if !strings.Contains(downSQL, "DROP COLUMN IF EXISTS default_sport_predicates") {
		t.Fatal("down migration must drop default_sport_predicates")
	}
}

func TestMigration000349GearDefaultPredicatesBackfillsLegacyDefaults(t *testing.T) {
	upSQL, downSQL := readMigration000349GearDefaultPredicatesSQL(t)
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

	if _, err := tx.Exec(ctx, `ALTER TABLE gear DROP COLUMN IF EXISTS default_sport_predicates`); err != nil {
		t.Fatalf("prepare missing column: %v", err)
	}
	userID := uuid.NewString()
	if _, err := tx.Exec(ctx, `
		INSERT INTO users (id, username, email, email_verified, registration_source, is_admin)
		VALUES ($1, $2, $3, true, 'test', false)
	`, userID, "mig349_"+userID[:8], "mig349_"+userID[:8]+"@example.com"); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO gear (user_id, name, type, default_for_sports)
		VALUES
			($1, 'Road', 'bike', ARRAY['cycling', 'running']),
			($1, 'Legacy MTB', 'bike', ARRAY['mountain_biking', 'e_mountain_biking', 'e_biking']),
			($1, 'Invalid Legacy', 'other', ARRAY['not_a_sport'])
	`, userID); err != nil {
		t.Fatalf("insert gear: %v", err)
	}

	if _, err := tx.Exec(ctx, upSQL); err != nil {
		t.Fatalf("apply up.sql: %v", err)
	}
	var predicates string
	if err := tx.QueryRow(ctx, `
		SELECT default_sport_predicates::text
		FROM gear
		WHERE user_id = $1 AND name = 'Road'
	`, userID).Scan(&predicates); err != nil {
		t.Fatalf("select predicates: %v", err)
	}
	for _, want := range []string{`"sport_type": "cycling"`, `"sport_type": "running"`} {
		if !strings.Contains(predicates, want) {
			t.Fatalf("predicates = %s, missing %s", predicates, want)
		}
	}
	if err := tx.QueryRow(ctx, `
		SELECT default_sport_predicates::text
		FROM gear
		WHERE user_id = $1 AND name = 'Legacy MTB'
	`, userID).Scan(&predicates); err != nil {
		t.Fatalf("select legacy mtb predicates: %v", err)
	}
	for _, want := range []string{`"sport_type": "cycling"`, `"sub_type": "mountain"`, `"is_electric": true`} {
		if !strings.Contains(predicates, want) {
			t.Fatalf("legacy predicates = %s, missing %s", predicates, want)
		}
	}
	if err := tx.QueryRow(ctx, `
		SELECT default_sport_predicates::text
		FROM gear
		WHERE user_id = $1 AND name = 'Invalid Legacy'
	`, userID).Scan(&predicates); err != nil {
		t.Fatalf("select invalid legacy predicates: %v", err)
	}
	if predicates != "[]" {
		t.Fatalf("invalid legacy predicates = %s, want []", predicates)
	}
	if _, err := tx.Exec(ctx, downSQL); err != nil {
		t.Fatalf("apply down.sql: %v", err)
	}
}
