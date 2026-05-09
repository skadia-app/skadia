package database

import (
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/skadia-app/skadia/services/api/internal/sport"
)

func TestSystemModifierTagSyncsMatchGeneratedRegistry(t *testing.T) {
	want := map[string]systemModifierTagSync{}
	for _, key := range sport.SportModifierKeys() {
		modifier, ok := sport.SportModifierByKey(key)
		if !ok || modifier.TagSync == nil {
			continue
		}
		want[key] = systemModifierTagSync{
			ModifierKey:   key,
			SystemTagName: modifier.TagSync.SystemTag,
			SystemType:    modifier.ActionParam + "_sync",
			RuleName:      systemRuleNameForModifierSync(modifier),
			OnlySportType: modifier.TagSync.OnlySportType,
			Color:         modifier.TagSync.Color,
			Description:   modifier.TagSync.Description,
			ReadonlySync:  modifier.TagSync.ReadonlySync,
		}
	}

	got := map[string]systemModifierTagSync{}
	for _, sync := range systemModifierTagSyncs() {
		got[sync.ModifierKey] = sync
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("system modifier tag syncs = %#v, want %#v", got, want)
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

	for key, want := range registryWithTagSync {
		_, covered := systemModifierTagSyncDatabaseCoverage[key]
		_, excluded := systemModifierTagSyncDatabaseExclusions[key]
		if !covered && !excluded {
			t.Errorf("modifier key %q has tag_sync in the generated registry but is missing "+
				"from systemModifierTagSyncDatabaseCoverage; add an entry or list it in "+
				"systemModifierTagSyncDatabaseExclusions with a tracking plan", key)
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
	source, err := os.ReadFile("tags_db.go")
	if err != nil {
		t.Fatalf("read tags_db.go: %v", err)
	}
	text := string(source)
	for _, needle := range []string{
		"for _, sync := range systemModifierTagSyncs()",
		"for _, sync := range readonlySystemModifierTagSyncs()",
		"sync.SystemTagName",
		"sync.SystemType",
	} {
		if !strings.Contains(text, needle) {
			t.Fatalf("SeedUser must use generated modifier tag-sync metadata; missing %q", needle)
		}
	}
}
