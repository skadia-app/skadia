package database

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/skadia-app/skadia/services/api/internal/database/testdb"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func TestChatUserPreferencesDefaultsAndUpsert(t *testing.T) {
	db, userID := chatPrefsTestDB(t)
	ctx := context.Background()

	defaults, err := db.GetChatPrefs(ctx, userID)
	if err != nil {
		t.Fatalf("GetChatPrefs defaults: %v", err)
	}
	if defaults.UserID != userID {
		t.Fatalf("default prefs user = %q, want %q", defaults.UserID, userID)
	}
	if defaults.ExposeDocumentExtractedData || defaults.ChatWriteToolsEnabled || len(defaults.ChatAllowedWriteTools) != 0 {
		t.Fatalf("unexpected defaults: %#v", defaults)
	}

	saved, err := db.UpsertChatPrefs(ctx, userID, ChatUserPreferences{
		ExposeDocumentExtractedData: true,
		ChatWriteToolsEnabled:       true,
		ChatAllowedWriteTools:       []string{"rename_activity", "set_activity_gear"},
	})
	if err != nil {
		t.Fatalf("UpsertChatPrefs: %v", err)
	}
	if !saved.ExposeDocumentExtractedData || !saved.ChatWriteToolsEnabled {
		t.Fatalf("saved booleans not set: %#v", saved)
	}
	if got := saved.ChatAllowedWriteTools; len(got) != 2 || got[0] != "rename_activity" || got[1] != "set_activity_gear" {
		t.Fatalf("saved allowed tools = %#v", got)
	}

	roundTrip, err := db.GetChatPrefs(ctx, userID)
	if err != nil {
		t.Fatalf("GetChatPrefs roundtrip: %v", err)
	}
	if !roundTrip.ExposeDocumentExtractedData || !roundTrip.ChatWriteToolsEnabled {
		t.Fatalf("roundtrip booleans not set: %#v", roundTrip)
	}
	if got := roundTrip.ChatAllowedWriteTools; len(got) != 2 || got[0] != "rename_activity" || got[1] != "set_activity_gear" {
		t.Fatalf("roundtrip allowed tools = %#v", got)
	}
}

func TestChatUserPreferencesRLS(t *testing.T) {
	db, userA := chatPrefsTestDB(t)
	_, userB := chatPrefsTestDB(t)
	ctx := context.Background()

	if _, err := db.UpsertChatPrefs(ctx, userA, ChatUserPreferences{
		ExposeDocumentExtractedData: true,
		ChatWriteToolsEnabled:       true,
		ChatAllowedWriteTools:       []string{"rename_activity"},
	}); err != nil {
		t.Fatalf("upsert userA prefs: %v", err)
	}
	if _, err := db.UpsertChatPrefs(ctx, userB, ChatUserPreferences{
		ChatAllowedWriteTools: []string{"set_activity_gear"},
	}); err != nil {
		t.Fatalf("upsert userB prefs: %v", err)
	}

	readonly := agentAuditReadonlyPool(t)
	if _, err := readonly.Exec(ctx, "SELECT set_config('app.current_user_id', $1, false)", userA); err != nil {
		t.Fatalf("set readonly RLS context: %v", err)
	}
	var count int
	if err := readonly.QueryRow(ctx, `SELECT count(*) FROM chat_user_preferences`).Scan(&count); err != nil {
		t.Fatalf("readonly RLS query userA: %v", err)
	}
	if count != 1 {
		t.Fatalf("readonly userA visible rows = %d, want 1", count)
	}

	if _, err := readonly.Exec(ctx, "SELECT set_config('app.current_user_id', $1, false)", userB); err != nil {
		t.Fatalf("set readonly RLS context userB: %v", err)
	}
	if err := readonly.QueryRow(ctx, `
		SELECT count(*)
		FROM chat_user_preferences
		WHERE chat_expose_document_extracted_data
	`).Scan(&count); err != nil {
		t.Fatalf("readonly RLS query userB: %v", err)
	}
	if count != 0 {
		t.Fatalf("readonly userB exposed rows = %d, want 0", count)
	}
}

func TestChatUserPreferencesDirectReadonlyWithoutGUCFails(t *testing.T) {
	db, userID := chatPrefsTestDB(t)
	ctx := context.Background()
	if _, err := db.UpsertChatPrefs(ctx, userID, ChatUserPreferences{
		ChatWriteToolsEnabled: true,
	}); err != nil {
		t.Fatalf("upsert prefs: %v", err)
	}

	readonly := agentAuditReadonlyPool(t)
	var count int
	err := readonly.QueryRow(ctx, `SELECT count(*) FROM chat_user_preferences`).Scan(&count)
	if err == nil && count != 0 {
		t.Fatalf("readonly without GUC visible rows = %d, want 0 or error", count)
	}
}

func TestChatUserPreferencesAllowedToolsMustBeJSONArray(t *testing.T) {
	db, userID := chatPrefsTestDB(t)
	ctx := context.Background()

	err := db.WithUserTx(ctx, userID, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `
			INSERT INTO chat_user_preferences (
				user_id, chat_allowed_write_tools
			)
			VALUES ($1, $2::jsonb)
		`, userID, json.RawMessage(`{"rename_activity":true}`))
		return err
	})
	if err == nil {
		t.Fatal("object chat_allowed_write_tools insert succeeded, want check constraint failure")
	}
}

func chatPrefsTestDB(t *testing.T) (*DB, string) {
	t.Helper()
	pool := testdb.Open(t)
	userID := uuid.NewString()
	_, err := pool.Exec(context.Background(),
		`INSERT INTO users (id, username, email, email_verified, registration_source)
		 VALUES ($1, $2, $3, true, 'test')`,
		userID, "chat-prefs-"+userID[:8], "chat-prefs-"+userID[:8]+"@example.com")
	if err != nil {
		t.Fatalf("insert user: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, userID)
	})
	return &DB{Pool: pool}, userID
}
