package database

import (
	"context"
	"fmt"

	"github.com/skadia-app/skadia/services/api/internal/model"
)

// CreateSession creates a new chat session for the given user.
func (db *DB) CreateSession(ctx context.Context, userID string) (*model.ChatSession, error) {
	var s model.ChatSession
	err := db.Pool.QueryRow(ctx,
		`INSERT INTO chat_sessions (user_id) VALUES ($1)
		 RETURNING id, user_id, title, created_at, updated_at`,
		userID,
	).Scan(&s.ID, &s.UserID, &s.Title, &s.CreatedAt, &s.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("create chat session: %w", err)
	}
	return &s, nil
}

// GetSession returns a chat session by ID, verifying ownership.
func (db *DB) GetSession(ctx context.Context, sessionID, userID string) (*model.ChatSession, error) {
	var s model.ChatSession
	err := db.Pool.QueryRow(ctx,
		`SELECT id, user_id, title, created_at, updated_at
		 FROM chat_sessions WHERE id = $1 AND user_id = $2`,
		sessionID, userID,
	).Scan(&s.ID, &s.UserID, &s.Title, &s.CreatedAt, &s.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("get chat session: %w", err)
	}
	return &s, nil
}

// ListSessions returns the 50 most recently updated sessions for a user.
func (db *DB) ListSessions(ctx context.Context, userID string) ([]*model.ChatSession, error) {
	rows, err := db.Pool.Query(ctx,
		`SELECT id, user_id, title, created_at, updated_at
		 FROM chat_sessions WHERE user_id = $1
		 ORDER BY updated_at DESC LIMIT 50`,
		userID,
	)
	if err != nil {
		return nil, fmt.Errorf("list chat sessions: %w", err)
	}
	defer rows.Close()

	var sessions []*model.ChatSession
	for rows.Next() {
		var s model.ChatSession
		if err := rows.Scan(&s.ID, &s.UserID, &s.Title, &s.CreatedAt, &s.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan chat session: %w", err)
		}
		sessions = append(sessions, &s)
	}
	return sessions, rows.Err()
}

// DeleteSession deletes a chat session by ID, verifying ownership.
func (db *DB) DeleteSession(ctx context.Context, sessionID, userID string) error {
	_, err := db.Pool.Exec(ctx,
		`DELETE FROM chat_sessions WHERE id = $1 AND user_id = $2`,
		sessionID, userID,
	)
	if err != nil {
		return fmt.Errorf("delete chat session: %w", err)
	}
	return nil
}

// CreateMessage persists a chat message and returns the stored record.
// It also touches the parent session's updated_at timestamp.
func (db *DB) CreateMessage(ctx context.Context, msg *model.ChatMessage) (*model.ChatMessage, error) {
	var m model.ChatMessage
	err := db.Pool.QueryRow(ctx,
		`INSERT INTO chat_messages (session_id, role, content, tool_calls, sources)
		 VALUES ($1, $2, $3, $4, $5)
		 RETURNING id, session_id, role, content, tool_calls, sources, created_at`,
		msg.SessionID, msg.Role, msg.Content, msg.ToolCalls, msg.Sources,
	).Scan(&m.ID, &m.SessionID, &m.Role, &m.Content, &m.ToolCalls, &m.Sources, &m.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("create chat message: %w", err)
	}
	// Best-effort touch on parent session
	_, _ = db.Pool.Exec(ctx,
		`UPDATE chat_sessions SET updated_at = NOW() WHERE id = $1`,
		msg.SessionID,
	)
	return &m, nil
}

// GetMessages returns up to limit messages for a session, ordered oldest-first.
func (db *DB) GetMessages(ctx context.Context, sessionID string, limit int) ([]*model.ChatMessage, error) {
	rows, err := db.Pool.Query(ctx,
		`SELECT id, session_id, role, content, tool_calls, sources, created_at
		 FROM chat_messages WHERE session_id = $1
		 ORDER BY created_at ASC LIMIT $2`,
		sessionID, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("get chat messages: %w", err)
	}
	defer rows.Close()

	var messages []*model.ChatMessage
	for rows.Next() {
		var m model.ChatMessage
		if err := rows.Scan(
			&m.ID, &m.SessionID, &m.Role, &m.Content,
			&m.ToolCalls, &m.Sources, &m.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan chat message: %w", err)
		}
		messages = append(messages, &m)
	}
	return messages, rows.Err()
}

// UpdateSessionTitle sets the title of a chat session.
func (db *DB) UpdateSessionTitle(ctx context.Context, sessionID, title string) error {
	_, err := db.Pool.Exec(ctx,
		`UPDATE chat_sessions SET title = $2, updated_at = NOW() WHERE id = $1`,
		sessionID, title,
	)
	if err != nil {
		return fmt.Errorf("update session title: %w", err)
	}
	return nil
}
