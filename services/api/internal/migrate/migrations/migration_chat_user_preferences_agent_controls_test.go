package migrations_test

import (
	"context"
	"os"
	"strings"
	"testing"
)

func TestMigration_ChatUserPreferencesAgentControlsSQLDefinesRLS(t *testing.T) {
	body, err := os.ReadFile("900002_chat_user_preferences_agent_controls.up.sql")
	if err != nil {
		t.Fatalf("read chat user preferences migration: %v", err)
	}
	sql := string(body)
	for _, want := range []string{
		"CREATE TABLE chat_user_preferences",
		"chat_expose_document_extracted_data BOOLEAN NOT NULL DEFAULT false",
		"chat_write_tools_enabled BOOLEAN NOT NULL DEFAULT false",
		"chat_allowed_write_tools JSONB NOT NULL DEFAULT '[]'::jsonb",
		"CHECK (jsonb_typeof(chat_allowed_write_tools) = 'array')",
		"FORCE ROW LEVEL SECURITY",
		"GRANT SELECT ON chat_user_preferences TO skadia_readonly",
		"current_setting('app.current_user_id', true)",
	} {
		if !strings.Contains(sql, want) {
			t.Fatalf("migration SQL missing %q", want)
		}
	}
}

func TestMigration_ChatUserPreferencesAgentControlsSchema(t *testing.T) {
	pool := migrationTestPool(t)
	ctx := context.Background()

	var forceRLS bool
	if err := pool.QueryRow(ctx, `
		SELECT relforcerowsecurity
		FROM pg_class
		WHERE relname = 'chat_user_preferences'
		  AND relnamespace = 'public'::regnamespace
	`).Scan(&forceRLS); err != nil {
		t.Fatalf("query relforcerowsecurity: %v", err)
	}
	if !forceRLS {
		t.Fatal("chat_user_preferences must FORCE ROW LEVEL SECURITY")
	}

	var policyCount int
	if err := pool.QueryRow(ctx, `
		SELECT count(*)
		FROM pg_policies
		WHERE tablename = 'chat_user_preferences'
		  AND policyname IN ('cup_read', 'cup_rw')
	`).Scan(&policyCount); err != nil {
		t.Fatalf("query policies: %v", err)
	}
	if policyCount != 2 {
		t.Fatalf("policy count = %d, want 2", policyCount)
	}

	var checkExists bool
	if err := pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM pg_constraint c
			JOIN pg_class t ON t.oid = c.conrelid
			WHERE t.relname = 'chat_user_preferences'
			  AND c.conname = 'chat_user_preferences_allowed_write_tools_array'
		)
	`).Scan(&checkExists); err != nil {
		t.Fatalf("query check constraint: %v", err)
	}
	if !checkExists {
		t.Fatal("chat_user_preferences_allowed_write_tools_array constraint missing")
	}
}
