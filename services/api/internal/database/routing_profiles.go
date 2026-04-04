package database

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// RoutingProfile is a persisted custom routing profile.
type RoutingProfile struct {
	ID          string    `json:"id"`
	UserID      *string   `json:"user_id,omitempty"`
	Name        string    `json:"name"`
	BaseProfile string    `json:"base_profile"`
	SportTypes  []string  `json:"sport_types"`
	Config      []byte    `json:"config"`
	Source      string    `json:"source"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// scanRoutingProfile scans one row into a RoutingProfile.
func scanRoutingProfile(row pgx.Row) (*RoutingProfile, error) {
	var p RoutingProfile
	err := row.Scan(
		&p.ID, &p.UserID, &p.Name, &p.BaseProfile,
		&p.SportTypes, &p.Config, &p.Source,
		&p.CreatedAt, &p.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

const routingProfileSelect = `
SELECT id, user_id, name, base_profile, sport_types, config, source, created_at, updated_at
FROM routing_profiles`

// ListRoutingProfiles returns all profiles visible to userID:
// those owned by the user and global profiles (user_id IS NULL).
func (db *DB) ListRoutingProfiles(ctx context.Context, userID string) ([]RoutingProfile, error) {
	rows, err := db.Pool.Query(ctx,
		routingProfileSelect+`
WHERE user_id = $1 OR user_id IS NULL
ORDER BY source, name`,
		userID,
	)
	if err != nil {
		return nil, fmt.Errorf("list routing profiles: %w", err)
	}
	defer rows.Close()

	var result []RoutingProfile
	for rows.Next() {
		var p RoutingProfile
		if err := rows.Scan(
			&p.ID, &p.UserID, &p.Name, &p.BaseProfile,
			&p.SportTypes, &p.Config, &p.Source,
			&p.CreatedAt, &p.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan routing profile: %w", err)
		}
		result = append(result, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate routing profiles: %w", err)
	}
	return result, nil
}

// GetRoutingProfile fetches a single routing profile by UUID.
// Returns nil, nil when not found.
func (db *DB) GetRoutingProfile(ctx context.Context, id string) (*RoutingProfile, error) {
	p, err := scanRoutingProfile(db.Pool.QueryRow(ctx,
		routingProfileSelect+` WHERE id = $1`,
		id,
	))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get routing profile: %w", err)
	}
	return p, nil
}

// GetRoutingProfileByName fetches a routing profile by owner + name.
// Used by SuggestRoutes to resolve custom profile references.
// Returns nil, nil when not found.
func (db *DB) GetRoutingProfileByName(ctx context.Context, userID, name string) (*RoutingProfile, error) {
	p, err := scanRoutingProfile(db.Pool.QueryRow(ctx,
		routingProfileSelect+` WHERE user_id = $1 AND name = $2`,
		userID, name,
	))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get routing profile by name: %w", err)
	}
	return p, nil
}

// GetRoutingProfileByNameOrGlobal fetches a routing profile by name, preferring
// user-owned profiles over global (user_id IS NULL) profiles.
// Used by SuggestRoutes to resolve custom profile references including filesystem profiles.
// Returns nil, nil when not found.
func (db *DB) GetRoutingProfileByNameOrGlobal(ctx context.Context, userID, name string) (*RoutingProfile, error) {
	// ORDER BY user_id IS NULL: FALSE (0) sorts before TRUE (1) in ASC,
	// so user-owned profiles take precedence over global filesystem profiles.
	p, err := scanRoutingProfile(db.Pool.QueryRow(ctx,
		routingProfileSelect+`
WHERE (user_id = $1 OR user_id IS NULL) AND name = $2
ORDER BY user_id IS NULL
LIMIT 1`,
		userID, name,
	))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get routing profile by name or global: %w", err)
	}
	return p, nil
}

// CreateRoutingProfile inserts a new user-owned routing profile and returns it.
func (db *DB) CreateRoutingProfile(ctx context.Context, userID, name, baseProfile string, sportTypes []string, config []byte) (*RoutingProfile, error) {
	if sportTypes == nil {
		sportTypes = []string{}
	}
	p, err := scanRoutingProfile(db.Pool.QueryRow(ctx, `
INSERT INTO routing_profiles (user_id, name, base_profile, sport_types, config, source)
VALUES ($1, $2, $3, $4, $5, 'user')
RETURNING id, user_id, name, base_profile, sport_types, config, source, created_at, updated_at`,
		userID, name, baseProfile, sportTypes, config,
	))
	if err != nil {
		return nil, fmt.Errorf("create routing profile: %w", err)
	}
	return p, nil
}

// UpdateRoutingProfile updates the name, sport types, and config of an existing profile.
// The profile is identified by id; caller is responsible for ownership checks.
func (db *DB) UpdateRoutingProfile(ctx context.Context, id, name string, sportTypes []string, config []byte) (*RoutingProfile, error) {
	if sportTypes == nil {
		sportTypes = []string{}
	}
	p, err := scanRoutingProfile(db.Pool.QueryRow(ctx, `
UPDATE routing_profiles
SET name = $2, sport_types = $3, config = $4, updated_at = now()
WHERE id = $1
RETURNING id, user_id, name, base_profile, sport_types, config, source, created_at, updated_at`,
		id, name, sportTypes, config,
	))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("update routing profile: profile %s not found", id)
	}
	if err != nil {
		return nil, fmt.Errorf("update routing profile: %w", err)
	}
	return p, nil
}

// DeleteRoutingProfile deletes a routing profile, enforcing ownership.
// Returns an error when the profile does not exist or belongs to a different user.
func (db *DB) DeleteRoutingProfile(ctx context.Context, id, userID string) error {
	tag, err := db.Pool.Exec(ctx,
		`DELETE FROM routing_profiles WHERE id = $1 AND user_id = $2`,
		id, userID,
	)
	if err != nil {
		return fmt.Errorf("delete routing profile: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("delete routing profile: profile %s not found or not owned by user", id)
	}
	return nil
}

// UpsertFilesystemProfile inserts or updates a global (user_id IS NULL) routing profile
// loaded from the filesystem. Conflicts on name (where user_id IS NULL) trigger an UPDATE.
func (db *DB) UpsertFilesystemProfile(ctx context.Context, name, baseProfile string, sportTypes []string, config []byte) (*RoutingProfile, error) {
	if sportTypes == nil {
		sportTypes = []string{}
	}
	p, err := scanRoutingProfile(db.Pool.QueryRow(ctx, `
INSERT INTO routing_profiles (user_id, name, base_profile, sport_types, config, source)
VALUES (NULL, $1, $2, $3, $4, 'filesystem')
ON CONFLICT (name) WHERE user_id IS NULL DO UPDATE
SET base_profile = EXCLUDED.base_profile,
    sport_types  = EXCLUDED.sport_types,
    config       = EXCLUDED.config,
    source       = EXCLUDED.source,
    updated_at   = now()
RETURNING id, user_id, name, base_profile, sport_types, config, source, created_at, updated_at`,
		name, baseProfile, sportTypes, config,
	))
	if err != nil {
		return nil, fmt.Errorf("upsert filesystem profile %q: %w", name, err)
	}
	return p, nil
}

// ExportRoutingProfiles returns all routing profiles owned by userID for GDPR export.
func (db *DB) ExportRoutingProfiles(ctx context.Context, userID string) ([]RoutingProfile, error) {
	rows, err := db.Pool.Query(ctx,
		routingProfileSelect+`
WHERE user_id = $1
ORDER BY name`,
		userID,
	)
	if err != nil {
		return nil, fmt.Errorf("export routing profiles: %w", err)
	}
	defer rows.Close()

	var result []RoutingProfile
	for rows.Next() {
		var p RoutingProfile
		if err := rows.Scan(
			&p.ID, &p.UserID, &p.Name, &p.BaseProfile,
			&p.SportTypes, &p.Config, &p.Source,
			&p.CreatedAt, &p.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan routing profile for export: %w", err)
		}
		result = append(result, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate routing profiles for export: %w", err)
	}
	return result, nil
}
