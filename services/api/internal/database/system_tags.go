package database

import (
	"github.com/skadia-app/skadia/services/api/internal/sport"
	"github.com/skadia-app/skadia/services/api/internal/tags"
)

// systemModifierTagSync is the database-layer view of a system tag sync rule.
// The sync.OnceValue cache from the previous implementation is removed because
// tags.SystemTagSyncs is already a package-level static var.
type systemModifierTagSync struct {
	ModifierKey    string
	SystemTagName  string
	SystemType     string
	RuleName       string
	OnlySportType  string
	Color          string
	Description    string
	DisplayKey     string
	DescriptionKey string
	RuleLabelKey   string
	ReadonlySync   bool
}

func systemModifierTagSyncs() []systemModifierTagSync {
	result := make([]systemModifierTagSync, 0, len(tags.SystemTagSyncs))
	for _, s := range tags.SystemTagSyncs {
		result = append(result, systemModifierTagSync{
			ModifierKey:    s.ModifierKey,
			SystemTagName:  s.SystemTag,
			SystemType:     systemTypeForModifierKey(s.ModifierKey),
			RuleName:       systemRuleNameForTagSync(s),
			OnlySportType:  s.OnlySportType,
			Color:          s.Color,
			Description:    s.Description,
			DisplayKey:     systemTagDisplayKeyForModifier(s.ModifierKey),
			DescriptionKey: systemTagDescriptionKeyForModifier(s.ModifierKey),
			RuleLabelKey:   systemRuleLabelKeyForSystemType(systemTypeForModifierKey(s.ModifierKey)),
			ReadonlySync:   s.ReadonlySync,
		})
	}
	return result
}

// systemTypeForModifierKey derives the legacy SystemType string (e.g. "electric_sync")
// from the modifier's ActionParam, which is still sourced from the sport registry.
func systemTypeForModifierKey(modifierKey string) string {
	modifier, ok := sport.SportModifierByKey(modifierKey)
	if !ok {
		return modifierKey + "_sync"
	}
	return modifier.ActionParam + "_sync"
}

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

func systemTagDisplayKeyForModifier(modifierKey string) string {
	return "tags.system." + modifierKey + ".label"
}

func systemTagDescriptionKeyForModifier(modifierKey string) string {
	return "tags.system." + modifierKey + ".description"
}

func systemRuleLabelKeyForSystemType(systemType string) string {
	return "automation.system_rules." + systemType + ".label"
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
// literals — renaming a reserved tag requires updating
// config/tags/system_tags.yaml (which regenerates
// internal/tags/system_tags_generated.go), AND the SQL in the 000353 hardening
// migration, in the same atomic commit. The parity test
// TestSystemModifierReservedNameLiteralsMatchRegistry enforces this at CI time.
func IsSystemModifierTagName(tagName string) bool {
	_, ok := matchSystemModifierTagSync(tagName)
	return ok
}

// systemRuleNameForTagSync derives the automation-rule display name for a system tag.
// For sport-conditional syncs (only_sport_type set) the system tag name is used directly
// (e.g. "E-Bike" for cycling-only electric sync). For general syncs, the German modifier
// label is preferred.
func systemRuleNameForTagSync(s tags.SystemTagSync) string {
	if s.OnlySportType != "" {
		return s.SystemTag
	}
	if label := sport.SportModifierLabelDE(s.ModifierKey); label != "" {
		return label
	}
	return s.SystemTag
}
