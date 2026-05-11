package database

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/sixdouglas/suncalc"

	gearpkg "github.com/skadia-app/skadia/services/api/internal/gear"
	"github.com/skadia-app/skadia/services/api/internal/gearcompat"
	"github.com/skadia-app/skadia/services/api/internal/gearmetrics"
	"github.com/skadia-app/skadia/services/api/internal/model"
	"github.com/skadia-app/skadia/services/api/internal/rules"
	"github.com/skadia-app/skadia/services/api/internal/sport"
)

// gearSelectCols are the columns returned by gear queries.
// total_moving_time_s + total_elevation_loss_m are aggregated server-side
// (recompute-flow in RecomputeGearAggregates) and exposed for ski / downhill
// disciplines where descent and moving-time are the primary load metrics.
const gearSelectCols = `id, user_id, name, type, scope, sub_type, brand, model,
	total_distance_m, total_time_s, total_moving_time_s,
	total_elevation_gain_m, total_elevation_loss_m, total_activities,
	purchased_at, retired_at, is_retired, status, applicable_sport_predicates, notes, created_at,
	attributes, modifiers`

// scanGear scans a gear row into a model.Gear.
// pgx v5 uses binary protocol for dates (OID 1082) which cannot be scanned
// directly into *string — use pgtype.Date as intermediate and format manually.
func scanGear(row pgx.Row) (*model.Gear, error) {
	var g model.Gear
	var purchasedAt, retiredAt pgtype.Date
	var predicateJSON, attributesJSON, modifiersJSON []byte
	err := row.Scan(
		&g.ID, &g.UserID, &g.Name, &g.Type, &g.Scope, &g.SubType, &g.Brand, &g.Model,
		&g.TotalDistanceM, &g.TotalTimeS, &g.TotalMovingTimeS,
		&g.TotalElevationGainM, &g.TotalElevationLossM, &g.TotalActivities,
		&purchasedAt, &retiredAt, &g.IsRetired, &g.Status, &predicateJSON, &g.Notes, &g.CreatedAt,
		&attributesJSON, &modifiersJSON,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if purchasedAt.Valid {
		s := purchasedAt.Time.Format("2006-01-02")
		g.PurchasedAt = &s
	}
	if retiredAt.Valid {
		s := retiredAt.Time.Format("2006-01-02")
		g.RetiredAt = &s
	}
	if len(predicateJSON) > 0 {
		if err := json.Unmarshal(predicateJSON, &g.ApplicableSportPredicates); err != nil {
			return nil, fmt.Errorf("scan gear applicable_sport_predicates: %w", err)
		}
	}
	if g.ApplicableSportPredicates == nil {
		g.ApplicableSportPredicates = []gearpkg.GearDefaultPredicate{}
	}
	// Keep legacy field in sync for backward compatibility.
	g.DefaultSportPredicates = g.ApplicableSportPredicates
	if len(attributesJSON) > 0 && string(attributesJSON) != "{}" {
		if err := json.Unmarshal(attributesJSON, &g.Attributes); err != nil {
			return nil, fmt.Errorf("scan gear attributes: %w", err)
		}
	}
	if g.Attributes == nil {
		g.Attributes = map[string]any{}
	}
	if len(modifiersJSON) > 0 && string(modifiersJSON) != "{}" {
		if err := json.Unmarshal(modifiersJSON, &g.Modifiers); err != nil {
			return nil, fmt.Errorf("scan gear modifiers: %w", err)
		}
	}
	if g.Modifiers == nil {
		g.Modifiers = map[string]any{}
	}
	return &g, nil
}

// componentSelectCols are the columns returned by component queries.
// Includes the 000355 columns: type_key, slot_key, preset_gear_type_key,
// attributes, plus descent / moving-time / activities totals.
const componentSelectCols = `id, gear_id, parent_component_id, name, type, type_key, slot_key,
	preset_gear_type_key,
	brand, model,
	total_distance_m, total_elevation_gain_m, total_elevation_loss_m,
	total_time_s, total_moving_time_s, total_activities,
	attributes,
	purchase_price, purchase_date, weight_g, serial_number,
	installed_at, removed_at, is_retired, status, notes`

// scanComponentRow scans the columns listed in componentSelectCols (in order)
// into c. attributesJSON is unmarshaled into c.Attributes. ErrNoRows is
// surfaced to the caller so it can decide whether absent rows are an error.
func scanComponentRow(c *model.GearComponent, row pgx.Row) error {
	var attributesJSON []byte
	err := row.Scan(
		&c.ID, &c.GearID, &c.ParentComponentID, &c.Name, &c.Type,
		&c.TypeKey, &c.SlotKey, &c.PresetGearTypeKey, &c.Brand, &c.Model,
		&c.TotalDistanceM, &c.TotalElevationGainM, &c.TotalElevationLossM,
		&c.TotalTimeS, &c.TotalMovingTimeS, &c.TotalActivities,
		&attributesJSON,
		&c.PurchasePrice, &c.PurchaseDate, &c.WeightG, &c.SerialNumber,
		&c.InstalledAt, &c.RemovedAt, &c.IsRetired, &c.Status, &c.Notes,
	)
	if err != nil {
		return err
	}
	if len(attributesJSON) > 0 && string(attributesJSON) != "{}" {
		if err := json.Unmarshal(attributesJSON, &c.Attributes); err != nil {
			return fmt.Errorf("scan component attributes: %w", err)
		}
	}
	if c.Attributes == nil {
		c.Attributes = map[string]any{}
	}
	return nil
}

// scanComponent scans a component row into a model.GearComponent.
// Returns (nil, nil) when the row is absent (pgx.ErrNoRows).
func scanComponent(row pgx.Row) (*model.GearComponent, error) {
	var c model.GearComponent
	if err := scanComponentRow(&c, row); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &c, nil
}

// --- Gear CRUD ---

func (db *DB) ListGear(ctx context.Context, userID string, status string) ([]model.Gear, error) {
	var rows pgx.Rows
	var err error
	if status != "" {
		rows, err = db.Pool.Query(ctx,
			`SELECT `+gearSelectCols+` FROM gear WHERE user_id = $1 AND status = $2 ORDER BY name`,
			userID, status)
	} else {
		rows, err = db.Pool.Query(ctx,
			`SELECT `+gearSelectCols+` FROM gear WHERE user_id = $1 ORDER BY name`,
			userID)
	}
	if err != nil {
		return nil, fmt.Errorf("list gear: %w", err)
	}
	defer rows.Close()

	var items []model.Gear
	for rows.Next() {
		var g model.Gear
		var purchasedAt, retiredAt pgtype.Date
		var predicateJSON, attributesJSON, modifiersJSON []byte
		if err := rows.Scan(
			&g.ID, &g.UserID, &g.Name, &g.Type, &g.Scope, &g.SubType, &g.Brand, &g.Model,
			&g.TotalDistanceM, &g.TotalTimeS, &g.TotalMovingTimeS,
			&g.TotalElevationGainM, &g.TotalElevationLossM, &g.TotalActivities,
			&purchasedAt, &retiredAt, &g.IsRetired, &g.Status, &predicateJSON, &g.Notes, &g.CreatedAt,
			&attributesJSON, &modifiersJSON,
		); err != nil {
			return nil, fmt.Errorf("scan gear: %w", err)
		}
		if purchasedAt.Valid {
			s := purchasedAt.Time.Format("2006-01-02")
			g.PurchasedAt = &s
		}
		if retiredAt.Valid {
			s := retiredAt.Time.Format("2006-01-02")
			g.RetiredAt = &s
		}
		if len(predicateJSON) > 0 {
			if err := json.Unmarshal(predicateJSON, &g.ApplicableSportPredicates); err != nil {
				return nil, fmt.Errorf("scan gear applicable_sport_predicates: %w", err)
			}
		}
		if g.ApplicableSportPredicates == nil {
			g.ApplicableSportPredicates = []gearpkg.GearDefaultPredicate{}
		}
		g.DefaultSportPredicates = g.ApplicableSportPredicates
		if len(attributesJSON) > 0 && string(attributesJSON) != "{}" {
			if err := json.Unmarshal(attributesJSON, &g.Attributes); err != nil {
				return nil, fmt.Errorf("scan gear attributes: %w", err)
			}
		}
		if g.Attributes == nil {
			g.Attributes = map[string]any{}
		}
		if len(modifiersJSON) > 0 && string(modifiersJSON) != "{}" {
			if err := json.Unmarshal(modifiersJSON, &g.Modifiers); err != nil {
				return nil, fmt.Errorf("scan gear modifiers: %w", err)
			}
		}
		if g.Modifiers == nil {
			g.Modifiers = map[string]any{}
		}
		items = append(items, g)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list gear: %w", err)
	}
	if items == nil {
		items = []model.Gear{}
	}
	return items, nil
}

func (db *DB) CreateGear(ctx context.Context, userID string, create *model.GearCreate) (*model.Gear, error) {
	// Three-state merge: prefer ApplicableSportPredicates, fall back to
	// DefaultSportPredicates; nil pointer means "not sent / use default".
	// A non-nil pointer to an empty slice explicitly clears predicates.
	var rawPredicates []gearpkg.GearDefaultPredicate
	if create.ApplicableSportPredicates != nil {
		rawPredicates = *create.ApplicableSportPredicates
	} else if create.DefaultSportPredicates != nil {
		rawPredicates = *create.DefaultSportPredicates
	}
	predicates, err := gearpkg.NormalizeGearDefaultPredicates(rawPredicates)
	if err != nil {
		return nil, fmt.Errorf("create gear: %w", err)
	}
	predicateJSON, err := json.Marshal(predicates)
	if err != nil {
		return nil, fmt.Errorf("create gear: %w", err)
	}

	// Derive scope from type key via schema; unknown types default to "other".
	scope := create.Scope
	if scope == "" {
		if gt, ok := gearpkg.LookupGearType(create.Type); ok {
			scope = gt.Scope
		}
		if scope == "" {
			scope = "other"
		}
	}

	attrs := create.Attributes
	if attrs == nil {
		attrs = map[string]any{}
	}
	attrsJSON, err := json.Marshal(attrs)
	if err != nil {
		return nil, fmt.Errorf("create gear: marshal attributes: %w", err)
	}
	mods := create.Modifiers
	if mods == nil {
		mods = map[string]any{}
	}
	modsJSON, err := json.Marshal(mods)
	if err != nil {
		return nil, fmt.Errorf("create gear: marshal modifiers: %w", err)
	}

	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("create gear: begin tx: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	g, err := scanGear(tx.QueryRow(ctx,
		`INSERT INTO gear (user_id, name, type, scope, sub_type, brand, model, purchased_at,
		                   applicable_sport_predicates, notes, attributes, modifiers)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9::jsonb, $10, $11::jsonb, $12::jsonb)
		 RETURNING `+gearSelectCols,
		userID, create.Name, create.Type, scope, create.SubType, create.Brand, create.Model,
		create.PurchasedAt, string(predicateJSON), create.Notes,
		string(attrsJSON), string(modsJSON),
	))
	if err != nil {
		return nil, fmt.Errorf("create gear: %w", err)
	}

	// Strip client-supplied IDs on create — they'd never collide with anything
	// that exists yet, but accepting them is a foot-gun for callers that copy
	// payloads between gear items.
	if len(create.Components) > 0 {
		clean := make([]model.GearComponentInput, len(create.Components))
		for i, c := range create.Components {
			c.ID = ""
			clean[i] = c
		}
		if err := db.ApplyGearComponents(ctx, tx, g.ID, userID, clean); err != nil {
			return nil, fmt.Errorf("create gear: apply components: %w", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("create gear: commit: %w", err)
	}

	// Re-read components so the response reflects server-assigned IDs.
	components, err := db.listComponents(ctx, g.ID)
	if err != nil {
		return nil, err
	}
	g.Components = components
	return g, nil
}

func (db *DB) GetGear(ctx context.Context, id string) (*model.Gear, error) {
	g, err := scanGear(db.Pool.QueryRow(ctx,
		`SELECT `+gearSelectCols+` FROM gear WHERE id = $1`, id,
	))
	if err != nil {
		return nil, fmt.Errorf("get gear: %w", err)
	}
	if g == nil {
		return nil, nil
	}

	// Load components
	components, err := db.listComponents(ctx, id)
	if err != nil {
		return nil, err
	}
	g.Components = components
	return g, nil
}

func (db *DB) UpdateGear(ctx context.Context, id string, update *model.GearUpdate) (*model.Gear, error) {
	b := newSetBuilder(1)

	if update.Name != nil {
		b.Set("name", *update.Name)
	}
	if update.Type != nil {
		b.Set("type", *update.Type)
	}
	if update.SubType != nil {
		b.Set("sub_type", *update.SubType)
	}
	if update.Brand != nil {
		b.Set("brand", *update.Brand)
	}
	if update.Model != nil {
		b.Set("model", *update.Model)
	}
	if update.PurchasedAt != nil {
		b.Set("purchased_at", *update.PurchasedAt)
	}
	if update.RetiredAt != nil {
		b.Set("retired_at", *update.RetiredAt)
	}
	if update.IsRetired != nil {
		b.Set("is_retired", *update.IsRetired)
	}
	// Three-state merge: prefer ApplicableSportPredicates, fall back to
	// DefaultSportPredicates. A nil pointer means the field was not sent —
	// leave the DB column unchanged. A non-nil pointer to an empty slice
	// explicitly clears the column to [].
	var rawPredicates []gearpkg.GearDefaultPredicate
	updatePredicates := false
	if update.ApplicableSportPredicates != nil {
		rawPredicates = *update.ApplicableSportPredicates
		updatePredicates = true
	} else if update.DefaultSportPredicates != nil {
		rawPredicates = *update.DefaultSportPredicates
		updatePredicates = true
	}
	if updatePredicates {
		predicates, err := gearpkg.NormalizeGearDefaultPredicates(rawPredicates)
		if err != nil {
			return nil, fmt.Errorf("update gear: %w", err)
		}
		predicateJSON, err := json.Marshal(predicates)
		if err != nil {
			return nil, fmt.Errorf("update gear: %w", err)
		}
		idx := b.NextIdx()
		b.SetExpr(fmt.Sprintf("applicable_sport_predicates = $%d::jsonb", idx), string(predicateJSON))
		b.idx++
	}
	if update.Attributes != nil {
		attrsJSON, err := json.Marshal(update.Attributes)
		if err != nil {
			return nil, fmt.Errorf("update gear: marshal attributes: %w", err)
		}
		idx := b.NextIdx()
		b.SetExpr(fmt.Sprintf("attributes = $%d::jsonb", idx), string(attrsJSON))
		b.idx++
	}
	if update.Modifiers != nil {
		modsJSON, err := json.Marshal(update.Modifiers)
		if err != nil {
			return nil, fmt.Errorf("update gear: marshal modifiers: %w", err)
		}
		idx := b.NextIdx()
		b.SetExpr(fmt.Sprintf("modifiers = $%d::jsonb", idx), string(modsJSON))
		b.idx++
	}
	if update.Scope != nil {
		b.Set("scope", *update.Scope)
	}
	if update.Notes != nil {
		b.Set("notes", *update.Notes)
	}
	if update.WeightG != nil {
		b.Set("weight_g", *update.WeightG)
	}

	// Open a tx so the gear UPDATE and component diff land atomically.
	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("update gear: begin tx: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	var g *model.Gear
	if b.HasSets() {
		args := append(b.Args(), id)
		query := fmt.Sprintf(
			`UPDATE gear SET %s WHERE id = $%d RETURNING `+gearSelectCols,
			b.Clause(), b.NextIdx(),
		)
		g, err = scanGear(tx.QueryRow(ctx, query, args...))
		if err != nil {
			return nil, fmt.Errorf("update gear: %w", err)
		}
		if g == nil {
			return nil, nil
		}
	} else {
		// No top-level fields changed — just look up the row inside the tx so
		// downstream component-apply still has a consistent owner snapshot.
		g, err = scanGear(tx.QueryRow(ctx,
			`SELECT `+gearSelectCols+` FROM gear WHERE id = $1`, id,
		))
		if err != nil {
			return nil, fmt.Errorf("update gear: lookup: %w", err)
		}
		if g == nil {
			return nil, nil
		}
	}

	if update.Components != nil {
		if err := db.ApplyGearComponents(ctx, tx, id, g.UserID, *update.Components); err != nil {
			return nil, fmt.Errorf("update gear: apply components: %w", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("update gear: commit: %w", err)
	}

	components, err := db.listComponents(ctx, id)
	if err != nil {
		return nil, err
	}
	g.Components = components
	return g, nil
}

func (db *DB) RetireGear(ctx context.Context, id string) error {
	tag, err := db.Pool.Exec(ctx, `UPDATE gear SET is_retired = true, status = 'retired' WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("retire gear: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrGearNotFound
	}
	return nil
}

// DeleteGear permanently removes a gear item from the database.
// FK constraints handle cascading:
//   - gear_components.gear_id ON DELETE CASCADE  -> components are deleted
//   - component_assignments.gear_id ON DELETE CASCADE -> assignments are deleted
//   - chain_rotation_groups.gear_id ON DELETE CASCADE -> rotation groups are deleted
//   - activity_gear_assignments.gear_id ON DELETE CASCADE -> activity-gear links are removed
//     (replaced the legacy activities.gear_id ON DELETE SET NULL after 000355)
func (db *DB) DeleteGear(ctx context.Context, userID, gearID string) error {
	tag, err := db.Pool.Exec(ctx,
		`DELETE FROM gear WHERE id = $1 AND user_id = $2`,
		gearID, userID,
	)
	if err != nil {
		return fmt.Errorf("delete gear: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrGearNotFound
	}
	return nil
}

// UpdateGearStatus changes the status of a gear item (active, stored, retired).
// userID is required to prevent cross-user updates (defense-in-depth; caller also checks ownership).
func (db *DB) UpdateGearStatus(ctx context.Context, id, userID, status string) error {
	isRetired := status == "retired"
	tag, err := db.Pool.Exec(ctx,
		`UPDATE gear SET status = $1, is_retired = $2 WHERE id = $3 AND user_id = $4`,
		status, isRetired, id, userID,
	)
	if err != nil {
		return fmt.Errorf("update gear status: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrGearNotFound
	}
	return nil
}

// --- Components ---

func (db *DB) listComponents(ctx context.Context, gearID string) ([]model.GearComponent, error) {
	rows, err := db.Pool.Query(ctx,
		`SELECT `+componentSelectCols+` FROM gear_components WHERE gear_id = $1 ORDER BY name`, gearID,
	)
	if err != nil {
		return nil, fmt.Errorf("list components: %w", err)
	}
	defer rows.Close()

	var items []model.GearComponent
	for rows.Next() {
		var c model.GearComponent
		if err := scanComponentRow(&c, rows); err != nil {
			return nil, fmt.Errorf("scan component: %w", err)
		}
		items = append(items, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list components: %w", err)
	}
	if items == nil {
		items = []model.GearComponent{}
	}
	return items, nil
}

func (db *DB) CreateComponent(ctx context.Context, gearID string, create *model.GearComponentCreate) (*model.GearComponent, error) {
	c, err := scanComponent(db.Pool.QueryRow(ctx,
		`INSERT INTO gear_components (gear_id, parent_component_id, name, type, brand, model,
		  purchase_price, purchase_date, weight_g, serial_number, notes, preset_gear_type_key)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		 RETURNING `+componentSelectCols,
		gearID, create.ParentComponentID, create.Name, create.Type, create.Brand, create.Model,
		create.PurchasePrice, create.PurchaseDate, create.WeightG, create.SerialNumber, create.Notes,
		create.PresetGearTypeKey,
	))
	if err != nil {
		return nil, fmt.Errorf("create component: %w", err)
	}
	return c, nil
}

// ApplyGearComponents reconciles the gear_components rows belonging to a gear
// item with the desired list. The gear/userID pair is verified explicitly on
// every UPDATE/DELETE so a malicious payload cannot edit another user's
// components by sending a foreign id.
//
// Diff semantics:
//   - row in DB but not in desired (matched by id) → DELETE
//   - row in desired with empty/zero id            → INSERT
//   - row in both                                  → UPDATE the editable fields
//
// Server-only analytics columns (totals, activity counts) and timestamps are
// never written from the client.
func (db *DB) ApplyGearComponents(ctx context.Context, tx pgx.Tx, gearID, userID string, desired []model.GearComponentInput) error {
	// Defense-in-depth: confirm the gear belongs to userID before any write.
	var ownerID string
	if err := tx.QueryRow(ctx,
		`SELECT user_id FROM gear WHERE id = $1`, gearID,
	).Scan(&ownerID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrGearNotFound
		}
		return fmt.Errorf("apply components: lookup gear owner: %w", err)
	}
	if ownerID != userID {
		return ErrGearNotFound
	}

	existingRows, err := tx.Query(ctx,
		`SELECT id FROM gear_components WHERE gear_id = $1`, gearID,
	)
	if err != nil {
		return fmt.Errorf("apply components: list existing: %w", err)
	}
	existingIDs := map[string]struct{}{}
	for existingRows.Next() {
		var id string
		if err := existingRows.Scan(&id); err != nil {
			existingRows.Close()
			return fmt.Errorf("apply components: scan id: %w", err)
		}
		existingIDs[id] = struct{}{}
	}
	existingRows.Close()
	if err := existingRows.Err(); err != nil {
		return fmt.Errorf("apply components: iterate existing: %w", err)
	}

	desiredIDs := map[string]struct{}{}
	for _, item := range desired {
		if item.ID != "" {
			desiredIDs[item.ID] = struct{}{}
		}
	}

	// Phase 1: delete rows present in DB but not in desired.
	// Constrained to gear_id so a forged id cannot reach another gear.
	for id := range existingIDs {
		if _, keep := desiredIDs[id]; keep {
			continue
		}
		tag, err := tx.Exec(ctx,
			`DELETE FROM gear_components WHERE id = $1 AND gear_id = $2`,
			id, gearID,
		)
		if err != nil {
			return fmt.Errorf("apply components: delete %s: %w", id, err)
		}
		if tag.RowsAffected() == 0 {
			return fmt.Errorf("apply components: delete %s: row vanished mid-tx", id)
		}
	}

	// Phase 2: upsert each desired row.
	for _, item := range desired {
		attrs := item.Attributes
		if attrs == nil {
			attrs = map[string]any{}
		}
		attrsJSON, err := json.Marshal(attrs)
		if err != nil {
			return fmt.Errorf("apply components: marshal attributes: %w", err)
		}

		isRetired := false
		if item.IsRetired != nil {
			isRetired = *item.IsRetired
		}

		if item.ID == "" {
			// INSERT — id is generated server-side.
			if _, err := tx.Exec(ctx,
				`INSERT INTO gear_components (gear_id, parent_component_id, name, type, type_key, slot_key,
				   preset_gear_type_key,
				   brand, model, purchase_price, purchase_date, weight_g, serial_number,
				   is_retired, notes, attributes)
				 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16::jsonb)`,
				gearID, item.ParentComponentID, item.Name, item.Type, item.TypeKey, item.SlotKey,
				item.PresetGearTypeKey,
				item.Brand, item.Model, item.PurchasePrice, item.PurchaseDate, item.WeightG, item.SerialNumber,
				isRetired, item.Notes, string(attrsJSON),
			); err != nil {
				return fmt.Errorf("apply components: insert: %w", err)
			}
			continue
		}

		// UPDATE — gear_id is also in the WHERE so a forged id pointing at
		// another gear (and thus another user's data) cannot be touched.
		if _, ok := existingIDs[item.ID]; !ok {
			return fmt.Errorf("apply components: id %s not found on gear %s", item.ID, gearID)
		}
		tag, err := tx.Exec(ctx,
			`UPDATE gear_components
			    SET parent_component_id   = $1,
			        name                  = $2,
			        type                  = $3,
			        type_key              = $4,
			        slot_key              = $5,
			        preset_gear_type_key  = $6,
			        brand                 = $7,
			        model                 = $8,
			        purchase_price        = $9,
			        purchase_date         = $10,
			        weight_g              = $11,
			        serial_number         = $12,
			        is_retired            = $13,
			        notes                 = $14,
			        attributes            = $15::jsonb
			  WHERE id = $16 AND gear_id = $17`,
			item.ParentComponentID, item.Name, item.Type, item.TypeKey, item.SlotKey,
			item.PresetGearTypeKey,
			item.Brand, item.Model, item.PurchasePrice, item.PurchaseDate, item.WeightG, item.SerialNumber,
			isRetired, item.Notes, string(attrsJSON),
			item.ID, gearID,
		)
		if err != nil {
			return fmt.Errorf("apply components: update %s: %w", item.ID, err)
		}
		if tag.RowsAffected() == 0 {
			return fmt.Errorf("apply components: update %s: id not on gear %s", item.ID, gearID)
		}
	}

	return nil
}

// UpdateComponent applies a single-field PATCH to gear_components. nil
// pointers leave the column unchanged. presetGearTypeKey supports clearing the
// preset back to NULL by passing a pointer to the empty string.
func (db *DB) UpdateComponent(ctx context.Context, id string, name, typ *string, brand, modelStr, notes *string, isRetired *bool, purchasePrice *float64, purchaseDate, serialNumber *string, weightG *int, presetGearTypeKey *string) (*model.GearComponent, error) {
	b := newSetBuilder(1)

	if name != nil {
		b.Set("name", *name)
	}
	if typ != nil {
		b.Set("type", *typ)
	}
	if brand != nil {
		b.Set("brand", *brand)
	}
	if modelStr != nil {
		b.Set("model", *modelStr)
	}
	if notes != nil {
		b.Set("notes", *notes)
	}
	if isRetired != nil {
		b.Set("is_retired", *isRetired)
	}
	if purchasePrice != nil {
		b.Set("purchase_price", *purchasePrice)
	}
	if purchaseDate != nil {
		b.Set("purchase_date", *purchaseDate)
	}
	if serialNumber != nil {
		b.Set("serial_number", *serialNumber)
	}
	if weightG != nil {
		b.Set("weight_g", *weightG)
	}
	if presetGearTypeKey != nil {
		// Empty string clears the preset (column allows NULL).
		if *presetGearTypeKey == "" {
			b.Set("preset_gear_type_key", nil)
		} else {
			b.Set("preset_gear_type_key", *presetGearTypeKey)
		}
	}

	if !b.HasSets() {
		// No changes, just return the existing component
		c, err := scanComponent(db.Pool.QueryRow(ctx,
			`SELECT `+componentSelectCols+` FROM gear_components WHERE id = $1`, id,
		))
		if err != nil {
			return nil, fmt.Errorf("get component: %w", err)
		}
		return c, nil
	}

	args := append(b.Args(), id)
	query := fmt.Sprintf(
		`UPDATE gear_components SET %s WHERE id = $%d RETURNING `+componentSelectCols,
		b.Clause(), b.NextIdx(),
	)

	c, err := scanComponent(db.Pool.QueryRow(ctx, query, args...))
	if err != nil {
		return nil, fmt.Errorf("update component: %w", err)
	}
	return c, nil
}

func (db *DB) DeleteComponent(ctx context.Context, id string) error {
	tag, err := db.Pool.Exec(ctx, `DELETE FROM gear_components WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete component: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrComponentNotFound
	}
	return nil
}

// RemoveComponent soft-removes a component: sets status to 'stored', stamps
// removed_at, and closes any open component_assignment row. The component row
// is kept for history.
func (db *DB) RemoveComponent(ctx context.Context, id string) error {
	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin remove component: %w", err)
	}
	defer tx.Rollback(ctx)
	tag, err := tx.Exec(ctx, `
		UPDATE gear_components
		SET status = 'stored', removed_at = NOW()
		WHERE id = $1 AND status != 'retired'`, id)
	if err != nil {
		return fmt.Errorf("remove component: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrComponentNotFound
	}
	if _, err := tx.Exec(ctx, `
		UPDATE component_assignments
		SET removed_at = NOW()
		WHERE component_id = $1 AND removed_at IS NULL`, id); err != nil {
		return fmt.Errorf("close component assignment: %w", err)
	}
	return tx.Commit(ctx)
}

// UpdateComponentStatus transitions a component to a new status ('active',
// 'stored', or 'retired'). The UPDATE is scoped to components owned by userID
// via the gear table — callers therefore don't have to verify ownership
// separately for the status change.
//
// Side effects:
//   - stored / retired: stamp removed_at (if not already set) and close any
//     open component_assignments row.
//   - active: clear removed_at on gear_components and open a fresh
//     component_assignments row IF no open one already exists. This keeps
//     metric-attribution joins (LEFT JOIN component_assignments WHERE
//     removed_at IS NULL) returning the reactivated component.
func (db *DB) UpdateComponentStatus(ctx context.Context, id, userID, status string) error {
	if status != "active" && status != "stored" && status != "retired" {
		return fmt.Errorf("invalid component status %q", status)
	}
	isRetired := status == "retired"

	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin update component status: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	// Scope the UPDATE by the owning user via the gear table.
	tag, err := tx.Exec(ctx, `
		UPDATE gear_components gc
		SET status = $2,
		    is_retired = $3,
		    removed_at = CASE
		        WHEN $2 IN ('stored', 'retired') THEN COALESCE(gc.removed_at, NOW())
		        WHEN $2 = 'active' THEN NULL
		        ELSE gc.removed_at
		    END
		FROM gear g
		WHERE gc.id = $1 AND gc.gear_id = g.id AND g.user_id = $4`,
		id, status, isRetired, userID)
	if err != nil {
		return fmt.Errorf("update component status: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrComponentNotFound
	}

	if status == "stored" || status == "retired" {
		if _, err := tx.Exec(ctx, `
			UPDATE component_assignments
			SET removed_at = NOW()
			WHERE component_id = $1 AND removed_at IS NULL`, id); err != nil {
			return fmt.Errorf("close component assignment: %w", err)
		}
	}

	if status == "active" {
		// Open a fresh assignment row if none is currently open. Idempotent on
		// repeated reactivation of an already-active component.
		if _, err := tx.Exec(ctx, `
			INSERT INTO component_assignments (component_id, gear_id, installed_at)
			SELECT gc.id, gc.gear_id, NOW()
			FROM gear_components gc
			WHERE gc.id = $1
			  AND NOT EXISTS (
			    SELECT 1 FROM component_assignments ca
			    WHERE ca.component_id = gc.id AND ca.removed_at IS NULL
			  )`, id); err != nil {
			return fmt.Errorf("open component assignment: %w", err)
		}
	}

	return tx.Commit(ctx)
}

// --- Component Transfer ---

// TransferComponent moves a component from one gear to another, closing the
// current assignment and creating a new one.
func (db *DB) TransferComponent(ctx context.Context, componentID, fromGearID, toGearID string) error {
	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	// Close current assignment
	_, err = tx.Exec(ctx,
		`UPDATE component_assignments SET removed_at = NOW() WHERE component_id = $1 AND removed_at IS NULL`,
		componentID,
	)
	if err != nil {
		return fmt.Errorf("close assignment: %w", err)
	}

	// Create new assignment
	_, err = tx.Exec(ctx,
		`INSERT INTO component_assignments (component_id, gear_id, installed_at)
		 VALUES ($1, $2, NOW())`,
		componentID, toGearID,
	)
	if err != nil {
		return fmt.Errorf("create assignment: %w", err)
	}

	// Update component's gear_id
	_, err = tx.Exec(ctx,
		`UPDATE gear_components SET gear_id = $1 WHERE id = $2`,
		toGearID, componentID,
	)
	if err != nil {
		return fmt.Errorf("update component gear: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit transfer: %w", err)
	}

	// Update stats for both gears (outside tx, best-effort).
	// Ownership of both gears was verified by the handler before TransferComponent
	// was called (VerifyGearOwnership for toGearID, component-to-gear path for fromGearID).
	_ = db.UpdateGearStatsOwnerVerified(ctx, fromGearID)
	_ = db.UpdateGearStatsOwnerVerified(ctx, toGearID)

	return nil
}

// ComponentTransferEligibility holds the schema-relevant fields needed to
// decide whether a component may be transferred to a target gear.
type ComponentTransferEligibility struct {
	Component  gearcompat.Component
	TargetGear gearcompat.Gear
	// FromGearID is the current (source) gear the component is attached to.
	// Used by the handler to record the transfer correctly.
	FromGearID string
}

// GetComponentTransferEligibilityOwnerVerified fetches the schema-classification
// fields for componentID and the target gear's type key. Returns a non-nil error
// when either row is not found or the query fails.
//
// The caller MUST verify ownership of componentID and toGearID (e.g. via
// VerifyComponentOwnership / VerifyGearOwnership) before invoking this function.
// This function trusts the caller — it does not re-check user_id ownership.
func (db *DB) GetComponentTransferEligibilityOwnerVerified(ctx context.Context, componentID, toGearID string) (ComponentTransferEligibility, error) {
	// Fetch component classification (type_key + preset_gear_type_key) and the
	// current gear's type in one query.
	var compTypeKey, presetGearTypeKey *string
	var sourceGearTypeKey string
	var fromGearID string
	err := db.Pool.QueryRow(ctx,
		`SELECT gc.type_key, gc.preset_gear_type_key, g.type, g.id
		 FROM gear_components gc
		 JOIN gear g ON g.id = gc.gear_id
		 WHERE gc.id = $1`,
		componentID,
	).Scan(&compTypeKey, &presetGearTypeKey, &sourceGearTypeKey, &fromGearID)
	if err != nil {
		return ComponentTransferEligibility{}, fmt.Errorf("get component eligibility: %w", err)
	}

	// Fetch target gear type.
	var targetTypeKey string
	err = db.Pool.QueryRow(ctx,
		`SELECT type FROM gear WHERE id = $1`,
		toGearID,
	).Scan(&targetTypeKey)
	if err != nil {
		return ComponentTransferEligibility{}, fmt.Errorf("get target gear type: %w", err)
	}

	comp := gearcompat.Component{
		SourceGearTypeKey: sourceGearTypeKey,
		PresetGearTypeKey: presetGearTypeKey,
	}
	if compTypeKey != nil {
		comp.TypeKey = *compTypeKey
	}

	return ComponentTransferEligibility{
		Component:  comp,
		TargetGear: gearcompat.Gear{TypeKey: targetTypeKey},
		FromGearID: fromGearID,
	}, nil
}

// --- Component History ---

// GetComponentHistoryOwnerVerified returns the ordered assignment history for
// componentID (most-recent first).
//
// The caller MUST verify ownership of componentID via VerifyComponentOwnership
// (or via an upstream RLS-scoped query) before invoking this function.
// This function trusts the caller — it does not re-check user_id ownership.
func (db *DB) GetComponentHistoryOwnerVerified(ctx context.Context, componentID string) ([]model.ComponentAssignment, error) {
	rows, err := db.Pool.Query(ctx,
		`SELECT id, component_id, gear_id, installed_at::text, removed_at::text,
		 COALESCE(distance_m, 0), COALESCE(elevation_gain_m, 0),
		 COALESCE(elevation_loss_m, 0), COALESCE(moving_time_s, 0),
		 COALESCE(time_s, 0), COALESCE(activities_count, 0), notes,
		 COALESCE(attributes, '{}'::jsonb)
		 FROM component_assignments WHERE component_id = $1
		 ORDER BY installed_at DESC`, componentID,
	)
	if err != nil {
		return nil, fmt.Errorf("get component history: %w", err)
	}
	defer rows.Close()

	var items []model.ComponentAssignment
	for rows.Next() {
		var a model.ComponentAssignment
		var attrsJSON []byte
		if err := rows.Scan(
			&a.ID, &a.ComponentID, &a.GearID, &a.InstalledAt, &a.RemovedAt,
			&a.DistanceM, &a.ElevationGainM, &a.ElevationLossM, &a.MovingTimeS,
			&a.TimeS, &a.ActivitiesCount, &a.Notes, &attrsJSON,
		); err != nil {
			return nil, fmt.Errorf("scan component assignment: %w", err)
		}
		if len(attrsJSON) > 0 && string(attrsJSON) != "{}" {
			if err := json.Unmarshal(attrsJSON, &a.Attributes); err != nil {
				return nil, fmt.Errorf("scan component assignment attributes: %w", err)
			}
		}
		if a.Attributes == nil {
			a.Attributes = map[string]any{}
		}
		items = append(items, a)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("get component history: %w", err)
	}
	if items == nil {
		items = []model.ComponentAssignment{}
	}
	return items, nil
}

// --- Chain Rotation ---

func (db *DB) GetChainRotation(ctx context.Context, gearID string) (*model.ChainRotationGroup, error) {
	var g model.ChainRotationGroup
	err := db.Pool.QueryRow(ctx,
		`SELECT id, user_id, gear_id, name, swap_interval_km, swap_interval_hm,
		        swap_interval_metric_key, swap_interval_value, created_at::text
		 FROM chain_rotation_groups WHERE gear_id = $1`, gearID,
	).Scan(&g.ID, &g.UserID, &g.GearID, &g.Name, &g.SwapIntervalKM, &g.SwapIntervalHM,
		&g.SwapIntervalMetricKey, &g.SwapIntervalValue, &g.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get chain rotation: %w", err)
	}

	members, err := db.listChainMembers(ctx, g.ID)
	if err != nil {
		return nil, err
	}
	g.Members = members
	return &g, nil
}

func (db *DB) listChainMembers(ctx context.Context, groupID string) ([]model.ChainRotationMember, error) {
	rows, err := db.Pool.Query(ctx,
		`SELECT m.id, m.group_id, m.component_id, COALESCE(gc.name, ''), m.position, m.is_active,
		 m.total_distance_m, m.total_elevation_gain_m, m.total_elevation_loss_m,
		 m.total_moving_time_s, m.total_time_s, m.total_activities, m.last_swapped_at::text
		 FROM chain_rotation_members m
		 LEFT JOIN gear_components gc ON gc.id = m.component_id
		 WHERE m.group_id = $1
		 ORDER BY m.position`, groupID,
	)
	if err != nil {
		return nil, fmt.Errorf("list chain members: %w", err)
	}
	defer rows.Close()

	var items []model.ChainRotationMember
	for rows.Next() {
		var m model.ChainRotationMember
		if err := rows.Scan(
			&m.ID, &m.GroupID, &m.ComponentID, &m.ComponentName, &m.Position, &m.IsActive,
			&m.TotalDistanceM, &m.TotalElevationGainM, &m.TotalElevationLossM,
			&m.TotalMovingTimeS, &m.TotalTimeS, &m.TotalActivities, &m.LastSwappedAt,
		); err != nil {
			return nil, fmt.Errorf("scan chain member: %w", err)
		}
		items = append(items, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list chain members: %w", err)
	}
	if items == nil {
		items = []model.ChainRotationMember{}
	}
	return items, nil
}

// ChainRotationGroupCreate is the input for creating a chain rotation group.
type ChainRotationGroupCreate struct {
	Name                  string   `json:"name"`
	SwapIntervalKM        *float64 `json:"swap_interval_km,omitempty"`
	SwapIntervalHM        *float64 `json:"swap_interval_hm,omitempty"`
	SwapIntervalMetricKey *string  `json:"swap_interval_metric_key,omitempty"`
	SwapIntervalValue     *float64 `json:"swap_interval_value,omitempty"`
	ComponentIDs          []string `json:"component_ids"`
}

func (db *DB) CreateChainRotation(ctx context.Context, userID, gearID string, create *ChainRotationGroupCreate) (*model.ChainRotationGroup, error) {
	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	var g model.ChainRotationGroup
	err = tx.QueryRow(ctx,
		`INSERT INTO chain_rotation_groups (user_id, gear_id, name, swap_interval_km, swap_interval_hm, swap_interval_metric_key, swap_interval_value)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)
		 RETURNING id, user_id, gear_id, name, swap_interval_km, swap_interval_hm,
		           swap_interval_metric_key, swap_interval_value, created_at::text`,
		userID, gearID, create.Name, create.SwapIntervalKM, create.SwapIntervalHM,
		create.SwapIntervalMetricKey, create.SwapIntervalValue,
	).Scan(&g.ID, &g.UserID, &g.GearID, &g.Name, &g.SwapIntervalKM, &g.SwapIntervalHM,
		&g.SwapIntervalMetricKey, &g.SwapIntervalValue, &g.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("create chain rotation: %w", err)
	}

	// Create members
	for i, compID := range create.ComponentIDs {
		isActive := i == 0 // First component is active
		_, err = tx.Exec(ctx,
			`INSERT INTO chain_rotation_members (group_id, component_id, position, is_active)
			 VALUES ($1, $2, $3, $4)`,
			g.ID, compID, i+1, isActive,
		)
		if err != nil {
			return nil, fmt.Errorf("create chain member: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit chain rotation: %w", err)
	}

	// Reload with members
	return db.GetChainRotation(ctx, gearID)
}

// UpdateChainRotation updates the swap-interval fields and, when name is non-nil,
// renames the rotation group. nil-valued pointers leave the corresponding column
// unchanged.
func (db *DB) UpdateChainRotation(ctx context.Context, groupID string, name *string, intervalKM, intervalHM *float64, intervalMetricKey *string, intervalValue *float64) error {
	b := newSetBuilder(1)
	if name != nil {
		b.Set("name", *name)
	}
	// The interval columns are always written (nullable) to keep the legacy
	// "clear by sending null" contract intact.
	b.Set("swap_interval_km", intervalKM)
	b.Set("swap_interval_hm", intervalHM)
	b.Set("swap_interval_metric_key", intervalMetricKey)
	b.Set("swap_interval_value", intervalValue)

	args := append(b.Args(), groupID)
	query := fmt.Sprintf(
		`UPDATE chain_rotation_groups SET %s WHERE id = $%d`,
		b.Clause(), b.NextIdx(),
	)
	if _, err := db.Pool.Exec(ctx, query, args...); err != nil {
		return fmt.Errorf("update chain rotation: %w", err)
	}
	return nil
}

func (db *DB) DeleteChainRotation(ctx context.Context, groupID string) error {
	tag, err := db.Pool.Exec(ctx, `DELETE FROM chain_rotation_groups WHERE id = $1`, groupID)
	if err != nil {
		return fmt.Errorf("delete chain rotation: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrChainRotationNotFound
	}
	return nil
}

// SwapChain deactivates the current active member and activates the next one in position order.
func (db *DB) SwapChain(ctx context.Context, groupID string) error {
	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	// Find current active position
	var currentPos int
	var totalMembers int
	err = tx.QueryRow(ctx,
		`SELECT position FROM chain_rotation_members WHERE group_id = $1 AND is_active = true`, groupID,
	).Scan(&currentPos)
	if errors.Is(err, pgx.ErrNoRows) {
		currentPos = 0
	} else if err != nil {
		return fmt.Errorf("find active member: %w", err)
	}

	err = tx.QueryRow(ctx,
		`SELECT COUNT(*) FROM chain_rotation_members WHERE group_id = $1`, groupID,
	).Scan(&totalMembers)
	if err != nil {
		return fmt.Errorf("count members: %w", err)
	}
	if totalMembers == 0 {
		return fmt.Errorf("no members in rotation")
	}

	// Deactivate current
	_, err = tx.Exec(ctx,
		`UPDATE chain_rotation_members SET is_active = false, last_swapped_at = NOW()
		 WHERE group_id = $1 AND is_active = true`, groupID,
	)
	if err != nil {
		return fmt.Errorf("deactivate current: %w", err)
	}

	// Activate next (wrap around)
	nextPos := currentPos%totalMembers + 1
	_, err = tx.Exec(ctx,
		`UPDATE chain_rotation_members SET is_active = true
		 WHERE group_id = $1 AND position = $2`, groupID, nextPos,
	)
	if err != nil {
		return fmt.Errorf("activate next: %w", err)
	}

	return tx.Commit(ctx)
}

// --- Service Events ---

// ServiceEventCreate is the input for creating a service event.
type ServiceEventCreate struct {
	IntervalID  *string  `json:"interval_id,omitempty"`
	PerformedAt string   `json:"performed_at"`
	DistanceAtM *float64 `json:"distance_at_m,omitempty"`
	Description *string  `json:"description,omitempty"`
	Cost        *float64 `json:"cost,omitempty"`
	Notes       *string  `json:"notes,omitempty"`
}

func (db *DB) ListServiceEvents(ctx context.Context, componentID string) ([]model.GearServiceEvent, error) {
	rows, err := db.Pool.Query(ctx,
		`SELECT id, component_id, interval_id, performed_at::text, distance_at_m,
		 description, cost, notes, metric_snapshot, created_at::text
		 FROM gear_service_events WHERE component_id = $1
		 ORDER BY performed_at DESC`, componentID,
	)
	if err != nil {
		return nil, fmt.Errorf("list service events: %w", err)
	}
	defer rows.Close()

	var items []model.GearServiceEvent
	for rows.Next() {
		var e model.GearServiceEvent
		if err := rows.Scan(
			&e.ID, &e.ComponentID, &e.IntervalID, &e.PerformedAt, &e.DistanceAtM,
			&e.Description, &e.Cost, &e.Notes, &e.MetricSnapshot, &e.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan service event: %w", err)
		}
		items = append(items, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list service events: %w", err)
	}
	if items == nil {
		items = []model.GearServiceEvent{}
	}
	return items, nil
}

func (db *DB) CreateServiceEvent(ctx context.Context, componentID string, create *ServiceEventCreate) (*model.GearServiceEvent, error) {
	var e model.GearServiceEvent
	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin service event: %w", err)
	}
	defer tx.Rollback(ctx)

	var metricKey *string
	if create.IntervalID != nil {
		var key string
		if err := tx.QueryRow(ctx, `
			SELECT gsi.metric_key
			FROM gear_service_intervals gsi
			WHERE gsi.id = $1 AND gsi.component_id = $2`,
			*create.IntervalID, componentID,
		).Scan(&key); err != nil {
			return nil, fmt.Errorf("verify service interval: %w", err)
		}
		metricKey = &key
	}

	err = tx.QueryRow(ctx,
		`WITH snapshot AS (
		     SELECT jsonb_build_object(
		         'distance_m', COALESCE(total_distance_m, 0),
		         'moving_time_s', COALESCE(total_moving_time_s, 0),
		         'elapsed_time_s', COALESCE(total_time_s, 0),
		         'activity_count', COALESCE(total_activities, 0),
		         'elevation_gain_m', COALESCE(total_elevation_gain_m, 0),
		         'elevation_loss_m', COALESCE(total_elevation_loss_m, 0)
		     ) AS value
		     FROM gear_components
		     WHERE id = $1
		 )
		 INSERT INTO gear_service_events (component_id, interval_id, performed_at, distance_at_m, description, cost, notes, metric_snapshot)
		 SELECT $1, $2, $3, $4, $5, $6, $7, value FROM snapshot
		 RETURNING id, component_id, interval_id, performed_at::text, distance_at_m,
		   description, cost, notes, metric_snapshot, created_at::text`,
		componentID, create.IntervalID, create.PerformedAt, create.DistanceAtM,
		create.Description, create.Cost, create.Notes,
	).Scan(
		&e.ID, &e.ComponentID, &e.IntervalID, &e.PerformedAt, &e.DistanceAtM,
		&e.Description, &e.Cost, &e.Notes, &e.MetricSnapshot, &e.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("create service event: %w", err)
	}

	// Reset the interval's last_service_metric_value to the snapshot value for that metric.
	if metricKey != nil {
		value := e.MetricSnapshot[*metricKey]
		_, err = tx.Exec(ctx, `
			UPDATE gear_service_intervals
			SET last_service_at = $1,
			    last_service_metric_value = $2,
			    last_service_distance_m = CASE WHEN metric_key = 'distance_m' THEN $2 ELSE last_service_distance_m END
			WHERE id = $3 AND component_id = $4`,
			create.PerformedAt, value, *create.IntervalID, componentID,
		)
		if err != nil {
			return nil, fmt.Errorf("reset service interval: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit service event: %w", err)
	}
	return &e, nil
}

func (db *DB) DeleteServiceEvent(ctx context.Context, eventID string) error {
	tag, err := db.Pool.Exec(ctx, `DELETE FROM gear_service_events WHERE id = $1`, eventID)
	if err != nil {
		return fmt.Errorf("delete service event: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrServiceEventNotFound
	}
	return nil
}

// --- Service Intervals ---

const serviceIntervalSelectCols = `id, component_id, name, interval_type, metric_key, interval_value,
	is_recurring, last_service_at::text, last_service_distance_m, last_service_metric_value, next_due_at::text`

func scanServiceInterval(row pgx.Row) (*model.GearServiceInterval, error) {
	var interval model.GearServiceInterval
	err := row.Scan(
		&interval.ID, &interval.ComponentID, &interval.Name, &interval.IntervalType, &interval.MetricKey,
		&interval.IntervalValue, &interval.IsRecurring, &interval.LastServiceAt,
		&interval.LastServiceDistanceM, &interval.LastServiceMetricValue, &interval.NextDueAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &interval, nil
}

func (db *DB) ListServiceIntervals(ctx context.Context, componentID string) ([]model.GearServiceInterval, error) {
	rows, err := db.Pool.Query(ctx,
		`SELECT `+serviceIntervalSelectCols+`
		 FROM gear_service_intervals
		 WHERE component_id = $1
		 ORDER BY name`, componentID)
	if err != nil {
		return nil, fmt.Errorf("list service intervals: %w", err)
	}
	defer rows.Close()
	var items []model.GearServiceInterval
	for rows.Next() {
		var item model.GearServiceInterval
		if err := rows.Scan(
			&item.ID, &item.ComponentID, &item.Name, &item.IntervalType, &item.MetricKey,
			&item.IntervalValue, &item.IsRecurring, &item.LastServiceAt,
			&item.LastServiceDistanceM, &item.LastServiceMetricValue, &item.NextDueAt,
		); err != nil {
			return nil, fmt.Errorf("scan service interval: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list service intervals: %w", err)
	}
	if items == nil {
		items = []model.GearServiceInterval{}
	}
	return items, nil
}

func (db *DB) CreateServiceInterval(ctx context.Context, componentID string, create *model.GearServiceIntervalCreate) (*model.GearServiceInterval, error) {
	if !gearmetrics.IsServiceMetric(create.MetricKey) {
		return nil, fmt.Errorf("unsupported service metric %q", create.MetricKey)
	}
	if create.IntervalValue <= 0 {
		return nil, fmt.Errorf("interval value must be positive")
	}
	isRecurring := true
	if create.IsRecurring != nil {
		isRecurring = *create.IsRecurring
	}
	interval, err := scanServiceInterval(db.Pool.QueryRow(ctx, `
		INSERT INTO gear_service_intervals (component_id, name, interval_type, metric_key, interval_value, is_recurring, last_service_at)
		VALUES ($1, $2, $3, $3, $4, $5, $6)
		RETURNING `+serviceIntervalSelectCols,
		componentID, create.Name, create.MetricKey, create.IntervalValue, isRecurring, create.LastServiceAt,
	))
	if err != nil {
		return nil, fmt.Errorf("create service interval: %w", err)
	}
	return interval, nil
}

func (db *DB) UpdateServiceInterval(ctx context.Context, intervalID string, update *model.GearServiceIntervalUpdate) (*model.GearServiceInterval, error) {
	b := newSetBuilder(1)
	if update.Name != nil {
		b.Set("name", *update.Name)
	}
	if update.MetricKey != nil {
		if !gearmetrics.IsServiceMetric(*update.MetricKey) {
			return nil, fmt.Errorf("unsupported service metric %q", *update.MetricKey)
		}
		b.Set("metric_key", *update.MetricKey)
		b.Set("interval_type", *update.MetricKey)
	}
	if update.IntervalValue != nil {
		if *update.IntervalValue <= 0 {
			return nil, fmt.Errorf("interval value must be positive")
		}
		b.Set("interval_value", *update.IntervalValue)
	}
	if update.IsRecurring != nil {
		b.Set("is_recurring", *update.IsRecurring)
	}
	if update.LastServiceAt != nil {
		b.Set("last_service_at", *update.LastServiceAt)
	}
	if !b.HasSets() {
		return scanServiceInterval(db.Pool.QueryRow(ctx, `SELECT `+serviceIntervalSelectCols+` FROM gear_service_intervals WHERE id = $1`, intervalID))
	}
	args := append(b.Args(), intervalID)
	query := fmt.Sprintf(`UPDATE gear_service_intervals SET %s WHERE id = $%d RETURNING `+serviceIntervalSelectCols, b.Clause(), b.NextIdx())
	interval, err := scanServiceInterval(db.Pool.QueryRow(ctx, query, args...))
	if err != nil {
		return nil, fmt.Errorf("update service interval: %w", err)
	}
	return interval, nil
}

func (db *DB) DeleteServiceInterval(ctx context.Context, intervalID string) error {
	tag, err := db.Pool.Exec(ctx, `DELETE FROM gear_service_intervals WHERE id = $1`, intervalID)
	if err != nil {
		return fmt.Errorf("delete service interval: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrServiceEventNotFound
	}
	return nil
}

// --- Component Presets ---

func (db *DB) ListComponentPresets(ctx context.Context, gearType string) ([]model.ComponentTypePreset, error) {
	query := `SELECT id, name, category, gear_type, default_interval_type, default_interval_value,
		track_elevation, sort_order
		FROM component_type_presets`
	var args []any
	if gearType != "" {
		query += ` WHERE gear_type = $1 OR gear_type IS NULL`
		args = append(args, gearType)
	}
	query += ` ORDER BY sort_order, name`

	rows, err := db.Pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list component presets: %w", err)
	}
	defer rows.Close()

	var items []model.ComponentTypePreset
	for rows.Next() {
		var p model.ComponentTypePreset
		if err := rows.Scan(
			&p.ID, &p.Name, &p.Category, &p.GearType, &p.DefaultIntervalType,
			&p.DefaultIntervalValue, &p.TrackElevation, &p.SortOrder,
		); err != nil {
			return nil, fmt.Errorf("scan component preset: %w", err)
		}
		items = append(items, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list component presets: %w", err)
	}
	if items == nil {
		items = []model.ComponentTypePreset{}
	}
	return items, nil
}

// --- Maintenance ---

func (db *DB) GetMaintenanceAlerts(ctx context.Context, userID string) ([]model.MaintenanceAlert, error) {
	rows, err := db.Pool.Query(ctx,
		`WITH evaluated AS (
		   SELECT
		     gsi.id AS interval_id,
		     g.id AS gear_id,
		     g.name AS gear_name,
		     gc.id AS component_id,
		     gc.name AS component_name,
		     gsi.name AS interval_name,
		     COALESCE(gsi.metric_key, gsi.interval_type) AS metric_key,
		     gsi.interval_value AS threshold_value,
		     GREATEST(
		       CASE COALESCE(gsi.metric_key, gsi.interval_type)
		         WHEN 'distance_m' THEN gc.total_distance_m - COALESCE(gsi.last_service_metric_value, 0)
		         WHEN 'moving_time_s' THEN gc.total_moving_time_s - COALESCE(gsi.last_service_metric_value, 0)
		         WHEN 'elapsed_time_s' THEN gc.total_time_s - COALESCE(gsi.last_service_metric_value, 0)
		         WHEN 'activity_count' THEN gc.total_activities::double precision - COALESCE(gsi.last_service_metric_value, 0)
		         WHEN 'elapsed_days' THEN EXTRACT(EPOCH FROM (now() - COALESCE(gsi.last_service_at, gc.installed_at, now()))) / 86400.0
		         WHEN 'elevation_gain_m' THEN gc.total_elevation_gain_m - COALESCE(gsi.last_service_metric_value, 0)
		         WHEN 'elevation_loss_m' THEN gc.total_elevation_loss_m - COALESCE(gsi.last_service_metric_value, 0)
		         ELSE 0
		       END,
		       0
		     ) AS current_value
		   FROM gear_service_intervals gsi
		   JOIN gear_components gc ON gc.id = gsi.component_id
		   JOIN gear g ON g.id = gc.gear_id
		   WHERE g.user_id = $1
		     AND g.status != 'retired'
		     AND gc.is_retired = false
		     AND gsi.interval_value > 0
		 )
		 SELECT interval_id, gear_id, gear_name, component_id, component_name,
		        interval_name, metric_key,
		        current_value >= threshold_value AS is_overdue,
		        current_value, threshold_value
		 FROM evaluated
		 WHERE current_value >= threshold_value * 0.9
		 ORDER BY (current_value >= threshold_value) DESC,
		          current_value / NULLIF(threshold_value, 0) DESC
		 LIMIT 20`, userID,
	)
	if err != nil {
		return nil, fmt.Errorf("get maintenance alerts: %w", err)
	}
	defer rows.Close()

	var alerts []model.MaintenanceAlert
	for rows.Next() {
		var a model.MaintenanceAlert
		if err := rows.Scan(
			&a.IntervalID, &a.GearID, &a.GearName, &a.ComponentID, &a.ComponentName,
			&a.IntervalName, &a.IntervalType, &a.IsOverdue, &a.CurrentValue, &a.ThresholdValue,
		); err != nil {
			return nil, fmt.Errorf("scan maintenance alert: %w", err)
		}
		alerts = append(alerts, a)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("get maintenance alerts: %w", err)
	}
	if alerts == nil {
		alerts = []model.MaintenanceAlert{}
	}
	return alerts, nil
}

// --- Automation Rules ---

const ruleSelectCols = `id, user_id, name, is_active, priority, conditions, conditions_logic, condition_groups, actions, times_applied, created_at, is_system, system_type, trigger_event, stop_on_match, apply_retroactively, last_evaluated_at, last_matched_count, last_warning_at, last_warning_code, last_warning_message, branches`

func scanRule(row pgx.Row) (*model.AutomationRule, error) {
	var r model.AutomationRule
	err := row.Scan(
		&r.ID, &r.UserID, &r.Name, &r.IsActive, &r.Priority,
		&r.Conditions, &r.ConditionsLogic, &r.ConditionGroups,
		&r.Actions, &r.TimesApplied, &r.CreatedAt,
		&r.IsSystem, &r.SystemType,
		&r.TriggerEvent, &r.StopOnMatch, &r.ApplyRetroactive,
		&r.LastEvaluatedAt, &r.LastMatchedCount,
		&r.LastWarningAt, &r.LastWarningCode, &r.LastWarningMsg,
		&r.Branches,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &r, nil
}

func (db *DB) ListRules(ctx context.Context, userID string) ([]model.AutomationRule, error) {
	readonlySyncs := readonlySystemModifierTagSyncs()
	args := []any{userID}
	caseParts := make([]string, 0, len(readonlySyncs))
	for _, sync := range readonlySyncs {
		systemTypeParam := len(args) + 1
		args = append(args, sync.SystemType)
		tagNameParam := len(args) + 1
		args = append(args, sync.SystemTagName)
		caseParts = append(caseParts, fmt.Sprintf(
			"WHEN ar.system_type = $%d THEN COALESCE((SELECT t.activity_count FROM tags t WHERE t.user_id = ar.user_id AND t.name = $%d LIMIT 1), 0)",
			systemTypeParam, tagNameParam,
		))
	}
	timesAppliedExpr := "ar.times_applied"
	if len(caseParts) > 0 {
		timesAppliedExpr = "CASE " + strings.Join(caseParts, " ") + " ELSE ar.times_applied END"
	}
	query := fmt.Sprintf(`SELECT ar.id, ar.user_id, ar.name, ar.is_active, ar.priority,
			        ar.conditions, ar.conditions_logic, ar.condition_groups, ar.actions,
				        %s,
				        ar.created_at, ar.is_system, ar.system_type,
				        ar.trigger_event, ar.stop_on_match, ar.apply_retroactively,
				        ar.last_evaluated_at, ar.last_matched_count,
				        ar.last_warning_at, ar.last_warning_code, ar.last_warning_message,
				        ar.branches
				 FROM automation_rules ar WHERE ar.user_id = $1 ORDER BY ar.priority, ar.name`, timesAppliedExpr)
	rows, err := db.Pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list rules: %w", err)
	}
	defer rows.Close()

	var items []model.AutomationRule
	for rows.Next() {
		var r model.AutomationRule
		if err := rows.Scan(
			&r.ID, &r.UserID, &r.Name, &r.IsActive, &r.Priority,
			&r.Conditions, &r.ConditionsLogic, &r.ConditionGroups,
			&r.Actions, &r.TimesApplied, &r.CreatedAt,
			&r.IsSystem, &r.SystemType,
			&r.TriggerEvent, &r.StopOnMatch, &r.ApplyRetroactive,
			&r.LastEvaluatedAt, &r.LastMatchedCount,
			&r.LastWarningAt, &r.LastWarningCode, &r.LastWarningMsg,
			&r.Branches,
		); err != nil {
			return nil, fmt.Errorf("scan rule: %w", err)
		}
		items = append(items, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list rules: %w", err)
	}
	if items == nil {
		items = []model.AutomationRule{}
	}
	return items, nil
}

func (db *DB) CreateRule(ctx context.Context, userID string, create *model.AutomationRuleCreate) (*model.AutomationRule, error) {
	condLogic := create.ConditionsLogic
	if condLogic == "" {
		condLogic = "and"
	}
	triggerEvent := create.TriggerEvent
	if triggerEvent == "" {
		triggerEvent = "on_import"
	}
	if err := validateAutomationRuleJSON(rules.SportTaxonomyRuleJSON{
		Conditions:      create.Conditions,
		ConditionGroups: create.ConditionGroups,
		Actions:         create.Actions,
		Branches:        create.Branches,
	}); err != nil {
		return nil, fmt.Errorf("validate rule: %w", err)
	}
	var r model.AutomationRule
	err := db.Pool.QueryRow(ctx,
		`INSERT INTO automation_rules (user_id, name, priority, conditions, conditions_logic, condition_groups, actions, trigger_event, stop_on_match, apply_retroactively, branches)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		 RETURNING `+ruleSelectCols,
		userID, create.Name, create.Priority, create.Conditions, condLogic, create.ConditionGroups, create.Actions,
		triggerEvent, create.StopOnMatch, create.ApplyRetroactive, create.Branches,
	).Scan(
		&r.ID, &r.UserID, &r.Name, &r.IsActive, &r.Priority,
		&r.Conditions, &r.ConditionsLogic, &r.ConditionGroups,
		&r.Actions, &r.TimesApplied, &r.CreatedAt,
		&r.IsSystem, &r.SystemType,
		&r.TriggerEvent, &r.StopOnMatch, &r.ApplyRetroactive,
		&r.LastEvaluatedAt, &r.LastMatchedCount,
		&r.LastWarningAt, &r.LastWarningCode, &r.LastWarningMsg,
		&r.Branches,
	)
	if err != nil {
		return nil, fmt.Errorf("create rule: %w", err)
	}
	// Refresh has_automation for all tags of this user
	db.Pool.Exec(ctx, `
		UPDATE tags t SET has_automation = EXISTS (
			SELECT 1 FROM automation_rules ar
			WHERE ar.user_id = t.user_id AND ar.is_active = true
			  AND (ar.actions::text LIKE '%' || t.id::text || '%' OR COALESCE(ar.branches::text, '') LIKE '%' || t.id::text || '%')
		) WHERE t.user_id = $1`, userID) //nolint:errcheck
	return &r, nil
}

func (db *DB) UpdateRule(ctx context.Context, id string, userID string, update *model.AutomationRuleUpdate) (*model.AutomationRule, error) {
	// Server-side guard: sync rules are fully read-only.
	var sysType *string
	var currentConditions, currentConditionGroups, currentActions, currentBranches json.RawMessage
	if err := db.Pool.QueryRow(ctx,
		`SELECT system_type, conditions, condition_groups, actions, branches FROM automation_rules WHERE id = $1 AND user_id = $2`,
		id, userID,
	).Scan(&sysType, &currentConditions, &currentConditionGroups, &currentActions, &currentBranches); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	if isReadonlySyncRuleType(sysType) {
		return nil, fmt.Errorf("this system rule is read-only")
	}

	b := newSetBuilder(1)

	if update.Name != nil {
		b.Set("name", *update.Name)
	}
	if update.IsActive != nil {
		b.Set("is_active", *update.IsActive)
	}
	if update.Priority != nil {
		b.Set("priority", *update.Priority)
	}
	if update.Conditions != nil {
		b.Set("conditions", *update.Conditions)
	}
	if update.ConditionsLogic != nil {
		b.Set("conditions_logic", *update.ConditionsLogic)
	}
	if update.ConditionGroups != nil {
		b.Set("condition_groups", *update.ConditionGroups)
	}
	if update.Actions != nil {
		b.Set("actions", *update.Actions)
	}
	if update.TriggerEvent != nil {
		b.Set("trigger_event", *update.TriggerEvent)
	}
	if update.StopOnMatch != nil {
		b.Set("stop_on_match", *update.StopOnMatch)
	}
	if update.ApplyRetroactive != nil {
		b.Set("apply_retroactively", *update.ApplyRetroactive)
	}
	if update.Branches != nil {
		b.Set("branches", update.Branches)
	}

	effective := rules.SportTaxonomyRuleJSON{
		Conditions:      currentConditions,
		ConditionGroups: currentConditionGroups,
		Actions:         currentActions,
		Branches:        currentBranches,
	}
	if update.Conditions != nil {
		effective.Conditions = *update.Conditions
	}
	if update.ConditionGroups != nil {
		effective.ConditionGroups = *update.ConditionGroups
	}
	if update.Actions != nil {
		effective.Actions = *update.Actions
	}
	if update.Branches != nil {
		effective.Branches = update.Branches
	}
	if err := validateAutomationRuleJSON(effective); err != nil {
		return nil, fmt.Errorf("validate rule: %w", err)
	}

	if !b.HasSets() {
		// No changes
		r, err := scanRule(db.Pool.QueryRow(ctx,
			`SELECT `+ruleSelectCols+` FROM automation_rules WHERE id = $1 AND user_id = $2`, id, userID,
		))
		if err != nil {
			return nil, fmt.Errorf("get rule: %w", err)
		}
		return r, nil
	}

	args := append(b.Args(), id, userID)
	query := fmt.Sprintf(
		`UPDATE automation_rules SET %s WHERE id = $%d AND user_id = $%d
		 RETURNING `+ruleSelectCols,
		b.Clause(), b.NextIdx(), b.NextIdx()+1,
	)

	var r model.AutomationRule
	err := db.Pool.QueryRow(ctx, query, args...).Scan(
		&r.ID, &r.UserID, &r.Name, &r.IsActive, &r.Priority,
		&r.Conditions, &r.ConditionsLogic, &r.ConditionGroups,
		&r.Actions, &r.TimesApplied, &r.CreatedAt,
		&r.IsSystem, &r.SystemType,
		&r.TriggerEvent, &r.StopOnMatch, &r.ApplyRetroactive,
		&r.LastEvaluatedAt, &r.LastMatchedCount,
		&r.LastWarningAt, &r.LastWarningCode, &r.LastWarningMsg,
		&r.Branches,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("update rule: %w", err)
	}
	// Refresh has_automation for all tags of this user
	db.Pool.Exec(ctx, `
		UPDATE tags t SET has_automation = EXISTS (
			SELECT 1 FROM automation_rules ar
			WHERE ar.user_id = t.user_id AND ar.is_active = true
			  AND (ar.actions::text LIKE '%' || t.id::text || '%' OR COALESCE(ar.branches::text, '') LIKE '%' || t.id::text || '%')
		) WHERE t.user_id = $1`, userID) //nolint:errcheck
	return &r, nil
}

func validateAutomationRuleJSON(input rules.SportTaxonomyRuleJSON) error {
	return rules.MarkSportTaxonomyValidationError(rules.ValidateSportTaxonomyRuleJSON(input))
}

func isReadonlySyncRuleType(systemType *string) bool {
	if systemType == nil {
		return false
	}
	for _, sync := range readonlySystemModifierTagSyncs() {
		if *systemType == sync.SystemType {
			return true
		}
	}
	return false
}

func (db *DB) DeleteRule(ctx context.Context, id string, userID string) error {
	// Server-side guard: system rules cannot be deleted
	var isSystem bool
	if err := db.Pool.QueryRow(ctx,
		`SELECT is_system FROM automation_rules WHERE id = $1 AND user_id = $2`,
		id, userID,
	).Scan(&isSystem); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("rule not found")
		}
		return err
	}
	if isSystem {
		return fmt.Errorf("system rules cannot be deleted")
	}

	tag, err := db.Pool.Exec(ctx, `DELETE FROM automation_rules WHERE id = $1 AND user_id = $2`, id, userID)
	if err != nil {
		return fmt.Errorf("delete rule: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("rule not found")
	}
	// Refresh has_automation for all tags of this user
	db.Pool.Exec(ctx, `
		UPDATE tags t SET has_automation = EXISTS (
			SELECT 1 FROM automation_rules ar
			WHERE ar.user_id = t.user_id AND ar.is_active = true
			  AND (ar.actions::text LIKE '%' || t.id::text || '%' OR COALESCE(ar.branches::text, '') LIKE '%' || t.id::text || '%')
		) WHERE t.user_id = $1`, userID) //nolint:errcheck
	return nil
}

func (db *DB) IncrementRuleApplied(ctx context.Context, id string) error {
	_, err := db.Pool.Exec(ctx, `UPDATE automation_rules SET times_applied = times_applied + 1 WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("increment rule applied: %w", err)
	}
	return nil
}

func (db *DB) SetRuleWarning(ctx context.Context, ruleID, userID, code, message string) error {
	_, err := db.Pool.Exec(ctx, `
		UPDATE automation_rules
		SET last_warning_at = NOW(), last_warning_code = $1, last_warning_message = $2
		WHERE id = $3 AND user_id = $4`,
		code, message, ruleID, userID)
	if err != nil {
		return fmt.Errorf("set rule warning: %w", err)
	}
	return nil
}

func (db *DB) ClearRuleWarning(ctx context.Context, ruleID, userID string) error {
	_, err := db.Pool.Exec(ctx, `
		UPDATE automation_rules
		SET last_warning_at = NULL, last_warning_code = NULL, last_warning_message = NULL
		WHERE id = $1 AND user_id = $2`,
		ruleID, userID)
	if err != nil {
		return fmt.Errorf("clear rule warning: %w", err)
	}
	return nil
}

// ValidateRuleGearAssignment checks whether gearID may be assigned to
// activityID by an automation rule. It validates ownership and that the
// gear is not retired before the activity's started_at date. Returns
// (valid, reason, error); reason is non-empty when valid is false.
func (db *DB) ValidateRuleGearAssignment(ctx context.Context, activityID, userID, gearID string) (bool, string, error) {
	if gearID == "" {
		return false, "gear_id is empty", nil
	}

	var gearExists bool
	var valid bool
	err := db.Pool.QueryRow(ctx, `
		SELECT
			COUNT(g.id) > 0 AS gear_exists,
			COALESCE(BOOL_OR(
				g.user_id = a.user_id
				AND (
					NOT g.is_retired
					OR (g.retired_at IS NOT NULL AND a.started_at::date <= g.retired_at)
				)
			), false) AS valid
		FROM activities a
		LEFT JOIN gear g ON g.id = $3
		WHERE a.id = $1 AND a.user_id = $2`,
		activityID, userID, gearID,
	).Scan(&gearExists, &valid)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, "activity no longer exists", nil
	}
	if err != nil {
		return false, "", fmt.Errorf("validate rule gear assignment: %w", err)
	}
	if !gearExists {
		return false, "gear no longer exists or belongs to another user", nil
	}
	if !valid {
		return false, "gear is retired for this activity date", nil
	}
	return true, "", nil
}

// --- Gear stats update ---

// UpdateGearStatsOwnerVerified refreshes the aggregate counters on the gear row
// for gearID (total_distance_m, total_time_s, total_moving_time_s,
// total_elevation_gain_m, total_elevation_loss_m, total_activities).
//
// The caller MUST verify ownership of gearID via VerifyGearOwnership
// (or via an upstream RLS-scoped query) before invoking this function.
// This function trusts the caller — it does not re-check user_id ownership.
func (db *DB) UpdateGearStatsOwnerVerified(ctx context.Context, gearID string) error {
	_, err := db.Pool.Exec(ctx,
		`UPDATE gear SET
		   total_distance_m = COALESCE((
		      SELECT SUM(COALESCE(aga.distance_m_override, a.distance_m, 0))
		      FROM activity_gear_assignments aga
		      JOIN activities a ON a.id = aga.activity_id
		      WHERE aga.gear_id = $1 AND NOT a.is_group_secondary
		   ), 0),
		   total_moving_time_s = COALESCE((
		      SELECT SUM(COALESCE(aga.moving_time_s_override, a.moving_time_s, 0))
		      FROM activity_gear_assignments aga
		      JOIN activities a ON a.id = aga.activity_id
		      WHERE aga.gear_id = $1 AND NOT a.is_group_secondary
		   ), 0),
		   total_time_s = COALESCE((
		      SELECT SUM(COALESCE(a.elapsed_time_s, 0))
		      FROM activity_gear_assignments aga
		      JOIN activities a ON a.id = aga.activity_id
		      WHERE aga.gear_id = $1 AND NOT a.is_group_secondary
		   ), 0),
		   total_elevation_gain_m = COALESCE((
		      SELECT SUM(COALESCE(a.elevation_gain_m, 0))
		      FROM activity_gear_assignments aga
		      JOIN activities a ON a.id = aga.activity_id
		      WHERE aga.gear_id = $1 AND NOT a.is_group_secondary
		   ), 0),
		   total_elevation_loss_m = COALESCE((
		      SELECT SUM(COALESCE(a.elevation_loss_m, 0))
		      FROM activity_gear_assignments aga
		      JOIN activities a ON a.id = aga.activity_id
		      WHERE aga.gear_id = $1 AND NOT a.is_group_secondary
		   ), 0),
		   total_activities = COALESCE((
		      SELECT COUNT(*)
		      FROM activity_gear_assignments aga
		      JOIN activities a ON a.id = aga.activity_id
		      WHERE aga.gear_id = $1 AND NOT a.is_group_secondary
		   ), 0)
		 WHERE id = $1`, gearID,
	)
	if err != nil {
		return fmt.Errorf("update gear stats: %w", err)
	}
	return nil
}

// UpdateComponentUsageStatsOwnerVerified refreshes per-component usage counters
// by aggregating activity_gear_assignments that fall within the component's
// installation window. Writes both gear_components.total_* aggregates and the
// per-assignment counters on component_assignments. Boundary uses the
// assignment's installed_at/removed_at, falling back to the component's
// installed_at/removed_at if no per-assignment row exists yet.
//
// The caller MUST verify ownership of componentID (e.g. via VerifyComponentOwnership
// or via an upstream RLS-scoped query) before invoking this function.
// This function trusts the caller — it does not re-check user_id ownership.
func (db *DB) UpdateComponentUsageStatsOwnerVerified(ctx context.Context, componentID string) error {
	_, err := db.Pool.Exec(ctx, `
		WITH usage AS (
			SELECT
				gc.id AS component_id,
				ca.id AS assignment_id,
				COALESCE(SUM(COALESCE(aga.distance_m_override, a.distance_m, 0)), 0) AS distance_m,
				COALESCE(SUM(COALESCE(aga.moving_time_s_override, a.moving_time_s, 0)), 0) AS moving_time_s,
				COALESCE(SUM(COALESCE(a.elapsed_time_s, 0)), 0) AS time_s,
				COALESCE(SUM(COALESCE(a.elevation_gain_m, 0)), 0) AS elevation_gain_m,
				COALESCE(SUM(COALESCE(a.elevation_loss_m, 0)), 0) AS elevation_loss_m,
				COUNT(a.id) AS activities_count
			FROM gear_components gc
			LEFT JOIN component_assignments ca ON ca.component_id = gc.id AND ca.removed_at IS NULL
			LEFT JOIN activity_gear_assignments aga ON aga.gear_id = gc.gear_id
			LEFT JOIN activities a ON a.id = aga.activity_id
				AND NOT a.is_group_secondary
				AND a.started_at >= COALESCE(ca.installed_at, gc.installed_at, '-infinity'::timestamptz)
				AND a.started_at < COALESCE(ca.removed_at, gc.removed_at, 'infinity'::timestamptz)
			WHERE gc.id = $1
			GROUP BY gc.id, ca.id
		),
		update_component AS (
			UPDATE gear_components gc
			SET total_distance_m = usage.distance_m,
			    total_moving_time_s = usage.moving_time_s,
			    total_time_s = usage.time_s,
			    total_elevation_gain_m = usage.elevation_gain_m,
			    total_elevation_loss_m = usage.elevation_loss_m,
			    total_activities = usage.activities_count
			FROM usage
			WHERE gc.id = usage.component_id
			RETURNING usage.*
		)
		UPDATE component_assignments ca
		SET distance_m = update_component.distance_m,
		    moving_time_s = update_component.moving_time_s,
		    time_s = update_component.time_s,
		    elevation_gain_m = update_component.elevation_gain_m,
		    elevation_loss_m = update_component.elevation_loss_m,
		    activities_count = update_component.activities_count
		FROM update_component
		WHERE ca.id = update_component.assignment_id`, componentID)
	if err != nil {
		return fmt.Errorf("update component usage stats: %w", err)
	}
	return nil
}

// UpdateChainRotationMemberStatsOwnerVerified refreshes per-member usage
// counters for a single chain-rotation member. Only activities that occurred
// AFTER the member's last swap (or after the rotation group's creation if never
// swapped) AND while the member's component was installed are counted. When the
// join produces no rows (e.g. the only assignment was deleted), the counters are
// reset to zero.
//
// The caller MUST verify ownership of memberID (via the parent gear's ownership
// or via an upstream RLS-scoped query) before invoking this function.
// This function trusts the caller — it does not re-check user_id ownership.
func (db *DB) UpdateChainRotationMemberStatsOwnerVerified(ctx context.Context, memberID string) error {
	tag, err := db.Pool.Exec(ctx, `
		UPDATE chain_rotation_members cm
		SET total_distance_m = COALESCE(src.distance_m, 0),
		    total_moving_time_s = COALESCE(src.moving_time_s, 0),
		    total_time_s = COALESCE(src.time_s, 0),
		    total_elevation_gain_m = COALESCE(src.elevation_gain_m, 0),
		    total_elevation_loss_m = COALESCE(src.elevation_loss_m, 0),
		    total_activities = COALESCE(src.activities_count, 0)
		FROM (
			SELECT
				cm.id AS member_id,
				SUM(COALESCE(aga.distance_m_override, a.distance_m, 0)) AS distance_m,
				SUM(COALESCE(aga.moving_time_s_override, a.moving_time_s, 0)) AS moving_time_s,
				SUM(COALESCE(a.elapsed_time_s, 0)) AS time_s,
				SUM(COALESCE(a.elevation_gain_m, 0)) AS elevation_gain_m,
				SUM(COALESCE(a.elevation_loss_m, 0)) AS elevation_loss_m,
				COUNT(a.id) AS activities_count
			FROM chain_rotation_members cm
			JOIN chain_rotation_groups crg ON crg.id = cm.group_id
			JOIN gear_components gc ON gc.id = cm.component_id
			JOIN activity_gear_assignments aga ON aga.gear_id = crg.gear_id
			JOIN activities a ON a.id = aga.activity_id
			WHERE cm.id = $1
			  AND NOT a.is_group_secondary
			  AND a.started_at >= COALESCE(cm.last_swapped_at, crg.created_at)
			  AND a.started_at >= COALESCE(gc.installed_at, '-infinity'::timestamptz)
			  AND a.started_at < COALESCE(gc.removed_at, 'infinity'::timestamptz)
			GROUP BY cm.id
		) src
		WHERE cm.id = $1 AND cm.id = src.member_id`, memberID)
	if err != nil {
		return fmt.Errorf("update chain rotation member stats: %w", err)
	}
	if tag.RowsAffected() == 0 {
		_, err = db.Pool.Exec(ctx, `
			UPDATE chain_rotation_members
			SET total_distance_m = 0,
			    total_moving_time_s = 0,
			    total_time_s = 0,
			    total_elevation_gain_m = 0,
			    total_elevation_loss_m = 0,
			    total_activities = 0
			WHERE id = $1`, memberID)
		if err != nil {
			return fmt.Errorf("reset chain rotation member stats: %w", err)
		}
	}
	return nil
}

// ListActiveComponentIDsForGearOwnerVerified returns the IDs of all
// currently-installed, non-retired components mounted on the given gear. Used
// to fan out usage-stat refreshes across all components when a gear's activity
// assignments change.
//
// The caller MUST verify ownership of gearID via VerifyGearOwnership
// (or via an upstream RLS-scoped query) before invoking this function.
// This function trusts the caller — it does not re-check user_id ownership.
func (db *DB) ListActiveComponentIDsForGearOwnerVerified(ctx context.Context, gearID string) ([]string, error) {
	rows, err := db.Pool.Query(ctx, `
		SELECT id::text
		FROM gear_components
		WHERE gear_id = $1
		  AND removed_at IS NULL
		  AND is_retired = false`, gearID)
	if err != nil {
		return nil, fmt.Errorf("list active components: %w", err)
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// ListImpactedChainRotationMemberIDsForGearOwnerVerified returns IDs of all
// chain-rotation members whose group is attached to the given gear and that are
// currently participating in the rotation (either marked active OR have a swap
// history). Used to fan out member-stat refreshes when a gear's activity totals
// change.
//
// The caller MUST verify ownership of gearID via VerifyGearOwnership
// (or via an upstream RLS-scoped query) before invoking this function.
// This function trusts the caller — it does not re-check user_id ownership.
func (db *DB) ListImpactedChainRotationMemberIDsForGearOwnerVerified(ctx context.Context, gearID string) ([]string, error) {
	rows, err := db.Pool.Query(ctx, `
		SELECT cm.id::text
		FROM chain_rotation_members cm
		JOIN chain_rotation_groups crg ON crg.id = cm.group_id
		WHERE crg.gear_id = $1
		  AND (cm.is_active = true OR cm.last_swapped_at IS NOT NULL)`, gearID)
	if err != nil {
		return nil, fmt.Errorf("list impacted chain members: %w", err)
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// RefreshGearAndDependentUsageStatsOwnerVerified refreshes gear-level totals
// plus the totals of every active component and every impacted chain-rotation
// member tied to that gear. After updating counters it invokes gear-lifecycle
// rule evaluation for the gear; rule errors are logged and do not abort the
// refresh.
//
// Call this whenever an activity_gear_assignments row or an activity metric
// (distance/time/elevation) changes.
//
// The caller MUST verify ownership of gearID via VerifyGearOwnership
// (or via an upstream RLS-scoped query) before invoking this function.
// This function trusts the caller — it does not re-check user_id ownership.
func (db *DB) RefreshGearAndDependentUsageStatsOwnerVerified(ctx context.Context, gearID string) error {
	if err := db.UpdateGearStatsOwnerVerified(ctx, gearID); err != nil {
		return err
	}
	componentIDs, err := db.ListActiveComponentIDsForGearOwnerVerified(ctx, gearID)
	if err != nil {
		return err
	}
	for _, componentID := range componentIDs {
		if err := db.UpdateComponentUsageStatsOwnerVerified(ctx, componentID); err != nil {
			return err
		}
	}
	memberIDs, err := db.ListImpactedChainRotationMemberIDsForGearOwnerVerified(ctx, gearID)
	if err != nil {
		return err
	}
	for _, memberID := range memberIDs {
		if err := db.UpdateChainRotationMemberStatsOwnerVerified(ctx, memberID); err != nil {
			return err
		}
	}
	if err := db.EvaluateGearUsageRulesForGear(ctx, []string{gearID}); err != nil {
		slog.Warn("evaluate gear usage rules", "gear_id", gearID, "error", err)
	}
	return nil
}

// RefreshGearUsageForActivityMetricChange refreshes usage stats for every gear
// assigned to the given activity. Called by UpdateActivityParserMetrics and
// UpdateActivityMetricsWithSportAndModifiers after the activity row is updated.
// The userID is enforced on both the assignment and the activity to keep the
// multi-user invariant; rows that do not match are silently ignored.
func (db *DB) RefreshGearUsageForActivityMetricChange(ctx context.Context, activityID string, userID string) ([]string, error) {
	rows, err := db.Pool.Query(ctx, `
		SELECT DISTINCT aga.gear_id::text
		FROM activity_gear_assignments aga
		JOIN activities a ON a.id = aga.activity_id
		WHERE aga.activity_id = $1
		  AND aga.user_id = $2
		  AND a.user_id = $2`, activityID, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var impacted []string
	for rows.Next() {
		var gearID string
		if err := rows.Scan(&gearID); err != nil {
			return nil, err
		}
		if err := db.RefreshGearAndDependentUsageStatsOwnerVerified(ctx, gearID); err != nil {
			return nil, err
		}
		impacted = append(impacted, gearID)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(impacted) > 0 {
		if err := db.EvaluateGearUsageRulesForGear(ctx, impacted); err != nil {
			slog.Warn("evaluate gear usage rules", "activity_id", activityID, "user_id", userID, "error", err)
		}
	}
	return impacted, nil
}

// EvaluateGearUsageRulesForGear evaluates all active on_gear_usage_update rules
// for the owners of the given gear IDs. For each matching (rule, gear) pair it
// upserts a row in gear_rule_action_state and increments times_applied when the
// rule body has changed since the last match.
//
// An early-exit query checks whether any qualifying rules exist before loading
// gear stats; for users with no on_gear_usage_update rules this costs one
// EXISTS query instead of a full gear + rules scan.
func (db *DB) EvaluateGearUsageRulesForGear(ctx context.Context, gearIDs []string) error {
	if len(gearIDs) == 0 {
		return nil
	}
	// Fast path: skip all work when no active on_gear_usage_update rules exist
	// for any owner of the supplied gear IDs. The subquery resolves user_ids
	// from the gear table, keeping the check self-contained.
	var hasRules bool
	err := db.Pool.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM automation_rules ar
			JOIN gear g ON g.user_id = ar.user_id AND g.id = ANY($1::uuid[])
			WHERE ar.trigger_event = 'on_gear_usage_update'
			  AND ar.is_active = true
			LIMIT 1
		)`, gearIDs).Scan(&hasRules)
	if err != nil {
		return fmt.Errorf("check gear usage rules existence: %w", err)
	}
	if !hasRules {
		return nil
	}
	rows, err := db.Pool.Query(ctx, `
		SELECT id::text, user_id::text, total_distance_m, total_moving_time_s, total_time_s,
		       total_elevation_gain_m, total_elevation_loss_m, total_activities
		FROM gear
		WHERE id = ANY($1::uuid[]) AND is_retired = false`, gearIDs)
	if err != nil {
		return fmt.Errorf("list gear usage rule candidates: %w", err)
	}
	defer rows.Close()

	type candidate struct {
		userID  string
		gearID  string
		gearMap map[string]any
	}
	var candidates []candidate
	userIDs := map[string]bool{}
	for rows.Next() {
		var c candidate
		var totalDistance, totalGain, totalLoss float64
		var totalMoving, totalTime, totalActivities int
		if err := rows.Scan(&c.gearID, &c.userID, &totalDistance, &totalMoving, &totalTime, &totalGain, &totalLoss, &totalActivities); err != nil {
			return fmt.Errorf("scan gear usage rule candidate: %w", err)
		}
		c.gearMap = map[string]any{
			"gear_id":                     c.gearID,
			"gear_total_distance_m":       totalDistance,
			"gear_total_moving_time_s":    totalMoving,
			"gear_total_time_s":           totalTime,
			"gear_total_elevation_gain_m": totalGain,
			"gear_total_elevation_loss_m": totalLoss,
			"gear_total_activities":       totalActivities,
		}
		candidates = append(candidates, c)
		userIDs[c.userID] = true
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("list gear usage rule candidates: %w", err)
	}
	for userID := range userIDs {
		rulesForUser, err := db.ListRules(ctx, userID)
		if err != nil {
			return err
		}
		for _, rule := range rulesForUser {
			if !rule.IsActive || rule.TriggerEvent != "on_gear_usage_update" {
				continue
			}
			matched := 0
			for _, c := range candidates {
				if c.userID != userID || !evaluateGearUsageRuleMap(rule, c.gearMap) {
					continue
				}
				signature := gearUsageRuleSignature(rule)
				changed, err := db.recordGearRuleActionState(ctx, userID, rule.ID, c.gearID, signature)
				if err != nil {
					return err
				}
				if changed {
					if err := db.IncrementRuleApplied(ctx, rule.ID); err != nil {
						return err
					}
				}
				matched++
			}
			if matched > 0 {
				if _, err := db.Pool.Exec(ctx,
					`UPDATE automation_rules SET last_evaluated_at = NOW(), last_matched_count = $1 WHERE id = $2 AND user_id = $3`,
					matched, rule.ID, userID); err != nil {
					return fmt.Errorf("update gear usage rule stats: %w", err)
				}
			}
		}
	}
	return nil
}

func evaluateGearUsageRuleMap(rule model.AutomationRule, gearMap map[string]any) bool {
	if len(rule.ConditionGroups) > 0 && string(rule.ConditionGroups) != "null" {
		var groups []rules.ConditionGroup
		if err := json.Unmarshal(rule.ConditionGroups, &groups); err == nil && len(groups) > 0 {
			return rules.EvaluateGroups(groups, gearMap)
		}
	}
	if len(rule.Conditions) > 0 && string(rule.Conditions) != "null" {
		var conditions []rules.Condition
		if err := json.Unmarshal(rule.Conditions, &conditions); err == nil && len(conditions) > 0 {
			return rules.Evaluate(conditions, gearMap)
		}
	}
	return false
}

func gearUsageRuleSignature(rule model.AutomationRule) string {
	raw, _ := json.Marshal([]json.RawMessage{rule.Conditions, rule.ConditionGroups, rule.Actions})
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func (db *DB) recordGearRuleActionState(ctx context.Context, userID, ruleID, gearID, signature string) (bool, error) {
	var changed bool
	err := db.Pool.QueryRow(ctx, `
		INSERT INTO gear_rule_action_state (user_id, rule_id, gear_id, last_signature)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (rule_id, gear_id)
		DO UPDATE SET last_matched_at = NOW(), last_signature = EXCLUDED.last_signature
		WHERE gear_rule_action_state.last_signature IS DISTINCT FROM EXCLUDED.last_signature
		RETURNING true`,
		userID, ruleID, gearID, signature,
	).Scan(&changed)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("record gear rule action state: %w", err)
	}
	return changed, nil
}

// SetPrimaryGearForActivityTx upserts the primary activity_gear_assignments row
// for activityID to point at gearID. The activity's user_id is read from the
// activities table (never from caller input) so the multi-user invariant holds:
// the activity_gear_assignments_user_match trigger requires
// aga.user_id = activities.user_id = gear.user_id.
//
// If a primary aga row already exists for the activity, it is updated in place
// (gear_id, source, updated_at). Otherwise a new row is inserted. The partial
// unique index idx_activity_gear_assignments_one_primary on
// (activity_id) WHERE role_key='primary' enforces single-primary semantics.
//
// Caller-supplied source must be one of: 'user', 'rule', 'import', 'legacy',
// 'fusion'. Use 'user' for direct UI assignments, 'rule' for automation,
// 'fusion' for fusion-merge gear inheritance, 'import' for GDPR import.
//
// gearID must be a non-empty UUID string. Use ClearPrimaryGearForActivityTx
// to remove the primary assignment.
func SetPrimaryGearForActivityTx(ctx context.Context, q Querier, activityID, gearID, source string) error {
	if gearID == "" {
		return fmt.Errorf("SetPrimaryGearForActivityTx: gearID must not be empty")
	}
	if source == "" {
		source = "user"
	}
	_, err := q.Exec(ctx, `
		INSERT INTO activity_gear_assignments
			(id, user_id, activity_id, gear_id, role_key, source, quantity)
		SELECT gen_random_uuid(), a.user_id, a.id, $2::uuid, 'primary', $3, 1
		FROM activities a
		WHERE a.id = $1::uuid
		ON CONFLICT (activity_id) WHERE role_key = 'primary'
		DO UPDATE SET
			gear_id    = EXCLUDED.gear_id,
			source     = EXCLUDED.source,
			updated_at = now()
	`, activityID, gearID, source)
	if err != nil {
		return fmt.Errorf("set primary gear assignment: %w", err)
	}
	return nil
}

// ClearPrimaryGearForActivityTx removes the primary activity_gear_assignments
// row for the given activity. No-op when none exists.
func ClearPrimaryGearForActivityTx(ctx context.Context, q Querier, activityID string) error {
	_, err := q.Exec(ctx, `
		DELETE FROM activity_gear_assignments
		 WHERE activity_id = $1::uuid AND role_key = 'primary'
	`, activityID)
	if err != nil {
		return fmt.Errorf("clear primary gear assignment: %w", err)
	}
	return nil
}

// SetPrimaryGearForActivityIfUnsetTx upserts primary aga only when no primary
// row currently exists. Used by automation rules so manual assignments are not
// overwritten by rule-driven defaults.
func SetPrimaryGearForActivityIfUnsetTx(ctx context.Context, q Querier, activityID, gearID, source string) error {
	if gearID == "" {
		return fmt.Errorf("SetPrimaryGearForActivityIfUnsetTx: gearID must not be empty")
	}
	if source == "" {
		source = "rule"
	}
	_, err := q.Exec(ctx, `
		INSERT INTO activity_gear_assignments
			(id, user_id, activity_id, gear_id, role_key, source, quantity)
		SELECT gen_random_uuid(), a.user_id, a.id, $2::uuid, 'primary', $3, 1
		FROM activities a
		WHERE a.id = $1::uuid
		ON CONFLICT (activity_id) WHERE role_key = 'primary'
		DO NOTHING
	`, activityID, gearID, source)
	if err != nil {
		return fmt.Errorf("conditional set primary gear assignment: %w", err)
	}
	return nil
}

func (db *DB) AssignGearByRules(ctx context.Context, userID, activityID, sportType string) error {
	return db.AssignGearByRulesForActivity(ctx, userID, activityID, sportType, "", sport.ModifierValues{})
}

func (db *DB) AssignGearByRulesForActivity(ctx context.Context, userID, activityID, sportType, subType string, modifiers sport.ModifierValues) error {
	activityTuple, _, err := sport.NormalizeSportTuple(sportType, subType)
	if err != nil {
		return fmt.Errorf("normalize activity sport for gear: %w", err)
	}
	gearID, err := db.findDefaultGearForActivity(ctx, userID, activityTuple, modifiers)
	if err != nil {
		return err
	}
	if gearID == "" {
		return nil
	}

	// Only assign if no primary aga row exists yet (preserves prior manual
	// or import-derived assignments; matches pre-DROP behaviour of the
	// `WHERE gear_id IS NULL` guard on the legacy activities.gear_id update).
	return SetPrimaryGearForActivityIfUnsetTx(ctx, db.Pool, activityID, gearID, "rule")
}

func (db *DB) findDefaultGearForActivity(ctx context.Context, userID string, activityTuple sport.Tuple, modifiers sport.ModifierValues) (string, error) {
	rows, err := db.Pool.Query(ctx,
		`SELECT id, applicable_sport_predicates
		 FROM gear
		 WHERE user_id = $1
		   AND status != 'retired'
		   AND jsonb_array_length(COALESCE(applicable_sport_predicates, '[]'::jsonb)) > 0
		 ORDER BY created_at`, userID,
	)
	if err != nil {
		return "", fmt.Errorf("find default gear: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var gearID string
		var predicateJSON []byte
		if err := rows.Scan(&gearID, &predicateJSON); err != nil {
			return "", fmt.Errorf("scan default gear: %w", err)
		}
		var predicates []gearpkg.GearDefaultPredicate
		if len(predicateJSON) > 0 {
			if err := json.Unmarshal(predicateJSON, &predicates); err != nil {
				return "", fmt.Errorf("scan default gear predicates: %w", err)
			}
		}
		for _, predicate := range predicates {
			if gearpkg.GearDefaultPredicateMatchesActivity(predicate, activityTuple.SportType, activityTuple.SubType, modifiers) {
				return gearID, nil
			}
		}
	}
	if err := rows.Err(); err != nil {
		return "", fmt.Errorf("find default gear: %w", err)
	}
	return "", nil
}

// GetActivityAsMap returns an activity as a map for rule evaluation.
func (db *DB) GetActivityAsMap(ctx context.Context, activityID string) (map[string]any, error) {
	row := db.Pool.QueryRow(ctx, `
		SELECT
			a.sport_type, a.sub_type, a.source, a.intensity, a.title, a.description,
			a.is_indoor, a.is_electric, a.is_virtual, a.has_gps, a.visibility, a.hide_map, a.is_round_trip,
			a.elapsed_time_s, a.moving_time_s,
			EXTRACT(DOW FROM a.started_at AT TIME ZONE 'UTC')::int AS weekday,
			EXTRACT(MONTH FROM a.started_at AT TIME ZONE 'UTC')::int AS month,
			a.time_of_day,
			a.distance_m,
			a.avg_speed_ms, a.max_speed_ms, a.min_speed_ms,
			a.elevation_gain_m, a.elevation_loss_m, a.max_elevation_m, a.min_elevation_m,
			a.avg_heart_rate, a.max_heart_rate, a.min_heart_rate,
			a.avg_cadence, a.max_cadence,
			a.avg_power_w, a.max_power_w, a.min_power_w, a.normalized_power_w, a.tss, a.trimp,
			a.avg_temperature_c, a.min_temperature_c, a.max_temperature_c,
			a.start_city, a.start_state, a.start_country, a.start_country_code, a.end_city,
			(SELECT aga.gear_id::text FROM activity_gear_assignments aga
			 WHERE aga.activity_id = a.id AND aga.role_key = 'primary' LIMIT 1) AS gear_id,
			a.tags,
			a.started_at,
			ST_Y(a.start_location::geometry) AS start_lat,
			ST_X(a.start_location::geometry) AS start_lon,
			ST_Y(a.end_location::geometry) AS end_lat,
			ST_X(a.end_location::geometry) AS end_lon,
			COALESCE(pc.cnt, 0) AS photo_count,
			COALESCE(lc.cnt, 0) AS detected_lap_count,
			a.lift_count, a.lift_ascent_m, a.lift_descent_m, a.lift_distance_m, a.lift_duration_s,
			a.net_ascent_m, a.net_descent_m, a.net_distance_m,
			COALESCE(cd.climb_count, 0) AS climb_count,
			cd.max_category AS climb_category_max,
			w.temp_c AS weather_temp_c,
			w.humidity AS weather_humidity,
			w.wind_speed_ms AS weather_wind_speed_ms,
			w.wind_gusts_ms AS weather_wind_gusts_ms,
			w.condition AS weather_condition,
			wm.temp_min AS weather_temp_min_c,
			wm.temp_max AS weather_temp_max_c,
			a.created_at AS activity_created_at,
			gt.summit_count, gt.has_pass, gt.border_crossing_count,
			st.has_gastronomy_stop, st.stop_count,
			iv.has_structured_intervals, iv.interval_count,
			hc.has_hc_climb,
			sf.surface_paved_pct, sf.surface_gravel_pct, sf.surface_trail_pct, sf.surface_unknown_pct,
			a.is_commute,
			a.swim_stroke, a.pool_length_m, a.num_active_lengths,
			a.max_depth_m, a.avg_depth_m, a.end_cns_pct,
			a.jump_count, a.total_grit, a.avg_flow,
			a.avg_lev_motor_power_w, a.lev_battery_consumption_pct,
				a.parent_id, a.num_sessions,
				a.total_calories,
				a.aerobic_training_effect, a.anaerobic_training_effect,
				a.training_load_peak,
				a.avg_spo2_pct, a.avg_respiration_rate, a.avg_stress_level,
				a.session_rmssd_ms, a.avg_core_temperature_c, a.threshold_power_w,
				a.aerobic_decoupling, a.variability_index, a.efficiency_factor,
				a.intensity_factor, a.workout_rpe, a.workout_feel
		FROM activities a
		LEFT JOIN (SELECT activity_id, COUNT(*)::int AS cnt FROM activity_photos GROUP BY activity_id) pc ON pc.activity_id = a.id
		LEFT JOIN (SELECT activity_id, COUNT(*)::int AS cnt FROM activity_detected_laps GROUP BY activity_id) lc ON lc.activity_id = a.id
		LEFT JOIN (
			SELECT activity_id,
			       COUNT(*)::int AS climb_count,
			       MIN(CASE WHEN category ~ '^[0-9]+$' THEN category::int END) AS max_category
			FROM activity_climbs GROUP BY activity_id
		) cd ON cd.activity_id = a.id
		LEFT JOIN LATERAL (
			SELECT temp_c, humidity, wind_speed_ms, wind_gusts_ms, condition
			FROM activity_weather
			WHERE activity_id = a.id
			ORDER BY CASE reading_type WHEN 'single' THEN 0 WHEN 'start' THEN 1 ELSE 2 END
			LIMIT 1
		) w ON true
		LEFT JOIN LATERAL (
			SELECT MIN(temp_c) AS temp_min, MAX(temp_c) AS temp_max
			FROM activity_weather WHERE activity_id = a.id
		) wm ON true
		LEFT JOIN LATERAL (
			SELECT
				COUNT(*) FILTER (WHERE category = 'summit')::int AS summit_count,
				(COUNT(*) FILTER (WHERE category = 'pass') > 0) AS has_pass,
				COUNT(*) FILTER (WHERE category = 'border' AND admin_level = 2)::int AS border_crossing_count
			FROM activity_geo_tags WHERE activity_id = a.id
		) gt ON true
		LEFT JOIN LATERAL (
			SELECT
				(COUNT(*) FILTER (WHERE poi_category = 'gastronomy') > 0) AS has_gastronomy_stop,
				COUNT(*)::int AS stop_count
			FROM activity_stops WHERE activity_id = a.id
		) st ON true
		LEFT JOIN LATERAL (
			SELECT
				(COUNT(*) > 0) AS has_structured_intervals,
				COUNT(*)::int AS interval_count
			FROM activity_intervals WHERE activity_id = a.id
		) iv ON true
		LEFT JOIN LATERAL (
			SELECT (COUNT(*) FILTER (WHERE category = 'HC') > 0) AS has_hc_climb
			FROM activity_climbs WHERE activity_id = a.id
		) hc ON true
		LEFT JOIN LATERAL (
			SELECT
				COALESCE(SUM(percentage) FILTER (WHERE type IN ('asphalt','paved','concrete','concrete:plates','concrete:lanes','paving_stones','sett')), 0) AS surface_paved_pct,
				COALESCE(SUM(percentage) FILTER (WHERE type IN ('gravel','compacted','fine_gravel','pebblestone')), 0) AS surface_gravel_pct,
				COALESCE(SUM(percentage) FILTER (WHERE type IN ('ground','dirt','earth','grass','mud','sand','wood','woodchips','rock','stepping_stones')), 0) AS surface_trail_pct,
				COALESCE(SUM(percentage) FILTER (WHERE type NOT IN ('asphalt','paved','concrete','concrete:plates','concrete:lanes','paving_stones','sett','gravel','compacted','fine_gravel','pebblestone','ground','dirt','earth','grass','mud','sand','wood','woodchips','rock','stepping_stones') OR type IS NULL), 0) AS surface_unknown_pct
			FROM activity_enrichments WHERE activity_id = a.id AND category = 'surface'
		) sf ON true
		WHERE a.id = $1`,
		activityID,
	)

	var (
		sportType, subType, source, intensity, title, description             *string
		isIndoor, isElectric, isVirtual, hasGPS, hideMap, isRoundTrip         bool
		visibility                                                            *string
		elapsedTimeS, movingTimeS                                             *int
		weekday, month                                                        *int
		timeOfDay                                                             []string
		distanceM, avgSpeedMS, maxSpeedMS, minSpeedMS                         *float64
		elevGainM, elevLossM, maxElevM, minElevM                              *float64
		avgHR, maxHR, minHR                                                   *float64
		avgCadence, maxCadence                                                *float64
		avgPowerW, maxPowerW, minPowerW, normPowerW, tss, trimp               *float64
		avgTempC, minTempC, maxTempC                                          *float64
		startCity, startState, startCountry, startCountryCode                 *string
		endCity                                                               *string
		gearID                                                                *string
		tags                                                                  []string
		startedAt                                                             time.Time
		startLat, startLon                                                    *float64
		endLat, endLon                                                        *float64
		photoCount, detectedLapCount                                          int
		liftCount                                                             *int
		liftAscentM, liftDescentM, liftDistM                                  *float64
		liftDurationS                                                         *int
		netAscentM, netDescentM, netDistM                                     *float64
		climbCount                                                            int
		climbCategoryMax                                                      *int
		weatherTempC, weatherHumidity                                         *float64
		weatherWindSpeedMS, weatherWindGustsMS                                *float64
		weatherCondition                                                      *string
		weatherTempMinC, weatherTempMaxC                                      *float64
		activityCreatedAt                                                     time.Time
		summitCount, borderCrossingCount, stopCount, intervalCount            int
		hasPass, hasGastroStop, hasStructuredIntervals, hasHCClimb            bool
		surfacePavedPct, surfaceGravelPct, surfaceTrailPct, surfaceUnknownPct float64
		isCommute                                                             bool
		swimStroke                                                            *string
		poolLengthM                                                           *float64
		numActiveLengths                                                      *int
		maxDepthM, avgDepthM                                                  *float64
		endCnsPct                                                             *int16
		jumpCount                                                             *int
		totalGrit, avgFlow                                                    *float64
		avgLevMotorPowerW                                                     *int
		levBatteryPct                                                         *float64
		parentID                                                              *string
		numSessions                                                           *int16
		totalCalories                                                         *int
		aerobicTE, anaerobicTE                                                *float64
		trainingLoadPeak, avgRespirationRate, avgCoreTemperature              *float64
		avgSpo2Pct, avgStressLevel, sessionRmssdMs, thresholdPowerW           *int
		aerobicDecoupling, variabilityIndex                                   *float64
		efficiencyFactor, intensityFactor                                     *float64
		workoutRPE, workoutFeel                                               *int
	)

	err := row.Scan(
		&sportType, &subType, &source, &intensity, &title, &description,
		&isIndoor, &isElectric, &isVirtual, &hasGPS, &visibility, &hideMap, &isRoundTrip,
		&elapsedTimeS, &movingTimeS, &weekday, &month, &timeOfDay,
		&distanceM,
		&avgSpeedMS, &maxSpeedMS, &minSpeedMS,
		&elevGainM, &elevLossM, &maxElevM, &minElevM,
		&avgHR, &maxHR, &minHR,
		&avgCadence, &maxCadence,
		&avgPowerW, &maxPowerW, &minPowerW, &normPowerW, &tss, &trimp,
		&avgTempC, &minTempC, &maxTempC,
		&startCity, &startState, &startCountry, &startCountryCode, &endCity,
		&gearID, &tags,
		&startedAt,
		&startLat, &startLon,
		&endLat, &endLon,
		&photoCount, &detectedLapCount,
		&liftCount, &liftAscentM, &liftDescentM, &liftDistM, &liftDurationS,
		&netAscentM, &netDescentM, &netDistM,
		&climbCount, &climbCategoryMax,
		&weatherTempC, &weatherHumidity, &weatherWindSpeedMS, &weatherWindGustsMS,
		&weatherCondition, &weatherTempMinC, &weatherTempMaxC,
		&activityCreatedAt,
		&summitCount, &hasPass, &borderCrossingCount,
		&hasGastroStop, &stopCount,
		&hasStructuredIntervals, &intervalCount,
		&hasHCClimb,
		&surfacePavedPct, &surfaceGravelPct, &surfaceTrailPct, &surfaceUnknownPct,
		&isCommute,
		&swimStroke, &poolLengthM, &numActiveLengths,
		&maxDepthM, &avgDepthM, &endCnsPct,
		&jumpCount, &totalGrit, &avgFlow,
		&avgLevMotorPowerW, &levBatteryPct,
		&parentID, &numSessions,
		&totalCalories,
		&aerobicTE, &anaerobicTE,
		&trainingLoadPeak, &avgSpo2Pct, &avgRespirationRate, &avgStressLevel,
		&sessionRmssdMs, &avgCoreTemperature, &thresholdPowerW,
		&aerobicDecoupling, &variabilityIndex,
		&efficiencyFactor, &intensityFactor, &workoutRPE, &workoutFeel,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get activity map: %w", err)
	}

	m := map[string]any{
		"is_indoor":          isIndoor,
		"is_electric":        isElectric,
		"is_virtual":         isVirtual,
		"is_commute":         isCommute,
		"has_gps":            hasGPS,
		"hide_map":           hideMap,
		"is_round_trip":      isRoundTrip,
		"photo_count":        photoCount,
		"detected_lap_count": detectedLapCount,
		"climb_count":        climbCount,
	}
	derefStr := func(s *string) any {
		if s == nil {
			return nil
		}
		return *s
	}
	derefInt := func(i *int) any {
		if i == nil {
			return nil
		}
		return *i
	}
	derefF := func(f *float64) any {
		if f == nil {
			return nil
		}
		return *f
	}
	setNN := func(key string, v any) {
		if v != nil {
			m[key] = v
		}
	}

	setNN("sport_type", derefStr(sportType))
	setNN("sub_type", derefStr(subType))
	setNN("source", derefStr(source))
	setNN("intensity", derefStr(intensity))
	setNN("title", derefStr(title))
	setNN("description", derefStr(description))
	setNN("visibility", derefStr(visibility))
	setNN("elapsed_time_s", derefInt(elapsedTimeS))
	setNN("moving_time_s", derefInt(movingTimeS))
	setNN("weekday", derefInt(weekday))
	setNN("month", derefInt(month))
	setNN("distance_m", derefF(distanceM))
	setNN("avg_speed_ms", derefF(avgSpeedMS))
	setNN("max_speed_ms", derefF(maxSpeedMS))
	setNN("min_speed_ms", derefF(minSpeedMS))
	setNN("elevation_gain_m", derefF(elevGainM))
	setNN("elevation_loss_m", derefF(elevLossM))
	setNN("max_elevation_m", derefF(maxElevM))
	setNN("min_elevation_m", derefF(minElevM))
	setNN("avg_heart_rate", derefF(avgHR))
	setNN("max_heart_rate", derefF(maxHR))
	setNN("min_heart_rate", derefF(minHR))
	setNN("avg_cadence", derefF(avgCadence))
	setNN("max_cadence", derefF(maxCadence))
	setNN("avg_power_w", derefF(avgPowerW))
	setNN("max_power_w", derefF(maxPowerW))
	setNN("min_power_w", derefF(minPowerW))
	setNN("normalized_power_w", derefF(normPowerW))
	setNN("tss", derefF(tss))
	setNN("trimp", derefF(trimp))
	setNN("avg_temperature_c", derefF(avgTempC))
	setNN("min_temperature_c", derefF(minTempC))
	setNN("max_temperature_c", derefF(maxTempC))
	setNN("start_city", derefStr(startCity))
	setNN("start_state", derefStr(startState))
	setNN("start_country", derefStr(startCountry))
	setNN("start_country_code", derefStr(startCountryCode))
	setNN("end_city", derefStr(endCity))
	setNN("gear_id", derefStr(gearID))
	setNN("start_lat", derefF(startLat))
	setNN("start_lon", derefF(startLon))
	setNN("end_lat", derefF(endLat))
	setNN("end_lon", derefF(endLon))
	setNN("lift_count", derefInt(liftCount))
	setNN("lift_ascent_m", derefF(liftAscentM))
	setNN("lift_descent_m", derefF(liftDescentM))
	setNN("lift_distance_m", derefF(liftDistM))
	setNN("lift_duration_s", derefInt(liftDurationS))
	setNN("net_ascent_m", derefF(netAscentM))
	setNN("net_descent_m", derefF(netDescentM))
	setNN("net_distance_m", derefF(netDistM))
	setNN("climb_category_max", derefInt(climbCategoryMax))
	setNN("weather_temp_c", derefF(weatherTempC))
	setNN("weather_temp_min_c", derefF(weatherTempMinC))
	setNN("weather_temp_max_c", derefF(weatherTempMaxC))
	setNN("weather_humidity", derefF(weatherHumidity))
	setNN("weather_wind_speed_ms", derefF(weatherWindSpeedMS))
	setNN("weather_wind_gusts_ms", derefF(weatherWindGustsMS))
	setNN("weather_condition", derefStr(weatherCondition))
	m["created_at"] = activityCreatedAt
	m["started_at"] = startedAt
	if len(timeOfDay) > 0 {
		m["time_of_day"] = timeOfDay
	}
	if len(tags) > 0 {
		m["tags"] = tags
	}

	// Dynamic computed fields for rules engine
	if startLat != nil && startLon != nil && !startedAt.IsZero() {
		pos := suncalc.GetPosition(startedAt, *startLat, *startLon)
		m["sun_elevation"] = pos.Altitude * 180 / math.Pi
	}
	// NULL-Guard: start_end_distance_m nur berechnen wenn BEIDE Endpunkte vorhanden
	// (Indoor-Aktivitäten ohne GPS haben kein end_location → sonst fälschlich 0m → Rundtour-Tag)
	if startLat != nil && startLon != nil && endLat != nil && endLon != nil {
		m["start_end_distance_m"] = rules.HaversineM(*startLat, *startLon, *endLat, *endLon)
	}
	// KEIN start_end_distance_m Key wenn Koordinaten fehlen → Rundtour-Rule matched nicht

	segEfforts, err := db.getSegmentEffortsForActivity(ctx, activityID)
	if err == nil && len(segEfforts) > 0 {
		m["segment_efforts"] = segEfforts
	}

	// recording_device_id + device metadata
	var recDeviceID, recDevProductName, recDevManufacturer *string
	_ = db.Pool.QueryRow(ctx, `
		SELECT a.recording_device_id::TEXT, d.product_name, d.manufacturer
		FROM activities a
		LEFT JOIN devices d ON d.id = a.recording_device_id
		WHERE a.id = $1`, activityID).Scan(&recDeviceID, &recDevProductName, &recDevManufacturer)
	if recDeviceID != nil {
		m["recording_device_id"] = *recDeviceID
	}
	setNN("recording_device_product_name", derefStr(recDevProductName))
	setNN("recording_device_manufacturer", derefStr(recDevManufacturer))

	// has_sensor_type — aggregated sensor types from activity_sensors
	var sensorTypes []string
	sensorTypeRows, sensorTypeErr := db.Pool.Query(ctx, `
		SELECT DISTINCT unnest(all_device_types)
		FROM activity_sensors WHERE activity_id = $1`, activityID)
	if sensorTypeErr == nil {
		for sensorTypeRows.Next() {
			var t string
			if sensorTypeRows.Scan(&t) == nil {
				sensorTypes = append(sensorTypes, t)
			}
		}
		sensorTypeRows.Close()
	}
	if len(sensorTypes) > 0 {
		m["has_sensor_type"] = sensorTypes
	}

	// sensor_device_ids — linked device UUIDs from activity_sensors
	var sensorDeviceIDs []string
	sensorDeviceRows, sensorDeviceErr := db.Pool.Query(ctx, `
		SELECT DISTINCT device_id::TEXT
		FROM activity_sensors WHERE activity_id = $1 AND device_id IS NOT NULL`, activityID)
	if sensorDeviceErr == nil {
		for sensorDeviceRows.Next() {
			var id string
			if sensorDeviceRows.Scan(&id) == nil {
				sensorDeviceIDs = append(sensorDeviceIDs, id)
			}
		}
		sensorDeviceRows.Close()
	}
	if len(sensorDeviceIDs) > 0 {
		m["sensor_device_ids"] = sensorDeviceIDs
	}

	// Enrichment-derived conditions
	m["has_summit"] = summitCount > 0
	m["summit_count"] = summitCount
	m["has_pass"] = hasPass
	m["border_crossing_count"] = borderCrossingCount
	m["has_gastronomy_stop"] = hasGastroStop
	m["stop_count"] = stopCount
	m["has_structured_intervals"] = hasStructuredIntervals
	m["interval_count"] = intervalCount
	m["has_hc_climb"] = hasHCClimb
	m["surface_paved_pct"] = surfacePavedPct
	m["surface_gravel_pct"] = surfaceGravelPct
	m["surface_trail_pct"] = surfaceTrailPct
	m["surface_unknown_pct"] = surfaceUnknownPct
	m["commute_detected"] = isCommute

	// Sport-specific fields
	derefInt16 := func(i *int16) any {
		if i == nil {
			return nil
		}
		return int(*i)
	}
	setNN("swim_stroke", derefStr(swimStroke))
	setNN("pool_length_m", derefF(poolLengthM))
	setNN("num_active_lengths", derefInt(numActiveLengths))
	setNN("max_depth_m", derefF(maxDepthM))
	setNN("avg_depth_m", derefF(avgDepthM))
	setNN("end_cns_pct", derefInt16(endCnsPct))
	setNN("jump_count", derefInt(jumpCount))
	setNN("total_grit", derefF(totalGrit))
	setNN("avg_flow", derefF(avgFlow))
	setNN("avg_lev_motor_power_w", derefInt(avgLevMotorPowerW))
	setNN("lev_battery_consumption_pct", derefF(levBatteryPct))
	m["is_multisport_child"] = parentID != nil
	m["is_multisport_parent"] = numSessions != nil && *numSessions > 1

	// Training metrics
	setNN("total_calories", derefInt(totalCalories))
	setNN("aerobic_training_effect", derefF(aerobicTE))
	setNN("anaerobic_training_effect", derefF(anaerobicTE))
	setNN("training_load_peak", derefF(trainingLoadPeak))
	setNN("avg_spo2_pct", derefInt(avgSpo2Pct))
	setNN("avg_respiration_rate", derefF(avgRespirationRate))
	setNN("avg_stress_level", derefInt(avgStressLevel))
	setNN("session_rmssd_ms", derefInt(sessionRmssdMs))
	setNN("avg_core_temperature_c", derefF(avgCoreTemperature))
	setNN("threshold_power_w", derefInt(thresholdPowerW))
	setNN("aerobic_decoupling", derefF(aerobicDecoupling))
	setNN("variability_index", derefF(variabilityIndex))
	setNN("efficiency_factor", derefF(efficiencyFactor))
	setNN("intensity_factor", derefF(intensityFactor))
	setNN("workout_rpe", derefInt(workoutRPE))
	setNN("workout_feel", derefInt(workoutFeel))

	return m, nil
}

func (db *DB) getSegmentEffortsForActivity(ctx context.Context, activityID string) ([]map[string]any, error) {
	rows, err := db.Pool.Query(ctx, `
		SELECT
			se.segment_id::text,
			se.elapsed_time_s,
			(SELECT COUNT(*) + 1 FROM segment_efforts se2
			 WHERE se2.segment_id = se.segment_id AND se2.elapsed_time_s < se.elapsed_time_s
			 AND se2.user_id = se.user_id)::int AS rank
		FROM segment_efforts se
		WHERE se.activity_id = $1`,
		activityID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []map[string]any
	for rows.Next() {
		var segID string
		var elapsedS, rank int
		if err := rows.Scan(&segID, &elapsedS, &rank); err != nil {
			continue
		}
		result = append(result, map[string]any{
			"segment_id":     segID,
			"elapsed_time_s": elapsedS,
			"rank":           rank,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

// ApplyRuleChange applies a single rule change to an activity.
func (db *DB) ApplyRuleChange(ctx context.Context, activityID string, field string, value any) error {
	switch field {
	case "gear_id":
		gearID, ok := value.(string)
		if !ok {
			return fmt.Errorf("gear_id rule change: value must be a UUID string, got %T", value)
		}
		if gearID == "" {
			return ClearPrimaryGearForActivityTx(ctx, db.Pool, activityID)
		}
		return SetPrimaryGearForActivityTx(ctx, db.Pool, activityID, gearID, "rule")
	case "add_tag":
		tagID, ok := value.(string)
		if !ok {
			return fmt.Errorf("add_tag value must be a tag_id string")
		}
		// Note: adding system tags 'Indoor', 'E-Bike', or 'Pendeln' via this path
		// triggers sync_system_flags_and_tags (migration 000268) which automatically
		// sets is_indoor, is_electric, or is_commute on the activities row.
		// This is intentional — the DB trigger keeps boolean flags and tags in sync.
		// Resolve tag name
		var tagName string
		var isSystem bool
		if err := db.Pool.QueryRow(ctx,
			`SELECT name, is_system FROM tags WHERE id = $1`, tagID,
		).Scan(&tagName, &isSystem); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				slog.Warn("add_tag: tag no longer exists, skipping", "tag_id", tagID, "activity_id", activityID)
				return nil
			}
			return fmt.Errorf("add_tag: resolve tag: %w", err)
		}
		// System-Tags: respect activity_tag_removals
		if isSystem {
			var removed bool
			_ = db.Pool.QueryRow(ctx,
				`SELECT EXISTS(SELECT 1 FROM activity_tag_removals WHERE activity_id = $1 AND tag_id = $2)`,
				activityID, tagID,
			).Scan(&removed)
			if removed {
				return nil // User explicitly removed this tag — skip
			}
		}
		// Junction table
		_, _ = db.Pool.Exec(ctx,
			`INSERT INTO activity_tags (activity_id, tag_id, assigned_by)
			 VALUES ($1, $2, 'rule') ON CONFLICT (activity_id, tag_id) DO NOTHING`,
			activityID, tagID)
		// TEXT[] cache
		_, _ = db.Pool.Exec(ctx,
			`UPDATE activities SET tags = array_append(tags, $1)
			 WHERE id = $2 AND NOT ($1 = ANY(COALESCE(tags, '{}')))`,
			tagName, activityID)
		// activity_count
		_, _ = db.Pool.Exec(ctx,
			`UPDATE tags SET activity_count = (SELECT COUNT(*) FROM activity_tags WHERE tag_id = $1) WHERE id = $1`,
			tagID)
		return nil
	case "remove_tag":
		tagID, ok := value.(string)
		if !ok {
			return fmt.Errorf("remove_tag value must be a tag_id string")
		}
		var tagName string
		if err := db.Pool.QueryRow(ctx,
			`SELECT name FROM tags WHERE id = $1`, tagID,
		).Scan(&tagName); err != nil {
			return fmt.Errorf("remove_tag: resolve tag: %w", err)
		}
		_, _ = db.Pool.Exec(ctx,
			`DELETE FROM activity_tags WHERE activity_id = $1 AND tag_id = $2`,
			activityID, tagID)
		_, _ = db.Pool.Exec(ctx,
			`UPDATE activities SET tags = array_remove(tags, $1) WHERE id = $2`,
			tagName, activityID)
		_, _ = db.Pool.Exec(ctx,
			`UPDATE tags SET activity_count = (SELECT COUNT(*) FROM activity_tags WHERE tag_id = $1) WHERE id = $1`,
			tagID)
		return nil
	case "visibility":
		_, err := db.Pool.Exec(ctx, `UPDATE activities SET visibility = $1 WHERE id = $2`, value, activityID)
		return err
	case "sport_type":
		rawSportType, ok := value.(string)
		if !ok {
			return fmt.Errorf("sport_type value must be a string")
		}
		tuple, _, err := sport.NormalizeSelectableSportTuple(rawSportType, "")
		if err != nil {
			return err
		}
		// Guard: don't override manually set sport types
		var isManual bool
		_ = db.Pool.QueryRow(ctx, `SELECT sport_type_is_manual FROM activities WHERE id = $1`, activityID).Scan(&isManual)
		if isManual {
			return nil // skip — user corrected this manually
		}
		_, err = db.Pool.Exec(ctx, `UPDATE activities SET sport_type = $1, sub_type = NULL WHERE id = $2`, tuple.SportType, activityID)
		return err
	case "sub_type":
		_, err := db.Pool.Exec(ctx, `UPDATE activities SET sub_type = $1 WHERE id = $2`, value, activityID)
		return err
	case "sport_tuple":
		tuple, ok := value.(rules.SportTupleChange)
		if !ok {
			return fmt.Errorf("sport_tuple value must be a SportTupleChange")
		}
		var isManual bool
		_ = db.Pool.QueryRow(ctx, `SELECT sport_type_is_manual FROM activities WHERE id = $1`, activityID).Scan(&isManual)
		if isManual {
			return nil
		}
		normalized, _, err := sport.NormalizeSelectableSportTuple(tuple.SportType, tuple.SubType)
		if err != nil {
			return err
		}
		_, err = db.Pool.Exec(ctx,
			`UPDATE activities SET sport_type = $1, sub_type = NULLIF($2, '') WHERE id = $3`,
			normalized.SportType, normalized.SubType, activityID)
		return err
	case "intensity":
		_, err := db.Pool.Exec(ctx, `UPDATE activities SET intensity = $1 WHERE id = $2`, value, activityID)
		return err
	case "is_indoor":
		_, err := db.Pool.Exec(ctx, `UPDATE activities SET is_indoor = $1 WHERE id = $2`, value, activityID)
		return err
	case "is_electric":
		_, err := db.Pool.Exec(ctx, `UPDATE activities SET is_electric = $1 WHERE id = $2`, value, activityID)
		return err
	case "is_virtual":
		_, err := db.Pool.Exec(ctx, `UPDATE activities SET is_virtual = $1 WHERE id = $2`, value, activityID)
		return err
	case "is_commute":
		_, err := db.Pool.Exec(ctx, `UPDATE activities SET is_commute = $1 WHERE id = $2`, value, activityID)
		return err
	case "is_round_trip":
		_, err := db.Pool.Exec(ctx, `UPDATE activities SET is_round_trip = $1 WHERE id = $2`, value, activityID)
		return err
	case "exclude_from_stats":
		_, err := db.Pool.Exec(ctx, `UPDATE activities SET exclude_from_stats = $1 WHERE id = $2`, value, activityID)
		return err
	case "exclude_from_heatmap":
		_, err := db.Pool.Exec(ctx, `UPDATE activities SET exclude_from_heatmap = $1 WHERE id = $2`, value, activityID)
		return err
	case "hide_map":
		_, err := db.Pool.Exec(ctx, `UPDATE activities SET hide_map = $1 WHERE id = $2`, value, activityID)
		return err
	case "set_name_template":
		title, ok := value.(string)
		if !ok {
			return fmt.Errorf("set_name_template value must be a resolved string")
		}
		_, err := db.Pool.Exec(ctx, `UPDATE activities SET title = $1, title_is_manual = true WHERE id = $2`, title, activityID)
		return err
	case "notify":
		// Handled upstream in ApplyRulesToActivity (requires userID).
		// ApplyRuleChange is intentionally not called for this field; this case
		// exists only so the default branch never fires for "notify".
		return nil
	case "webhook":
		// Handled upstream in ApplyRulesToActivity (requires HTTP call).
		// ApplyRuleChange is intentionally not called for this field; this case
		// exists only so the default branch never fires for "webhook".
		return nil
	default:
		return fmt.Errorf("unknown field: %s", field)
	}
}

// --- Bike Setups ---

const bikeSetupCols = `id, gear_id, name, rider_weight_kg, intended_use, notes,
	suspension_front, suspension_rear, tires, contact_points, brakes, drivetrain,
	is_active, component_snapshot, created_at::text, updated_at::text`

func scanBikeSetup(row pgx.Row) (*model.BikeSetup, error) {
	var s model.BikeSetup
	err := row.Scan(
		&s.ID, &s.GearID, &s.Name, &s.RiderWeightKg, &s.IntendedUse, &s.Notes,
		&s.SuspensionFront, &s.SuspensionRear, &s.Tires, &s.ContactPoints, &s.Brakes, &s.Drivetrain,
		&s.IsActive, &s.ComponentSnapshot, &s.CreatedAt, &s.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &s, nil
}

// componentSnapshotEnvelope is the versioned JSONB envelope stored in
// bike_setups.component_snapshot. Version 1 fields must never be renamed;
// future versions add new top-level fields.
type componentSnapshotEnvelope struct {
	Version    int                   `json:"version"`
	CapturedAt string                `json:"captured_at"` // RFC3339
	Components []componentSnapshotV1 `json:"components"`
}

type componentSnapshotV1 struct {
	ID                string          `json:"id"`
	Name              string          `json:"name"`
	Type              string          `json:"type"`
	TypeKey           *string         `json:"type_key,omitempty"`
	SlotKey           *string         `json:"slot_key,omitempty"`
	Brand             *string         `json:"brand,omitempty"`
	Model             *string         `json:"model,omitempty"`
	Attributes        json.RawMessage `json:"attributes,omitempty"`
	PresetGearTypeKey *string         `json:"preset_gear_type_key,omitempty"`
	InstalledAt       *string         `json:"installed_at,omitempty"`
	RemovedAt         *string         `json:"removed_at,omitempty"`
	Status            string          `json:"status"`
}

// buildComponentSnapshotForGear captures the current gear_components state for
// the given gearID and serialises it as a componentSnapshotEnvelope (version 1).
func (db *DB) buildComponentSnapshotForGear(ctx context.Context, gearID string) (json.RawMessage, error) {
	components, err := db.listComponents(ctx, gearID)
	if err != nil {
		return nil, fmt.Errorf("build component snapshot: %w", err)
	}
	items := make([]componentSnapshotV1, 0, len(components))
	for _, c := range components {
		var attrsRaw json.RawMessage
		if len(c.Attributes) > 0 {
			b, err := json.Marshal(c.Attributes)
			if err != nil {
				return nil, fmt.Errorf("build component snapshot marshal attrs: %w", err)
			}
			attrsRaw = b
		}
		items = append(items, componentSnapshotV1{
			ID:                c.ID,
			Name:              c.Name,
			Type:              c.Type,
			TypeKey:           c.TypeKey,
			SlotKey:           c.SlotKey,
			Brand:             c.Brand,
			Model:             c.Model,
			Attributes:        attrsRaw,
			PresetGearTypeKey: c.PresetGearTypeKey,
			InstalledAt:       c.InstalledAt,
			RemovedAt:         c.RemovedAt,
			Status:            c.Status,
		})
	}
	envelope := componentSnapshotEnvelope{
		Version:    1,
		CapturedAt: time.Now().UTC().Format(time.RFC3339),
		Components: items,
	}
	raw, err := json.Marshal(envelope)
	if err != nil {
		return nil, fmt.Errorf("build component snapshot marshal envelope: %w", err)
	}
	return raw, nil
}

func (db *DB) ListBikeSetups(ctx context.Context, gearID string) ([]model.BikeSetup, error) {
	rows, err := db.Pool.Query(ctx,
		`SELECT `+bikeSetupCols+` FROM bike_setups WHERE gear_id = $1 ORDER BY is_active DESC, name`, gearID,
	)
	if err != nil {
		return nil, fmt.Errorf("list bike setups: %w", err)
	}
	defer rows.Close()

	var items []model.BikeSetup
	for rows.Next() {
		var s model.BikeSetup
		if err := rows.Scan(
			&s.ID, &s.GearID, &s.Name, &s.RiderWeightKg, &s.IntendedUse, &s.Notes,
			&s.SuspensionFront, &s.SuspensionRear, &s.Tires, &s.ContactPoints, &s.Brakes, &s.Drivetrain,
			&s.IsActive, &s.ComponentSnapshot, &s.CreatedAt, &s.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan bike setup: %w", err)
		}
		items = append(items, s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list bike setups: %w", err)
	}
	if items == nil {
		items = []model.BikeSetup{}
	}
	return items, nil
}

func (db *DB) CreateBikeSetup(ctx context.Context, gearID string, create *model.BikeSetupCreate) (*model.BikeSetup, error) {
	var snapshot json.RawMessage
	if create.CaptureComponents {
		var err error
		snapshot, err = db.buildComponentSnapshotForGear(ctx, gearID)
		if err != nil {
			return nil, fmt.Errorf("create bike setup: capture snapshot: %w", err)
		}
	}
	s, err := scanBikeSetup(db.Pool.QueryRow(ctx,
		`INSERT INTO bike_setups (gear_id, name, rider_weight_kg, intended_use, notes,
		  suspension_front, suspension_rear, tires, contact_points, brakes, drivetrain,
		  is_active, component_snapshot)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
		 RETURNING `+bikeSetupCols,
		gearID, create.Name, create.RiderWeightKg, create.IntendedUse, create.Notes,
		create.SuspensionFront, create.SuspensionRear, create.Tires,
		create.ContactPoints, create.Brakes, create.Drivetrain, create.IsActive,
		snapshot,
	))
	if err != nil {
		return nil, fmt.Errorf("create bike setup: %w", err)
	}
	return s, nil
}

func (db *DB) UpdateBikeSetup(ctx context.Context, id string, update *model.BikeSetupUpdate) (*model.BikeSetup, error) {
	b := newSetBuilder(1)

	if update.Name != nil {
		b.Set("name", *update.Name)
	}
	if update.RiderWeightKg != nil {
		b.Set("rider_weight_kg", *update.RiderWeightKg)
	}
	if update.IntendedUse != nil {
		b.Set("intended_use", *update.IntendedUse)
	}
	if update.Notes != nil {
		b.Set("notes", *update.Notes)
	}
	if update.SuspensionFront != nil {
		b.Set("suspension_front", update.SuspensionFront)
	}
	if update.SuspensionRear != nil {
		b.Set("suspension_rear", update.SuspensionRear)
	}
	if update.Tires != nil {
		b.Set("tires", update.Tires)
	}
	if update.ContactPoints != nil {
		b.Set("contact_points", update.ContactPoints)
	}
	if update.Brakes != nil {
		b.Set("brakes", update.Brakes)
	}
	if update.Drivetrain != nil {
		b.Set("drivetrain", update.Drivetrain)
	}
	if update.IsActive != nil {
		b.Set("is_active", *update.IsActive)
	}

	// CaptureComponents three-state:
	//   nil  → leave component_snapshot unchanged (no-op)
	//   true → rebuild snapshot from current gear_components
	//   false → clear snapshot (set to NULL)
	if update.CaptureComponents != nil {
		if *update.CaptureComponents {
			// Fetch gear_id so we can look up components.
			var gearID string
			if err := db.Pool.QueryRow(ctx,
				`SELECT gear_id FROM bike_setups WHERE id = $1`, id,
			).Scan(&gearID); err != nil {
				if errors.Is(err, pgx.ErrNoRows) {
					return nil, nil
				}
				return nil, fmt.Errorf("update bike setup: fetch gear_id for snapshot: %w", err)
			}
			snapshot, err := db.buildComponentSnapshotForGear(ctx, gearID)
			if err != nil {
				return nil, fmt.Errorf("update bike setup: capture snapshot: %w", err)
			}
			b.Set("component_snapshot", snapshot)
		} else {
			b.Set("component_snapshot", nil)
		}
	}

	if !b.HasSets() {
		s, err := scanBikeSetup(db.Pool.QueryRow(ctx,
			`SELECT `+bikeSetupCols+` FROM bike_setups WHERE id = $1`, id,
		))
		if err != nil {
			return nil, fmt.Errorf("get bike setup: %w", err)
		}
		return s, nil
	}

	args := append(b.Args(), id)
	query := fmt.Sprintf(
		`UPDATE bike_setups SET %s WHERE id = $%d RETURNING `+bikeSetupCols,
		b.Clause(), b.NextIdx(),
	)

	s, err := scanBikeSetup(db.Pool.QueryRow(ctx, query, args...))
	if err != nil {
		return nil, fmt.Errorf("update bike setup: %w", err)
	}
	return s, nil
}

func (db *DB) DeleteBikeSetup(ctx context.Context, id string) error {
	tag, err := db.Pool.Exec(ctx, `DELETE FROM bike_setups WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete bike setup: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrBikeSetupNotFound
	}
	return nil
}

// --- Gear Documents ---

const gearDocSelectCols = `id, user_id, gear_id, component_id, service_event_id, doc_type, title, original_filename, content_type, file_size_bytes, file_hash, file_id, extracted_text, processing_status, created_at`

func scanGearDocument(row pgx.Row) (*model.GearDocument, error) {
	var d model.GearDocument
	err := row.Scan(&d.ID, &d.UserID, &d.GearID, &d.ComponentID, &d.ServiceEventID, &d.DocType, &d.Title, &d.OriginalFilename, &d.ContentType, &d.FileSizeBytes, &d.FileHash, &d.FileID, &d.ExtractedText, &d.ProcessingStatus, &d.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("scan gear document: %w", err)
	}
	return &d, nil
}

func (db *DB) GearDocumentExistsByHash(ctx context.Context, userID, hash string) (bool, error) {
	var exists bool
	err := db.Pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM gear_documents WHERE user_id = $1 AND file_hash = $2)`,
		userID, hash,
	).Scan(&exists)
	return exists, err
}

func (db *DB) GearDocumentExistsByHashAndGear(ctx context.Context, userID, hash, gearID string) (bool, error) {
	var exists bool
	err := db.Pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM gear_documents WHERE user_id = $1 AND file_hash = $2 AND gear_id = $3)`,
		userID, hash, gearID,
	).Scan(&exists)
	return exists, err
}

func (db *DB) GearDocumentExistsByHashAndComponent(ctx context.Context, userID, hash, componentID string) (bool, error) {
	var exists bool
	err := db.Pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM gear_documents WHERE user_id = $1 AND file_hash = $2 AND component_id = $3)`,
		userID, hash, componentID,
	).Scan(&exists)
	return exists, err
}

func (db *DB) InsertGearDocument(ctx context.Context, doc *model.GearDocument) (*model.GearDocument, error) {
	return scanGearDocument(db.Pool.QueryRow(ctx, `
		INSERT INTO gear_documents (user_id, gear_id, component_id, service_event_id, doc_type, title, original_filename, content_type, file_size_bytes, file_hash, file_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		RETURNING `+gearDocSelectCols,
		doc.UserID, doc.GearID, doc.ComponentID, doc.ServiceEventID, doc.DocType, doc.Title, doc.OriginalFilename, doc.ContentType, doc.FileSizeBytes, doc.FileHash, doc.FileID,
	))
}

func (db *DB) ListGearDocuments(ctx context.Context, gearID string) ([]model.GearDocument, error) {
	rows, err := db.Pool.Query(ctx, `SELECT `+gearDocSelectCols+` FROM gear_documents WHERE gear_id = $1 ORDER BY created_at DESC`, gearID)
	if err != nil {
		return nil, fmt.Errorf("list gear documents: %w", err)
	}
	defer rows.Close()
	var docs []model.GearDocument
	for rows.Next() {
		var d model.GearDocument
		if err := rows.Scan(&d.ID, &d.UserID, &d.GearID, &d.ComponentID, &d.ServiceEventID, &d.DocType, &d.Title, &d.OriginalFilename, &d.ContentType, &d.FileSizeBytes, &d.FileHash, &d.FileID, &d.ExtractedText, &d.ProcessingStatus, &d.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan gear document: %w", err)
		}
		docs = append(docs, d)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list gear documents: %w", err)
	}
	if docs == nil {
		docs = []model.GearDocument{}
	}
	return docs, nil
}

func (db *DB) ListComponentDocuments(ctx context.Context, componentID string) ([]model.GearDocument, error) {
	rows, err := db.Pool.Query(ctx, `SELECT `+gearDocSelectCols+` FROM gear_documents WHERE component_id = $1 ORDER BY created_at DESC`, componentID)
	if err != nil {
		return nil, fmt.Errorf("list component documents: %w", err)
	}
	defer rows.Close()
	var docs []model.GearDocument
	for rows.Next() {
		var d model.GearDocument
		if err := rows.Scan(&d.ID, &d.UserID, &d.GearID, &d.ComponentID, &d.ServiceEventID, &d.DocType, &d.Title, &d.OriginalFilename, &d.ContentType, &d.FileSizeBytes, &d.FileHash, &d.FileID, &d.ExtractedText, &d.ProcessingStatus, &d.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan component document: %w", err)
		}
		docs = append(docs, d)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list component documents: %w", err)
	}
	if docs == nil {
		docs = []model.GearDocument{}
	}
	return docs, nil
}

func (db *DB) GetGearDocument(ctx context.Context, docID string) (*model.GearDocument, error) {
	return scanGearDocument(db.Pool.QueryRow(ctx, `SELECT `+gearDocSelectCols+` FROM gear_documents WHERE id = $1`, docID))
}

// DeleteGearDocument deletes a gear document and returns its file_id for cleanup.
func (db *DB) DeleteGearDocument(ctx context.Context, docID, userID string) (fileID string, err error) {
	var fID *string
	err = db.Pool.QueryRow(ctx,
		`DELETE FROM gear_documents WHERE id = $1 AND user_id = $2 RETURNING file_id::text`,
		docID, userID,
	).Scan(&fID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("delete gear document: %w", err)
	}
	if fID != nil {
		return *fID, nil
	}
	return "", nil
}

// UpdateGearDocumentFileID sets the file_id (ShadowFS registry ID) on a gear document.
func (db *DB) UpdateGearDocumentFileID(ctx context.Context, docID, fileID string) error {
	_, err := db.Pool.Exec(ctx,
		`UPDATE gear_documents SET file_id = $1 WHERE id = $2`,
		fileID, docID)
	return err
}

// ListGearDocumentFileIDsByGearID returns all (docID, fileID) pairs for documents
// belonging to a gear item (directly or via its components).
// Used by DeleteGear to clean up file storage.
func (db *DB) ListGearDocumentFileIDsByGearID(ctx context.Context, gearID string) ([]struct {
	DocID  string
	FileID string
}, error) {
	rows, err := db.Pool.Query(ctx, `
		SELECT id, COALESCE(file_id::text, '')
		FROM gear_documents
		WHERE gear_id = $1
		   OR component_id IN (SELECT id FROM gear_components WHERE gear_id = $1)
	`, gearID)
	if err != nil {
		return nil, fmt.Errorf("list gear document file ids: %w", err)
	}
	defer rows.Close()

	var results []struct {
		DocID  string
		FileID string
	}
	for rows.Next() {
		var e struct {
			DocID  string
			FileID string
		}
		if err := rows.Scan(&e.DocID, &e.FileID); err != nil {
			return nil, fmt.Errorf("scan gear document file id: %w", err)
		}
		results = append(results, e)
	}
	return results, rows.Err()
}

// ListGearDocumentFileIDsByServiceEventID returns the file_id for all documents
// linked to a service event. Used by DeleteServiceEvent to clean up files before DB delete.
func (db *DB) ListGearDocumentFileIDsByServiceEventID(ctx context.Context, eventID string) ([]struct {
	FileID string
}, error) {
	rows, err := db.Pool.Query(ctx, `
		SELECT COALESCE(file_id::text, '')
		FROM gear_documents
		WHERE service_event_id = $1
	`, eventID)
	if err != nil {
		return nil, fmt.Errorf("list service event document file ids: %w", err)
	}
	defer rows.Close()

	var results []struct {
		FileID string
	}
	for rows.Next() {
		var e struct {
			FileID string
		}
		if err := rows.Scan(&e.FileID); err != nil {
			return nil, fmt.Errorf("scan service event document file id: %w", err)
		}
		results = append(results, e)
	}
	return results, rows.Err()
}

func (db *DB) GetGearDocumentsByGearIDs(ctx context.Context, gearIDs []string) (map[string]string, error) {
	rows, err := db.Pool.Query(ctx, `
		SELECT DISTINCT ON (gear_id) gear_id, id
		FROM gear_documents
		WHERE gear_id = ANY($1) AND doc_type = 'photo' AND content_type LIKE 'image/%'
		ORDER BY gear_id, created_at ASC`, gearIDs)
	if err != nil {
		return nil, fmt.Errorf("get gear preview docs: %w", err)
	}
	defer rows.Close()
	result := make(map[string]string)
	for rows.Next() {
		var gearID, docID string
		if err := rows.Scan(&gearID, &docID); err != nil {
			return nil, fmt.Errorf("scan gear preview doc: %w", err)
		}
		result[gearID] = docID
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("get gear preview docs: %w", err)
	}
	return result, nil
}

func (db *DB) ActivateBikeSetup(ctx context.Context, id string) (*model.BikeSetup, error) {
	s, err := scanBikeSetup(db.Pool.QueryRow(ctx,
		`UPDATE bike_setups SET is_active = true
		 WHERE id = $1 RETURNING `+bikeSetupCols, id,
	))
	if err != nil {
		return nil, fmt.Errorf("activate bike setup: %w", err)
	}
	return s, nil
}

// GetGearCostSummary returns the cost overview for a gear item.
func (db *DB) GetGearCostSummary(ctx context.Context, gearID string) (*model.GearCostSummary, error) {
	var s model.GearCostSummary
	err := db.Pool.QueryRow(ctx,
		`SELECT gear_id, user_id, name, total_cost, currency, cost_per_km, cost_per_hour
		 FROM gear_cost_summary WHERE gear_id = $1`,
		gearID,
	).Scan(&s.GearID, &s.UserID, &s.Name, &s.TotalCost, &s.Currency, &s.CostPerKm, &s.CostPerHour)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get gear cost summary: %w", err)
	}
	return &s, nil
}

// UpdateGearDocumentText updates the extracted_text field of a gear document.
func (db *DB) UpdateGearDocumentText(ctx context.Context, docID, text string) error {
	_, err := db.Pool.Exec(ctx,
		`UPDATE gear_documents SET extracted_text = $1, processing_status = 'done' WHERE id = $2`,
		text, docID,
	)
	return err
}

// UpdateGearReviewStatus updates the review_status of a gear item.
func (db *DB) UpdateGearReviewStatus(ctx context.Context, gearID, userID, status string) (*model.Gear, error) {
	var g model.Gear
	err := db.Pool.QueryRow(ctx,
		`UPDATE gear SET review_status = $1 WHERE id = $2 AND user_id = $3
		 RETURNING id, name, type, review_status, created_at`,
		status, gearID, userID,
	).Scan(&g.ID, &g.Name, &g.Type, &g.ReviewStatus, &g.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("update gear review status: %w", err)
	}
	return &g, nil
}

// ListPendingReviewGear returns gear items pending user review.
func (db *DB) ListPendingReviewGear(ctx context.Context, userID string) ([]model.Gear, error) {
	rows, err := db.Pool.Query(ctx,
		`SELECT id, name, type, review_status, source_document_id, created_at
		 FROM gear WHERE user_id = $1 AND review_status = 'pending_review'
		 ORDER BY created_at DESC`,
		userID,
	)
	if err != nil {
		return nil, fmt.Errorf("list pending review gear: %w", err)
	}
	defer rows.Close()
	var items []model.Gear
	for rows.Next() {
		var g model.Gear
		if err := rows.Scan(&g.ID, &g.Name, &g.Type, &g.ReviewStatus, &g.SourceDocumentID, &g.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan pending review gear: %w", err)
		}
		items = append(items, g)
	}
	if items == nil {
		items = []model.Gear{}
	}
	return items, rows.Err()
}

// MergeGear merges secondaryID into primaryID.
// Fields of the secondary fill gaps of the primary (via COALESCE), activities,
// activity_gear_assignments, and components are reassigned, and the secondary
// record is deleted — all within a single transaction.
//
// aga.gear_id has ON DELETE CASCADE on gear, so the final
// `DELETE FROM gear WHERE id = secondaryID` would destroy any aga row still
// pointing at the secondary. We reassign aga rows to primaryID before the DELETE.
func (db *DB) MergeGear(ctx context.Context, userID, primaryID, secondaryID string) error {
	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	// 1. Fill empty fields of primary from secondary (COALESCE keeps primary value when non-NULL).
	// g1.user_id = $3 enforces ownership: a user cannot merge another user's gear as primary.
	_, err = tx.Exec(ctx, `
		UPDATE gear g1 SET
			brand        = COALESCE(g1.brand,        g2.brand),
			model        = COALESCE(g1.model,        g2.model),
			sub_type     = COALESCE(g1.sub_type,     g2.sub_type),
			notes        = COALESCE(g1.notes,        g2.notes),
			purchased_at = COALESCE(g1.purchased_at, g2.purchased_at),
			weight_g     = COALESCE(g1.weight_g,     g2.weight_g)
		FROM gear g2
		WHERE g1.id = $1 AND g2.id = $2 AND g1.user_id = $3`,
		primaryID, secondaryID, userID,
	)
	if err != nil {
		return fmt.Errorf("merge gear fields: %w", err)
	}

	// 2. Reassign aga rows from secondary to primary, before the secondary
	// gear is deleted (ON DELETE CASCADE on aga.gear_id would otherwise
	// drop user-source rows). Conflict resolution on the partial-unique
	// idx_activity_gear_assignments_one_primary: if the same activity already
	// has a primary aga row pointing at primaryID (rare; happens when both
	// gears were assigned to the activity at different roles), keep the
	// older row by created_at and delete the secondary one before the UPDATE.
	if _, err = tx.Exec(ctx, `
		DELETE FROM activity_gear_assignments aga_sec
		 USING activity_gear_assignments aga_prim
		 WHERE aga_sec.gear_id  = $2
		   AND aga_prim.gear_id = $1
		   AND aga_sec.activity_id = aga_prim.activity_id
		   AND aga_sec.role_key    = aga_prim.role_key
		   AND aga_sec.user_id     = $3
		   AND aga_prim.user_id    = $3
		   AND aga_sec.created_at >= aga_prim.created_at
	`, primaryID, secondaryID, userID); err != nil {
		return fmt.Errorf("merge gear: dedupe aga conflicts: %w", err)
	}

	if _, err = tx.Exec(ctx, `
		UPDATE activity_gear_assignments
		   SET gear_id    = $1,
		       updated_at = now()
		 WHERE gear_id = $2
		   AND user_id = $3
	`, primaryID, secondaryID, userID); err != nil {
		return fmt.Errorf("reassign aga rows: %w", err)
	}

	// 3. Reassign components from secondary to primary
	_, err = tx.Exec(ctx,
		`UPDATE gear_components SET gear_id = $1 WHERE gear_id = $2`,
		primaryID, secondaryID,
	)
	if err != nil {
		return fmt.Errorf("reassign components: %w", err)
	}

	// 4. Recompute analytics totals on primary so distance/time/elevation
	// reflect the merged set of activities. Mirrors the SQL pattern used in
	// migration 000355's final analytics-recompute step.
	if _, err = tx.Exec(ctx, `
		UPDATE gear g
		   SET total_distance_m       = COALESCE(src.total_distance_m, 0),
		       total_moving_time_s    = COALESCE(src.total_moving_time_s, 0),
		       total_time_s           = COALESCE(src.total_time_s, 0),
		       total_elevation_gain_m = COALESCE(src.total_elevation_gain_m, 0),
		       total_elevation_loss_m = COALESCE(src.total_elevation_loss_m, 0),
		       total_activities       = COALESCE(src.total_activities, 0)
		  FROM (
		      SELECT
		          COALESCE(SUM(COALESCE(aga.distance_m_override,    a.distance_m,    0)), 0) AS total_distance_m,
		          COALESCE(SUM(COALESCE(aga.moving_time_s_override, a.moving_time_s, 0)), 0) AS total_moving_time_s,
		          COALESCE(SUM(COALESCE(a.elapsed_time_s,    0)), 0)                          AS total_time_s,
		          COALESCE(SUM(COALESCE(a.elevation_gain_m,  0)), 0)                          AS total_elevation_gain_m,
		          COALESCE(SUM(COALESCE(a.elevation_loss_m,  0)), 0)                          AS total_elevation_loss_m,
		          COUNT(*)                                                                     AS total_activities
		      FROM activity_gear_assignments aga
		      JOIN activities a ON a.id = aga.activity_id
		      WHERE aga.gear_id = $1
		        AND NOT a.is_group_secondary
		  ) src
		 WHERE g.id = $1
	`, primaryID); err != nil {
		return fmt.Errorf("recompute primary totals: %w", err)
	}

	// 5. Delete secondary gear item. After the aga reassignment above, the
	// CASCADE on aga.gear_id is a no-op (no aga rows still point at
	// secondaryID); only chain_rotation_groups, gear_documents etc. that
	// genuinely belong to the secondary cascade away.
	_, err = tx.Exec(ctx,
		`DELETE FROM gear WHERE id = $1 AND user_id = $2`, secondaryID, userID,
	)
	if err != nil {
		return fmt.Errorf("delete secondary gear: %w", err)
	}

	return tx.Commit(ctx)
}

// VerifyGearOwnership returns nil if the gear belongs to the user, pgx.ErrNoRows otherwise.
func (db *DB) VerifyGearOwnership(ctx context.Context, gearID, userID string) error {
	var id string
	return db.Pool.QueryRow(ctx,
		`SELECT id FROM gear WHERE id = $1 AND user_id = $2`, gearID, userID,
	).Scan(&id)
}

// VerifyComponentOwnership returns nil if the component's gear belongs to the user.
func (db *DB) VerifyComponentOwnership(ctx context.Context, componentID, userID string) error {
	var id string
	return db.Pool.QueryRow(ctx,
		`SELECT gc.id FROM gear_components gc JOIN gear g ON gc.gear_id = g.id
		 WHERE gc.id = $1 AND g.user_id = $2`, componentID, userID,
	).Scan(&id)
}

// VerifyChainRotationOwnership returns nil if the chain rotation group's gear belongs to the user.
func (db *DB) VerifyChainRotationOwnership(ctx context.Context, groupID, userID string) error {
	var id string
	return db.Pool.QueryRow(ctx,
		`SELECT crg.id FROM chain_rotation_groups crg JOIN gear g ON crg.gear_id = g.id
		 WHERE crg.id = $1 AND g.user_id = $2`, groupID, userID,
	).Scan(&id)
}

// VerifyServiceEventOwnership returns nil if the service event's component's gear belongs to the user.
func (db *DB) VerifyServiceEventOwnership(ctx context.Context, eventID, userID string) error {
	var id string
	return db.Pool.QueryRow(ctx,
		`SELECT gse.id FROM gear_service_events gse
		 JOIN gear_components gc ON gse.component_id = gc.id
		 JOIN gear g ON gc.gear_id = g.id
		 WHERE gse.id = $1 AND g.user_id = $2`, eventID, userID,
	).Scan(&id)
}

// VerifyServiceIntervalOwnership returns nil if the service interval's component's gear belongs to the user.
func (db *DB) VerifyServiceIntervalOwnership(ctx context.Context, intervalID, userID string) error {
	var id string
	return db.Pool.QueryRow(ctx,
		`SELECT gsi.id FROM gear_service_intervals gsi
		 JOIN gear_components gc ON gsi.component_id = gc.id
		 JOIN gear g ON gc.gear_id = g.id
		 WHERE gsi.id = $1 AND g.user_id = $2`, intervalID, userID,
	).Scan(&id)
}

// VerifyGearDocumentOwnership returns nil if the gear document belongs to the user.
func (db *DB) VerifyGearDocumentOwnership(ctx context.Context, docID, userID string) error {
	var id string
	return db.Pool.QueryRow(ctx,
		`SELECT id FROM gear_documents WHERE id = $1 AND user_id = $2`, docID, userID,
	).Scan(&id)
}

// VerifyBikeSetupOwnership returns nil if the bike setup's gear belongs to the user.
func (db *DB) VerifyBikeSetupOwnership(ctx context.Context, setupID, userID string) error {
	var id string
	return db.Pool.QueryRow(ctx,
		`SELECT bs.id FROM bike_setups bs JOIN gear g ON bs.gear_id = g.id
		 WHERE bs.id = $1 AND g.user_id = $2`, setupID, userID,
	).Scan(&id)
}

// FindGearDuplicates returns pairs of gear items that are potential duplicates.
// Criteria: same type and Levenshtein distance <= 3 between names (requires fuzzystrmatch extension).
// Returns nil (no error) when fuzzystrmatch is not available.
func (db *DB) FindGearDuplicates(ctx context.Context, userID string) ([][2]string, error) {
	rows, err := db.Pool.Query(ctx, `
		SELECT a.id, b.id
		FROM gear a
		JOIN gear b ON a.user_id = b.user_id
			AND a.id < b.id
			AND a.type = b.type
			AND levenshtein(lower(a.name), lower(b.name)) <= 3
		WHERE a.user_id = $1`,
		userID,
	)
	if err != nil {
		// fuzzystrmatch extension not available — graceful fallback
		return nil, nil
	}
	defer rows.Close()
	var pairs [][2]string
	for rows.Next() {
		var a, b string
		if err := rows.Scan(&a, &b); err != nil {
			return nil, err
		}
		pairs = append(pairs, [2]string{a, b})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return pairs, nil
}

// GetRuleBySystemType returns the first is_system rule with the given system_type for a user.
// Returns nil, nil when no such rule exists.
func (db *DB) GetRuleBySystemType(ctx context.Context, userID, systemType string) (*model.AutomationRule, error) {
	var r model.AutomationRule
	err := db.Pool.QueryRow(ctx,
		`SELECT `+ruleSelectCols+`
		 FROM automation_rules
		 WHERE user_id = $1 AND system_type = $2 AND is_system = true
		 LIMIT 1`,
		userID, systemType,
	).Scan(
		&r.ID, &r.UserID, &r.Name, &r.IsActive, &r.Priority,
		&r.Conditions, &r.ConditionsLogic, &r.ConditionGroups,
		&r.Actions, &r.TimesApplied, &r.CreatedAt,
		&r.IsSystem, &r.SystemType,
		&r.TriggerEvent, &r.StopOnMatch, &r.ApplyRetroactive,
		&r.LastEvaluatedAt, &r.LastMatchedCount,
		&r.LastWarningAt, &r.LastWarningCode, &r.LastWarningMsg,
		&r.Branches,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get rule by system type: %w", err)
	}
	return &r, nil
}

// --- Gear idempotency keys ---

// IdempotencyResult holds the result of a gear idempotency key touch.
type IdempotencyResult struct {
	ResponseJSON []byte
	StatusCode   int  // HTTP status of the stored response (0 when not yet stored)
	Inserted     bool // true = cache miss; false = replayed
}

// TouchGearIdempotencyKey upserts a key row with an empty placeholder response.
// Returns (result, nil) where result.Inserted=true means cache miss (caller
// must run the mutation and call UpdateGearIdempotencyResponse).
// result.Inserted=false means replay — result.ResponseJSON and result.StatusCode
// hold the stored body and the original HTTP status code.
func (db *DB) TouchGearIdempotencyKey(ctx context.Context, userID, route, key string) (*IdempotencyResult, error) {
	var respJSON []byte
	var statusCode int16
	var inserted bool
	err := db.Pool.QueryRow(ctx, `
		INSERT INTO gear_idempotency_keys (user_id, route, key, response_jsonb, response_status_code)
		VALUES ($1, $2, $3, 'null'::jsonb, 0)
		ON CONFLICT (user_id, route, key) DO UPDATE
			SET key = EXCLUDED.key
		RETURNING response_jsonb, response_status_code, (xmax = 0) AS inserted`,
		userID, route, key,
	).Scan(&respJSON, &statusCode, &inserted)
	if err != nil {
		return nil, fmt.Errorf("touch idempotency key: %w", err)
	}
	return &IdempotencyResult{ResponseJSON: respJSON, StatusCode: int(statusCode), Inserted: inserted}, nil
}

// UpdateGearIdempotencyResponse stores the final response body and HTTP status
// for a key after a mutation. Callers invoke this only when
// TouchGearIdempotencyKey returned Inserted=true.
func (db *DB) UpdateGearIdempotencyResponse(ctx context.Context, userID, route, key string, statusCode int, responseJSON []byte) error {
	_, err := db.Pool.Exec(ctx, `
		UPDATE gear_idempotency_keys
		SET response_jsonb       = $4::jsonb,
		    response_status_code = $5
		WHERE user_id = $1 AND route = $2 AND key = $3`,
		userID, route, key, string(responseJSON), int16(statusCode),
	)
	if err != nil {
		return fmt.Errorf("update idempotency response: %w", err)
	}
	return nil
}

// --- Gear lookup-by-name ---

// GearNameCandidate is a slim gear row returned for name-ambiguity resolution.
type GearNameCandidate struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	Type        string  `json:"type"`
	PurchasedAt *string `json:"purchased_at,omitempty"`
}

// FindGearByName returns 0 candidates (no match), 1 candidate (unique match),
// or >1 candidates (ambiguous) for the given name within a user's gear.
func (db *DB) FindGearByName(ctx context.Context, userID, name string) ([]GearNameCandidate, error) {
	rows, err := db.Pool.Query(ctx,
		`SELECT id, name, type, purchased_at
		 FROM gear
		 WHERE user_id = $1 AND name = $2
		 ORDER BY created_at`,
		userID, name,
	)
	if err != nil {
		return nil, fmt.Errorf("find gear by name: %w", err)
	}
	defer rows.Close()

	var candidates []GearNameCandidate
	for rows.Next() {
		var c GearNameCandidate
		var purchasedAt pgtype.Date
		if err := rows.Scan(&c.ID, &c.Name, &c.Type, &purchasedAt); err != nil {
			return nil, fmt.Errorf("scan gear name candidate: %w", err)
		}
		if purchasedAt.Valid {
			s := purchasedAt.Time.Format("2006-01-02")
			c.PurchasedAt = &s
		}
		candidates = append(candidates, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("find gear by name: %w", err)
	}
	return candidates, nil
}

// --- Gear summary ---

// GearSummaryFilter holds filter parameters for the gear summary aggregation.
type GearSummaryFilter struct {
	Scope          *string
	TypeKey        *string
	Since          *string // YYYY-MM-DD
	Until          *string // YYYY-MM-DD (exclusive)
	IncludeRetired bool
}

// GearSummary holds aggregated gear usage totals.
type GearSummary struct {
	TotalDistanceM      float64 `json:"total_distance_m"`
	TotalMovingTimeS    int64   `json:"total_moving_time_s"`
	TotalTimeS          int64   `json:"total_time_s"`
	TotalElevationGainM float64 `json:"total_elevation_gain_m"`
	TotalElevationLossM float64 `json:"total_elevation_loss_m"`
	ActivityCount       int64   `json:"activity_count"`
	GearCount           int64   `json:"gear_count"`
}

// GetGearSummary returns aggregated usage statistics for a user's gear items.
func (db *DB) GetGearSummary(ctx context.Context, userID string, filter GearSummaryFilter) (*GearSummary, error) {
	var summary GearSummary
	err := db.Pool.QueryRow(ctx, `
		SELECT
		  COALESCE(SUM(COALESCE(aga.distance_m_override, a.distance_m, 0)), 0)        AS total_distance_m,
		  COALESCE(SUM(COALESCE(aga.moving_time_s_override, a.moving_time_s, 0)), 0)  AS total_moving_time_s,
		  COALESCE(SUM(COALESCE(a.elapsed_time_s, 0)), 0)                              AS total_time_s,
		  COALESCE(SUM(COALESCE(a.elevation_gain_m, 0)), 0)                            AS total_elevation_gain_m,
		  COALESCE(SUM(COALESCE(a.elevation_loss_m, 0)), 0)                            AS total_elevation_loss_m,
		  COUNT(DISTINCT a.id)                                                          AS activity_count,
		  COUNT(DISTINCT aga.gear_id)                                                   AS gear_count
		FROM activities a
		JOIN activity_gear_assignments aga ON aga.activity_id = a.id AND aga.role_key = 'primary' AND aga.user_id = $1
		JOIN gear g ON g.id = aga.gear_id AND g.user_id = a.user_id
		WHERE a.user_id = $1
		  AND NOT a.is_group_secondary
		  AND ($2::text IS NULL OR g.scope = $2)
		  AND ($3::text IS NULL OR g.type = $3)
		  AND ($4::date IS NULL OR a.started_at >= $4)
		  AND ($5::date IS NULL OR a.started_at <  $5)
		  AND ($6::bool OR g.is_retired = false)`,
		userID,
		filter.Scope,
		filter.TypeKey,
		filter.Since,
		filter.Until,
		filter.IncludeRetired,
	).Scan(
		&summary.TotalDistanceM,
		&summary.TotalMovingTimeS,
		&summary.TotalTimeS,
		&summary.TotalElevationGainM,
		&summary.TotalElevationLossM,
		&summary.ActivityCount,
		&summary.GearCount,
	)
	if err != nil {
		return nil, fmt.Errorf("get gear summary: %w", err)
	}
	return &summary, nil
}

// --- Gear component swap ---

// SwapComponentResult holds the result of an atomic component swap.
type SwapComponentResult struct {
	RetiredComponentID *string `json:"retired_component_id,omitempty"`
	NewComponentID     string  `json:"new_component_id"`
	SwapEventID        string  `json:"swap_event_id"`
}

// SwapGearComponent atomically retires oldComponentID (if provided), inserts
// newComponent, and records a swap event. Runs inside a transaction.
func (db *DB) SwapGearComponent(ctx context.Context, userID, gearID, slotKey string, oldComponentID *string, newCreate *model.GearComponentCreate, swapDate, swapReason *string) (*SwapComponentResult, error) {
	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("swap component: begin tx: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	var retiredID *string
	if oldComponentID != nil && *oldComponentID != "" {
		// Retire old component at swap date.
		var retiredAt interface{}
		if swapDate != nil && *swapDate != "" {
			retiredAt = *swapDate
		} else {
			retiredAt = "now()"
		}
		var swapDateStr interface{} = swapDate
		tag, err := tx.Exec(ctx,
			`UPDATE gear_components
			 SET is_retired = true, status = 'retired', removed_at = $1
			 WHERE id = $2 AND gear_id = $3`,
			swapDateStr, *oldComponentID, gearID,
		)
		if err != nil {
			return nil, fmt.Errorf("swap component: retire old: %w", err)
		}
		if tag.RowsAffected() == 0 {
			return nil, fmt.Errorf("swap component: old component %s not found on gear %s", *oldComponentID, gearID)
		}
		_ = retiredAt
		retiredID = oldComponentID
	}

	// Insert new component.
	var newComponentID string
	err = tx.QueryRow(ctx,
		`INSERT INTO gear_components (gear_id, parent_component_id, name, type, brand, model,
		  purchase_price, purchase_date, weight_g, serial_number, notes)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		 RETURNING id`,
		gearID, newCreate.ParentComponentID, newCreate.Name, newCreate.Type,
		newCreate.Brand, newCreate.Model,
		newCreate.PurchasePrice, newCreate.PurchaseDate,
		newCreate.WeightG, newCreate.SerialNumber, newCreate.Notes,
	).Scan(&newComponentID)
	if err != nil {
		return nil, fmt.Errorf("swap component: insert new: %w", err)
	}

	// Record swap event.
	var swapEventID string
	var swapDateVal interface{}
	if swapDate != nil && *swapDate != "" {
		swapDateVal = *swapDate
	} else {
		swapDateVal = "now()"
	}
	_ = swapDateVal
	err = tx.QueryRow(ctx,
		`INSERT INTO gear_component_swap_events
		  (user_id, gear_id, slot_key, retired_component_id, new_component_id, swap_date, swap_reason)
		 VALUES ($1, $2, $3, $4, $5, COALESCE($6::timestamptz, now()), $7)
		 RETURNING id`,
		userID, gearID, slotKey, retiredID, newComponentID, swapDate, swapReason,
	).Scan(&swapEventID)
	if err != nil {
		return nil, fmt.Errorf("swap component: insert swap event: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("swap component: commit: %w", err)
	}

	return &SwapComponentResult{
		RetiredComponentID: retiredID,
		NewComponentID:     newComponentID,
		SwapEventID:        swapEventID,
	}, nil
}

// ComponentNameCandidate is a slim component row for name-ambiguity resolution.
type ComponentNameCandidate struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Type string `json:"type"`
}

// FindComponentsByName returns all active components on a gear item with the given name.
func (db *DB) FindComponentsByName(ctx context.Context, gearID, name string) ([]ComponentNameCandidate, error) {
	rows, err := db.Pool.Query(ctx,
		`SELECT id, name, type FROM gear_components
		 WHERE gear_id = $1 AND name = $2 AND is_retired = false
		 ORDER BY id`,
		gearID, name,
	)
	if err != nil {
		return nil, fmt.Errorf("find components by name: %w", err)
	}
	defer rows.Close()

	var candidates []ComponentNameCandidate
	for rows.Next() {
		var c ComponentNameCandidate
		if err := rows.Scan(&c.ID, &c.Name, &c.Type); err != nil {
			return nil, fmt.Errorf("scan component name candidate: %w", err)
		}
		candidates = append(candidates, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("find components by name: %w", err)
	}
	return candidates, nil
}
