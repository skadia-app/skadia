package database

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/sync/singleflight"
)

type DB struct {
	Pool          *pgxpool.Pool
	ReadonlyPool  *pgxpool.Pool
	terraInflight singleflight.Group // deduplicates concurrent terra incognita computations per user
	// syncMetrics holds Prometheus instruments for WithSystemModifierTagSyncSuspended.
	// Nil means "use the package default". Tests inject an isolated registry.
	syncMetrics *systemModifierTagSyncMetrics
}

func New(ctx context.Context, databaseURL string) (*DB, error) {
	config, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("parse database config: %w", err)
	}
	// Ensure enough connections for parallel geo-enrichment (6 modules × semaphore 3)
	// + recompute workers (4) + normal API requests + 4 parallel tile builds (2 conns each).
	if config.MaxConns < 40 {
		config.MaxConns = 40
	}
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("connect to database: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}
	slog.Info("database connected")
	return &DB{Pool: pool}, nil
}

func (db *DB) Close() {
	if db.ReadonlyPool != nil {
		db.ReadonlyPool.Close()
	}
	if db.Pool != nil {
		db.Pool.Close()
	}
}

func (db *DB) Ping(ctx context.Context) error {
	return db.Pool.Ping(ctx)
}

func (db *DB) OpenReadonly(ctx context.Context, databaseURL string) error {
	if strings.TrimSpace(databaseURL) == "" {
		return fmt.Errorf("readonly database url is required")
	}
	config, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return fmt.Errorf("parse readonly database config: %w", err)
	}
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return fmt.Errorf("connect to readonly database: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return fmt.Errorf("ping readonly database: %w", err)
	}
	db.ReadonlyPool = pool
	return nil
}

func (db *DB) WithUserTx(ctx context.Context, userID string, fn func(pgx.Tx) error) error {
	if strings.TrimSpace(userID) == "" {
		return fmt.Errorf("RLS context not initialized: userID is required")
	}
	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, "SELECT set_config('app.current_user_id', $1, true)", userID); err != nil {
		return fmt.Errorf("set local rls context: %w", err)
	}
	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// Helper functions

func derefOr(p *string, fallback string) string {
	if p != nil {
		return *p
	}
	return fallback
}

// visibilityOrDefault returns the given visibility if non-empty, otherwise "private".
func visibilityOrDefault(v string) string {
	if v != "" {
		return v
	}
	return "private"
}

func nilIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func nilIfZeroF64(f float64) *float64 {
	if f == 0 {
		return nil
	}
	return &f
}

func nilIfZeroInt(i int) *int {
	if i == 0 {
		return nil
	}
	return &i
}

func nilIfZeroI16(i int16) *int16 {
	if i == 0 {
		return nil
	}
	return &i
}

func nilIfZeroI32(i int32) *int32 {
	if i == 0 {
		return nil
	}
	return &i
}

func nilIfZeroTime(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	return &t
}

func ptrString(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
