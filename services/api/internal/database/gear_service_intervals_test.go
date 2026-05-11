package database

import (
	"context"
	"errors"
	"testing"

	"github.com/skadia-app/skadia/services/api/internal/model"
	"github.com/jackc/pgx/v5"
)

// TestServiceIntervalCRUD exercises create/list/update/delete for service intervals
// and verifies ownership enforcement.
func TestServiceIntervalCRUD(t *testing.T) {
	db := mapLayerTestDB(t)
	ctx := context.Background()

	userID := insertTestUser(t, db)
	gearID := insertGearForUsageTest(t, db, userID, "CRUD Bike")
	componentID := insertComponentForUsageTest(t, db, gearID, "Kette")

	// --- Create ---
	isRecurring := true
	created, err := db.CreateServiceInterval(ctx, componentID, &model.GearServiceIntervalCreate{
		Name:        "Ölwechsel",
		MetricKey:   "distance_m",
		IntervalValue: 2000,
		IsRecurring: &isRecurring,
	})
	if err != nil {
		t.Fatalf("CreateServiceInterval: %v", err)
	}
	if created == nil {
		t.Fatal("CreateServiceInterval returned nil")
	}
	if created.Name != "Ölwechsel" {
		t.Fatalf("name = %q, want Ölwechsel", created.Name)
	}
	if created.MetricKey != "distance_m" {
		t.Fatalf("metric_key = %q, want distance_m", created.MetricKey)
	}
	if created.IntervalValue != 2000 {
		t.Fatalf("interval_value = %v, want 2000", created.IntervalValue)
	}

	// --- List ---
	intervals, err := db.ListServiceIntervals(ctx, componentID)
	if err != nil {
		t.Fatalf("ListServiceIntervals: %v", err)
	}
	if len(intervals) != 1 {
		t.Fatalf("len = %d, want 1", len(intervals))
	}
	if intervals[0].ID != created.ID {
		t.Fatalf("id mismatch: got %q want %q", intervals[0].ID, created.ID)
	}

	// --- Update ---
	newValue := 3000.0
	updated, err := db.UpdateServiceInterval(ctx, created.ID, &model.GearServiceIntervalUpdate{
		IntervalValue: &newValue,
	})
	if err != nil {
		t.Fatalf("UpdateServiceInterval: %v", err)
	}
	if updated == nil {
		t.Fatal("UpdateServiceInterval returned nil")
	}
	if updated.IntervalValue != 3000 {
		t.Fatalf("interval_value after update = %v, want 3000", updated.IntervalValue)
	}

	// --- Ownership check ---
	otherUserID := insertTestUser(t, db)
	err = db.VerifyServiceIntervalOwnership(ctx, created.ID, otherUserID)
	if !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("ownership check for wrong user: want pgx.ErrNoRows, got %v", err)
	}
	err = db.VerifyServiceIntervalOwnership(ctx, created.ID, userID)
	if err != nil {
		t.Fatalf("ownership check for correct user: %v", err)
	}

	// --- Delete ---
	if err := db.DeleteServiceInterval(ctx, created.ID); err != nil {
		t.Fatalf("DeleteServiceInterval: %v", err)
	}
	intervals, err = db.ListServiceIntervals(ctx, componentID)
	if err != nil {
		t.Fatalf("ListServiceIntervals after delete: %v", err)
	}
	if len(intervals) != 0 {
		t.Fatalf("expected 0 intervals after delete, got %d", len(intervals))
	}
}

// TestServiceIntervalCreateUnsupportedMetric verifies that an invalid metric_key is rejected.
func TestServiceIntervalCreateUnsupportedMetric(t *testing.T) {
	db := mapLayerTestDB(t)
	ctx := context.Background()

	userID := insertTestUser(t, db)
	gearID := insertGearForUsageTest(t, db, userID, "Bad Metric Bike")
	componentID := insertComponentForUsageTest(t, db, gearID, "Rahmen")

	_, err := db.CreateServiceInterval(ctx, componentID, &model.GearServiceIntervalCreate{
		Name:          "Ungültiger Test",
		MetricKey:     "net_ascent_m", // valid usage metric but not a service metric
		IntervalValue: 100,
	})
	if err == nil {
		t.Fatal("expected error for unsupported service metric, got nil")
	}
}
