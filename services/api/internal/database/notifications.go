package database

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/skadia-app/skadia/services/api/internal/model"
)

// CreateNotification inserts a new notification for a user.
func (db *DB) CreateNotification(ctx context.Context, userID, category, title, body string) error {
	_, err := db.Pool.Exec(ctx,
		"INSERT INTO notifications (user_id, category, title, body) VALUES ($1, $2, $3, $4)",
		userID, category, title, body,
	)
	if err != nil {
		return fmt.Errorf("create notification: %w", err)
	}
	return nil
}

// ListNotifications returns up to limit recent notifications for a user, newest first.
func (db *DB) ListNotifications(ctx context.Context, userID string, limit int) ([]model.Notification, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := db.Pool.Query(ctx, `
		SELECT id, user_id, category, title, body, read, created_at
		FROM notifications
		WHERE user_id = $1
		ORDER BY created_at DESC
		LIMIT $2
	`, userID, limit)
	if err != nil {
		return nil, fmt.Errorf("list notifications: %w", err)
	}
	defer rows.Close()

	var results []model.Notification
	for rows.Next() {
		var n model.Notification
		if err := rows.Scan(&n.ID, &n.UserID, &n.Category, &n.Title, &n.Body, &n.Read, &n.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan notification: %w", err)
		}
		results = append(results, n)
	}
	return results, rows.Err()
}

// GetUnreadCount returns the number of unread notifications for a user.
func (db *DB) GetUnreadCount(ctx context.Context, userID string) (int, error) {
	var count int
	err := db.Pool.QueryRow(ctx,
		"SELECT COUNT(*) FROM notifications WHERE user_id = $1 AND NOT read",
		userID,
	).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("get unread count: %w", err)
	}
	return count, nil
}

// MarkRead marks a single notification as read, checking ownership.
func (db *DB) MarkRead(ctx context.Context, notifID, userID string) error {
	tag, err := db.Pool.Exec(ctx,
		"UPDATE notifications SET read = true WHERE id = $1 AND user_id = $2",
		notifID, userID,
	)
	if err != nil {
		return fmt.Errorf("mark read: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

// MarkAllRead marks all unread notifications as read for a user.
func (db *DB) MarkAllRead(ctx context.Context, userID string) error {
	_, err := db.Pool.Exec(ctx,
		"UPDATE notifications SET read = true WHERE user_id = $1 AND NOT read",
		userID,
	)
	if err != nil {
		return fmt.Errorf("mark all read: %w", err)
	}
	return nil
}

// DeleteNotification löscht eine einzelne Benachrichtigung (Ownership-Check via user_id).
func (db *DB) DeleteNotification(ctx context.Context, notifID, userID string) error {
	tag, err := db.Pool.Exec(ctx,
		"DELETE FROM notifications WHERE id = $1 AND user_id = $2",
		notifID, userID,
	)
	if err != nil {
		return fmt.Errorf("delete notification: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

// DeleteAllNotifications löscht alle Benachrichtigungen eines Users.
func (db *DB) DeleteAllNotifications(ctx context.Context, userID string) error {
	_, err := db.Pool.Exec(ctx,
		"DELETE FROM notifications WHERE user_id = $1",
		userID,
	)
	if err != nil {
		return fmt.Errorf("delete all notifications: %w", err)
	}
	return nil
}
