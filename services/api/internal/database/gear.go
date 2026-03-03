package database

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/skadia-app/skadia/services/api/internal/model"
)

// gearSelectCols are the columns returned by gear queries.
const gearSelectCols = `id, user_id, name, type, sub_type, brand, model,
	total_distance_m, total_time_s, total_elevation_gain_m, total_activities,
	purchased_at, retired_at, is_retired, status, default_for_sports, notes, created_at`

// scanGear scans a gear row into a model.Gear.
func scanGear(row pgx.Row) (*model.Gear, error) {
	var g model.Gear
	err := row.Scan(
		&g.ID, &g.UserID, &g.Name, &g.Type, &g.SubType, &g.Brand, &g.Model,
		&g.TotalDistanceM, &g.TotalTimeS, &g.TotalElevationGainM, &g.TotalActivities,
		&g.PurchasedAt, &g.RetiredAt, &g.IsRetired, &g.Status, &g.DefaultForSports, &g.Notes, &g.CreatedAt,
	)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
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
	if err == pgx.ErrNoRows {
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
		if err := rows.Scan(
			&g.ID, &g.UserID, &g.Name, &g.Type, &g.SubType, &g.Brand, &g.Model,
			&g.TotalDistanceM, &g.TotalTimeS, &g.TotalElevationGainM, &g.TotalActivities,
			&g.PurchasedAt, &g.RetiredAt, &g.IsRetired, &g.Status, &g.DefaultForSports, &g.Notes, &g.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan gear: %w", err)
		}
		if g.DefaultForSports == nil {
			g.DefaultForSports = []string{}
		}
		items = append(items, g)
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
	var sets []string
	var args []any
	argIdx := 1

	if update.Name != nil {
		sets = append(sets, fmt.Sprintf("name = $%d", argIdx))
		args = append(args, *update.Name)
		argIdx++
	}
	if update.Type != nil {
		sets = append(sets, fmt.Sprintf("type = $%d", argIdx))
		args = append(args, *update.Type)
		argIdx++
	}
	if update.SubType != nil {
		sets = append(sets, fmt.Sprintf("sub_type = $%d", argIdx))
		args = append(args, *update.SubType)
		argIdx++
	}
	if update.Brand != nil {
		sets = append(sets, fmt.Sprintf("brand = $%d", argIdx))
		args = append(args, *update.Brand)
		argIdx++
	}
	if update.Model != nil {
		sets = append(sets, fmt.Sprintf("model = $%d", argIdx))
		args = append(args, *update.Model)
		argIdx++
	}
	if update.PurchasedAt != nil {
		sets = append(sets, fmt.Sprintf("purchased_at = $%d", argIdx))
		args = append(args, *update.PurchasedAt)
		argIdx++
	}
	if update.RetiredAt != nil {
		sets = append(sets, fmt.Sprintf("retired_at = $%d", argIdx))
		args = append(args, *update.RetiredAt)
		argIdx++
	}
	if update.IsRetired != nil {
		sets = append(sets, fmt.Sprintf("is_retired = $%d", argIdx))
		args = append(args, *update.IsRetired)
		argIdx++
	}
	if update.DefaultForSports != nil {
		sets = append(sets, fmt.Sprintf("default_for_sports = $%d", argIdx))
		args = append(args, update.DefaultForSports)
		argIdx++
	}
	if update.Notes != nil {
		sets = append(sets, fmt.Sprintf("notes = $%d", argIdx))
		args = append(args, *update.Notes)
		argIdx++
	}

	if len(sets) == 0 {
		return db.GetGear(ctx, id)
	}

	args = append(args, id)
	query := fmt.Sprintf(
		`UPDATE gear SET %s WHERE id = $%d RETURNING `+gearSelectCols,
		strings.Join(sets, ", "), argIdx,
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
		return fmt.Errorf("gear not found")
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
		return fmt.Errorf("gear not found")
	}
	return nil
}

// UpdateGearStatus changes the status of a gear item (active, stored, retired).
func (db *DB) UpdateGearStatus(ctx context.Context, id, status string) error {
	isRetired := status == "retired"
	tag, err := db.Pool.Exec(ctx,
		`UPDATE gear SET status = $1, is_retired = $2 WHERE id = $3`,
		status, isRetired, id,
	)
	if err != nil {
		return fmt.Errorf("update gear status: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("gear not found")
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
	var sets []string
	var args []any
	argIdx := 1

	if name != nil {
		sets = append(sets, fmt.Sprintf("name = $%d", argIdx))
		args = append(args, *name)
		argIdx++
	}
	if typ != nil {
		sets = append(sets, fmt.Sprintf("type = $%d", argIdx))
		args = append(args, *typ)
		argIdx++
	}
	if brand != nil {
		sets = append(sets, fmt.Sprintf("brand = $%d", argIdx))
		args = append(args, *brand)
		argIdx++
	}
	if modelStr != nil {
		sets = append(sets, fmt.Sprintf("model = $%d", argIdx))
		args = append(args, *modelStr)
		argIdx++
	}
	if notes != nil {
		sets = append(sets, fmt.Sprintf("notes = $%d", argIdx))
		args = append(args, *notes)
		argIdx++
	}
	if isRetired != nil {
		sets = append(sets, fmt.Sprintf("is_retired = $%d", argIdx))
		args = append(args, *isRetired)
		argIdx++
	}
	if purchasePrice != nil {
		sets = append(sets, fmt.Sprintf("purchase_price = $%d", argIdx))
		args = append(args, *purchasePrice)
		argIdx++
	}
	if purchaseDate != nil {
		sets = append(sets, fmt.Sprintf("purchase_date = $%d", argIdx))
		args = append(args, *purchaseDate)
		argIdx++
	}
	if serialNumber != nil {
		sets = append(sets, fmt.Sprintf("serial_number = $%d", argIdx))
		args = append(args, *serialNumber)
		argIdx++
	}
	if weightG != nil {
		sets = append(sets, fmt.Sprintf("weight_g = $%d", argIdx))
		args = append(args, *weightG)
		argIdx++
	}

	if len(sets) == 0 {
		// No changes, just return the existing component
		c, err := scanComponent(db.Pool.QueryRow(ctx,
			`SELECT `+componentSelectCols+` FROM gear_components WHERE id = $1`, id,
		))
		if err != nil {
			return nil, fmt.Errorf("get component: %w", err)
		}
		return c, nil
	}

	args = append(args, id)
	query := fmt.Sprintf(
		`UPDATE gear_components SET %s WHERE id = $%d RETURNING `+componentSelectCols,
		strings.Join(sets, ", "), argIdx,
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
		return fmt.Errorf("component not found")
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
	if err == pgx.ErrNoRows {
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
		return fmt.Errorf("chain rotation not found")
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
	if err == pgx.ErrNoRows {
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
		return fmt.Errorf("service event not found")
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
	if alerts == nil {
		alerts = []model.MaintenanceAlert{}
	}
	return alerts, nil
}

// --- Automation Rules ---

func (db *DB) ListRules(ctx context.Context, userID string) ([]model.AutomationRule, error) {
	rows, err := db.Pool.Query(ctx,
		`SELECT id, user_id, name, is_active, priority, conditions, actions, times_applied, created_at
		 FROM automation_rules WHERE user_id = $1 ORDER BY priority, name`, userID,
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
			&r.Conditions, &r.Actions, &r.TimesApplied, &r.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan rule: %w", err)
		}
		items = append(items, r)
	}
	if items == nil {
		items = []model.AutomationRule{}
	}
	return items, nil
}

func (db *DB) CreateRule(ctx context.Context, userID string, create *model.AutomationRuleCreate) (*model.AutomationRule, error) {
	var r model.AutomationRule
	err := db.Pool.QueryRow(ctx,
		`INSERT INTO automation_rules (user_id, name, priority, conditions, actions)
		 VALUES ($1, $2, $3, $4, $5)
		 RETURNING id, user_id, name, is_active, priority, conditions, actions, times_applied, created_at`,
		userID, create.Name, create.Priority, create.Conditions, create.Actions,
	).Scan(
		&r.ID, &r.UserID, &r.Name, &r.IsActive, &r.Priority,
		&r.Conditions, &r.Actions, &r.TimesApplied, &r.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("create rule: %w", err)
	}
	return &r, nil
}

func (db *DB) UpdateRule(ctx context.Context, id string, update *model.AutomationRuleUpdate) (*model.AutomationRule, error) {
	var sets []string
	var args []any
	argIdx := 1

	if update.Name != nil {
		sets = append(sets, fmt.Sprintf("name = $%d", argIdx))
		args = append(args, *update.Name)
		argIdx++
	}
	if update.IsActive != nil {
		sets = append(sets, fmt.Sprintf("is_active = $%d", argIdx))
		args = append(args, *update.IsActive)
		argIdx++
	}
	if update.Priority != nil {
		sets = append(sets, fmt.Sprintf("priority = $%d", argIdx))
		args = append(args, *update.Priority)
		argIdx++
	}
	if update.Conditions != nil {
		sets = append(sets, fmt.Sprintf("conditions = $%d", argIdx))
		args = append(args, *update.Conditions)
		argIdx++
	}
	if update.Actions != nil {
		sets = append(sets, fmt.Sprintf("actions = $%d", argIdx))
		args = append(args, *update.Actions)
		argIdx++
	}

	if len(sets) == 0 {
		// No changes
		var r model.AutomationRule
		err := db.Pool.QueryRow(ctx,
			`SELECT id, user_id, name, is_active, priority, conditions, actions, times_applied, created_at
			 FROM automation_rules WHERE id = $1`, id,
		).Scan(&r.ID, &r.UserID, &r.Name, &r.IsActive, &r.Priority,
			&r.Conditions, &r.Actions, &r.TimesApplied, &r.CreatedAt)
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		if err != nil {
			return nil, fmt.Errorf("get rule: %w", err)
		}
		return &r, nil
	}

	args = append(args, id)
	query := fmt.Sprintf(
		`UPDATE automation_rules SET %s WHERE id = $%d
		 RETURNING id, user_id, name, is_active, priority, conditions, actions, times_applied, created_at`,
		strings.Join(sets, ", "), argIdx,
	)

	var r model.AutomationRule
	err := db.Pool.QueryRow(ctx, query, args...).Scan(
		&r.ID, &r.UserID, &r.Name, &r.IsActive, &r.Priority,
		&r.Conditions, &r.Actions, &r.TimesApplied, &r.CreatedAt,
	)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("update rule: %w", err)
	}
	return &r, nil
}

func (db *DB) DeleteRule(ctx context.Context, id string) error {
	tag, err := db.Pool.Exec(ctx, `DELETE FROM automation_rules WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete rule: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("rule not found")
	}
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
		   total_distance_m = COALESCE((SELECT SUM(COALESCE(a.distance_m, 0)) FROM activities a WHERE a.gear_id = $1), 0),
		   total_time_s = COALESCE((SELECT SUM(COALESCE(a.elapsed_time_s, 0)) FROM activities a WHERE a.gear_id = $1), 0),
		   total_elevation_gain_m = COALESCE((SELECT SUM(COALESCE(a.elevation_gain_m, 0)) FROM activities a WHERE a.gear_id = $1), 0),
		   total_activities = COALESCE((SELECT COUNT(*) FROM activities a WHERE a.gear_id = $1), 0)
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
	if err == pgx.ErrNoRows {
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
	var sportType string
	var distanceM, avgSpeedMS *float64
	var elapsedTimeS *int
	var isIndoor bool
	var intensity *string
	var timeOfDay []string

	err := db.Pool.QueryRow(ctx,
		`SELECT sport_type, distance_m, elapsed_time_s, avg_speed_ms,
		 is_indoor, intensity, time_of_day
		 FROM activities WHERE id = $1`, activityID,
	).Scan(&sportType, &distanceM, &elapsedTimeS, &avgSpeedMS,
		&isIndoor, &intensity, &timeOfDay)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get activity map: %w", err)
	}

	m := map[string]any{
		"sport_type": sportType,
		"is_indoor":  isIndoor,
	}
	if distanceM != nil {
		m["distance_m"] = *distanceM
	}
	if elapsedTimeS != nil {
		m["elapsed_time_s"] = *elapsedTimeS
	}
	if avgSpeedMS != nil {
		m["avg_speed_ms"] = *avgSpeedMS
	}
	if intensity != nil {
		m["intensity"] = *intensity
	}
	if len(timeOfDay) > 0 {
		m["time_of_day"] = timeOfDay
	}
	return m, nil
}

// ApplyRuleChange applies a single rule change to an activity.
func (db *DB) ApplyRuleChange(ctx context.Context, activityID string, field string, value any) error {
	switch field {
	case "gear_id":
		_, err := db.Pool.Exec(ctx,
			`UPDATE activities SET gear_id = $1 WHERE id = $2`,
			value, activityID)
		return err
	case "add_tag":
		tag, ok := value.(string)
		if !ok {
			return fmt.Errorf("add_tag value must be a string")
		}
		_, err := db.Pool.Exec(ctx,
			`UPDATE activities SET tags = array_append(tags, $1) WHERE id = $2 AND NOT ($1 = ANY(tags))`,
			tag, activityID)
		return err
	case "visibility":
		_, err := db.Pool.Exec(ctx,
			`UPDATE activities SET visibility = $1 WHERE id = $2`,
			value, activityID)
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
	if err == pgx.ErrNoRows {
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
	var sets []string
	var args []any
	argIdx := 1

	if update.Name != nil {
		sets = append(sets, fmt.Sprintf("name = $%d", argIdx))
		args = append(args, *update.Name)
		argIdx++
	}
	if update.RiderWeightKg != nil {
		sets = append(sets, fmt.Sprintf("rider_weight_kg = $%d", argIdx))
		args = append(args, *update.RiderWeightKg)
		argIdx++
	}
	if update.IntendedUse != nil {
		sets = append(sets, fmt.Sprintf("intended_use = $%d", argIdx))
		args = append(args, *update.IntendedUse)
		argIdx++
	}
	if update.Notes != nil {
		sets = append(sets, fmt.Sprintf("notes = $%d", argIdx))
		args = append(args, *update.Notes)
		argIdx++
	}
	if update.SuspensionFront != nil {
		sets = append(sets, fmt.Sprintf("suspension_front = $%d", argIdx))
		args = append(args, update.SuspensionFront)
		argIdx++
	}
	if update.SuspensionRear != nil {
		sets = append(sets, fmt.Sprintf("suspension_rear = $%d", argIdx))
		args = append(args, update.SuspensionRear)
		argIdx++
	}
	if update.Tires != nil {
		sets = append(sets, fmt.Sprintf("tires = $%d", argIdx))
		args = append(args, update.Tires)
		argIdx++
	}
	if update.ContactPoints != nil {
		sets = append(sets, fmt.Sprintf("contact_points = $%d", argIdx))
		args = append(args, update.ContactPoints)
		argIdx++
	}
	if update.Brakes != nil {
		sets = append(sets, fmt.Sprintf("brakes = $%d", argIdx))
		args = append(args, update.Brakes)
		argIdx++
	}
	if update.Drivetrain != nil {
		sets = append(sets, fmt.Sprintf("drivetrain = $%d", argIdx))
		args = append(args, update.Drivetrain)
		argIdx++
	}
	if update.IsActive != nil {
		sets = append(sets, fmt.Sprintf("is_active = $%d", argIdx))
		args = append(args, *update.IsActive)
		argIdx++
	}

	if len(sets) == 0 {
		s, err := scanBikeSetup(db.Pool.QueryRow(ctx,
			`SELECT `+bikeSetupCols+` FROM bike_setups WHERE id = $1`, id,
		))
		if err != nil {
			return nil, fmt.Errorf("get bike setup: %w", err)
		}
		return s, nil
	}

	sets = append(sets, "updated_at = NOW()")
	args = append(args, id)
	query := fmt.Sprintf(
		`UPDATE bike_setups SET %s WHERE id = $%d RETURNING `+bikeSetupCols,
		strings.Join(sets, ", "), argIdx,
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
		return fmt.Errorf("bike setup not found")
	}
	return nil
}

// --- Gear Documents ---

const gearDocSelectCols = `id, user_id, gear_id, component_id, service_event_id, doc_type, title, original_filename, content_type, file_size_bytes, file_hash, s3_bucket, s3_key, created_at`

func scanGearDocument(row pgx.Row) (*model.GearDocument, error) {
	var d model.GearDocument
	err := row.Scan(&d.ID, &d.UserID, &d.GearID, &d.ComponentID, &d.ServiceEventID, &d.DocType, &d.Title, &d.OriginalFilename, &d.ContentType, &d.FileSizeBytes, &d.FileHash, &d.S3Bucket, &d.S3Key, &d.CreatedAt)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("scan gear document: %w", err)
	}
	return &d, nil
}

func (db *DB) InsertGearDocument(ctx context.Context, doc *model.GearDocument) (*model.GearDocument, error) {
	return scanGearDocument(db.Pool.QueryRow(ctx, `
		INSERT INTO gear_documents (user_id, gear_id, component_id, service_event_id, doc_type, title, original_filename, content_type, file_size_bytes, file_hash, s3_bucket, s3_key)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		RETURNING `+gearDocSelectCols,
		doc.UserID, doc.GearID, doc.ComponentID, doc.ServiceEventID, doc.DocType, doc.Title, doc.OriginalFilename, doc.ContentType, doc.FileSizeBytes, doc.FileHash, doc.S3Bucket, doc.S3Key,
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
		if err := rows.Scan(&d.ID, &d.UserID, &d.GearID, &d.ComponentID, &d.ServiceEventID, &d.DocType, &d.Title, &d.OriginalFilename, &d.ContentType, &d.FileSizeBytes, &d.FileHash, &d.S3Bucket, &d.S3Key, &d.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan gear document: %w", err)
		}
		docs = append(docs, d)
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
		if err := rows.Scan(&d.ID, &d.UserID, &d.GearID, &d.ComponentID, &d.ServiceEventID, &d.DocType, &d.Title, &d.OriginalFilename, &d.ContentType, &d.FileSizeBytes, &d.FileHash, &d.S3Bucket, &d.S3Key, &d.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan component document: %w", err)
		}
		docs = append(docs, d)
	}
	if docs == nil {
		docs = []model.GearDocument{}
	}
	return docs, nil
}

func (db *DB) GetGearDocument(ctx context.Context, docID string) (*model.GearDocument, error) {
	return scanGearDocument(db.Pool.QueryRow(ctx, `SELECT `+gearDocSelectCols+` FROM gear_documents WHERE id = $1`, docID))
}

func (db *DB) DeleteGearDocument(ctx context.Context, docID string) (string, string, error) {
	var bucket, key string
	err := db.Pool.QueryRow(ctx, `DELETE FROM gear_documents WHERE id = $1 RETURNING s3_bucket, s3_key`, docID).Scan(&bucket, &key)
	if err == pgx.ErrNoRows {
		return "", "", nil
	}
	if err != nil {
		return "", "", fmt.Errorf("delete gear document: %w", err)
	}
	return bucket, key, nil
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
	return result, nil
}

func (db *DB) ActivateBikeSetup(ctx context.Context, id string) (*model.BikeSetup, error) {
	s, err := scanBikeSetup(db.Pool.QueryRow(ctx,
		`UPDATE bike_setups SET is_active = true, updated_at = NOW()
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
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get gear cost summary: %w", err)
	}
	return &s, nil
}
