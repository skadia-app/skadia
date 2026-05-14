package migrations_test

import (
	"context"
	"testing"
)

func TestMigration_LLMContextWindowColumn(t *testing.T) {
	pool := migrationTestPool(t)
	ctx := context.Background()

	var dataType, nullable string
	if err := pool.QueryRow(ctx, `
		SELECT data_type, is_nullable
		FROM information_schema.columns
		WHERE table_name = 'llm_system_config' AND column_name = 'context_window'
	`).Scan(&dataType, &nullable); err != nil {
		t.Fatalf("query context_window column: %v", err)
	}
	if dataType != "integer" {
		t.Fatalf("data_type = %q, want integer", dataType)
	}
	if nullable != "YES" {
		t.Fatalf("is_nullable = %q, want YES", nullable)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO llm_system_config (role, backend_type, context_window)
		VALUES ('context_window_low_probe_' || gen_random_uuid()::text, 'disabled', 512)
	`); err == nil {
		t.Fatal("insert with too-small context_window succeeded, want check violation")
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO llm_system_config (role, backend_type, context_window)
		VALUES ('context_window_min_probe_' || gen_random_uuid()::text, 'disabled', 1024)
	`); err != nil {
		t.Fatalf("insert with min context_window failed: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO llm_system_config (role, backend_type, context_window)
		VALUES ('context_window_max_probe_' || gen_random_uuid()::text, 'disabled', 2000000)
	`); err != nil {
		t.Fatalf("insert with max context_window failed: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO llm_system_config (role, backend_type, context_window)
		VALUES ('context_window_high_probe_' || gen_random_uuid()::text, 'disabled', 2000001)
	`); err == nil {
		t.Fatal("insert with too-large context_window succeeded, want check violation")
	}
}
