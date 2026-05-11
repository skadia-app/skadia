package dataimport

// TestGearRebuildTablesImportRoundtrip verifies that the three tables added in
// migration 000355 (activity_gear_assignments, gear_schema_definitions,
// gear_component_swap_events) are registered in allTables() and survive a
// full export->wipe->import cycle with correct user_id isolation.
//
// Phase ordering: gear_schema_definitions = Phase 3 (user FK only);
// activity_gear_assignments + gear_component_swap_events = Phase 4 (FK to gear
// and activities, which are imported in earlier phases).

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
)

// TestGearRebuildTablesRegisteredInAllTables verifies that all three new gear
// tables are present in allTables() with the expected phase numbers.
func TestGearRebuildTablesRegisteredInAllTables(t *testing.T) {
	want := map[string]int{
		"gear_schema_definitions":    3,
		"activity_gear_assignments":  6,
		"gear_component_swap_events": 4,
	}

	found := make(map[string]int)
	for _, spec := range allTables() {
		if _, ok := want[spec.Name]; ok {
			found[spec.Name] = spec.Phase
		}
	}

	for name, wantPhase := range want {
		if gotPhase, ok := found[name]; !ok {
			t.Errorf("allTables(): missing spec for %q", name)
		} else if gotPhase != wantPhase {
			t.Errorf("allTables(): %q phase = %d, want %d", name, gotPhase, wantPhase)
		}
	}
}

