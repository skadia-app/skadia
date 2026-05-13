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
	`, []string{"tool_agent", "document_extract", "markdown_extract", "vision"})
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
	wantRoles := []string{"document_extract", "markdown_extract", "tool_agent", "vision"}
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

func TestMigration_LLMRoleCapabilities_LegacyDefaultBackfillAndSeedDisable(t *testing.T) {
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
	assertLLMRoleRow(t, ctx, tx, "document_extract", "disabled", "", "", false)
	assertLLMRoleRow(t, ctx, tx, "vision", "disabled", "", "", false)

	var enabled bool
	if err := tx.QueryRow(ctx, `SELECT enabled FROM llm_system_config WHERE role = 'vlm_fallback'`).Scan(&enabled); err != nil {
		t.Fatalf("query vlm_fallback: %v", err)
	}
	if !enabled {
		t.Fatal("vlm_fallback was modified by 000367, want stale legacy VLM rows left untouched")
	}
}

func TestMigration_LLMRoleCapabilities_DownRestoresDefaultSeedOnly(t *testing.T) {
	pool := migrationTestPool(t)
	ctx := context.Background()

	upSQL, err := os.ReadFile("000367_llm_role_capabilities.up.sql")
	if err != nil {
		t.Fatalf("read up migration: %v", err)
	}
	downSQL, err := os.ReadFile("000367_llm_role_capabilities.down.sql")
	if err != nil {
		t.Fatalf("read down migration: %v", err)
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
			('default', 'ollama', 'http://ollama:11434', 'qwen3:8b', NULL, true),
			('vlm', 'ollama', 'http://ollama:11434', 'qwen3-vl:8b', NULL, true),
			('vlm_fallback', 'ollama', 'http://ollama:11434', 'qwen3-vl:4b', NULL, true)
	`); err != nil {
		t.Fatalf("insert legacy seed rows: %v", err)
	}

	if _, err := tx.Exec(ctx, string(upSQL)); err != nil {
		t.Fatalf("apply up migration: %v", err)
	}
	if _, err := tx.Exec(ctx, string(downSQL)); err != nil {
		t.Fatalf("apply down migration: %v", err)
	}

	rows, err := tx.Query(ctx, `
		SELECT role, enabled
		FROM llm_system_config
		WHERE role IN ('default', 'vlm', 'vlm_fallback')
		ORDER BY role
	`)
	if err != nil {
		t.Fatalf("query legacy seed roles: %v", err)
	}
	defer rows.Close()

	got := map[string]bool{}
	for rows.Next() {
		var role string
		var enabled bool
		if err := rows.Scan(&role, &enabled); err != nil {
			t.Fatalf("scan legacy seed role: %v", err)
		}
		got[role] = enabled
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("legacy seed role rows: %v", err)
	}
	for _, role := range []string{"default", "vlm", "vlm_fallback"} {
		if role == "default" && !got[role] {
			t.Fatalf("%s enabled after down = false, want true", role)
		}
		if role != "default" && got[role] != true {
			t.Fatalf("%s was modified by 000367 down = false, want untouched true", role)
		}
	}
}

func TestMigration_LLMRoleCapabilities_DownKeepsOperatorModifiedRoleRows(t *testing.T) {
	pool := migrationTestPool(t)
	ctx := context.Background()

	downSQL, err := os.ReadFile("000367_llm_role_capabilities.down.sql")
	if err != nil {
		t.Fatalf("read down migration: %v", err)
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
		INSERT INTO llm_system_config (
			role, backend_type, api_base, model_name, api_key_enc, enabled,
			capability_flags, eval_status, eval_report
		)
		VALUES
			('document_extract', 'openai', 'https://llm.example.invalid/v1', 'operator-doc', NULL, true,
			 ARRAY['document_extract']::text[], 'passed', '{"operator_modified": true}'::jsonb),
			('markdown_extract', 'disabled', NULL, NULL, NULL, false,
			 ARRAY['operator_custom']::text[], 'untested', '{}'::jsonb),
			('vision', 'disabled', NULL, NULL, NULL, false,
			 ARRAY['vision']::text[], 'untested', '{}'::jsonb)
	`); err != nil {
		t.Fatalf("insert role rows: %v", err)
	}

	if _, err := tx.Exec(ctx, string(downSQL)); err != nil {
		t.Fatalf("apply down migration: %v", err)
	}

	var backend, model string
	if err := tx.QueryRow(ctx, `
		SELECT backend_type, model_name
		FROM llm_system_config
		WHERE role = 'document_extract'
	`).Scan(&backend, &model); err != nil {
		t.Fatalf("operator-modified document_extract row was deleted: %v", err)
	}
	if backend != "openai" || model != "operator-doc" {
		t.Fatalf("document_extract = backend=%q model=%q, want operator values", backend, model)
	}
	var modifiedSeedCount int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM llm_system_config WHERE role = 'markdown_extract'`).Scan(&modifiedSeedCount); err != nil {
		t.Fatalf("query modified seed row count: %v", err)
	}
	if modifiedSeedCount != 1 {
		t.Fatalf("operator-modified markdown_extract rows = %d, want 1", modifiedSeedCount)
	}

	var seedCount int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM llm_system_config WHERE role = 'vision'`).Scan(&seedCount); err != nil {
		t.Fatalf("query seed row count: %v", err)
	}
	if seedCount != 0 {
		t.Fatalf("pristine vision seed rows = %d, want 0", seedCount)
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
	gotAPIBaseValue := ""
	if gotAPIBase != nil {
		gotAPIBaseValue = *gotAPIBase
	}
	gotModelNameValue := ""
	if gotModelName != nil {
		gotModelNameValue = *gotModelName
	}
	if gotBackend != backend || gotAPIBaseValue != apiBase || gotModelNameValue != modelName || gotEnabled != enabled {
		t.Fatalf("%s = backend=%q api_base=%v model=%v enabled=%v, want backend=%q api_base=%q model=%q enabled=%v",
			role, gotBackend, gotAPIBase, gotModelName, gotEnabled, backend, apiBase, modelName, enabled)
	}
}
