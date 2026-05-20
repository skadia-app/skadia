package database

import (
	"context"
	"fmt"
	"strings"

	"github.com/skadia-app/skadia/services/api/internal/gear"
	"github.com/skadia-app/skadia/services/api/internal/model"
	"github.com/jackc/pgx/v5"
)

func activityGearRoleLabel(roleKey string) string {
	for _, role := range gear.Schema.AssignmentRoles {
		if role.Key == roleKey {
			return role.LabelDE
		}
	}
	return roleKey
}

func validActivityGearRole(roleKey string) bool {
	for _, role := range gear.Schema.AssignmentRoles {
		if role.Key == roleKey {
			return true
		}
	}
	return false
}

func defaultActivityGearRole(gearType string) string {
	if gt, ok := gear.LookupGearType(gearType); ok && gt.DefaultAssignmentRoleKey != "" {
		return gt.DefaultAssignmentRoleKey
	}
	return "primary"
}

func (db *DB) ListActivityGearAssignments(ctx context.Context, userID, activityID string) ([]model.ActivityGearAssignment, error) {
	rows, err := db.Pool.Query(ctx, `
		SELECT aga.id::text, aga.gear_id::text, g.name, g.type, aga.role_key, aga.quantity
		  FROM activity_gear_assignments aga
		  JOIN activities a ON a.id = aga.activity_id AND a.user_id = aga.user_id
		  JOIN gear g ON g.id = aga.gear_id AND g.user_id = aga.user_id
		 WHERE aga.activity_id = $1
		   AND aga.user_id = $2
		   AND a.user_id = $2
		 ORDER BY
		   CASE WHEN aga.role_key = 'primary' THEN 0 ELSE 1 END,
		   aga.created_at,
		   g.name`, activityID, userID)
	if err != nil {
		return nil, fmt.Errorf("list activity gear assignments: %w", err)
	}
	defer rows.Close()

	assignments := make([]model.ActivityGearAssignment, 0)
	for rows.Next() {
		var a model.ActivityGearAssignment
		if err := rows.Scan(&a.ID, &a.GearID, &a.GearName, &a.GearType, &a.RoleKey, &a.Quantity); err != nil {
			return nil, fmt.Errorf("scan activity gear assignment: %w", err)
		}
		a.RoleLabel = activityGearRoleLabel(a.RoleKey)
		assignments = append(assignments, a)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate activity gear assignments: %w", err)
	}
	return assignments, nil
}

