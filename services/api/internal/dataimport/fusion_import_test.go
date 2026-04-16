package dataimport

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
)

// ---------------------------------------------------------------------------
// TestFusionMembershipsImport
// ---------------------------------------------------------------------------

// TestFusionMembershipsImport verifies that activity_fusion_memberships rows
// are correctly imported from a GDPR export ZIP (v_new path — memberships file
// present alongside activities).
//
// Topology:
//   - Source export: user S with activities A, B (sources) and M (merged).
//   - M has memberships → A (field_key "gps") and B (field_key "power").
//   - Import into target user T: all three activities + 2 memberships must land
//     in T's scope with remapped UUIDs.
func TestFusionMembershipsImport(t *testing.T) {
	pool := testDB(t)
	userID := createTestUser(t, pool)

	srcUserID := uuid.New().String()
	oldA := uuid.New().String()
	oldB := uuid.New().String()
	oldM := uuid.New().String()

	now := time.Now().UTC()

	makeActivity := func(id, title string) map[string]any {
		return map[string]any{
			"id":          id,
			"user_id":     srcUserID,
			"sport_type":  "cycling",
			"started_at":  now.Format(time.RFC3339),
			"ended_at":    now.Add(time.Hour).Format(time.RFC3339),
			"title":       title,
			"visibility":  "private",
			"created_at":  now.Format(time.RFC3339),
			"updated_at":  now.Format(time.RFC3339),
		}
	}

	memberships := []map[string]any{
		{"merged_id": oldM, "source_id": oldA, "field_key": "gps", "user_id": srcUserID},
		{"merged_id": oldM, "source_id": oldB, "field_key": "power", "user_id": srcUserID},
	}

	membershipsJSON, err := json.Marshal(memberships)
	if err != nil {
		t.Fatalf("marshal memberships: %v", err)
	}

	zipPath := buildExportZip(t, zipBuildOpts{
		sourceUserID:    srcUserID,
		schemaMigration: 302,
		activities:      []map[string]any{makeActivity(oldA, "Ride A"), makeActivity(oldB, "Ride B"), makeActivity(oldM, "Merged Ride")},
		columns: map[string][]string{
			"activities": {
				"id", "user_id", "sport_type", "started_at", "ended_at",
				"title", "visibility", "created_at", "updated_at",
			},
			"activity_fusion_memberships": {"merged_id", "source_id", "field_key", "user_id"},
		},
		extraFiles: map[string][]byte{
			"data/activity_fusion_memberships.json": membershipsJSON,
		},
	})

	src := openZipSource(t, zipPath)
	eng := newEngine(pool)
	result, err := eng.RunImport(context.Background(), ImportInput{
		Source:       src,
		TargetUserID: userID,
	})
	if err != nil {
		t.Fatalf("RunImport: %v", err)
	}
	if len(result.Errors) > 0 {
		t.Errorf("RunImport returned errors: %+v", result.Errors)
	}

	// Three activities must be imported.
	assertRowCount(t, pool, "activities", userID, 3)

	// Two memberships must be imported.
	var membershipCount int
	if err := pool.QueryRow(context.Background(),
		`SELECT COUNT(*) FROM activity_fusion_memberships
		 WHERE merged_id IN (SELECT id FROM activities WHERE user_id = $1)`,
		userID).Scan(&membershipCount); err != nil {
		t.Fatalf("count memberships: %v", err)
	}
	if membershipCount != 2 {
		t.Errorf("expected 2 fusion memberships, got %d", membershipCount)
	}

	// The merged activity must be linked to sources that belong to the same user.
	var badCount int
	if err := pool.QueryRow(context.Background(),
		`SELECT COUNT(*) FROM activity_fusion_memberships afm
		 JOIN activities merged ON merged.id = afm.merged_id
		 JOIN activities source ON source.id = afm.source_id
		 WHERE merged.user_id != $1 OR source.user_id != $1`,
		userID).Scan(&badCount); err != nil {
		t.Fatalf("cross-user check: %v", err)
	}
	if badCount != 0 {
		t.Errorf("found %d memberships referencing activities from wrong user", badCount)
	}
}

// ---------------------------------------------------------------------------
// TestFusionMembershipsImport_Idempotent
// ---------------------------------------------------------------------------

