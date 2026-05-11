package database

import (
	"sort"
	"testing"

	"github.com/skadia-app/skadia/services/api/internal/sport"
	"github.com/skadia-app/skadia/services/api/internal/tags"
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
		sync, ok := tags.SystemTagSyncByModifierKey(key)
		if !ok {
			continue
		}
		registryWithTagSync[key] = sync.SystemTag
	}

	// Iterate in sorted key order so failure output is deterministic
	// across runs (map iteration order is randomised in Go).
	registryKeys := make([]string, 0, len(registryWithTagSync))
	for k := range registryWithTagSync {
		registryKeys = append(registryKeys, k)
	}
	sort.Strings(registryKeys)

	var missingDBCoverage []string
	for _, key := range registryKeys {
		want := registryWithTagSync[key]
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
			t.Errorf("modifier %q system tag = %q, want %q (tags.SystemTagSync.SystemTag)",
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

// TestSystemTagSyncRegistryHasNoOrphanModifierKeys ensures every entry in
// tags.SystemTagSyncs references a modifier key that exists in the sport
// modifier registry. The tagsgen codegen rejects orphan keys at generation
// time (services/api/cmd/tagsgen/main.go: cross-reference against
// sport.SportModifierExists), but a hand-edit of system_tags_generated.go
// would otherwise slip past every other parity test in this file because
// the existing checks iterate sport.SportModifierKeys() and skip keys for
// which tags.SystemTagSyncByModifierKey returns false. This test runs the
// reverse direction so an orphan in the generated registry fails loudly
// regardless of how it got there.
func TestSystemTagSyncRegistryHasNoOrphanModifierKeys(t *testing.T) {
	var orphans []string
	for _, s := range tags.SystemTagSyncs {
		if !sport.SportModifierExists(s.ModifierKey) {
			orphans = append(orphans, s.ModifierKey)
		}
	}
	if len(orphans) > 0 {
		sort.Strings(orphans)
		t.Fatalf("tags.SystemTagSyncs references modifier keys not present in sport.SportModifierKeys(): %v; "+
			"either remove the entry from config/tags/system_tags.yaml (and regenerate via "+
			"scripts/generate-tags.sh) or add the modifier to config/sport-taxonomy/modifiers.yaml", orphans)
	}
}
