package database

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/skadia-app/skadia/services/api/internal/model"
)

// applyComponentsFixture seeds a gear row for the given user and returns its id.
func applyComponentsFixture(t *testing.T, db *DB, userID string) string {
	t.Helper()
	g, err := db.CreateGear(context.Background(), userID, &model.GearCreate{
		Name: "ApplyComponentsFixture",
		Type: "bike",
	})
	if err != nil {
		t.Fatalf("CreateGear: %v", err)
	}
	return g.ID
}

func TestApplyGearComponents_InsertsRowsWithoutID(t *testing.T) {
	db, userID := gearTestDB(t)
	gearID := applyComponentsFixture(t, db, userID)

	tx, err := db.Pool.Begin(context.Background())
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer tx.Rollback(context.Background())

	slot := "fork"
	desired := []model.GearComponentInput{{
		Name:    "Front Fork",
		Type:    "bike.fork",
		SlotKey: &slot,
		Attributes: map[string]any{
			"travel_mm": float64(160),
		},
	}}
	if err := db.ApplyGearComponents(context.Background(), tx, gearID, userID, desired); err != nil {
		t.Fatalf("ApplyGearComponents: %v", err)
	}
	if err := tx.Commit(context.Background()); err != nil {
		t.Fatalf("commit: %v", err)
	}

	got, err := db.listComponents(context.Background(), gearID)
	if err != nil {
		t.Fatalf("listComponents: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len(components) = %d, want 1", len(got))
	}
	if got[0].SlotKey == nil || *got[0].SlotKey != "fork" {
		t.Fatalf("SlotKey = %v, want fork", got[0].SlotKey)
	}
	if v, _ := got[0].Attributes["travel_mm"].(float64); v != 160 {
		t.Fatalf("Attributes[travel_mm] = %v, want 160", got[0].Attributes["travel_mm"])
	}
}

func TestApplyGearComponents_UpdatesExistingByID(t *testing.T) {
	db, userID := gearTestDB(t)
	gearID := applyComponentsFixture(t, db, userID)

	// Seed one component via the existing CreateComponent helper.
	created, err := db.CreateComponent(context.Background(), gearID, &model.GearComponentCreate{
		Name: "Original Fork",
		Type: "bike.fork",
	})
	if err != nil {
		t.Fatalf("CreateComponent: %v", err)
	}

	tx, err := db.Pool.Begin(context.Background())
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer tx.Rollback(context.Background())

	slot := "fork"
	desired := []model.GearComponentInput{{
		ID:      created.ID,
		Name:    "Renamed Fork",
		Type:    "bike.fork",
		SlotKey: &slot,
		Attributes: map[string]any{
			"travel_mm": float64(170),
		},
	}}
	if err := db.ApplyGearComponents(context.Background(), tx, gearID, userID, desired); err != nil {
		t.Fatalf("ApplyGearComponents: %v", err)
	}
	if err := tx.Commit(context.Background()); err != nil {
		t.Fatalf("commit: %v", err)
	}

	got, err := db.listComponents(context.Background(), gearID)
	if err != nil {
		t.Fatalf("listComponents: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len(components) = %d, want 1", len(got))
	}
	if got[0].Name != "Renamed Fork" {
		t.Fatalf("Name = %q, want Renamed Fork", got[0].Name)
	}
	if got[0].SlotKey == nil || *got[0].SlotKey != "fork" {
		t.Fatalf("SlotKey = %v, want fork", got[0].SlotKey)
	}
	if v, _ := got[0].Attributes["travel_mm"].(float64); v != 170 {
		t.Fatalf("Attributes[travel_mm] = %v, want 170", got[0].Attributes["travel_mm"])
	}
}

func TestApplyGearComponents_DeletesMissing(t *testing.T) {
	db, userID := gearTestDB(t)
	gearID := applyComponentsFixture(t, db, userID)

	a, _ := db.CreateComponent(context.Background(), gearID, &model.GearComponentCreate{
		Name: "Keeper",
		Type: "bike.fork",
	})
	_, _ = db.CreateComponent(context.Background(), gearID, &model.GearComponentCreate{
		Name: "Goner",
		Type: "bike.shock",
	})

	tx, err := db.Pool.Begin(context.Background())
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer tx.Rollback(context.Background())

	desired := []model.GearComponentInput{{
		ID:   a.ID,
		Name: "Keeper",
		Type: "bike.fork",
	}}
	if err := db.ApplyGearComponents(context.Background(), tx, gearID, userID, desired); err != nil {
		t.Fatalf("ApplyGearComponents: %v", err)
	}
	if err := tx.Commit(context.Background()); err != nil {
		t.Fatalf("commit: %v", err)
	}

	got, err := db.listComponents(context.Background(), gearID)
	if err != nil {
		t.Fatalf("listComponents: %v", err)
	}
	if len(got) != 1 || got[0].ID != a.ID {
		t.Fatalf("got = %#v, want only id=%s", got, a.ID)
	}
}

