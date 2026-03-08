package database

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/skadia-app/skadia/services/api/internal/model"
)

// RegisterDevice upserts a device token for a user.
// On conflict (same token) it updates user_id, device_name and last_used_at.
func (db *DB) RegisterDevice(ctx context.Context, userID, token, platform, deviceName string) error {
	_, err := db.Pool.Exec(ctx, `
		INSERT INTO device_tokens (user_id, token, platform, device_name, last_used_at)
		VALUES ($1, $2, $3, $4, NOW())
		ON CONFLICT (token) DO UPDATE
		SET user_id      = EXCLUDED.user_id,
		    device_name  = EXCLUDED.device_name,
		    last_used_at = NOW()
	`, userID, token, platform, deviceName)
	if err != nil {
		return fmt.Errorf("register device: %w", err)
	}
	return nil
}

// ListDevices returns all device tokens registered by a user.
func (db *DB) ListDevices(ctx context.Context, userID string) ([]model.DeviceToken, error) {
	rows, err := db.Pool.Query(ctx, `
		SELECT id, user_id, token, platform, device_name, created_at, last_used_at
		FROM device_tokens
		WHERE user_id = $1
		ORDER BY created_at DESC
	`, userID)
	if err != nil {
		return nil, fmt.Errorf("list devices: %w", err)
	}
	defer rows.Close()

	var results []model.DeviceToken
	for rows.Next() {
		var d model.DeviceToken
		if err := rows.Scan(
			&d.ID, &d.UserID, &d.Token, &d.Platform, &d.DeviceName,
			&d.CreatedAt, &d.LastUsedAt,
		); err != nil {
			return nil, fmt.Errorf("scan device token: %w", err)
		}
		results = append(results, d)
	}
	return results, rows.Err()
}

// DeleteDevice removes a device token that belongs to the given user.
func (db *DB) DeleteDevice(ctx context.Context, deviceID, userID string) error {
	tag, err := db.Pool.Exec(ctx,
		"DELETE FROM device_tokens WHERE id = $1 AND user_id = $2",
		deviceID, userID,
	)
	if err != nil {
		return fmt.Errorf("delete device: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}