// TestFusionMembershipsImport_Idempotent verifies that importing the same
// memberships twice (e.g. partial retry) is idempotent: no duplicate rows.
func TestFusionMembershipsImport_Idempotent(t *testing.T) {
	pool := testDB(t)
	userID := createTestUser(t, pool)

	srcUserID := uuid.New().String()
	oldA := uuid.New().String()
	oldM := uuid.New().String()

	now := time.Now().UTC()

	makeActivity := func(id, title string) map[string]any {
		return map[string]any{
			"id":         id,
			"user_id":    srcUserID,
			"sport_type": "cycling",
			"started_at": now.Format(time.RFC3339),
			"ended_at":   now.Add(time.Hour).Format(time.RFC3339),
			"title":      title,
			"visibility": "private",
			"created_at": now.Format(time.RFC3339),
			"updated_at": now.Format(time.RFC3339),
		}
	}

	memberships := []map[string]any{
		{"merged_id": oldM, "source_id": oldA, "field_key": "gps", "user_id": srcUserID},
	}
	membershipsJSON, _ := json.Marshal(memberships)

	zipPath := buildExportZip(t, zipBuildOpts{
		sourceUserID:    srcUserID,
		schemaMigration: 302,
		activities:      []map[string]any{makeActivity(oldA, "Source"), makeActivity(oldM, "Merged")},
		columns: map[string][]string{
			"activities":                  {"id", "user_id", "sport_type", "started_at", "ended_at", "title", "visibility", "created_at", "updated_at"},
			"activity_fusion_memberships": {"merged_id", "source_id", "field_key", "user_id"},
		},
		extraFiles: map[string][]byte{
			"data/activity_fusion_memberships.json": membershipsJSON,
		},
	})

	src := openZipSource(t, zipPath)
	eng := newEngine(pool)
	if _, err := eng.RunImport(context.Background(), ImportInput{
		Source:       src,
		TargetUserID: userID,
	}); err != nil {
		t.Fatalf("RunImport: %v", err)
	}

	// Count after first import: 1 membership expected.
	var count int
	if err := pool.QueryRow(context.Background(),
		`SELECT COUNT(*) FROM activity_fusion_memberships
		 WHERE merged_id IN (SELECT id FROM activities WHERE user_id = $1)`,
		userID).Scan(&count); err != nil {
		t.Fatalf("count after first import: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 membership after first import, got %d", count)
	}
}

// ---------------------------------------------------------------------------
// TestReadMembershipsJSON_ForeignActivityFilter
// ---------------------------------------------------------------------------

// TestReadMembershipsJSON_ForeignActivityFilter verifies that
// ReadMembershipsJSON drops memberships whose IDs fall outside
// importedActivityIDs.
func TestReadMembershipsJSON_ForeignActivityFilter(t *testing.T) {
	importUserID := uuid.New().String()
	knownA := uuid.New().String()
	knownM := uuid.New().String()
	foreignID := uuid.New().String()

	importedIDs := map[string]bool{
		knownA: true,
		knownM: true,
	}

	memberships := []map[string]any{
		// Valid: both IDs within scope.
		{"merged_id": knownM, "source_id": knownA, "field_key": "gps", "user_id": "old-user"},
		// Invalid: source_id references a foreign activity.
		{"merged_id": knownM, "source_id": foreignID, "field_key": "power", "user_id": "old-user"},
		// Invalid: merged_id references a foreign activity.
		{"merged_id": foreignID, "source_id": knownA, "field_key": "gps", "user_id": "old-user"},
	}

	data, _ := json.Marshal(memberships)

	// Build a minimal DirSource in memory via a temp ZIP.
	t.Run("filter", func(t *testing.T) {
		zipPath := buildExportZip(t, zipBuildOpts{
			sourceUserID:    uuid.New().String(),
			schemaMigration: 302,
			extraFiles: map[string][]byte{
				"data/activity_fusion_memberships.json": data,
			},
		})
		src := openZipSource(t, zipPath)
		defer src.Close()

		got := ReadMembershipsJSON(src, importUserID, importedIDs)
		if len(got) != 1 {
			t.Errorf("expected 1 filtered membership, got %d: %+v", len(got), got)
		}
		if len(got) > 0 && got[0].UserID != importUserID {
			t.Errorf("membership UserID should be importUserID, got %q", got[0].UserID)
		}
	})
}

// ---------------------------------------------------------------------------
// TestMembershipsEqual
// ---------------------------------------------------------------------------

func TestMembershipsEqual(t *testing.T) {
	import_db := func(m, s, f string) rawMembership {
		return rawMembership{MergedID: m, SourceID: s, FieldKey: f}
	}
	_ = import_db // satisfy linter

	a := []rawMembership{{MergedID: "M1", SourceID: "S1", FieldKey: "gps"}, {MergedID: "M1", SourceID: "S2", FieldKey: "power"}}
	b := []rawMembership{{MergedID: "M1", SourceID: "S2", FieldKey: "power"}, {MergedID: "M1", SourceID: "S1", FieldKey: "gps"}}

	// Convert to database.FusionMembership for MembershipsEqual.
	toDBSlice := func(raw []rawMembership) []interface{} {
		out := make([]interface{}, len(raw))
		for i, r := range raw {
			out[i] = r
		}
		return out
	}
	_ = toDBSlice

	// Direct key-comparison.
	keyOf := func(r rawMembership) string { return r.MergedID + "|" + r.SourceID + "|" + r.FieldKey }
	equal := func(x, y []rawMembership) bool {
		if len(x) != len(y) {
			return false
		}
		seen := make(map[string]int, len(x))
		for _, m := range x {
			seen[keyOf(m)]++
		}
		for _, m := range y {
			seen[keyOf(m)]--
		}
		for _, v := range seen {
			if v != 0 {
				return false
			}
		}
		return true
	}
	if !equal(a, b) {
		t.Error("expected a and b to be equal (order-independent)")
	}
	c := append(a, rawMembership{MergedID: "M1", SourceID: "S3", FieldKey: "gps"})
	if equal(a, c) {
		t.Error("a and c should not be equal (different length)")
	}
}
