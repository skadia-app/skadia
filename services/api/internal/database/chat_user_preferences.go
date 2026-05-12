package database

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

type ChatUserPreferences struct {
	UserID                      string
	ExposeDocumentExtractedData bool
	ChatWriteToolsEnabled       bool
	ChatAllowedWriteTools       []string
	CreatedAt                   time.Time
	UpdatedAt                   time.Time
}

func DefaultChatUserPreferences(userID string) *ChatUserPreferences {
	return &ChatUserPreferences{
		UserID:                userID,
		ChatAllowedWriteTools: []string{},
	}
}

func (db *DB) GetChatPrefs(ctx context.Context, userID string) (*ChatUserPreferences, error) {
	var prefs *ChatUserPreferences
	err := db.WithUserTx(ctx, userID, func(tx pgx.Tx) error {
		row, err := scanChatUserPreferences(tx.QueryRow(ctx, `
			SELECT user_id::text, chat_expose_document_extracted_data,
			       chat_write_tools_enabled, chat_allowed_write_tools,
			       created_at, updated_at
			FROM chat_user_preferences
			WHERE user_id = $1
		`, userID))
		if err == pgx.ErrNoRows {
			prefs = DefaultChatUserPreferences(userID)
			return nil
		}
		if err != nil {
			return err
		}
		prefs = row
		return nil
	})
	if err != nil {
		return nil, err
	}
	return prefs, nil
}

func (db *DB) UpsertChatPrefs(ctx context.Context, userID string, prefs ChatUserPreferences) (*ChatUserPreferences, error) {
	if prefs.UserID != "" && prefs.UserID != userID {
		return nil, fmt.Errorf("chat preferences user mismatch")
	}
	if prefs.ChatAllowedWriteTools == nil {
		prefs.ChatAllowedWriteTools = []string{}
	}
	allowed, err := json.Marshal(prefs.ChatAllowedWriteTools)
	if err != nil {
		return nil, fmt.Errorf("marshal chat allowed write tools: %w", err)
	}

	var saved *ChatUserPreferences
	err = db.WithUserTx(ctx, userID, func(tx pgx.Tx) error {
		row, err := scanChatUserPreferences(tx.QueryRow(ctx, `
			INSERT INTO chat_user_preferences (
				user_id, chat_expose_document_extracted_data,
				chat_write_tools_enabled, chat_allowed_write_tools
			)
			VALUES ($1, $2, $3, $4::jsonb)
			ON CONFLICT (user_id) DO UPDATE
			SET chat_expose_document_extracted_data = EXCLUDED.chat_expose_document_extracted_data,
			    chat_write_tools_enabled = EXCLUDED.chat_write_tools_enabled,
			    chat_allowed_write_tools = EXCLUDED.chat_allowed_write_tools,
			    updated_at = now()
			RETURNING user_id::text, chat_expose_document_extracted_data,
			          chat_write_tools_enabled, chat_allowed_write_tools,
			          created_at, updated_at
		`, userID, prefs.ExposeDocumentExtractedData, prefs.ChatWriteToolsEnabled, allowed))
		if err != nil {
			return err
		}
		saved = row
		return nil
	})
	if err != nil {
		return nil, err
	}
	return saved, nil
}

func scanChatUserPreferences(row pgx.Row) (*ChatUserPreferences, error) {
	var prefs ChatUserPreferences
	var allowed []byte
	if err := row.Scan(
		&prefs.UserID,
		&prefs.ExposeDocumentExtractedData,
		&prefs.ChatWriteToolsEnabled,
		&allowed,
		&prefs.CreatedAt,
		&prefs.UpdatedAt,
	); err != nil {
		return nil, err
	}
	if len(allowed) == 0 {
		prefs.ChatAllowedWriteTools = []string{}
		return &prefs, nil
	}
	if err := json.Unmarshal(allowed, &prefs.ChatAllowedWriteTools); err != nil {
		return nil, fmt.Errorf("unmarshal chat allowed write tools: %w", err)
	}
	if prefs.ChatAllowedWriteTools == nil {
		prefs.ChatAllowedWriteTools = []string{}
	}
	return &prefs, nil
}
