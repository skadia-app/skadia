package database

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/sync/singleflight"
)

type DB struct {
	Pool          *pgxpool.Pool
	terraInflight singleflight.Group // deduplicates concurrent terra incognita computations per user
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
	db.Pool.Close()
}

func (db *DB) Ping(ctx context.Context) error {
	return db.Pool.Ping(ctx)
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
