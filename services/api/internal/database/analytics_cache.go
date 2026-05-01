package database

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// AnalyticsCacheRow is the metadata-aware view of an analytics_cache entry.
// ExpiresAt is nil when the column is NULL (legacy rows without TTL).
type AnalyticsCacheRow struct {
	Data       json.RawMessage
	ComputedAt time.Time
	ExpiresAt  *time.Time
}

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

// GetAnalyticsCacheWithMeta reads a cache entry together with its computed_at
// and (nullable) expires_at metadata. Returns (nil, nil) when no row exists.
func (db *DB) GetAnalyticsCacheWithMeta(ctx context.Context, userID, cacheKey string) (*AnalyticsCacheRow, error) {
	var (
		data       json.RawMessage
		computedAt time.Time
		expiresAt  sql.NullTime
	)
	err := db.Pool.QueryRow(ctx,
		`SELECT data, computed_at, expires_at
		 FROM analytics_cache
		 WHERE user_id = $1 AND cache_key = $2`,
		userID, cacheKey,
	).Scan(&data, &computedAt, &expiresAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get analytics cache with meta %s: %w", cacheKey, err)
	}
	row := &AnalyticsCacheRow{
		Data:       data,
		ComputedAt: computedAt,
	}
	if expiresAt.Valid {
		t := expiresAt.Time
		row.ExpiresAt = &t
	}
	return row, nil
}

// UpsertAnalyticsCacheWithExpiry stores a cache entry with an explicit TTL.
// The expires_at column is set to NOW() + ttl. ttl is passed as integer seconds
// to make_interval — never as a formatted string.
func (db *DB) UpsertAnalyticsCacheWithExpiry(ctx context.Context, userID, cacheKey string, data json.RawMessage, ttl time.Duration) error {
	_, err := db.Pool.Exec(ctx,
		`INSERT INTO analytics_cache (user_id, cache_key, data, computed_at, expires_at)
		 VALUES ($1, $2, $3, NOW(), NOW() + make_interval(secs => $4))
		 ON CONFLICT (user_id, cache_key) DO UPDATE
		 SET data        = EXCLUDED.data,
		     computed_at = EXCLUDED.computed_at,
		     expires_at  = EXCLUDED.expires_at`,
		userID, cacheKey, data, int(ttl.Seconds()),
	)
	if err != nil {
		return fmt.Errorf("upsert analytics cache with expiry %s: %w", cacheKey, err)
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
