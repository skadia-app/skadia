package migrations_test

import (
	"context"
	"os"
	"strings"
	"testing"
)

func TestMigration_AgentAuditLogSQLDefinesRLS(t *testing.T) {
	body, err := os.ReadFile("900001_agent_audit_log.up.sql")
	if err != nil {
		t.Fatalf("read agent audit migration: %v", err)
	}
	sql := string(body)
	for _, want := range []string{
		"CREATE TABLE agent_audit_log",
		"FORCE ROW LEVEL SECURITY",
		"current_setting('app.current_user_id', true)",
		"agent_audit_unattributed_update",
		"UNIQUE (user_id, session_id, tool_call_id)",
	} {
		if !strings.Contains(sql, want) {
			t.Fatalf("migration SQL missing %q", want)
		}
	}
}

func TestMigration_AgentAuditLogSchema(t *testing.T) {
	pool := migrationTestPool(t)
	ctx := context.Background()

	var forceRLS bool
	if err := pool.QueryRow(ctx, `
		SELECT relforcerowsecurity
		FROM pg_class
		WHERE relname = 'agent_audit_log'
		  AND relnamespace = 'public'::regnamespace
	`).Scan(&forceRLS); err != nil {
		t.Fatalf("query relforcerowsecurity: %v", err)
	}
	if !forceRLS {
		t.Fatal("agent_audit_log must FORCE ROW LEVEL SECURITY")
	}

	var policyCount int
	if err := pool.QueryRow(ctx, `
		SELECT count(*)
		FROM pg_policies
		WHERE tablename = 'agent_audit_log'
		  AND policyname IN ('agent_audit_select_readonly', 'agent_audit_owner_rw', 'agent_audit_unattributed_update')
	`).Scan(&policyCount); err != nil {
		t.Fatalf("query policies: %v", err)
	}
	if policyCount != 3 {
		t.Fatalf("policy count = %d, want 3", policyCount)
	}

	var constraintExists bool
	if err := pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM pg_constraint c
			JOIN pg_class t ON t.oid = c.conrelid
			WHERE t.relname = 'agent_audit_log'
			  AND c.conname = 'agent_audit_uniq'
		)
	`).Scan(&constraintExists); err != nil {
		t.Fatalf("query constraint: %v", err)
	}
	if !constraintExists {
		t.Fatal("agent_audit_uniq constraint missing")
	}
}
