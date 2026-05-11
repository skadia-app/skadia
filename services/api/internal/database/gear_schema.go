package database

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/skadia-app/skadia/services/api/internal/gear"
)

// gearSchemaRow is a single row from gear_schema_definitions.
type gearSchemaRow struct {
	TypeKey    string
	Definition []byte // JSONB
}

// listGearSchemaDefinitions returns all user-override rows for the given user.
func (db *DB) listGearSchemaDefinitions(ctx context.Context, userID string) ([]gearSchemaRow, error) {
	rows, err := db.Pool.Query(ctx,
		`SELECT type_key, definition FROM gear_schema_definitions WHERE user_id = $1`,
		userID,
	)
	if err != nil {
		return nil, fmt.Errorf("list gear schema definitions: %w", err)
	}
	defer rows.Close()

	var result []gearSchemaRow
	for rows.Next() {
		var r gearSchemaRow
		if err := rows.Scan(&r.TypeKey, &r.Definition); err != nil {
			return nil, fmt.Errorf("scan gear schema definition: %w", err)
		}
		result = append(result, r)
	}
	return result, rows.Err()
}

// GetEffectiveGearSchema loads the generated base schema and overlays
// per-user customizations from gear_schema_definitions.
func (db *DB) GetEffectiveGearSchema(ctx context.Context, userID string) (gear.GearSchema, error) {
	rows, err := db.listGearSchemaDefinitions(ctx, userID)
	if err != nil {
		return gear.GearSchema{}, err
	}
	if len(rows) == 0 {
		return gear.Schema, nil
	}

	custom, err := rowsToUserDefinitions(rows)
	if err != nil {
		return gear.GearSchema{}, err
	}

	return gear.MergeSchemaWithUserCustom(gear.Schema, custom)
}

// UpsertGearSchemaDefinition inserts or replaces the user-override for a
// single type_key. The definition is JSON-encoded from def.
func (db *DB) UpsertGearSchemaDefinition(ctx context.Context, userID, typeKey string, def gear.UserSchemaDefinition) error {
	data, err := json.Marshal(def)
	if err != nil {
		return fmt.Errorf("marshal gear schema definition: %w", err)
	}
	_, err = db.Pool.Exec(ctx, `
		INSERT INTO gear_schema_definitions (user_id, type_key, definition)
		VALUES ($1, $2, $3)
		ON CONFLICT (user_id, type_key) DO UPDATE SET definition = EXCLUDED.definition, updated_at = now()`,
		userID, typeKey, data,
	)
	if err != nil {
		return fmt.Errorf("upsert gear schema definition: %w", err)
	}
	return nil
}

// DeleteGearSchemaDefinition removes the user override for a type_key.
func (db *DB) DeleteGearSchemaDefinition(ctx context.Context, userID, typeKey string) error {
	_, err := db.Pool.Exec(ctx,
		`DELETE FROM gear_schema_definitions WHERE user_id = $1 AND type_key = $2`,
		userID, typeKey,
	)
	if err != nil {
		return fmt.Errorf("delete gear schema definition: %w", err)
	}
	return nil
}

// ListGearSchemaDefinitions returns all user-override definitions for the given user.
func (db *DB) ListGearSchemaDefinitions(ctx context.Context, userID string) ([]gear.UserSchemaDefinition, error) {
	rows, err := db.listGearSchemaDefinitions(ctx, userID)
	if err != nil {
		return nil, err
	}
	return rowsToUserDefinitions(rows)
}

func rowsToUserDefinitions(rows []gearSchemaRow) ([]gear.UserSchemaDefinition, error) {
	defs := make([]gear.UserSchemaDefinition, 0, len(rows))
	for _, r := range rows {
		var def gear.UserSchemaDefinition
		if err := json.Unmarshal(r.Definition, &def); err != nil {
			return nil, fmt.Errorf("unmarshal gear schema definition for type %q: %w", r.TypeKey, err)
		}
		// type_key from DB row is authoritative (not the JSON body).
		def.TypeKey = r.TypeKey
		defs = append(defs, def)
	}
	return defs, nil
}