// TestGearRebuildTablesImportRoundtrip seeds three rows (one per new table),
// runs the import engine, and asserts they appear under the target user_id.
func TestGearRebuildTablesImportRoundtrip(t *testing.T) {
	pool := testDB(t)
	targetUserID := createTestUser(t, pool)
	srcUser := uuid.New().String()
	ctx := context.Background()

	gearOldID := uuid.New().String()
	actOldID := uuid.New().String()
	schemaDefOldID := uuid.New().String()
	assignmentOldID := uuid.New().String()
	swapEventOldID := uuid.New().String()
	now := time.Now().UTC()

	// Wire columns.json so the engine knows which columns to expect.
	// activities.gear_id is included so the activities-INSERT fires the
	// fn_mirror_activity_gear_to_aga trigger; this is the realistic shape
	// of a GDPR export and the original failure surface of Critic-4 #1.
	cols := map[string][]string{
		"gear": {
			"id", "user_id", "name", "type", "status", "scope",
			"created_at", "updated_at",
		},
		"activities": {
			"id", "user_id", "title", "sport_type", "started_at", "ended_at",
			"visibility", "is_manual_entry", "gear_id", "created_at", "updated_at",
		},
		"gear_schema_definitions": {
			"id", "user_id", "type_key", "definition", "created_at", "updated_at",
		},
		"activity_gear_assignments": {
			"id", "user_id", "activity_id", "gear_id", "role_key", "source",
			"created_at", "updated_at",
		},
		"gear_component_swap_events": {
			"id", "user_id", "gear_id", "slot_key", "swap_date", "created_at",
		},
	}

	colsBytes, _ := json.Marshal(cols)
	gearRows, _ := json.Marshal([]map[string]any{{
		"id": gearOldID, "user_id": srcUser, "name": "Roundtrip Bike",
		"type": "bike", "status": "active", "scope": "bike",
		"created_at": now.Format(time.RFC3339), "updated_at": now.Format(time.RFC3339),
	}})
	actRows, _ := json.Marshal([]map[string]any{{
		"id": actOldID, "user_id": srcUser, "title": "Roundtrip Ride",
		"sport_type": "cycling", "started_at": now.Format(time.RFC3339),
		"ended_at": now.Add(time.Hour).Format(time.RFC3339),
		"visibility": "private", "is_manual_entry": false,
		"gear_id":    gearOldID,
		"created_at": now.Format(time.RFC3339), "updated_at": now.Format(time.RFC3339),
	}})
	schemaDefRows, _ := json.Marshal([]map[string]any{{
		"id": schemaDefOldID, "user_id": srcUser, "type_key": "custom_roundtrip_bike",
		"definition": `{"label":"Custom Bike"}`,
		"created_at": now.Format(time.RFC3339), "updated_at": now.Format(time.RFC3339),
	}})
	assignmentRows, _ := json.Marshal([]map[string]any{{
		"id": assignmentOldID, "user_id": srcUser,
		"activity_id": actOldID, "gear_id": gearOldID,
		"role_key": "primary", "source": "test",
		"created_at": now.Format(time.RFC3339), "updated_at": now.Format(time.RFC3339),
	}})
	swapEventRows, _ := json.Marshal([]map[string]any{{
		"id": swapEventOldID, "user_id": srcUser, "gear_id": gearOldID,
		"slot_key": "chain", "swap_date": now.Format(time.RFC3339),
		"created_at": now.Format(time.RFC3339),
	}})

	zipPath := buildExportZip(t, zipBuildOpts{
		sourceUserID:    srcUser,
		schemaMigration: 354,
		extraFiles: map[string][]byte{
			"columns.json":                         colsBytes,
			"data/gear.json":                       gearRows,
			"data/activities.json":                 actRows,
			"data/gear_schema_definitions.json":    schemaDefRows,
			"data/activity_gear_assignments.json":  assignmentRows,
			"data/gear_component_swap_events.json": swapEventRows,
		},
	})

	src := openZipSource(t, zipPath)
	eng := newEngine(pool)
	_, err := eng.RunImport(ctx, ImportInput{
		Source:       src,
		TargetUserID: targetUserID,
	})
	if err != nil {
		t.Fatalf("RunImport: %v", err)
	}

	// Assert all three tables have exactly one row for the target user.
	// activity_gear_assignments: exactly one row, and it must be the
	// user-authoritative one (source='test'), not a mirror row produced
	// by the activities-INSERT trigger. Critic-4 #1: without the
	// importer-side skip flag the trigger creates a source='mirror'
	// row first; the user-source INSERT then collides with the partial
	// unique index on (activity_id) WHERE role_key='primary' and the
	// authoritative row is silently lost.
	assertRowCount(t, pool, "gear_schema_definitions", targetUserID, 1)
	assertRowCount(t, pool, "activity_gear_assignments", targetUserID, 1)
	assertRowCount(t, pool, "gear_component_swap_events", targetUserID, 1)

	var agaSource string
	if err := pool.QueryRow(ctx, `
		SELECT source FROM activity_gear_assignments
		 WHERE user_id = $1 AND role_key = 'primary'
	`, targetUserID).Scan(&agaSource); err != nil {
		t.Fatalf("query aga source: %v", err)
	}
	if agaSource != "test" {
		t.Errorf("aga.source = %q, want %q (user-authoritative export row should not be replaced by mirror row)", agaSource, "test")
	}

	// Assert per-user isolation: source user_id never leaks into the target user's data.
	var count int
	for _, table := range []string{
		"gear_schema_definitions", "activity_gear_assignments", "gear_component_swap_events",
	} {
		if err := pool.QueryRow(ctx,
			"SELECT COUNT(*) FROM "+table+" WHERE user_id = $1", srcUser,
		).Scan(&count); err != nil {
			t.Fatalf("isolation check %s: %v", table, err)
		}
		if count > 0 {
			t.Errorf("user isolation breach: %s has %d rows for srcUser", table, count)
		}
	}

	// Assert activity_gear_assignments FKs were remapped to the target user's rows.
	var assignedGearUserID, assignedActUserID string
	if err := pool.QueryRow(ctx, `
		SELECT g.user_id, a.user_id
		FROM activity_gear_assignments aga
		JOIN gear g ON g.id = aga.gear_id
		JOIN activities a ON a.id = aga.activity_id
		WHERE aga.user_id = $1
	`, targetUserID).Scan(&assignedGearUserID, &assignedActUserID); err != nil {
		t.Fatalf("query assignment join: %v", err)
	}
	if assignedGearUserID != targetUserID {
		t.Errorf("assignment gear_id points to wrong user: want %q, got %q", targetUserID, assignedGearUserID)
	}
	if assignedActUserID != targetUserID {
		t.Errorf("assignment activity_id points to wrong user: want %q, got %q", targetUserID, assignedActUserID)
	}
}

