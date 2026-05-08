package database

import (
	"context"
	"reflect"
	"sort"
	"testing"

	"github.com/skadia-app/skadia/services/api/internal/database/testdb"
	"github.com/skadia-app/skadia/services/api/internal/sport"
	"github.com/skadia-app/skadia/services/api/internal/tags"
	"github.com/google/uuid"
)

func TestSystemModifierTagSyncsMatchGeneratedRegistry(t *testing.T) {
	// Build expected values from the tags package (single source of truth after Phase 8).
	want := map[string]systemModifierTagSync{}
	for _, s := range tags.SystemTagSyncs {
		want[s.ModifierKey] = systemModifierTagSync{
			ModifierKey:   s.ModifierKey,
			SystemTagName: s.SystemTag,
			SystemType:    systemTypeForModifierKey(s.ModifierKey),
			RuleName:      systemRuleNameForTagSync(s),
			OnlySportType: s.OnlySportType,
			Color:         s.Color,
			Description:   s.Description,
			ReadonlySync:  s.ReadonlySync,
		}
	}

	got := map[string]systemModifierTagSync{}
	for _, sync := range systemModifierTagSyncs() {
		got[sync.ModifierKey] = sync
	}

	for key, wantSync := range want {
		gotSync, ok := got[key]
		if !ok {
			t.Fatalf("missing modifier key %q in systemModifierTagSyncs()", key)
		}
		if gotSync != wantSync {
			t.Fatalf("systemModifierTagSync[%q] = %#v, want %#v", key, gotSync, wantSync)
		}
	}
	if len(got) != len(want) {
		t.Fatalf("got %d syncs, want %d", len(got), len(want))
	}

	for _, row := range []struct {
		modifierKey string
		tagName     string
	}{
		{modifierKey: "is_electric", tagName: "E-Bike"},
		{modifierKey: "is_indoor", tagName: "Indoor"},
		{modifierKey: "is_virtual", tagName: "virtuell"},
		{modifierKey: "is_commute", tagName: "Pendeln"},
	} {
		if got[row.modifierKey].SystemTagName != row.tagName {
			t.Fatalf("%s system tag = %q, want %q", row.modifierKey, got[row.modifierKey].SystemTagName, row.tagName)
		}
	}
}

func TestReadonlySystemModifierSyncRulesPreserveEBikeScope(t *testing.T) {
	got := map[string]systemModifierTagSync{}
	for _, sync := range readonlySystemModifierTagSyncs() {
		got[sync.ModifierKey] = sync
	}
	if _, ok := got["is_commute"]; ok {
		t.Fatal("is_commute is a synced system tag, not a read-only automation sync rule")
	}
	electric := got["is_electric"]
	if electric.SystemTagName != "E-Bike" || electric.RuleName != "E-Bike" || electric.SystemType != "electric_sync" || electric.OnlySportType != "cycling" {
		t.Fatalf("electric sync = %#v, want cycling-only E-Bike electric_sync", electric)
	}
	virtual := got["is_virtual"]
	if virtual.SystemTagName != "virtuell" || virtual.RuleName != "Virtuell" || virtual.SystemType != "virtual_sync" {
		t.Fatalf("virtual sync = %#v, want lower-case tag and German rule label", virtual)
	}
	if got := sport.SportModifierLabelDE("is_electric"); got == "E-Bike" {
		t.Fatal("generic is_electric label must not be E-Bike")
	}
}

// systemModifierTagSyncDatabaseCoverage names every registry-backed
// tag_sync modifier the database layer must cover. The Go-side parity test
// asserts that the systemModifierTagSyncs() helper exposes exactly these
// keys (modulo entries in systemModifierTagSyncDatabaseExclusions). A
// future fifth modifier with tag_sync that lacks Go coverage must fail
// this test loudly, naming its key.
var systemModifierTagSyncDatabaseCoverage = map[string]string{
	"is_indoor":   "Indoor",
	"is_commute":  "Pendeln",
	"is_virtual":  "virtuell",
	"is_electric": "E-Bike",
}

// systemModifierTagSyncDatabaseExclusions mirrors the SQL-side exclusions:
// a registry tag_sync modifier that intentionally has NO Go-side handling
// must list itself here with a tracking reason. Empty in the initial
// implementation.
var systemModifierTagSyncDatabaseExclusions = map[string]string{
	// "future_modifier": "reason and tracking plan",
}