func TestApplyGearComponents_NoOpEmptyInputAndExistingEmpty(t *testing.T) {
	db, userID := gearTestDB(t)
	gearID := applyComponentsFixture(t, db, userID)

	tx, err := db.Pool.Begin(context.Background())
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer tx.Rollback(context.Background())

	if err := db.ApplyGearComponents(context.Background(), tx, gearID, userID, nil); err != nil {
		t.Fatalf("ApplyGearComponents nil: %v", err)
	}
	if err := db.ApplyGearComponents(context.Background(), tx, gearID, userID, []model.GearComponentInput{}); err != nil {
		t.Fatalf("ApplyGearComponents empty: %v", err)
	}
	if err := tx.Commit(context.Background()); err != nil {
		t.Fatalf("commit: %v", err)
	}

	got, err := db.listComponents(context.Background(), gearID)
	if err != nil {
		t.Fatalf("listComponents: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("len(components) = %d, want 0", len(got))
	}
}

func TestApplyGearComponents_WipesAllOnEmptyInput(t *testing.T) {
	db, userID := gearTestDB(t)
	gearID := applyComponentsFixture(t, db, userID)

	_, _ = db.CreateComponent(context.Background(), gearID, &model.GearComponentCreate{
		Name: "Wipe me",
		Type: "bike.shock",
	})

	tx, err := db.Pool.Begin(context.Background())
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer tx.Rollback(context.Background())

	if err := db.ApplyGearComponents(context.Background(), tx, gearID, userID, []model.GearComponentInput{}); err != nil {
		t.Fatalf("ApplyGearComponents empty: %v", err)
	}
	if err := tx.Commit(context.Background()); err != nil {
		t.Fatalf("commit: %v", err)
	}

	got, err := db.listComponents(context.Background(), gearID)
	if err != nil {
		t.Fatalf("listComponents: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("len(components) = %d, want 0 after wipe", len(got))
	}
}

func TestApplyGearComponents_RejectsCrossUserGear(t *testing.T) {
	db, userA := gearTestDB(t)
	_, userB := gearTestDB(t)
	gearAID := applyComponentsFixture(t, db, userA)

	tx, err := db.Pool.Begin(context.Background())
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer tx.Rollback(context.Background())

	err = db.ApplyGearComponents(context.Background(), tx, gearAID, userB, []model.GearComponentInput{{
		Name: "Sneaky", Type: "bike.fork",
	}})
	if err == nil {
		t.Fatal("ApplyGearComponents: want error when userB updates userA gear, got nil")
	}
}

func TestApplyGearComponents_RejectsForeignComponentID(t *testing.T) {
	db, userA := gearTestDB(t)
	_, userB := gearTestDB(t)
	gearAID := applyComponentsFixture(t, db, userA)
	gearBID := applyComponentsFixture(t, db, userB)

	// Insert a component on userB's gear.
	other, err := db.CreateComponent(context.Background(), gearBID, &model.GearComponentCreate{
		Name: "userB's component",
		Type: "bike.fork",
	})
	if err != nil {
		t.Fatalf("CreateComponent: %v", err)
	}

	tx, err := db.Pool.Begin(context.Background())
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer tx.Rollback(context.Background())

	// userA tries to UPDATE userB's component by passing its id while editing
	// userA's own gear — must be rejected.
	err = db.ApplyGearComponents(context.Background(), tx, gearAID, userA, []model.GearComponentInput{{
		ID:   other.ID,
		Name: "Hijacked",
		Type: "bike.fork",
	}})
	if err == nil {
		t.Fatal("ApplyGearComponents: want error on cross-gear id, got nil")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Fatalf("error %q, want 'not found' message", err)
	}

	// And userB's component must still exist.
	got, err := db.listComponents(context.Background(), gearBID)
	if err != nil {
		t.Fatalf("listComponents: %v", err)
	}
	if len(got) != 1 || got[0].Name != "userB's component" {
		t.Fatalf("userB components = %#v, want untouched", got)
	}
}

func TestApplyGearComponents_RejectsNonexistentGear(t *testing.T) {
	db, userID := gearTestDB(t)
	tx, err := db.Pool.Begin(context.Background())
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer tx.Rollback(context.Background())

	err = db.ApplyGearComponents(context.Background(), tx, uuid.NewString(), userID, []model.GearComponentInput{{
		Name: "x", Type: "bike.fork",
	}})
	if err == nil {
		t.Fatal("ApplyGearComponents: want error on missing gear, got nil")
	}
}

func TestUpdateGear_AppliesComponentsDiff(t *testing.T) {
	db, userID := gearTestDB(t)
	gearID := applyComponentsFixture(t, db, userID)

	a, _ := db.CreateComponent(context.Background(), gearID, &model.GearComponentCreate{
		Name: "Existing", Type: "bike.fork",
	})

	slot := "fork"
	desired := []model.GearComponentInput{
		{ID: a.ID, Name: "Existing renamed", Type: "bike.fork", SlotKey: &slot},
		{Name: "Brand new", Type: "bike.shock"},
	}
	g, err := db.UpdateGear(context.Background(), gearID, &model.GearUpdate{
		Components: &desired,
	})
	if err != nil {
		t.Fatalf("UpdateGear: %v", err)
	}
	if len(g.Components) != 2 {
		t.Fatalf("Components = %d, want 2", len(g.Components))
	}
	hasRenamed, hasNew := false, false
	for _, c := range g.Components {
		if c.Name == "Existing renamed" {
			hasRenamed = true
		}
		if c.Name == "Brand new" {
			hasNew = true
		}
	}
	if !hasRenamed || !hasNew {
		t.Fatalf("got %+v, want renamed + brand new", g.Components)
	}
}

func TestUpdateGear_NilComponentsLeavesExistingAlone(t *testing.T) {
	db, userID := gearTestDB(t)
	gearID := applyComponentsFixture(t, db, userID)

	_, _ = db.CreateComponent(context.Background(), gearID, &model.GearComponentCreate{
		Name: "Keep me", Type: "bike.fork",
	})

	newName := "Renamed gear"
	g, err := db.UpdateGear(context.Background(), gearID, &model.GearUpdate{
		Name: &newName,
	})
	if err != nil {
		t.Fatalf("UpdateGear: %v", err)
	}
	if len(g.Components) != 1 {
		t.Fatalf("Components = %d, want 1 (unchanged)", len(g.Components))
	}
}
