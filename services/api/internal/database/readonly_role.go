package database

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

const readonlyPasswordPlaceholder = "skadia_readonly_password"

func ReadonlyRolePasswordFromDSN(databaseURL string) (string, bool, error) {
	config, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return "", false, fmt.Errorf("parse readonly database config: %w", err)
	}
	if config.ConnConfig.User != "skadia_readonly" {
		return "", false, nil
	}
	if config.ConnConfig.Password == "" {
		return "", false, nil
	}
	if config.ConnConfig.Password == readonlyPasswordPlaceholder {
		return "", false, fmt.Errorf("refusing to use placeholder password for skadia_readonly")
	}
	return config.ConnConfig.Password, true, nil
}

func IsReadonlyAuthError(err error) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == "28P01"
	}
	return false
}

func (db *DB) EnsureReadonlyRolePassword(ctx context.Context, databaseURL string) error {
	password, ok, err := ReadonlyRolePasswordFromDSN(databaseURL)
	if err != nil {
		return err
	}
	if !ok {
		return nil
	}

	var stmt string
	if err := db.Pool.QueryRow(ctx,
		`SELECT format('ALTER ROLE skadia_readonly WITH PASSWORD %L', $1)`,
		password,
	).Scan(&stmt); err != nil {
		return fmt.Errorf("render readonly role password update: %w", err)
	}
	if _, err := db.Pool.Exec(ctx, stmt); err != nil {
		return fmt.Errorf("alter readonly role password: %w", err)
	}
	return nil
}
