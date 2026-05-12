package migrations_test

import (
	"context"
	"os"
	"testing"

	"github.com/google/uuid"

	"github.com/skadia-app/skadia/services/api/internal/database/testdb"
)

func TestMigration_AutoTitleDisplayPolicy_AddsDefaultAndCheck(t *testing.T) {
	pool := testdb.Open(t)
	ctx := context.Background()

	sqlBytes, err := os.ReadFile("000361_auto_title_display_policy.up.sql")
	if err != nil {
		t.Fatalf("read migration: %v", err)
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `ALTER TABLE user_enrichment_settings DROP COLUMN IF EXISTS auto_title_display_policy`); err != nil {
		t.Fatalf("drop pre-existing column: %v", err)
	}
	if _, err := tx.Exec(ctx, string(sqlBytes)); err != nil {
		t.Fatalf("run migration: %v", err)
	}

	userID := uuid.NewString()
	if _, err := tx.Exec(ctx, `
		INSERT INTO users (id, username, email, email_verified, registration_source)
		VALUES ($1, $2, $3, true, 'test')`,
		userID, "migration-title-policy-"+userID[:8], "migration-title-policy-"+userID[:8]+"@example.com"); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO user_enrichment_settings (user_id) VALUES ($1)`, userID); err != nil {
		t.Fatalf("insert enrichment settings: %v", err)
	}

	var policy string
	if err := tx.QueryRow(ctx,
		`SELECT auto_title_display_policy FROM user_enrichment_settings WHERE user_id = $1`,
		userID,
	).Scan(&policy); err != nil {
		t.Fatalf("select default policy: %v", err)
	}
	if policy != "source_unless_generic" {
		t.Fatalf("auto_title_display_policy = %q, want source_unless_generic", policy)
	}

	if _, err := tx.Exec(ctx,
		`UPDATE user_enrichment_settings SET auto_title_display_policy = 'source_first' WHERE user_id = $1`,
		userID,
	); err != nil {
		t.Fatalf("valid policy rejected: %v", err)
	}
	if _, err := tx.Exec(ctx,
		`UPDATE user_enrichment_settings SET auto_title_display_policy = 'always_generated' WHERE user_id = $1`,
		userID,
	); err == nil {
		t.Fatal("invalid policy accepted, want CHECK violation")
	}
}
