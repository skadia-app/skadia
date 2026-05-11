package database

import (
	"context"
	"testing"

	"github.com/skadia-app/skadia/services/api/internal/model"
	"github.com/google/uuid"
)

// insertBikeGearWithType inserts a gear row of the given gear-type key and
// returns its UUID. The type and scope are set explicitly so the caller can
// pick a leaf type (e.g. "bike.road" or "bike.mtb.enduro") that exercises
// the schema's parent-chain walk in GearTypeAppliesToTarget.
func insertBikeGearWithType(t *testing.T, db *DB, userID, name, typeKey string) string {
	t.Helper()
	id := uuid.NewString()
	_, err := db.Pool.Exec(context.Background(), `
		INSERT INTO gear (id, user_id, name, type, scope)
		VALUES ($1, $2, $3, $4, 'bike')`, id, userID, name, typeKey)
	if err != nil {
		t.Fatalf("insert gear %q (%s): %v", name, typeKey, err)
	}
	t.Cleanup(func() {
		_, _ = db.Pool.Exec(context.Background(), `DELETE FROM gear WHERE id = $1`, id)
	})
	return id
}

// TestCreateComponentWithPresetGearTypeKey verifies that CreateComponent
// persists preset_gear_type_key and that a follow-up read returns the value
// unchanged.
func TestCreateComponentWithPresetGearTypeKey(t *testing.T) {
	db := mapLayerTestDB(t)
	ctx := context.Background()
	userID := insertTestUser(t, db)

	gearID := insertBikeGearWithType(t, db, userID, "TestBike", "bike.mtb")

	preset := "bike.mtb"
	created, err := db.CreateComponent(ctx, gearID, &model.GearComponentCreate{
		Name:              "MTB Frame",
		Type:              "bike.frame",
		PresetGearTypeKey: &preset,
	})
	if err != nil {
		t.Fatalf("CreateComponent: %v", err)
	}
	if created == nil {
		t.Fatal("CreateComponent returned nil component")
	}
	if created.PresetGearTypeKey == nil || *created.PresetGearTypeKey != preset {
		t.Errorf("created.PresetGearTypeKey = %v, want %q", created.PresetGearTypeKey, preset)
	}

	// Round-trip: confirm the saved value survives a fresh read of the row.
	c2, err := scanComponent(db.Pool.QueryRow(ctx,
		`SELECT `+componentSelectCols+` FROM gear_components WHERE id = $1`, created.ID,
	))
	if err != nil {
		t.Fatalf("re-read component: %v", err)
	}
	if c2 == nil {
		t.Fatal("re-read component returned nil")
	}
	if c2.PresetGearTypeKey == nil || *c2.PresetGearTypeKey != preset {
		t.Errorf("round-trip PresetGearTypeKey = %v, want %q", c2.PresetGearTypeKey, preset)
	}

	// A second component without preset should round-trip with PresetGearTypeKey == nil.
	c3, err := db.CreateComponent(ctx, gearID, &model.GearComponentCreate{
		Name: "Saddle",
		Type: "bike.saddle",
	})
	if err != nil {
		t.Fatalf("CreateComponent (no preset): %v", err)
	}
	if c3 == nil {
		t.Fatal("CreateComponent (no preset) returned nil")
	}
	if c3.PresetGearTypeKey != nil {
		t.Errorf("expected PresetGearTypeKey nil for no-preset component, got %q", *c3.PresetGearTypeKey)
	}
}

// TestGetComponentTransferEligibilityReturnsPresetGearTypeKey verifies that
// the eligibility query populates gearcompat.Component.PresetGearTypeKey from
// the DB column, which the transfer handler uses to enforce preset locks.
func TestGetComponentTransferEligibilityReturnsPresetGearTypeKey(t *testing.T) {
	db := mapLayerTestDB(t)
	ctx := context.Background()
	userID := insertTestUser(t, db)

	srcGearID := insertBikeGearWithType(t, db, userID, "SrcBike", "bike.mtb")
	dstGearID := insertBikeGearWithType(t, db, userID, "DstBike", "bike.road")

	preset := "bike.mtb"
	created, err := db.CreateComponent(ctx, srcGearID, &model.GearComponentCreate{
		Name:              "Locked Frame",
		Type:              "bike.frame",
		PresetGearTypeKey: &preset,
	})
	if err != nil {
		t.Fatalf("CreateComponent: %v", err)
	}

	elig, err := db.GetComponentTransferEligibilityOwnerVerified(ctx, created.ID, dstGearID)
	if err != nil {
		t.Fatalf("GetComponentTransferEligibilityOwnerVerified: %v", err)
	}
	if elig.Component.PresetGearTypeKey == nil || *elig.Component.PresetGearTypeKey != preset {
		t.Errorf("eligibility.Component.PresetGearTypeKey = %v, want %q",
			elig.Component.PresetGearTypeKey, preset)
	}
	if elig.TargetGear.TypeKey != "bike.road" {
		t.Errorf("eligibility.TargetGear.TypeKey = %q, want %q", elig.TargetGear.TypeKey, "bike.road")
	}
	if elig.FromGearID != srcGearID {
		t.Errorf("eligibility.FromGearID = %q, want %q", elig.FromGearID, srcGearID)
	}
}