func (db *DB) AddActivityGearAssignment(ctx context.Context, userID, activityID, gearID, roleKey string) (*model.ActivityGearAssignment, error) {
	activityID = strings.TrimSpace(activityID)
	gearID = strings.TrimSpace(gearID)
	roleKey = strings.TrimSpace(roleKey)
	if activityID == "" || gearID == "" {
		return nil, fmt.Errorf("activity and gear are required")
	}

	var gearType string
	if err := db.Pool.QueryRow(ctx, `
		SELECT g.type
		  FROM gear g
		 WHERE g.id = $1
		   AND g.user_id = $2`, gearID, userID).Scan(&gearType); err != nil {
		if err == pgx.ErrNoRows {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("load gear for assignment: %w", err)
	}
	if roleKey == "" {
		roleKey = defaultActivityGearRole(gearType)
	}
	if !validActivityGearRole(roleKey) {
		return nil, fmt.Errorf("unknown activity gear role %q", roleKey)
	}

	var assignmentID string
	if roleKey == "primary" {
		var previousGearID string
		if err := db.Pool.QueryRow(ctx, `
			SELECT gear_id::text
			  FROM activity_gear_assignments
			 WHERE activity_id = $1
			   AND user_id = $2
			   AND role_key = 'primary'`, activityID, userID).Scan(&previousGearID); err != nil && err != pgx.ErrNoRows {
			return nil, fmt.Errorf("load previous primary gear assignment: %w", err)
		}
		if err := SetPrimaryGearForActivityTx(ctx, db.Pool, activityID, gearID, "user"); err != nil {
			return nil, err
		}
		if err := db.Pool.QueryRow(ctx, `
			SELECT id::text
			  FROM activity_gear_assignments
			 WHERE activity_id = $1
			   AND user_id = $2
			   AND role_key = 'primary'`, activityID, userID).Scan(&assignmentID); err != nil {
			return nil, fmt.Errorf("load primary assignment: %w", err)
		}
		if previousGearID != "" && previousGearID != gearID {
			if err := db.RefreshGearAndDependentUsageStatsOwnerVerified(ctx, previousGearID); err != nil {
				return nil, fmt.Errorf("refresh previous primary gear usage: %w", err)
			}
		}
	} else if err := db.Pool.QueryRow(ctx, `
		WITH owned AS (
			SELECT a.user_id, a.id AS activity_id, g.id AS gear_id
			  FROM activities a
			  JOIN gear g ON g.id = $3 AND g.user_id = a.user_id
			 WHERE a.id = $1
			   AND a.user_id = $2
		), inserted AS (
			INSERT INTO activity_gear_assignments
				(id, user_id, activity_id, gear_id, role_key, source, quantity)
			SELECT gen_random_uuid(), user_id, activity_id, gear_id, $4, 'user', 1
			  FROM owned
			 WHERE NOT EXISTS (
				SELECT 1 FROM activity_gear_assignments existing
				 WHERE existing.activity_id = owned.activity_id
				   AND existing.gear_id = owned.gear_id
				   AND existing.role_key = $4
			 )
			RETURNING id::text
		)
		SELECT id FROM inserted
		UNION ALL
		SELECT existing.id::text
		  FROM activity_gear_assignments existing
		  JOIN owned ON owned.activity_id = existing.activity_id AND owned.gear_id = existing.gear_id
		 WHERE existing.role_key = $4
		LIMIT 1`, activityID, userID, gearID, roleKey).Scan(&assignmentID); err != nil {
		if err == pgx.ErrNoRows {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("add activity gear assignment: %w", err)
	}

	if _, err := db.RefreshGearUsageForActivityMetricChange(ctx, activityID, userID); err != nil {
		return nil, fmt.Errorf("refresh activity gear usage: %w", err)
	}

	return db.GetActivityGearAssignment(ctx, userID, activityID, assignmentID)
}

func (db *DB) GetActivityGearAssignment(ctx context.Context, userID, activityID, assignmentID string) (*model.ActivityGearAssignment, error) {
	var a model.ActivityGearAssignment
	if err := db.Pool.QueryRow(ctx, `
		SELECT aga.id::text, aga.gear_id::text, g.name, g.type, aga.role_key, aga.quantity
		  FROM activity_gear_assignments aga
		  JOIN activities act ON act.id = aga.activity_id AND act.user_id = aga.user_id
		  JOIN gear g ON g.id = aga.gear_id AND g.user_id = aga.user_id
		 WHERE aga.id = $1
		   AND aga.activity_id = $2
		   AND aga.user_id = $3`, assignmentID, activityID, userID).
		Scan(&a.ID, &a.GearID, &a.GearName, &a.GearType, &a.RoleKey, &a.Quantity); err != nil {
		if err == pgx.ErrNoRows {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get activity gear assignment: %w", err)
	}
	a.RoleLabel = activityGearRoleLabel(a.RoleKey)
	return &a, nil
}

func (db *DB) RemoveActivityGearAssignment(ctx context.Context, userID, activityID, assignmentID string) error {
	var gearID string
	if err := db.Pool.QueryRow(ctx, `
		DELETE FROM activity_gear_assignments aga
		 USING activities a
		 WHERE aga.id = $1
		   AND aga.activity_id = $2
		   AND aga.user_id = $3
		   AND a.id = aga.activity_id
		   AND a.user_id = aga.user_id
		RETURNING aga.gear_id::text`, assignmentID, activityID, userID).Scan(&gearID); err != nil {
		if err == pgx.ErrNoRows {
			return ErrNotFound
		}
		return fmt.Errorf("remove activity gear assignment: %w", err)
	}
	if err := db.RefreshGearAndDependentUsageStatsOwnerVerified(ctx, gearID); err != nil {
		return fmt.Errorf("refresh removed gear usage: %w", err)
	}
	if _, err := db.RefreshGearUsageForActivityMetricChange(ctx, activityID, userID); err != nil {
		return fmt.Errorf("refresh remaining activity gear usage: %w", err)
	}
	return nil
}
