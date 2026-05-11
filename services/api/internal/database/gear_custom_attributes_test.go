package database

import (
	"context"
	"testing"

	"github.com/skadia-app/skadia/services/api/internal/model"
)

// TestComponentAttributesAcceptUnknownKeyAndRoundTrip creates a component with
// an attributes JSONB map that contains keys not defined in the gear schema
// (simulating a user-defined custom field). It asserts that the values
// survive a full write→read round-trip without modification.
func TestComponentAttributesAcceptUnknownKeyAndRoundTrip(t *testing.T) {
	db, userID := gearTestDB(t)
	ctx := context.Background()

	g, err := db.CreateGear(ctx, userID, &model.GearCreate{Name: "Custom Attr Gear", Type: "bike"})
	if err != nil {
		t.Fatalf("CreateGear: %v", err)
	}

	customAttrs := map[string]any{
		"fancy_custom_field":       "user-value",
		"weight_kg_calibrated":     1.234,
		"some_boolean_custom_flag": true,
	}

	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer tx.Rollback(ctx)

	desired := []model.GearComponentInput{{
		Name:       "Custom Component",
		Type:       "bike.fork",
		Attributes: customAttrs,
	}}
	if err := db.ApplyGearComponents(ctx, tx, g.ID, userID, desired); err != nil {
		t.Fatalf("ApplyGearComponents: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit: %v", err)
	}

	components, err := db.listComponents(ctx, g.ID)
	if err != nil {
		t.Fatalf("listComponents: %v", err)
	}
	if len(components) != 1 {
		t.Fatalf("len(components) = %d, want 1", len(components))
	}

	attrs := components[0].Attributes
	if v, _ := attrs["fancy_custom_field"].(string); v != "user-value" {
		t.Errorf(`attributes["fancy_custom_field"] = %v (%T), want "user-value"`, attrs["fancy_custom_field"], attrs["fancy_custom_field"])
	}
	if v, _ := attrs["weight_kg_calibrated"].(float64); v != 1.234 {
		t.Errorf(`attributes["weight_kg_calibrated"] = %v, want 1.234`, attrs["weight_kg_calibrated"])
	}
	if v, _ := attrs["some_boolean_custom_flag"].(bool); !v {
		t.Errorf(`attributes["some_boolean_custom_flag"] = %v, want true`, attrs["some_boolean_custom_flag"])
	}
}

// TestComponentAttributesPreserveAcrossUpdate verifies that a custom attribute
// on a component is NOT silently dropped when the component is updated via
// ApplyGearComponents. The client re-sends the same custom key, and it must
// survive the UPDATE path.
func TestComponentAttributesPreserveAcrossUpdate(t *testing.T) {
	db, userID := gearTestDB(t)
	ctx := context.Background()

	g, err := db.CreateGear(ctx, userID, &model.GearCreate{Name: "Preserve Attrs Gear", Type: "bike"})
	if err != nil {
		t.Fatalf("CreateGear: %v", err)
	}

	// Initial INSERT with custom attribute.
	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer tx.Rollback(ctx)

	initial := []model.GearComponentInput{{
		Name: "Fork",
		Type: "bike.fork",
		Attributes: map[string]any{
			"fancy_custom_field": "initial-value",
		},
	}}
	if err := db.ApplyGearComponents(ctx, tx, g.ID, userID, initial); err != nil {
		t.Fatalf("ApplyGearComponents (insert): %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit (insert): %v", err)
	}

	// Fetch the assigned ID.
	components, err := db.listComponents(ctx, g.ID)
	if err != nil {
		t.Fatalf("listComponents after insert: %v", err)
	}
	if len(components) != 1 {
		t.Fatalf("len(components) = %d after insert, want 1", len(components))
	}
	componentID := components[0].ID

	// UPDATE — resend the custom attribute alongside a name change.
	tx2, err := db.Pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin (update): %v", err)
	}
	defer tx2.Rollback(ctx)

	updated := []model.GearComponentInput{{
		ID:   componentID,
		Name: "Fork (renamed)",
		Type: "bike.fork",
		Attributes: map[string]any{
			"fancy_custom_field": "initial-value",
		},
	}}
	if err := db.ApplyGearComponents(ctx, tx2, g.ID, userID, updated); err != nil {
		t.Fatalf("ApplyGearComponents (update): %v", err)
	}
	if err := tx2.Commit(ctx); err != nil {
		t.Fatalf("commit (update): %v", err)
	}

	after, err := db.listComponents(ctx, g.ID)
	if err != nil {
		t.Fatalf("listComponents after update: %v", err)
	}
	if len(after) != 1 {
		t.Fatalf("len(components) = %d after update, want 1", len(after))
	}
	if after[0].Name != "Fork (renamed)" {
		t.Errorf("Name = %q, want %q", after[0].Name, "Fork (renamed)")
	}
	if v, _ := after[0].Attributes["fancy_custom_field"].(string); v != "initial-value" {
		t.Errorf(`attributes["fancy_custom_field"] = %v, want "initial-value" after update`, after[0].Attributes["fancy_custom_field"])
	}
}

