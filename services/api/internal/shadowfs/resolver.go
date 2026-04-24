// services/api/internal/shadowfs/resolver.go
package shadowfs

import (
	"context"
	"errors"
	"fmt"

	"github.com/skadia-app/skadia/services/api/internal/settings"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ResolveUserSchema returns the effective schema template for (userID, fileType).
// Resolution priority (spec §4.2):
//  1. library.user_can_edit_schemas = "true" AND user_file_schemas override exists → user override
//  2. server_settings['library.default_schema.<fileType>'] if non-empty → global default
//  3. renderer.DefaultSchema(fileType) → built-in last-ditch fallback
//
// Lockdown (user_can_edit_schemas="false") skips the user override even if
// stored — the user value stays in DB, reactivates when lock lifts.
func ResolveUserSchema(ctx context.Context, pool *pgxpool.Pool,
	store *settings.SettingsStore, renderer *SchemaRenderer,
	userID, fileType string) (string, error) {

	canEdit := store.Get("library.user_can_edit_schemas").Value == "true"

	if canEdit {
		var userSchema string
		err := pool.QueryRow(ctx,
			`SELECT schema_template FROM user_file_schemas WHERE user_id = $1 AND file_type = $2`,
			userID, fileType).Scan(&userSchema)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return "", fmt.Errorf("resolve user schema: query user override: %w", err)
		}
		if userSchema != "" {
			return userSchema, nil
		}
	}

	globalDefault := store.Get("library.default_schema." + fileType).Value
	if globalDefault != "" {
		return globalDefault, nil
	}

	return renderer.DefaultSchema(fileType), nil
}

// ResolveLibraryDirName returns the effective library directory prefix for userID.
// Resolution (spec §4.2):
//  1. users.library_dir_name IS NOT NULL → that value
//  2. users.id::text → UUID fallback
//
// Lockdown on user_can_edit_dir_name does NOT override an admin-set value.
// The flag only gates the user-facing PUT endpoint, not resolution.
func ResolveLibraryDirName(ctx context.Context, pool *pgxpool.Pool, userID string) (string, error) {
	var name *string
	err := pool.QueryRow(ctx,
		`SELECT library_dir_name FROM users WHERE id = $1`, userID).Scan(&name)
	if err != nil {
		return "", fmt.Errorf("resolve library dir: query: %w", err)
	}
	if name != nil && *name != "" {
		return *name, nil
	}
	return userID, nil
}
