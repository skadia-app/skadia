package database

import (
	"sync"

	"github.com/skadia-app/skadia/services/api/internal/sport"
)

type systemModifierTagSync struct {
	ModifierKey   string
	SystemTagName string
	SystemType    string
	RuleName      string
	OnlySportType string
	Color         string
	Description   string
	ReadonlySync  bool
}

func systemModifierTagSyncs() []systemModifierTagSync {
	return append([]systemModifierTagSync(nil), cachedSystemModifierTagSyncs()...)
}

var cachedSystemModifierTagSyncs = sync.OnceValue(func() []systemModifierTagSync {
	syncs := []systemModifierTagSync{}
	for _, key := range sport.SportModifierKeys() {
		modifier, ok := sport.SportModifierByKey(key)
		if !ok || modifier.TagSync == nil {
			continue
		}
		syncs = append(syncs, systemModifierTagSync{
			ModifierKey:   key,
			SystemTagName: modifier.TagSync.SystemTag,
			SystemType:    modifier.ActionParam + "_sync",
			RuleName:      systemRuleNameForModifierSync(modifier),
			OnlySportType: modifier.TagSync.OnlySportType,
			Color:         modifier.TagSync.Color,
			Description:   modifier.TagSync.Description,
			ReadonlySync:  modifier.TagSync.ReadonlySync,
		})
	}
	return syncs
})

func readonlySystemModifierTagSyncs() []systemModifierTagSync {
	syncs := []systemModifierTagSync{}
	for _, sync := range systemModifierTagSyncs() {
		if !sync.ReadonlySync {
			continue
		}
		syncs = append(syncs, sync)
	}
	return syncs
}

func systemTagNameForModifier(modifierKey string) string {
	for _, sync := range systemModifierTagSyncs() {
		if sync.ModifierKey == modifierKey {
			return sync.SystemTagName
		}
	}
	return ""
}

// IsSystemModifierTagName reports whether tagName is the reserved system tag
// name of a registry-backed tag_sync modifier (e.g. "Indoor", "Pendeln",
// "virtuell", "E-Bike"). Used by HTTP handlers to decide whether removal
// records should be written by the handler or are reconciled by the resync
// function inside the helper transaction.
//
// Name-matching contract (Critic Round-2 #11): the match is exact-string and
// case-sensitive. "indoor" and "Indoor" are different names; unicode lookalikes
// and whitespace variants are not detected. The SQL trigger bodies and the
// skadia_resync_system_modifier_tags function hard-code the same string
// literals — renaming a reserved tag requires updating the sport/modifiers.yaml
// registry entry (which regenerates modifiers_generated.go), AND the SQL in
// the 000353 hardening migration, in the same atomic commit. The parity test
// TestSystemModifierReservedNameLiteralsMatchRegistry enforces this at CI time.
func IsSystemModifierTagName(tagName string) bool {
	_, ok := matchSystemModifierTagSync(tagName)
	return ok
}

func systemRuleNameForModifierSync(modifier sport.SportModifierDef) string {
	if modifier.TagSync != nil && modifier.TagSync.OnlySportType != "" {
		return modifier.TagSync.SystemTag
	}
	if label := sport.SportModifierLabelDE(modifier.Key); label != "" {
		return label
	}
	if modifier.TagSync == nil {
		return modifier.Key
	}
	return modifier.TagSync.SystemTag
}
