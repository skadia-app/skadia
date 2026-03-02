package database

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/skadia-app/skadia/services/api/internal/model"
)

// --- Gear CRUD ---

func (db *DB) ListGear(ctx context.Context, userID string, includeRetired bool) ([]model.Gear, error) {
	query := `SELECT id, user_id, name, type, sub_type, brand, model,
		total_distance_m, total_time_s, total_activities,
		purchased_at, retired_at, is_retired, default_for_sports, notes, created_at
		FROM gear WHERE user_id = $1`
	if !includeRetired {
		query += ` AND is_retired = false`
	}
	query += ` ORDER BY name`

	rows, err := db.Pool.Query(ctx, query, userID)
	if err != nil {
		return nil, fmt.Errorf("list gear: %w", err)
	}
	defer rows.Close()

	var items []model.Gear
	for rows.Next() {
		var g model.Gear
		if err := rows.Scan(
			&g.ID, &g.UserID, &g.Name, &g.Type, &g.SubType, &g.Brand, &g.Model,
			&g.TotalDistanceM, &g.TotalTimeS, &g.TotalActivities,
			&g.PurchasedAt, &g.RetiredAt, &g.IsRetired, &g.DefaultForSports, &g.Notes, &g.CreatedAt,
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

	var g model.Gear
	err := db.Pool.QueryRow(ctx,
		`INSERT INTO gear (user_id, name, type, sub_type, brand, model, purchased_at, default_for_sports, notes)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		 RETURNING id, user_id, name, type, sub_type, brand, model,
		   total_distance_m, total_time_s, total_activities,
		   purchased_at, retired_at, is_retired, default_for_sports, notes, created_at`,
		userID, create.Name, create.Type, create.SubType, create.Brand, create.Model,
		create.PurchasedAt, sports, create.Notes,
	).Scan(
		&g.ID, &g.UserID, &g.Name, &g.Type, &g.SubType, &g.Brand, &g.Model,
		&g.TotalDistanceM, &g.TotalTimeS, &g.TotalActivities,
		&g.PurchasedAt, &g.RetiredAt, &g.IsRetired, &g.DefaultForSports, &g.Notes, &g.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("create gear: %w", err)
	}
	if g.DefaultForSports == nil {
		g.DefaultForSports = []string{}
	}
	g.Components = []model.GearComponent{}
	return &g, nil
}

func (db *DB) GetGear(ctx context.Context, id string) (*model.Gear, error) {
	var g model.Gear
	err := db.Pool.QueryRow(ctx,
		`SELECT id, user_id, name, type, sub_type, brand, model,
		 total_distance_m, total_time_s, total_activities,
		 purchased_at, retired_at, is_retired, default_for_sports, notes, created_at
		 FROM gear WHERE id = $1`, id,
	).Scan(
		&g.ID, &g.UserID, &g.Name, &g.Type, &g.SubType, &g.Brand, &g.Model,
		&g.TotalDistanceM, &g.TotalTimeS, &g.TotalActivities,
		&g.PurchasedAt, &g.RetiredAt, &g.IsRetired, &g.DefaultForSports, &g.Notes, &g.CreatedAt,
	)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get gear: %w", err)
	}
	if g.DefaultForSports == nil {
		g.DefaultForSports = []string{}
	}

	// Load components
	components, err := db.listComponents(ctx, id)
	if err != nil {
		return nil, err
	}
	g.Components = components
	return &g, nil
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
		`UPDATE gear SET %s WHERE id = $%d
		 RETURNING id, user_id, name, type, sub_type, brand, model,
		   total_distance_m, total_time_s, total_activities,
		   purchased_at, retired_at, is_retired, default_for_sports, notes, created_at`,
		strings.Join(sets, ", "), argIdx,
	)

	var g model.Gear
	err := db.Pool.QueryRow(ctx, query, args...).Scan(
		&g.ID, &g.UserID, &g.Name, &g.Type, &g.SubType, &g.Brand, &g.Model,
		&g.TotalDistanceM, &g.TotalTimeS, &g.TotalActivities,
		&g.PurchasedAt, &g.RetiredAt, &g.IsRetired, &g.DefaultForSports, &g.Notes, &g.CreatedAt,
	)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("update gear: %w", err)
	}
	if g.DefaultForSports == nil {
		g.DefaultForSports = []string{}
	}

	components, err := db.listComponents(ctx, id)
	if err != nil {
		return nil, err
	}
	g.Components = components
	return &g, nil
}

