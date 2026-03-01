package database

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/skadia-app/skadia/services/api/internal/model"
)

// GetOrCreateUserByOIDCSubject finds a user by their OIDC subject ID,
// creating a new user row if none exists yet.
func (db *DB) GetOrCreateUserByOIDCSubject(ctx context.Context, legacyOIDCSubject, username, email string) (*model.User, error) {
	var u model.User

	// Attempt to find existing user first.
	err := db.Pool.QueryRow(ctx,
		`SELECT id, username, display_name, email, legacy_oidc_subject, is_admin, is_active, created_at, updated_at
		 FROM users WHERE legacy_oidc_subject = $1`,
		legacyOIDCSubject,
	).Scan(&u.ID, &u.Username, &u.DisplayName, &u.Email, &u.LegacyOIDCSubject,
		&u.IsAdmin, &u.IsActive, &u.CreatedAt, &u.UpdatedAt)

	if err == nil {
		return &u, nil
	}
	if err != pgx.ErrNoRows {
		return nil, fmt.Errorf("get user by legacy_oidc_subject: %w", err)
	}

	// User does not exist yet – insert.
	// ON CONFLICT (username): link the existing user to this OIDC ID
	// (covers dev users pre-created without legacy_oidc_subject).
	err = db.Pool.QueryRow(ctx,
		`INSERT INTO users (username, email, legacy_oidc_subject)
		 VALUES ($1, $2, $3)
		 ON CONFLICT (username) DO UPDATE
		   SET legacy_oidc_subject = EXCLUDED.legacy_oidc_subject,
		       email        = COALESCE(EXCLUDED.email, users.email),
		       updated_at   = NOW()
		 RETURNING id, username, display_name, email, legacy_oidc_subject, is_admin, is_active, created_at, updated_at`,
		username, nilIfEmpty(email), legacyOIDCSubject,
	).Scan(&u.ID, &u.Username, &u.DisplayName, &u.Email, &u.LegacyOIDCSubject,
		&u.IsAdmin, &u.IsActive, &u.CreatedAt, &u.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("create user: %w", err)
	}

	return &u, nil
}