// TestGearRebuildImportBackfillsLegacyAGAForLegacyGearID covers the orphan
// case (Critic-4 #1, second branch): the export has activities.gear_id set but
// no corresponding aga export row. After Wave-5/Cluster I drops the
// activities.gear_id column, the legacy field is captured during Phase 5
// (in-memory map[newActID]newGearID) and replayed by Phase 6c
// (backfillLegacyGearAssignments). The synthesized aga row carries
// source='legacy' to mark its origin; ON CONFLICT DO NOTHING preserves
// any user-authoritative aga from the export.
func TestGearRebuildImportBackfillsLegacyAGAForLegacyGearID(t *testing.T) {
	pool := testDB(t)
	targetUserID := createTestUser(t, pool)
	srcUser := uuid.New().String()
	ctx := context.Background()

	gearOldID := uuid.New().String()
	actOldID := uuid.New().String()
	now := time.Now().UTC()

	cols := map[string][]string{
		"gear": {
			"id", "user_id", "name", "type", "status", "scope",
			"created_at", "updated_at",
		},
		"activities": {
			"id", "user_id", "title", "sport_type", "started_at", "ended_at",
			"visibility", "is_manual_entry", "gear_id", "created_at", "updated_at",
		},
	}

	colsBytes, _ := json.Marshal(cols)
	gearRows, _ := json.Marshal([]map[string]any{{
		"id": gearOldID, "user_id": srcUser, "name": "Legacy Bike",
		"type": "bike", "status": "active", "scope": "bike",
		"created_at": now.Format(time.RFC3339), "updated_at": now.Format(time.RFC3339),
	}})
	actRows, _ := json.Marshal([]map[string]any{{
		"id": actOldID, "user_id": srcUser, "title": "Legacy Ride",
		"sport_type": "cycling", "started_at": now.Format(time.RFC3339),
		"ended_at":  now.Add(time.Hour).Format(time.RFC3339),
		"visibility": "private", "is_manual_entry": false,
		"gear_id":    gearOldID,
		"created_at": now.Format(time.RFC3339), "updated_at": now.Format(time.RFC3339),
	}})
	// Note: NO data/activity_gear_assignments.json — simulates legacy export.

	zipPath := buildExportZip(t, zipBuildOpts{
		sourceUserID:    srcUser,
		schemaMigration: 354,
		extraFiles: map[string][]byte{
			"columns.json":         colsBytes,
			"data/gear.json":       gearRows,
			"data/activities.json": actRows,
		},
	})

	src := openZipSource(t, zipPath)
	eng := newEngine(pool)
	if _, err := eng.RunImport(ctx, ImportInput{
		Source:       src,
		TargetUserID: targetUserID,
	}); err != nil {
		t.Fatalf("RunImport: %v", err)
	}

	// Exactly one aga row, source='legacy' (synthesized by Phase 6c backfill
	// from the captured rawRow["gear_id"] map; the column itself was dropped).
	var count int
	var source string
	if err := pool.QueryRow(ctx, `
		SELECT COUNT(*), MAX(source) FROM activity_gear_assignments
		 WHERE user_id = $1 AND role_key = 'primary'
	`, targetUserID).Scan(&count, &source); err != nil {
		t.Fatalf("count aga: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 backfilled aga row, got %d", count)
	}
	if source != "legacy" {
		t.Errorf("expected source='legacy' from Phase 6c backfill, got %q", source)
	}
}