// TestApplyGearComponentsPersistsPresetGearTypeKey verifies that the
// gear-create/update reconciliation path also honors PresetGearTypeKey on
// both INSERT and UPDATE phases.
func TestApplyGearComponentsPersistsPresetGearTypeKey(t *testing.T) {
	db := mapLayerTestDB(t)
	ctx := context.Background()
	userID := insertTestUser(t, db)

	gearID := insertBikeGearWithType(t, db, userID, "ApplyBike", "bike.mtb")

	preset := "bike.mtb"
	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	defer tx.Rollback(ctx)

	if err := db.ApplyGearComponents(ctx, tx, gearID, userID, []model.GearComponentInput{{
		Name:              "Locked Frame",
		Type:              "bike.frame",
		PresetGearTypeKey: &preset,
	}}); err != nil {
		t.Fatalf("ApplyGearComponents insert: %v", err)
	}

	var insertedID string
	var insertedPreset *string
	if err := tx.QueryRow(ctx,
		`SELECT id, preset_gear_type_key FROM gear_components WHERE gear_id = $1`,
		gearID,
	).Scan(&insertedID, &insertedPreset); err != nil {
		t.Fatalf("read inserted component: %v", err)
	}
	if insertedPreset == nil || *insertedPreset != preset {
		t.Errorf("inserted preset_gear_type_key = %v, want %q", insertedPreset, preset)
	}

	// Update phase: clear the preset via explicit nil pointer.
	if err := db.ApplyGearComponents(ctx, tx, gearID, userID, []model.GearComponentInput{{
		ID:                insertedID,
		Name:              "Locked Frame",
		Type:              "bike.frame",
		PresetGearTypeKey: nil,
	}}); err != nil {
		t.Fatalf("ApplyGearComponents update: %v", err)
	}

	var updatedPreset *string
	if err := tx.QueryRow(ctx,
		`SELECT preset_gear_type_key FROM gear_components WHERE id = $1`, insertedID,
	).Scan(&updatedPreset); err != nil {
		t.Fatalf("read updated component: %v", err)
	}
	if updatedPreset != nil {
		t.Errorf("expected preset_gear_type_key to be NULL after update, got %q", *updatedPreset)
	}
}

// TestUpdateComponentPatchesPresetGearTypeKey verifies that the single-field
// PATCH path (UpdateComponent) accepts preset_gear_type_key, supports clearing
// the value back to NULL by passing a pointer to "", and does not touch other
// columns when their pointers are nil.
func TestUpdateComponentPatchesPresetGearTypeKey(t *testing.T) {
	db := mapLayerTestDB(t)
	ctx := context.Background()
	userID := insertTestUser(t, db)

	gearID := insertBikeGearWithType(t, db, userID, "PresetBike", "bike.mtb")

	created, err := db.CreateComponent(ctx, gearID, &model.GearComponentCreate{
		Name: "Cassette",
		Type: "bike.cassette",
	})
	if err != nil {
		t.Fatalf("CreateComponent: %v", err)
	}
	if created.PresetGearTypeKey != nil {
		t.Fatalf("preset_gear_type_key = %v on create, want NULL", *created.PresetGearTypeKey)
	}
	originalName := created.Name

	// PATCH only preset_gear_type_key — other columns untouched.
	preset := "bike.cassette"
	updated, err := db.UpdateComponent(ctx, created.ID,
		nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, &preset)
	if err != nil {
		t.Fatalf("UpdateComponent set preset: %v", err)
	}
	if updated.PresetGearTypeKey == nil || *updated.PresetGearTypeKey != preset {
		t.Fatalf("preset_gear_type_key = %v after PATCH, want %q",
			updated.PresetGearTypeKey, preset)
	}
	if updated.Name != originalName {
		t.Fatalf("name = %q after preset-only PATCH, want %q (unchanged)", updated.Name, originalName)
	}

	// PATCH with empty string clears the preset back to NULL.
	empty := ""
	cleared, err := db.UpdateComponent(ctx, created.ID,
		nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, &empty)
	if err != nil {
		t.Fatalf("UpdateComponent clear preset: %v", err)
	}
	if cleared.PresetGearTypeKey != nil {
		t.Fatalf("preset_gear_type_key = %q after clear PATCH, want NULL",
			*cleared.PresetGearTypeKey)
	}

	// PATCH with nil leaves the preset unchanged.
	preset2 := "bike.frame"
	if _, err := db.UpdateComponent(ctx, created.ID,
		nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, &preset2); err != nil {
		t.Fatalf("UpdateComponent set preset 2: %v", err)
	}
	newName := "Different Name"
	afterNilPreset, err := db.UpdateComponent(ctx, created.ID,
		&newName, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("UpdateComponent rename only: %v", err)
	}
	if afterNilPreset.PresetGearTypeKey == nil || *afterNilPreset.PresetGearTypeKey != preset2 {
		t.Fatalf("preset_gear_type_key = %v after nil-preset PATCH, want %q (unchanged)",
			afterNilPreset.PresetGearTypeKey, preset2)
	}
	if afterNilPreset.Name != newName {
		t.Fatalf("name = %q, want %q", afterNilPreset.Name, newName)
	}
}
