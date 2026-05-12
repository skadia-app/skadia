package migrations_test

import (
	"context"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
)

func TestMigration_LLMRoleCapabilities_ColumnsAndRoles(t *testing.T) {
	pool := migrationTestPool(t)
	ctx := context.Background()

	columns := map[string]struct {
		dataType string
		nullable string
		def      string
	}{
		"capability_flags": {"ARRAY", "NO", "'{}'::text[]"},
		"eval_status":      {"text", "NO", "'untested'::text"},
		"eval_report":      {"jsonb", "NO", "'{}'::jsonb"},
	}

	for column, want := range columns {
		t.Run(column, func(t *testing.T) {
			var dataType, isNullable string
			var columnDefault *string
			if err := pool.QueryRow(ctx, `
				SELECT data_type, is_nullable, column_default
				FROM information_schema.columns
				WHERE table_name = 'llm_system_config' AND column_name = $1
			`, column).Scan(&dataType, &isNullable, &columnDefault); err != nil {
				t.Fatalf("query column metadata: %v", err)
			}
			if dataType != want.dataType {
				t.Errorf("%s data_type = %q, want %q", column, dataType, want.dataType)
			}
			if isNullable != want.nullable {
				t.Errorf("%s is_nullable = %q, want %q", column, isNullable, want.nullable)
			}
			if columnDefault == nil || !strings.Contains(*columnDefault, want.def) {
				t.Errorf("%s default = %v, want containing %q", column, columnDefault, want.def)
			}
		})
	}

	rows, err := pool.Query(ctx, `
		SELECT role, eval_status
		FROM llm_system_config
		WHERE role = ANY($1)
		ORDER BY role
	`, []string{"tool_agent", "sql_readonly", "write_preview", "document_extract", "markdown_extract", "vision"})
	if err != nil {
		t.Fatalf("query roles: %v", err)
	}
	defer rows.Close()

	var got []string
	for rows.Next() {
		var role, status string
		if err := rows.Scan(&role, &status); err != nil {
			t.Fatalf("scan role: %v", err)
		}
		if !slices.Contains([]string{"untested", "passed", "failed", "waived"}, status) {
			t.Errorf("%s eval_status = %q, want valid eval status", role, status)
		}
		got = append(got, role)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("roles rows: %v", err)
	}
	wantRoles := []string{"document_extract", "markdown_extract", "sql_readonly", "tool_agent", "vision", "write_preview"}
	if !slices.Equal(got, wantRoles) {
		t.Fatalf("roles = %#v, want %#v", got, wantRoles)
	}

	if _, err := pool.Exec(ctx, `
		INSERT INTO llm_system_config (role, backend_type, eval_status)
		VALUES ('invalid_status_probe', 'disabled', 'unknown')
	`); err == nil {
		t.Fatal("insert with invalid eval_status succeeded, want check violation")
	}
}

func TestMigration_LLMRoleCapabilities_LegacyBackfillAndSeedDisable(t *testing.T) {
	pool := migrationTestPool(t)
	ctx := context.Background()

	upSQL, err := os.ReadFile("000367_llm_role_capabilities.up.sql")
	if err != nil {
		t.Fatalf("read up migration: %v", err)
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `DELETE FROM llm_system_config`); err != nil {
		t.Fatalf("clear table: %v", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO llm_system_config (role, backend_type, api_base, model_name, api_key_enc, enabled)
		VALUES
			('default', 'ollama', 'http://custom.example', 'operator-model', NULL, true),
			('vlm', 'ollama', 'http://ollama:11434', 'custom-vlm', 'encrypted-key', true),
			('vlm_fallback', 'ollama', 'http://ollama:11434', 'qwen3-vl:4b', NULL, true),
			('analysis', 'disabled', NULL, NULL, NULL, false)
	`); err != nil {
		t.Fatalf("insert legacy rows: %v", err)
	}

	if _, err := tx.Exec(ctx, string(upSQL)); err != nil {
		t.Fatalf("reapply migration: %v", err)
	}

	assertLLMRoleRow(t, ctx, tx, "tool_agent", "ollama", "http://custom.example", "operator-model", true)
	assertLLMRoleRow(t, ctx, tx, "document_extract", "ollama", "http://ollama:11434", "custom-vlm", true)
	assertLLMRoleRow(t, ctx, tx, "vision", "ollama", "http://ollama:11434", "custom-vlm", true)

	var enabled bool
	if err := tx.QueryRow(ctx, `SELECT enabled FROM llm_system_config WHERE role = 'vlm_fallback'`).Scan(&enabled); err != nil {
		t.Fatalf("query vlm_fallback: %v", err)
	}
	if enabled {
		t.Fatal("vlm_fallback old seed enabled = true, want disabled")
	}
}

type llmRoleQuerier interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

func assertLLMRoleRow(t *testing.T, ctx context.Context, q llmRoleQuerier, role, backend, apiBase, modelName string, enabled bool) {
	t.Helper()
	var gotBackend string
	var gotAPIBase, gotModelName *string
	var gotEnabled bool
	if err := q.QueryRow(ctx, `
		SELECT backend_type, api_base, model_name, enabled
		FROM llm_system_config
		WHERE role = $1
	`, role).Scan(&gotBackend, &gotAPIBase, &gotModelName, &gotEnabled); err != nil {
		t.Fatalf("query %s: %v", role, err)
	}
	if gotBackend != backend || gotAPIBase == nil || *gotAPIBase != apiBase || gotModelName == nil || *gotModelName != modelName || gotEnabled != enabled {
		t.Fatalf("%s = backend=%q api_base=%v model=%v enabled=%v, want backend=%q api_base=%q model=%q enabled=%v",
			role, gotBackend, gotAPIBase, gotModelName, gotEnabled, backend, apiBase, modelName, enabled)
	}
}
