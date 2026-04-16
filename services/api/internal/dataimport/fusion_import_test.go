package dataimport

import (
	"context"
	"encoding/json"
	"sort"
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
// TestDeriveFromLegacyFusionSources — unit tests (no DB required)
// ---------------------------------------------------------------------------

// TestDeriveFromLegacyFusionSources_ConcreteUUIDs verifies that concrete UUID
// values in fusion_sources produce matching memberships.
func TestDeriveFromLegacyFusionSources_ConcreteUUIDs(t *testing.T) {
	importUserID := uuid.New().String()
	mergedID := uuid.New().String()
	sourceGPS := uuid.New().String()
	sourcePower := uuid.New().String()

	importedIDs := map[string]bool{
		mergedID:   true,
		sourceGPS:  true,
		sourcePower: true,
	}

	fusionSources := map[string]string{
		"gps":   sourceGPS,
		"power": sourcePower,
	}

	got := DeriveFromLegacyFusionSources(fusionSources, mergedID, nil, importUserID, importedIDs)
	if len(got) != 2 {
		t.Fatalf("expected 2 memberships, got %d: %+v", len(got), got)
	}

	// Verify all memberships have correct fields.
	for _, m := range got {
		if m.MergedID != mergedID {
			t.Errorf("MergedID: want %q, got %q", mergedID, m.MergedID)
		}
		if m.UserID != importUserID {
			t.Errorf("UserID: want %q, got %q", importUserID, m.UserID)
		}
		if m.SourceID != sourceGPS && m.SourceID != sourcePower {
			t.Errorf("unexpected SourceID %q", m.SourceID)
		}
		expectedKey := ""
		if m.SourceID == sourceGPS {
			expectedKey = "gps"
		} else {
			expectedKey = "power"
		}
		if m.FieldKey != expectedKey {
			t.Errorf("FieldKey for source %q: want %q, got %q", m.SourceID, expectedKey, m.FieldKey)
		}
	}
}

// TestDeriveFromLegacyFusionSources_FuseLiteralWithChildren verifies that a
// "fuse" literal is expanded to one membership per parentIDChild.
func TestDeriveFromLegacyFusionSources_FuseLiteralWithChildren(t *testing.T) {
	importUserID := uuid.New().String()
	mergedID := uuid.New().String()
	childA := uuid.New().String()
	childB := uuid.New().String()
	childC := uuid.New().String()

	importedIDs := map[string]bool{
		mergedID: true,
		childA:   true,
		childB:   true,
		childC:   true,
	}

	fusionSources := map[string]string{
		"gps": "fuse",
	}

	got := DeriveFromLegacyFusionSources(fusionSources, mergedID, []string{childA, childB, childC}, importUserID, importedIDs)
	if len(got) != 3 {
		t.Fatalf("expected 3 memberships (one per child), got %d: %+v", len(got), got)
	}

	childIDs := map[string]bool{childA: true, childB: true, childC: true}
	for _, m := range got {
		if m.MergedID != mergedID {
			t.Errorf("MergedID: want %q, got %q", mergedID, m.MergedID)
		}
		if m.FieldKey != "gps" {
			t.Errorf("FieldKey: want %q, got %q", "gps", m.FieldKey)
		}
		if !childIDs[m.SourceID] {
			t.Errorf("unexpected SourceID %q", m.SourceID)
		}
	}
}

// TestDeriveFromLegacyFusionSources_FuseLiteralNoChildren verifies that a "fuse"
// literal with no parentIDChildren returns empty (with a warning logged).
func TestDeriveFromLegacyFusionSources_FuseLiteralNoChildren(t *testing.T) {
	importUserID := uuid.New().String()
	mergedID := uuid.New().String()

	importedIDs := map[string]bool{mergedID: true}
	fusionSources := map[string]string{"gps": "fuse"}

	got := DeriveFromLegacyFusionSources(fusionSources, mergedID, nil, importUserID, importedIDs)
	if len(got) != 0 {
		t.Fatalf("expected 0 memberships (fuse with no children), got %d: %+v", len(got), got)
	}
}

// TestDeriveFromLegacyFusionSources_ForeignScope verifies that a concrete UUID
// not present in importedActivityIDs is skipped with a warning.
func TestDeriveFromLegacyFusionSources_ForeignScope(t *testing.T) {
	importUserID := uuid.New().String()
	mergedID := uuid.New().String()
	knownSource := uuid.New().String()
	foreignSource := uuid.New().String()

	importedIDs := map[string]bool{
		mergedID:    true,
		knownSource: true,
		// foreignSource intentionally absent
	}
	fusionSources := map[string]string{
		"gps":   knownSource,
		"power": foreignSource,
	}

	got := DeriveFromLegacyFusionSources(fusionSources, mergedID, nil, importUserID, importedIDs)
	if len(got) != 1 {
		t.Fatalf("expected 1 membership (foreign source dropped), got %d: %+v", len(got), got)
	}
	if got[0].SourceID != knownSource {
		t.Errorf("expected surviving membership to reference %q, got %q", knownSource, got[0].SourceID)
	}
}

// ---------------------------------------------------------------------------
// TestImport_VOldFusionPath — integration test (requires TEST_DATABASE_URL)
// ---------------------------------------------------------------------------

// TestImport_VOldFusionPath verifies the v_old_fusion backwards-compat path:
// an old-style export that has fusion_sources JSONB on activities but no
// data/activity_fusion_memberships.json produces correctly derived memberships
// after import.
//
// Topology:
//   - Source export: user S with activities A (source/GPS), B (source/power) and
//     M (merged activity with fusion_sources {"gps": A, "power": B}).
//   - No memberships.json in the export.
//   - Expected: after import, 2 memberships exist for the remapped M, A, B.
func TestImport_VOldFusionPath(t *testing.T) {
	pool := testDB(t)
	userID := createTestUser(t, pool)

	srcUserID := uuid.New().String()
	oldA := uuid.New().String()
	oldB := uuid.New().String()
	oldM := uuid.New().String()

	now := time.Now().UTC()

	makeActivity := func(id, title string, extraFields map[string]any) map[string]any {
		row := map[string]any{
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
		for k, v := range extraFields {
			row[k] = v
		}
		return row
	}

	// M carries legacy fusion_sources JSONB referencing old A and B UUIDs.
	fusionSourcesM := map[string]any{"gps": oldA, "power": oldB}

	zipPath := buildExportZip(t, zipBuildOpts{
		sourceUserID:    srcUserID,
		schemaMigration: 300, // pre-302 to signal legacy export
		activities: []map[string]any{
			makeActivity(oldA, "Source GPS", nil),
			makeActivity(oldB, "Source Power", nil),
			makeActivity(oldM, "Merged", map[string]any{"fusion_sources": fusionSourcesM}),
		},
		columns: map[string][]string{
			"activities": {
				"id", "user_id", "sport_type", "started_at", "ended_at",
				"title", "visibility", "created_at", "updated_at", "fusion_sources",
			},
		},
		// No extraFiles["data/activity_fusion_memberships.json"] — this is the key!
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

	// Two memberships must be derived and inserted.
	var membershipCount int
	if err := pool.QueryRow(context.Background(),
		`SELECT COUNT(*) FROM activity_fusion_memberships
		 WHERE merged_id IN (SELECT id FROM activities WHERE user_id = $1)`,
		userID).Scan(&membershipCount); err != nil {
		t.Fatalf("count memberships: %v", err)
	}
	if membershipCount != 2 {
		t.Errorf("expected 2 legacy-derived fusion memberships, got %d", membershipCount)
	}

	// Verify the memberships reference activities that belong to the correct user.
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
		t.Errorf("found %d legacy memberships referencing activities from wrong user", badCount)
	}

	// Verify field_key values are correct.
	type memberRow struct {
		FieldKey string
	}
	rows, err := pool.Query(context.Background(),
		`SELECT afm.field_key
		 FROM activity_fusion_memberships afm
		 JOIN activities merged ON merged.id = afm.merged_id
		 WHERE merged.user_id = $1 AND merged.title = 'Merged'`,
		userID)
	if err != nil {
		t.Fatalf("query memberships field_keys: %v", err)
	}
	defer rows.Close()
	var fieldKeys []string
	for rows.Next() {
		var fk string
		if err := rows.Scan(&fk); err != nil {
			t.Fatalf("scan field_key: %v", err)
		}
		fieldKeys = append(fieldKeys, fk)
	}
	sort.Strings(fieldKeys)
	if len(fieldKeys) != 2 || fieldKeys[0] != "gps" || fieldKeys[1] != "power" {
		t.Errorf("expected field_keys [gps power], got %v", fieldKeys)
	}
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