// TestComponentAttributesAllowsDeletionOfCustomKey verifies that a custom
// attribute key can be explicitly removed by sending an empty map (or a map
// without the key) in a subsequent ApplyGearComponents call.
func TestComponentAttributesAllowsDeletionOfCustomKey(t *testing.T) {
	db, userID := gearTestDB(t)
	ctx := context.Background()

	g, err := db.CreateGear(ctx, userID, &model.GearCreate{Name: "Delete Custom Attr Gear", Type: "bike"})
	if err != nil {
		t.Fatalf("CreateGear: %v", err)
	}

	// INSERT with a custom key.
	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin (insert): %v", err)
	}
	defer tx.Rollback(ctx)

	initial := []model.GearComponentInput{{
		Name: "Shock",
		Type: "bike.shock",
		Attributes: map[string]any{
			"custom_notes": "to be deleted",
		},
	}}
	if err := db.ApplyGearComponents(ctx, tx, g.ID, userID, initial); err != nil {
		t.Fatalf("ApplyGearComponents (insert): %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit (insert): %v", err)
	}

	// Verify it round-tripped.
	components, err := db.listComponents(ctx, g.ID)
	if err != nil {
		t.Fatalf("listComponents after insert: %v", err)
	}
	if len(components) != 1 {
		t.Fatalf("len(components) = %d after insert, want 1", len(components))
	}
	if v, _ := components[0].Attributes["custom_notes"].(string); v != "to be deleted" {
		t.Fatalf(`attributes["custom_notes"] = %v, want "to be deleted"`, components[0].Attributes["custom_notes"])
	}
	componentID := components[0].ID

	// UPDATE — omit the custom key entirely by passing an empty attributes map.
	tx2, err := db.Pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin (update): %v", err)
	}
	defer tx2.Rollback(ctx)

	withoutKey := []model.GearComponentInput{{
		ID:         componentID,
		Name:       "Shock",
		Type:       "bike.shock",
		Attributes: map[string]any{}, // explicitly empty — clears all custom keys
	}}
	if err := db.ApplyGearComponents(ctx, tx2, g.ID, userID, withoutKey); err != nil {
		t.Fatalf("ApplyGearComponents (delete-key update): %v", err)
	}
	if err := tx2.Commit(ctx); err != nil {
		t.Fatalf("commit (delete-key update): %v", err)
	}

	after, err := db.listComponents(ctx, g.ID)
	if err != nil {
		t.Fatalf("listComponents after key deletion: %v", err)
	}
	if len(after) != 1 {
		t.Fatalf("len(components) = %d after key deletion, want 1", len(after))
	}
	if _, exists := after[0].Attributes["custom_notes"]; exists {
		t.Errorf(`attributes["custom_notes"] still present after deletion, got %v`, after[0].Attributes["custom_notes"])
	}
}
