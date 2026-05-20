package database

import (
	"context"
	"testing"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/model"
	"github.com/google/uuid"
)

func TestListActivityGearAssignmentsReturnsAllRoles(t *testing.T) {
	db, userID := gearTestDB(t)
	ctx := context.Background()

	bike, err := db.CreateGear(ctx, userID, &model.GearCreate{Name: "Primary Bike", Type: "bike"})
	if err != nil {
		t.Fatalf("CreateGear bike: %v", err)
	}
	helmet, err := db.CreateGear(ctx, userID, &model.GearCreate{Name: "Trail Helmet", Type: "helmet"})
	if err != nil {
		t.Fatalf("CreateGear helmet: %v", err)
	}

	activityID := uuid.NewString()
	if _, err := db.Pool.Exec(ctx, `
		INSERT INTO activities
			(id, user_id, sport_type, title, started_at, file_hash, visibility, enrichment_status, moving_time_s)
		VALUES
			($1, $2, 'cycling', 'gear-assignment-act', $3, $4, 'private', 'complete', 600)`,
		activityID, userID, time.Now().UTC(), "hash-aga-"+activityID[:8],
	); err != nil {
		t.Fatalf("insert activity: %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.Pool.Exec(context.Background(), `DELETE FROM activities WHERE id = $1`, activityID)
	})

	if _, err := db.Pool.Exec(ctx, `
		INSERT INTO activity_gear_assignments (user_id, activity_id, gear_id, role_key, source)
		VALUES ($1, $2, $3, 'primary', 'test'), ($1, $2, $4, 'worn', 'test')`,
		userID, activityID, bike.ID, helmet.ID,
	); err != nil {
		t.Fatalf("insert assignments: %v", err)
	}

	assignments, err := db.ListActivityGearAssignments(ctx, userID, activityID)
	if err != nil {
		t.Fatalf("ListActivityGearAssignments: %v", err)
	}
	if len(assignments) != 2 {
		t.Fatalf("assignments len = %d, want 2: %#v", len(assignments), assignments)
	}
	if assignments[0].GearID != bike.ID || assignments[0].RoleKey != "primary" || assignments[0].RoleLabel != "Hauptausrüstung" {
		t.Fatalf("first assignment = %#v, want primary bike", assignments[0])
	}
	if assignments[1].GearID != helmet.ID || assignments[1].RoleKey != "worn" || assignments[1].RoleLabel != "Getragen" {
		t.Fatalf("second assignment = %#v, want worn helmet", assignments[1])
	}
}

func TestAddAndRemoveActivityGearAssignmentUsesGearDefaultRole(t *testing.T) {
	db, userID := gearTestDB(t)
	ctx := context.Background()

	helmet, err := db.CreateGear(ctx, userID, &model.GearCreate{Name: "Default Helmet", Type: "helmet"})
	if err != nil {
		t.Fatalf("CreateGear helmet: %v", err)
	}

	activityID := uuid.NewString()
	if _, err := db.Pool.Exec(ctx, `
		INSERT INTO activities
			(id, user_id, sport_type, title, started_at, file_hash, visibility, enrichment_status, moving_time_s)
		VALUES
			($1, $2, 'cycling', 'gear-add-act', $3, $4, 'private', 'complete', 600)`,
		activityID, userID, time.Now().UTC(), "hash-add-"+activityID[:8],
	); err != nil {
		t.Fatalf("insert activity: %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.Pool.Exec(context.Background(), `DELETE FROM activities WHERE id = $1`, activityID)
	})

	assignment, err := db.AddActivityGearAssignment(ctx, userID, activityID, helmet.ID, "")
	if err != nil {
		t.Fatalf("AddActivityGearAssignment: %v", err)
	}
	if assignment.RoleKey != "worn" || assignment.RoleLabel != "Getragen" {
		t.Fatalf("assignment role = %s/%s, want worn/Getragen", assignment.RoleKey, assignment.RoleLabel)
	}

	if err := db.RemoveActivityGearAssignment(ctx, userID, activityID, assignment.ID); err != nil {
		t.Fatalf("RemoveActivityGearAssignment: %v", err)
	}
	assignments, err := db.ListActivityGearAssignments(ctx, userID, activityID)
	if err != nil {
		t.Fatalf("ListActivityGearAssignments after remove: %v", err)
	}
	if len(assignments) != 0 {
		t.Fatalf("assignments after remove = %#v, want none", assignments)
	}
}
