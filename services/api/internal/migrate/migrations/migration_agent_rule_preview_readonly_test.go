package migrations_test

import (
	"context"
	"os"
	"strings"
	"testing"
)

func TestMigration_AgentRulePreviewReadonlySQLDefinesRLS(t *testing.T) {
	body, err := os.ReadFile("000364_agent_rule_preview_readonly.up.sql")
	if err != nil {
		t.Fatalf("read agent rule preview migration: %v", err)
	}
	sql := string(body)
	for _, want := range []string{
		"ALTER TABLE automation_rules ENABLE ROW LEVEL SECURITY",
		"ALTER TABLE automation_rules FORCE ROW LEVEL SECURITY",
		"GRANT SELECT ON automation_rules TO skadia_readonly",
		"CREATE POLICY automation_rules_readonly_select",
		"CREATE POLICY automation_rules_owner_all",
		"current_setting('app.current_user_id', true)",
	} {
		if !strings.Contains(sql, want) {
			t.Fatalf("migration SQL missing %q", want)
		}
	}
}

func TestMigration_AgentRulePreviewReadonlySchema(t *testing.T) {
	pool := migrationTestPool(t)
	ctx := context.Background()

	var rowSecurity, forceRLS bool
	if err := pool.QueryRow(ctx, `
		SELECT relrowsecurity, relforcerowsecurity
		FROM pg_class
		WHERE relname = 'automation_rules'
		  AND relnamespace = 'public'::regnamespace
	`).Scan(&rowSecurity, &forceRLS); err != nil {
		t.Fatalf("query automation_rules RLS flags: %v", err)
	}
	if !rowSecurity || !forceRLS {
		t.Fatalf("automation_rules RLS flags row=%v force=%v, want both true", rowSecurity, forceRLS)
	}

	var policyCount int
	if err := pool.QueryRow(ctx, `
		SELECT count(*)
		FROM pg_policies
		WHERE tablename = 'automation_rules'
		  AND policyname IN ('automation_rules_readonly_select', 'automation_rules_owner_all')
	`).Scan(&policyCount); err != nil {
		t.Fatalf("query policies: %v", err)
	}
	if policyCount != 2 {
		t.Fatalf("policy count = %d, want 2", policyCount)
	}
}
