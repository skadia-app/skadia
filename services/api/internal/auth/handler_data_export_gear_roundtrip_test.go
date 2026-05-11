package auth

// TestGearExportRoundtrip verifies that all three gear tables added in Mig-354
// appear in the GDPR export ZIP for a user that has data in each:
//
//   - activity_gear_assignments
//   - gear_schema_definitions  (User-Decision A3: user-custom rows only; no
//                               system-defaults expected in the export)
//   - gear_component_swap_events
//
// All three tables are registered in dataimport/tables.go (commit 1f7b6fe09).
// This test covers export only; import-side coverage is exercised by the
// dataimport package's own tests.

import (
	"context"
	"os"
	"testing"
	"time"
)

func TestGearExportRoundtrip(t *testing.T) {
	pool := openGearExportDB(t)
	ctx := context.Background()
	_ = ctx

	// ---- 1. Seed one user with gear data in all three tables ----------------

	userID := createUserForGearTest(t, pool)

	gearID := insertGearRow(t, pool, userID, "Test Road Bike")

	activityID := insertActivityForGear(t, pool, userID)

	assignmentID := insertAssignment(t, pool, userID, activityID, gearID)

	// Schema override with a type_key unique enough to survive parallel test runs.
	schemaTypeKey := "custom_road_" + time.Now().Format("20060102150405")
	schemaDefID := insertSchemaDefinition(t, pool, userID, schemaTypeKey)

	swapEventID := insertSwapEvent(t, pool, userID, gearID)

	// ---- 2. Run export -------------------------------------------------------

	zipPath := runExportForUser(t, pool, userID)
	t.Cleanup(func() { os.Remove(zipPath) })
	t.Logf("gear export ZIP: %s", zipPath)

	// ---- 3. Assert all three tables are present in the ZIP ------------------

	// 3a. activity_gear_assignments
	assignments := readZIPEntry(t, zipPath, "data/activity_gear_assignments.json")
	if assignments == nil {
		t.Fatal("data/activity_gear_assignments.json missing from export ZIP")
	}
	foundAssignment := false
	for _, row := range assignments {
		if id, ok := row["id"].(string); ok && id == assignmentID {
			foundAssignment = true
			break
		}
	}
	if !foundAssignment {
		t.Errorf("activity_gear_assignments: expected row %s not found in ZIP (got %d rows)", assignmentID, len(assignments))
	}

	// 3b. gear_schema_definitions (user-custom only — no system rows expected per User-Decision A3)
	schemaDefs := readZIPEntry(t, zipPath, "data/gear_schema_definitions.json")
	if schemaDefs == nil {
		t.Fatal("data/gear_schema_definitions.json missing from export ZIP")
	}
	foundSchema := false
	for _, row := range schemaDefs {
		if id, ok := row["id"].(string); ok && id == schemaDefID {
			foundSchema = true
			// Verify no is_system field exists (User-Decision A3: column was removed).
			if _, hasIsSystem := row["is_system"]; hasIsSystem {
				t.Error("gear_schema_definitions row has 'is_system' field — should not exist per User-Decision A3")
			}
			break
		}
	}
	if !foundSchema {
		t.Errorf("gear_schema_definitions: expected row %s not found in ZIP (got %d rows)", schemaDefID, len(schemaDefs))
	}

	// 3c. gear_component_swap_events
	swapEvents := readZIPEntry(t, zipPath, "data/gear_component_swap_events.json")
	if swapEvents == nil {
		t.Fatal("data/gear_component_swap_events.json missing from export ZIP")
	}
	foundSwap := false
	for _, row := range swapEvents {
		if id, ok := row["id"].(string); ok && id == swapEventID {
			foundSwap = true
			break
		}
	}
	if !foundSwap {
		t.Errorf("gear_component_swap_events: expected row %s not found in ZIP (got %d rows)", swapEventID, len(swapEvents))
	}

	t.Logf("TestGearExportRoundtrip: all three gear tables present in export (user=%s)", userID)
}
