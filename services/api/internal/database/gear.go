package database

import (
	"context"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/sixdouglas/suncalc"

	"github.com/skadia-app/skadia/services/api/internal/model"
	"github.com/skadia-app/skadia/services/api/internal/rules"
)

// gearSelectCols are the columns returned by gear queries.
const gearSelectCols = `id, user_id, name, type, sub_type, brand, model,
	total_distance_m, total_time_s, total_elevation_gain_m, total_activities,
	purchased_at, retired_at, is_retired, status, default_for_sports, notes, created_at`

// scanGear scans a gear row into a model.Gear.
// pgx v5 uses binary protocol for dates (OID 1082) which cannot be scanned
// directly into *string — use pgtype.Date as intermediate and format manually.
func scanGear(row pgx.Row) (*model.Gear, error) {
	var g model.Gear
	var purchasedAt, retiredAt pgtype.Date
	err := row.Scan(
		&g.ID, &g.UserID, &g.Name, &g.Type, &g.SubType, &g.Brand, &g.Model,
		&g.TotalDistanceM, &g.TotalTimeS, &g.TotalElevationGainM, &g.TotalActivities,
		&purchasedAt, &retiredAt, &g.IsRetired, &g.Status, &g.DefaultForSports, &g.Notes, &g.CreatedAt,
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
	if g.DefaultForSports == nil {
		g.DefaultForSports = []string{}
	}
	return &g, nil
}

// componentSelectCols are the columns returned by component queries.
const componentSelectCols = `id, gear_id, parent_component_id, name, type, brand, model,
	total_distance_m, total_elevation_gain_m, total_time_s,
	purchase_price, purchase_date, weight_g, serial_number,
	installed_at, removed_at, is_retired, status, notes`

// scanComponent scans a component row into a model.GearComponent.
func scanComponent(row pgx.Row) (*model.GearComponent, error) {
	var c model.GearComponent
	err := row.Scan(
		&c.ID, &c.GearID, &c.ParentComponentID, &c.Name, &c.Type, &c.Brand, &c.Model,
		&c.TotalDistanceM, &c.TotalElevationGainM, &c.TotalTimeS,
		&c.PurchasePrice, &c.PurchaseDate, &c.WeightG, &c.SerialNumber,
		&c.InstalledAt, &c.RemovedAt, &c.IsRetired, &c.Status, &c.Notes,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
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
		if err := rows.Scan(
			&g.ID, &g.UserID, &g.Name, &g.Type, &g.SubType, &g.Brand, &g.Model,
			&g.TotalDistanceM, &g.TotalTimeS, &g.TotalElevationGainM, &g.TotalActivities,
			&purchasedAt, &retiredAt, &g.IsRetired, &g.Status, &g.DefaultForSports, &g.Notes, &g.CreatedAt,
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
		if g.DefaultForSports == nil {
			g.DefaultForSports = []string{}
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
	sports := create.DefaultForSports
	if sports == nil {
		sports = []string{}
	}

	g, err := scanGear(db.Pool.QueryRow(ctx,
		`INSERT INTO gear (user_id, name, type, sub_type, brand, model, purchased_at, default_for_sports, notes)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		 RETURNING `+gearSelectCols,
		userID, create.Name, create.Type, create.SubType, create.Brand, create.Model,
		create.PurchasedAt, sports, create.Notes,
	))
	if err != nil {
		return nil, fmt.Errorf("create gear: %w", err)
	}
	g.Components = []model.GearComponent{}
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
	if update.DefaultForSports != nil {
		b.Set("default_for_sports", update.DefaultForSports)
	}
	if update.Notes != nil {
		b.Set("notes", *update.Notes)
	}
	if update.WeightG != nil {
		b.Set("weight_g", *update.WeightG)
	}

	if !b.HasSets() {
		return db.GetGear(ctx, id)
	}

	args := append(b.Args(), id)
	query := fmt.Sprintf(
		`UPDATE gear SET %s WHERE id = $%d RETURNING `+gearSelectCols,
		b.Clause(), b.NextIdx(),
	)

	g, err := scanGear(db.Pool.QueryRow(ctx, query, args...))
	if err != nil {
		return nil, fmt.Errorf("update gear: %w", err)
	}
	if g == nil {
		return nil, nil
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
//   - activities.gear_id ON DELETE SET NULL -> activity references are nullified
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
		if err := rows.Scan(
			&c.ID, &c.GearID, &c.ParentComponentID, &c.Name, &c.Type, &c.Brand, &c.Model,
			&c.TotalDistanceM, &c.TotalElevationGainM, &c.TotalTimeS,
			&c.PurchasePrice, &c.PurchaseDate, &c.WeightG, &c.SerialNumber,
			&c.InstalledAt, &c.RemovedAt, &c.IsRetired, &c.Status, &c.Notes,
		); err != nil {
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
		  purchase_price, purchase_date, weight_g, serial_number, notes)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		 RETURNING `+componentSelectCols,
		gearID, create.ParentComponentID, create.Name, create.Type, create.Brand, create.Model,
		create.PurchasePrice, create.PurchaseDate, create.WeightG, create.SerialNumber, create.Notes,
	))
	if err != nil {
		return nil, fmt.Errorf("create component: %w", err)
	}
	return c, nil
}

func (db *DB) UpdateComponent(ctx context.Context, id string, name, typ *string, brand, modelStr, notes *string, isRetired *bool, purchasePrice *float64, purchaseDate, serialNumber *string, weightG *int) (*model.GearComponent, error) {
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

	// Update stats for both gears (outside tx, best-effort)
	_ = db.UpdateGearStats(ctx, fromGearID)
	_ = db.UpdateGearStats(ctx, toGearID)

	return nil
}

// --- Component History ---

func (db *DB) GetComponentHistory(ctx context.Context, componentID string) ([]model.ComponentAssignment, error) {
	rows, err := db.Pool.Query(ctx,
		`SELECT id, component_id, gear_id, installed_at::text, removed_at::text,
		 COALESCE(distance_m, 0), COALESCE(elevation_gain_m, 0), COALESCE(time_s, 0), notes
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
		if err := rows.Scan(
			&a.ID, &a.ComponentID, &a.GearID, &a.InstalledAt, &a.RemovedAt,
			&a.DistanceM, &a.ElevationGainM, &a.TimeS, &a.Notes,
		); err != nil {
			return nil, fmt.Errorf("scan component assignment: %w", err)
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
		`SELECT id, user_id, gear_id, name, swap_interval_km, swap_interval_hm, created_at::text
		 FROM chain_rotation_groups WHERE gear_id = $1`, gearID,
	).Scan(&g.ID, &g.UserID, &g.GearID, &g.Name, &g.SwapIntervalKM, &g.SwapIntervalHM, &g.CreatedAt)
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
		 m.total_distance_m, m.total_elevation_gain_m, m.last_swapped_at::text
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
			&m.TotalDistanceM, &m.TotalElevationGainM, &m.LastSwappedAt,
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
	Name           string   `json:"name"`
	SwapIntervalKM *float64 `json:"swap_interval_km,omitempty"`
	SwapIntervalHM *float64 `json:"swap_interval_hm,omitempty"`
	ComponentIDs   []string `json:"component_ids"`
}

func (db *DB) CreateChainRotation(ctx context.Context, userID, gearID string, create *ChainRotationGroupCreate) (*model.ChainRotationGroup, error) {
	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	var g model.ChainRotationGroup
	err = tx.QueryRow(ctx,
		`INSERT INTO chain_rotation_groups (user_id, gear_id, name, swap_interval_km, swap_interval_hm)
		 VALUES ($1, $2, $3, $4, $5)
		 RETURNING id, user_id, gear_id, name, swap_interval_km, swap_interval_hm, created_at::text`,
		userID, gearID, create.Name, create.SwapIntervalKM, create.SwapIntervalHM,
	).Scan(&g.ID, &g.UserID, &g.GearID, &g.Name, &g.SwapIntervalKM, &g.SwapIntervalHM, &g.CreatedAt)
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

func (db *DB) UpdateChainRotation(ctx context.Context, groupID string, intervalKM, intervalHM *float64) error {
	_, err := db.Pool.Exec(ctx,
		`UPDATE chain_rotation_groups SET swap_interval_km = $1, swap_interval_hm = $2 WHERE id = $3`,
		intervalKM, intervalHM, groupID,
	)
	if err != nil {
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
		 description, cost, notes, created_at::text
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
			&e.Description, &e.Cost, &e.Notes, &e.CreatedAt,
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
	err := db.Pool.QueryRow(ctx,
		`INSERT INTO gear_service_events (component_id, interval_id, performed_at, distance_at_m, description, cost, notes)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)
		 RETURNING id, component_id, interval_id, performed_at::text, distance_at_m,
		   description, cost, notes, created_at::text`,
		componentID, create.IntervalID, create.PerformedAt, create.DistanceAtM,
		create.Description, create.Cost, create.Notes,
	).Scan(
		&e.ID, &e.ComponentID, &e.IntervalID, &e.PerformedAt, &e.DistanceAtM,
		&e.Description, &e.Cost, &e.Notes, &e.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("create service event: %w", err)
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
		`SELECT g.id, g.name, gc.id, gc.name, gsi.name, gsi.interval_type,
		 CASE
		   WHEN gsi.interval_type = 'distance_m' THEN
		     gc.total_distance_m - COALESCE(gsi.last_service_distance_m, 0) >= gsi.interval_value
		   ELSE false
		 END AS is_overdue,
		 CASE
		   WHEN gsi.interval_type = 'distance_m' THEN
		     gc.total_distance_m - COALESCE(gsi.last_service_distance_m, 0)
		   ELSE 0
		 END AS current_value,
		 gsi.interval_value
		 FROM gear_service_intervals gsi
		 JOIN gear_components gc ON gc.id = gsi.component_id
		 JOIN gear g ON g.id = gc.gear_id
		 WHERE g.user_id = $1 AND g.is_retired = false AND gc.is_retired = false
		 ORDER BY is_overdue DESC, current_value DESC`, userID,
	)
	if err != nil {
		return nil, fmt.Errorf("get maintenance alerts: %w", err)
	}
	defer rows.Close()

	var alerts []model.MaintenanceAlert
	for rows.Next() {
		var a model.MaintenanceAlert
		if err := rows.Scan(
			&a.GearID, &a.GearName, &a.ComponentID, &a.ComponentName,
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

const ruleSelectCols = `id, user_id, name, is_active, priority, conditions, conditions_logic, condition_groups, actions, times_applied, created_at, is_system, system_type`

func scanRule(row pgx.Row) (*model.AutomationRule, error) {
	var r model.AutomationRule
	err := row.Scan(
		&r.ID, &r.UserID, &r.Name, &r.IsActive, &r.Priority,
		&r.Conditions, &r.ConditionsLogic, &r.ConditionGroups,
		&r.Actions, &r.TimesApplied, &r.CreatedAt,
		&r.IsSystem, &r.SystemType,
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
	rows, err := db.Pool.Query(ctx,
		`SELECT ar.id, ar.user_id, ar.name, ar.is_active, ar.priority,
		        ar.conditions, ar.conditions_logic, ar.condition_groups, ar.actions,
		        CASE
		            WHEN ar.system_type = 'electric_sync'    THEN COALESCE((SELECT t.activity_count FROM tags t WHERE t.user_id = ar.user_id AND t.name = 'E-Bike'   LIMIT 1), 0)
		            WHEN ar.system_type = 'virtual_sync'  THEN COALESCE((SELECT t.activity_count FROM tags t WHERE t.user_id = ar.user_id AND t.name = 'virtuell' LIMIT 1), 0)
		            ELSE ar.times_applied
		        END,
		        ar.created_at, ar.is_system, ar.system_type
		 FROM automation_rules ar WHERE ar.user_id = $1 ORDER BY ar.priority, ar.name`, userID,
	)
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
	var r model.AutomationRule
	err := db.Pool.QueryRow(ctx,
		`INSERT INTO automation_rules (user_id, name, priority, conditions, conditions_logic, condition_groups, actions)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)
		 RETURNING `+ruleSelectCols,
		userID, create.Name, create.Priority, create.Conditions, condLogic, create.ConditionGroups, create.Actions,
	).Scan(
		&r.ID, &r.UserID, &r.Name, &r.IsActive, &r.Priority,
		&r.Conditions, &r.ConditionsLogic, &r.ConditionGroups,
		&r.Actions, &r.TimesApplied, &r.CreatedAt,
		&r.IsSystem, &r.SystemType,
	)
	if err != nil {
		return nil, fmt.Errorf("create rule: %w", err)
	}
	// Refresh has_automation for all tags of this user
	db.Pool.Exec(ctx, `
		UPDATE tags t SET has_automation = EXISTS (
			SELECT 1 FROM automation_rules ar
			WHERE ar.user_id = t.user_id AND ar.is_active = true
			  AND ar.actions::text LIKE '%' || t.id::text || '%'
		) WHERE t.user_id = $1`, userID) //nolint:errcheck
	return &r, nil
}

func (db *DB) UpdateRule(ctx context.Context, id string, userID string, update *model.AutomationRuleUpdate) (*model.AutomationRule, error) {
	// Server-side guard: ebike_sync rules are fully read-only
	var sysType *string
	if err := db.Pool.QueryRow(ctx,
		`SELECT system_type FROM automation_rules WHERE id = $1 AND user_id = $2`,
		id, userID,
	).Scan(&sysType); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	if sysType != nil && (*sysType == "electric_sync" || *sysType == "virtual_sync") {
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
			  AND ar.actions::text LIKE '%' || t.id::text || '%'
		) WHERE t.user_id = $1`, userID) //nolint:errcheck
	return &r, nil
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
			  AND ar.actions::text LIKE '%' || t.id::text || '%'
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

// --- Gear stats update ---

func (db *DB) UpdateGearStats(ctx context.Context, gearID string) error {
	_, err := db.Pool.Exec(ctx,
		`UPDATE gear SET
		   total_distance_m = COALESCE((SELECT SUM(COALESCE(a.distance_m, 0)) FROM activities a WHERE a.gear_id = $1 AND NOT a.is_group_secondary), 0),
		   total_time_s = COALESCE((SELECT SUM(COALESCE(a.elapsed_time_s, 0)) FROM activities a WHERE a.gear_id = $1 AND NOT a.is_group_secondary), 0),
		   total_elevation_gain_m = COALESCE((SELECT SUM(COALESCE(a.elevation_gain_m, 0)) FROM activities a WHERE a.gear_id = $1 AND NOT a.is_group_secondary), 0),
		   total_activities = COALESCE((SELECT COUNT(*) FROM activities a WHERE a.gear_id = $1 AND NOT a.is_group_secondary), 0)
		 WHERE id = $1`, gearID,
	)
	if err != nil {
		return fmt.Errorf("update gear stats: %w", err)
	}
	return nil
}

func (db *DB) AssignGearByRules(ctx context.Context, userID, activityID, sportType string) error {
	// Find default gear for the sport type
	var gearID string
	err := db.Pool.QueryRow(ctx,
		`SELECT id FROM gear
		 WHERE user_id = $1 AND is_retired = false AND $2 = ANY(default_for_sports)
		 ORDER BY created_at LIMIT 1`, userID, sportType,
	).Scan(&gearID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil // No default gear for this sport
	}
	if err != nil {
		return fmt.Errorf("find default gear: %w", err)
	}

	_, err = db.Pool.Exec(ctx,
		`UPDATE activities SET gear_id = $1 WHERE id = $2 AND gear_id IS NULL`,
		gearID, activityID,
	)
	if err != nil {
		return fmt.Errorf("assign gear: %w", err)
	}
	return nil
}

// GetActivityAsMap returns an activity as a map for rule evaluation.
func (db *DB) GetActivityAsMap(ctx context.Context, activityID string) (map[string]any, error) {
	row := db.Pool.QueryRow(ctx, `
		SELECT
			a.sport_type, a.sub_type, a.source, a.intensity, a.title, a.description,
			a.is_indoor, a.is_electric, a.has_gps, a.visibility, a.hide_map, a.is_round_trip,
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
			a.gear_id::text,
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
			cd.max_category AS climb_category_max
		FROM activities a
		LEFT JOIN (SELECT activity_id, COUNT(*)::int AS cnt FROM activity_photos GROUP BY activity_id) pc ON pc.activity_id = a.id
		LEFT JOIN (SELECT activity_id, COUNT(*)::int AS cnt FROM activity_detected_laps GROUP BY activity_id) lc ON lc.activity_id = a.id
		LEFT JOIN (SELECT activity_id, COUNT(*)::int AS climb_count, MIN(category)::int AS max_category FROM activity_climbs GROUP BY activity_id) cd ON cd.activity_id = a.id
		WHERE a.id = $1`,
		activityID,
	)

	var (
		sportType, subType, source, intensity, title, description *string
		isIndoor, isElectric, hasGPS, hideMap, isRoundTrip           bool
		visibility                                                *string
		elapsedTimeS, movingTimeS                                 *int
		weekday, month                                            *int
		timeOfDay                                                 []string
		distanceM, avgSpeedMS, maxSpeedMS, minSpeedMS             *float64
		elevGainM, elevLossM, maxElevM, minElevM                  *float64
		avgHR, maxHR, minHR                                       *float64
		avgCadence, maxCadence                                    *float64
		avgPowerW, maxPowerW, minPowerW, normPowerW, tss, trimp   *float64
		avgTempC, minTempC, maxTempC                              *float64
		startCity, startState, startCountry, startCountryCode     *string
		endCity                                                   *string
		gearID                                                    *string
		tags                                                      []string
		startedAt                                                 time.Time
		startLat, startLon                                        *float64
		endLat, endLon                                            *float64
		photoCount, detectedLapCount                              int
		liftCount                                                 *int
		liftAscentM, liftDescentM, liftDistM                      *float64
		liftDurationS                                             *int
		netAscentM, netDescentM, netDistM                         *float64
		climbCount                                                int
		climbCategoryMax                                          *int
	)

	err := row.Scan(
		&sportType, &subType, &source, &intensity, &title, &description,
		&isIndoor, &isElectric, &hasGPS, &visibility, &hideMap, &isRoundTrip,
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
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get activity map: %w", err)
	}

	m := map[string]any{
		"is_indoor":          isIndoor,
		"is_electric":           isElectric,
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
	setNN("lift_count", derefInt(liftCount))
	setNN("lift_ascent_m", derefF(liftAscentM))
	setNN("lift_descent_m", derefF(liftDescentM))
	setNN("lift_distance_m", derefF(liftDistM))
	setNN("lift_duration_s", derefInt(liftDurationS))
	setNN("net_ascent_m", derefF(netAscentM))
	setNN("net_descent_m", derefF(netDescentM))
	setNN("net_distance_m", derefF(netDistM))
	setNN("climb_category_max", derefInt(climbCategoryMax))
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

	// recording_device_id
	var recDeviceID *string
	_ = db.Pool.QueryRow(ctx, `SELECT recording_device_id::TEXT FROM activities WHERE id = $1`, activityID).Scan(&recDeviceID)
	if recDeviceID != nil {
		m["recording_device_id"] = *recDeviceID
	}

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
		_, err := db.Pool.Exec(ctx, `UPDATE activities SET gear_id = $1 WHERE id = $2`, value, activityID)
		return err
	case "add_tag":
		tagID, ok := value.(string)
		if !ok {
			return fmt.Errorf("add_tag value must be a tag_id string")
		}
		// Resolve tag name
		var tagName string
		var isSystem bool
		if err := db.Pool.QueryRow(ctx,
			`SELECT name, is_system FROM tags WHERE id = $1`, tagID,
		).Scan(&tagName, &isSystem); err != nil {
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
		_, err := db.Pool.Exec(ctx, `UPDATE activities SET sport_type = $1 WHERE id = $2`, value, activityID)
		return err
	case "sub_type":
		_, err := db.Pool.Exec(ctx, `UPDATE activities SET sub_type = $1 WHERE id = $2`, value, activityID)
		return err
	case "intensity":
		_, err := db.Pool.Exec(ctx, `UPDATE activities SET intensity = $1 WHERE id = $2`, value, activityID)
		return err
	case "is_indoor":
		_, err := db.Pool.Exec(ctx, `UPDATE activities SET is_indoor = $1 WHERE id = $2`, value, activityID)
		return err
	case "is_round_trip":
		_, err := db.Pool.Exec(ctx, `UPDATE activities SET is_round_trip = $1 WHERE id = $2`, value, activityID)
		return err
	case "set_name_template":
		title, ok := value.(string)
		if !ok {
			return fmt.Errorf("set_name_template value must be a resolved string")
		}
		_, err := db.Pool.Exec(ctx, `UPDATE activities SET title = $1, title_is_manual = true WHERE id = $2`, title, activityID)
		return err
	default:
		return fmt.Errorf("unknown field: %s", field)
	}
}

// --- Bike Setups ---

const bikeSetupCols = `id, gear_id, name, rider_weight_kg, intended_use, notes,
	suspension_front, suspension_rear, tires, contact_points, brakes, drivetrain,
	is_active, created_at::text, updated_at::text`

func scanBikeSetup(row pgx.Row) (*model.BikeSetup, error) {
	var s model.BikeSetup
	err := row.Scan(
		&s.ID, &s.GearID, &s.Name, &s.RiderWeightKg, &s.IntendedUse, &s.Notes,
		&s.SuspensionFront, &s.SuspensionRear, &s.Tires, &s.ContactPoints, &s.Brakes, &s.Drivetrain,
		&s.IsActive, &s.CreatedAt, &s.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &s, nil
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
			&s.IsActive, &s.CreatedAt, &s.UpdatedAt,
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
	s, err := scanBikeSetup(db.Pool.QueryRow(ctx,
		`INSERT INTO bike_setups (gear_id, name, rider_weight_kg, intended_use, notes,
		  suspension_front, suspension_rear, tires, contact_points, brakes, drivetrain, is_active)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		 RETURNING `+bikeSetupCols,
		gearID, create.Name, create.RiderWeightKg, create.IntendedUse, create.Notes,
		create.SuspensionFront, create.SuspensionRear, create.Tires,
		create.ContactPoints, create.Brakes, create.Drivetrain, create.IsActive,
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
// Fields of the secondary fill gaps of the primary (via COALESCE), activities and components are
// reassigned, and the secondary record is deleted — all within a single transaction.
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

	// 2. Reassign activities from secondary to primary
	_, err = tx.Exec(ctx,
		`UPDATE activities SET gear_id = $1 WHERE gear_id = $2 AND user_id = $3`,
		primaryID, secondaryID, userID,
	)
	if err != nil {
		return fmt.Errorf("reassign activities: %w", err)
	}

	// 3. Reassign components from secondary to primary
	_, err = tx.Exec(ctx,
		`UPDATE gear_components SET gear_id = $1 WHERE gear_id = $2`,
		primaryID, secondaryID,
	)
	if err != nil {
		return fmt.Errorf("reassign components: %w", err)
	}

	// 4. Delete secondary gear item
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