func TestSystemModifierTagSyncDatabaseCoverageNamesMissingModifierKey(t *testing.T) {
	registryWithTagSync := map[string]string{}
	for _, key := range sport.SportModifierKeys() {
		modifier, ok := sport.SportModifierByKey(key)
		if !ok || modifier.TagSync == nil {
			continue
		}
		registryWithTagSync[key] = modifier.TagSync.SystemTag
	}

	var missingDBCoverage []string
	for key, want := range registryWithTagSync {
		_, covered := systemModifierTagSyncDatabaseCoverage[key]
		_, excluded := systemModifierTagSyncDatabaseExclusions[key]
		if !covered && !excluded {
			missingDBCoverage = append(missingDBCoverage, key)
			continue
		}
		if covered && excluded {
			t.Errorf("modifier key %q is both covered and excluded; pick one", key)
			continue
		}
		if covered && systemModifierTagSyncDatabaseCoverage[key] != want {
			t.Errorf("modifier %q system tag = %q, want %q (registry TagSync.SystemTag)",
				key, systemModifierTagSyncDatabaseCoverage[key], want)
		}
	}
	if len(missingDBCoverage) > 0 {
		sort.Strings(missingDBCoverage)
		t.Fatalf("modifier keys have tag_sync in the generated registry but are missing "+
			"from systemModifierTagSyncDatabaseCoverage: %v; add each entry or list in "+
			"systemModifierTagSyncDatabaseExclusions with a tracking plan", missingDBCoverage)
	}
	for key := range systemModifierTagSyncDatabaseCoverage {
		if _, ok := registryWithTagSync[key]; !ok {
			t.Errorf("systemModifierTagSyncDatabaseCoverage entry %q references a modifier that "+
				"no longer has tag_sync in the generated registry", key)
		}
	}
	// Cross-check the existing helper now resolves the same set of keys.
	exposed := map[string]bool{}
	for _, sync := range systemModifierTagSyncs() {
		exposed[sync.ModifierKey] = true
	}
	for key := range systemModifierTagSyncDatabaseCoverage {
		if !exposed[key] {
			t.Errorf("systemModifierTagSyncs() does not expose modifier %q; "+
				"the generated registry says it has tag_sync but the helper drops it", key)
		}
	}
}

func TestSeedUserUsesModifierTagSyncMetadata(t *testing.T) {
	// DB-backed: call SeedSystemDefaults for a fresh user and verify that the
	// four registry-backed modifier system tags (Indoor, Pendeln, virtuell,
	// E-Bike) exist with is_system=true. This replaces the former file-grep.
	pool := testdb.Open(t)
	ctx := context.Background()

	userID := uuid.NewString()
	if _, err := pool.Exec(ctx,
		`INSERT INTO users (id, username, email, email_verified, registration_source)
		 VALUES ($1, $2, $3, true, 'test')`,
		userID, "smtseed-"+userID[:8], "smtseed-"+userID[:8]+"@example.com"); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	t.Cleanup(func() {
		bg := context.Background()
		_, _ = pool.Exec(bg, `DELETE FROM activity_tag_removals WHERE activity_id IN (SELECT id FROM activities WHERE user_id = $1)`, userID)
		_, _ = pool.Exec(bg, `DELETE FROM activity_tags WHERE activity_id IN (SELECT id FROM activities WHERE user_id = $1)`, userID)
		_, _ = pool.Exec(bg, `DELETE FROM automation_rules WHERE user_id = $1`, userID)
		_, _ = pool.Exec(bg, `UPDATE tags SET is_system = false WHERE user_id = $1`, userID)
		_, _ = pool.Exec(bg, `DELETE FROM tags WHERE user_id = $1`, userID)
		_, _ = pool.Exec(bg, `DELETE FROM users WHERE id = $1`, userID)
	})

	db := &DB{Pool: pool}
	if err := db.SeedSystemDefaults(ctx, userID); err != nil {
		t.Fatalf("SeedSystemDefaults: %v", err)
	}

	// Query for the four reserved modifier system tags.
	rows, err := pool.Query(ctx,
		`SELECT name FROM tags WHERE user_id = $1 AND is_system = true AND name IN ('Indoor', 'Pendeln', 'virtuell', 'E-Bike')
		 ORDER BY name`,
		userID)
	if err != nil {
		t.Fatalf("query modifier system tags: %v", err)
	}
	defer rows.Close()

	var got []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatalf("scan: %v", err)
		}
		got = append(got, name)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows error: %v", err)
	}

	want := []string{"E-Bike", "Indoor", "Pendeln", "virtuell"}
	sort.Strings(got)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("modifier system tags after seed = %v, want %v", got, want)
	}
}
