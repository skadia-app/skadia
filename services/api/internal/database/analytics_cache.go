package database

import (
	"errors"
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// GetAnalyticsCache liest einen Cache-Eintrag und gibt die JSONB-Daten zurück.
// Gibt pgx.ErrNoRows zurück wenn kein Eintrag vorhanden.
func (db *DB) GetAnalyticsCache(ctx context.Context, userID string, cacheKey string) (json.RawMessage, error) {
	var data json.RawMessage
	err := db.Pool.QueryRow(ctx,
		`SELECT data FROM analytics_cache WHERE user_id = $1 AND cache_key = $2`,
		userID, cacheKey,
	).Scan(&data)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, pgx.ErrNoRows
	}
	if err != nil {
		return nil, fmt.Errorf("get analytics cache %s: %w", cacheKey, err)
	}
	return data, nil
}

// UpsertAnalyticsCache speichert Daten im Cache (Insert or Update).
func (db *DB) UpsertAnalyticsCache(ctx context.Context, userID string, cacheKey string, data json.RawMessage) error {
	_, err := db.Pool.Exec(ctx,
		`INSERT INTO analytics_cache (user_id, cache_key, data, computed_at)
         VALUES ($1, $2, $3, NOW())
         ON CONFLICT (user_id, cache_key) DO UPDATE
         SET data = EXCLUDED.data, computed_at = EXCLUDED.computed_at`,
		userID, cacheKey, data,
	)
	if err != nil {
		return fmt.Errorf("upsert analytics cache %s: %w", cacheKey, err)
	}
	return nil
}

// InvalidateAnalyticsCache löscht alle Cache-Einträge eines Users.
func (db *DB) InvalidateAnalyticsCache(ctx context.Context, userID string) error {
	_, err := db.Pool.Exec(ctx,
		`DELETE FROM analytics_cache WHERE user_id = $1`,
		userID,
	)
	return err
}