func (db *DB) RetireGear(ctx context.Context, id string) error {
	tag, err := db.Pool.Exec(ctx, `UPDATE gear SET is_retired = true WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("retire gear: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("gear not found")
	}
	return nil
}

// --- Components ---

func (db *DB) listComponents(ctx context.Context, gearID string) ([]model.GearComponent, error) {
	rows, err := db.Pool.Query(ctx,
		`SELECT id, gear_id, parent_component_id, name, type, brand, model,
		 total_distance_m, installed_at, removed_at, is_retired, notes
		 FROM gear_components WHERE gear_id = $1 ORDER BY name`, gearID,
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
			&c.TotalDistanceM, &c.InstalledAt, &c.RemovedAt, &c.IsRetired, &c.Notes,
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
	var c model.GearComponent
	err := db.Pool.QueryRow(ctx,
		`INSERT INTO gear_components (gear_id, parent_component_id, name, type, brand, model, notes)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)
		 RETURNING id, gear_id, parent_component_id, name, type, brand, model,
		   total_distance_m, installed_at, removed_at, is_retired, notes`,
		gearID, create.ParentComponentID, create.Name, create.Type, create.Brand, create.Model, create.Notes,
	).Scan(
		&c.ID, &c.GearID, &c.ParentComponentID, &c.Name, &c.Type, &c.Brand, &c.Model,
		&c.TotalDistanceM, &c.InstalledAt, &c.RemovedAt, &c.IsRetired, &c.Notes,
	)
	if err != nil {
		return nil, fmt.Errorf("create component: %w", err)
	}
	return &c, nil
}

func (db *DB) UpdateComponent(ctx context.Context, id string, name, typ *string, brand, modelStr, notes *string, isRetired *bool) (*model.GearComponent, error) {
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

	if len(sets) == 0 {
		// No changes, just return the existing component
		var c model.GearComponent
		err := db.Pool.QueryRow(ctx,
			`SELECT id, gear_id, parent_component_id, name, type, brand, model,
			 total_distance_m, installed_at, removed_at, is_retired, notes
			 FROM gear_components WHERE id = $1`, id,
		).Scan(&c.ID, &c.GearID, &c.ParentComponentID, &c.Name, &c.Type, &c.Brand, &c.Model,
			&c.TotalDistanceM, &c.InstalledAt, &c.RemovedAt, &c.IsRetired, &c.Notes)
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		if err != nil {
			return nil, fmt.Errorf("get component: %w", err)
		}
		return &c, nil
	}

	args = append(args, id)
	query := fmt.Sprintf(
		`UPDATE gear_components SET %s WHERE id = $%d
		 RETURNING id, gear_id, parent_component_id, name, type, brand, model,
		   total_distance_m, installed_at, removed_at, is_retired, notes`,
		strings.Join(sets, ", "), argIdx,
	)

	var c model.GearComponent
	err := db.Pool.QueryRow(ctx, query, args...).Scan(
		&c.ID, &c.GearID, &c.ParentComponentID, &c.Name, &c.Type, &c.Brand, &c.Model,
		&c.TotalDistanceM, &c.InstalledAt, &c.RemovedAt, &c.IsRetired, &c.Notes,
	)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("update component: %w", err)
	}
	return &c, nil
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
	var isIndoor, hasGPS bool
	var intensity, timeOfDay *string

	err := db.Pool.QueryRow(ctx,
		`SELECT sport_type, distance_m, elapsed_time_s, avg_speed_ms,
		 is_indoor, has_gps, intensity, time_of_day
		 FROM activities WHERE id = $1`, activityID,
	).Scan(&sportType, &distanceM, &elapsedTimeS, &avgSpeedMS,
		&isIndoor, &hasGPS, &intensity, &timeOfDay)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get activity map: %w", err)
	}

	m := map[string]any{
		"sport_type": sportType,
		"is_indoor":  isIndoor,
		"has_gps":    hasGPS,
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
	if timeOfDay != nil {
		m["time_of_day"] = *timeOfDay
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

