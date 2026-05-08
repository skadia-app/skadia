package database

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/sport"
	"github.com/jackc/pgx/v5"
)

// RoutingProfile is a persisted custom routing profile.
type RoutingProfile struct {
	ID              string                `json:"id"`
	UserID          *string               `json:"user_id,omitempty"`
	Name            string                `json:"name"`
	BaseProfile     string                `json:"base_profile"`
	SportTypes      []string              `json:"sport_types"`
	Predicates      []sport.Predicate     `json:"predicates"`
	ModifierFilters sport.ModifierFilters `json:"modifier_filters"`
	Config          []byte                `json:"config"`
	Source          string                `json:"source"`
	CreatedAt       time.Time             `json:"created_at"`
	UpdatedAt       time.Time             `json:"updated_at"`
}

// scanRoutingProfile scans one row into a RoutingProfile.
func scanRoutingProfile(row pgx.Row) (*RoutingProfile, error) {
	var p RoutingProfile
	var predicatesJSON, modifierFiltersJSON []byte
	err := row.Scan(
		&p.ID, &p.UserID, &p.Name, &p.BaseProfile,
		&p.SportTypes, &predicatesJSON, &modifierFiltersJSON, &p.Config, &p.Source,
		&p.CreatedAt, &p.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	if len(predicatesJSON) > 0 {
		if err := json.Unmarshal(predicatesJSON, &p.Predicates); err != nil {
			return nil, fmt.Errorf("unmarshal predicates: %w", err)
		}
	}
	if p.Predicates == nil {
		p.Predicates = []sport.Predicate{}
	}
	if len(modifierFiltersJSON) > 0 {
		if err := json.Unmarshal(modifierFiltersJSON, &p.ModifierFilters); err != nil {
			return nil, fmt.Errorf("unmarshal modifier_filters: %w", err)
		}
	}
	return &p, nil
}

const routingProfileSelect = `
SELECT id, user_id, name, base_profile, sport_types, predicates, modifier_filters, config, source, created_at, updated_at
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
		p, err := scanRoutingProfile(rows)
		if err != nil {
			return nil, fmt.Errorf("scan routing profile: %w", err)
		}
		result = append(result, *p)
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
func (db *DB) CreateRoutingProfile(ctx context.Context, userID, name, baseProfile string, predicates []sport.Predicate, modifierFilters sport.ModifierFilters, config []byte) (*RoutingProfile, error) {
	if predicates == nil {
		predicates = []sport.Predicate{}
	}
	if err := sport.ValidatePredicates(predicates, false); err != nil {
		return nil, fmt.Errorf("validate routing profile predicates: %w", err)
	}
	sportTypes := sport.SportTypesFromPredicates(predicates)
	predicatesJSON, err := json.Marshal(predicates)
	if err != nil {
		return nil, fmt.Errorf("marshal predicates: %w", err)
	}
	modifierFiltersJSON, err := json.Marshal(modifierFilters)
	if err != nil {
		return nil, fmt.Errorf("marshal modifier_filters: %w", err)
	}
	p, err := scanRoutingProfile(db.Pool.QueryRow(ctx, `
INSERT INTO routing_profiles (user_id, name, base_profile, sport_types, predicates, modifier_filters, config, source)
VALUES ($1, $2, $3, $4, $5, $6, $7, 'user')
RETURNING id, user_id, name, base_profile, sport_types, predicates, modifier_filters, config, source, created_at, updated_at`,
		userID, name, baseProfile, sportTypes, predicatesJSON, modifierFiltersJSON, config,
	))
	if err != nil {
		return nil, fmt.Errorf("create routing profile: %w", err)
	}
	return p, nil
}

// UpdateRoutingProfile updates the name, sport types, and config of an existing profile.
// The profile is identified by id; caller is responsible for ownership checks.
func (db *DB) UpdateRoutingProfile(ctx context.Context, id, name string, predicates []sport.Predicate, modifierFilters sport.ModifierFilters, config []byte) (*RoutingProfile, error) {
	if predicates == nil {
		predicates = []sport.Predicate{}
	}
	if err := sport.ValidatePredicates(predicates, false); err != nil {
		return nil, fmt.Errorf("validate routing profile predicates: %w", err)
	}
	sportTypes := sport.SportTypesFromPredicates(predicates)
	predicatesJSON, err := json.Marshal(predicates)
	if err != nil {
		return nil, fmt.Errorf("marshal predicates: %w", err)
	}
	modifierFiltersJSON, err := json.Marshal(modifierFilters)
	if err != nil {
		return nil, fmt.Errorf("marshal modifier_filters: %w", err)
	}
	p, err := scanRoutingProfile(db.Pool.QueryRow(ctx, `
UPDATE routing_profiles
SET name = $2, sport_types = $3, predicates = $4, modifier_filters = $5, config = $6, updated_at = now()
WHERE id = $1
RETURNING id, user_id, name, base_profile, sport_types, predicates, modifier_filters, config, source, created_at, updated_at`,
		id, name, sportTypes, predicatesJSON, modifierFiltersJSON, config,
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

// UpsertGlobalProfile inserts or updates a global (user_id IS NULL) routing profile.
// source should be "system" (from config/badrouter/) or "filesystem" (from data/profiles/badrouter/).
// Conflicts on name (where user_id IS NULL) trigger an UPDATE.
func (db *DB) UpsertGlobalProfile(ctx context.Context, name, baseProfile string, predicates []sport.Predicate, modifierFilters sport.ModifierFilters, config []byte, source string) (*RoutingProfile, error) {
	if predicates == nil {
		predicates = []sport.Predicate{}
	}
	if err := sport.ValidatePredicates(predicates, false); err != nil {
		return nil, fmt.Errorf("validate routing profile predicates: %w", err)
	}
	sportTypes := sport.SportTypesFromPredicates(predicates)
	predicatesJSON, err := json.Marshal(predicates)
	if err != nil {
		return nil, fmt.Errorf("marshal predicates: %w", err)
	}
	modifierFiltersJSON, err := json.Marshal(modifierFilters)
	if err != nil {
		return nil, fmt.Errorf("marshal modifier_filters: %w", err)
	}
	p, err := scanRoutingProfile(db.Pool.QueryRow(ctx, `
INSERT INTO routing_profiles (user_id, name, base_profile, sport_types, predicates, modifier_filters, config, source)
VALUES (NULL, $1, $2, $3, $4, $5, $6, $7)
ON CONFLICT (name) WHERE user_id IS NULL DO UPDATE
SET base_profile = EXCLUDED.base_profile,
    sport_types  = EXCLUDED.sport_types,
    predicates   = EXCLUDED.predicates,
    modifier_filters = EXCLUDED.modifier_filters,
    config       = EXCLUDED.config,
    source       = EXCLUDED.source,
    updated_at   = CASE
        WHEN routing_profiles.config IS DISTINCT FROM EXCLUDED.config
          OR routing_profiles.base_profile IS DISTINCT FROM EXCLUDED.base_profile
          OR routing_profiles.sport_types IS DISTINCT FROM EXCLUDED.sport_types
          OR routing_profiles.predicates IS DISTINCT FROM EXCLUDED.predicates
          OR routing_profiles.modifier_filters IS DISTINCT FROM EXCLUDED.modifier_filters
          OR routing_profiles.source IS DISTINCT FROM EXCLUDED.source
        THEN now()
        ELSE routing_profiles.updated_at
    END
RETURNING id, user_id, name, base_profile, sport_types, predicates, modifier_filters, config, source, created_at, updated_at`,
		name, baseProfile, sportTypes, predicatesJSON, modifierFiltersJSON, config, source,
	))
	if err != nil {
		return nil, fmt.Errorf("upsert global profile %q: %w", name, err)
	}
	return p, nil
}
