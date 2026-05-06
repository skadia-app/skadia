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
		{modifierKey: "is_electric", tagName: systemTagNameEBike},
		{modifierKey: "is_indoor", tagName: systemTagNameIndoor},
		{modifierKey: "is_virtual", tagName: systemTagNameVirtual},
		{modifierKey: "is_commute", tagName: systemTagNameCommute},
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
