package database

import "github.com/skadia-app/skadia/services/api/internal/sport"

const (
	systemTagNameEBike   = "E-Bike"
	systemTagNameIndoor  = "Indoor"
	systemTagNameVirtual = "virtuell"
	systemTagNameCommute = "Pendeln"
)

type systemModifierTagSync struct {
	ModifierKey   string
	SystemTagName string
	SystemType    string
	RuleName      string
	OnlySportType string
}

func systemModifierTagSyncs() []systemModifierTagSync {
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
		})
	}
	return syncs
}

func readonlySystemModifierTagSyncs() []systemModifierTagSync {
	syncs := []systemModifierTagSync{}
	for _, sync := range systemModifierTagSyncs() {
		if sync.ModifierKey == "is_commute" {
			continue
		}
		syncs = append(syncs, sync)
	}
	return syncs
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

func systemModifierTagStyle(modifierKey string) (color string, description string) {
	switch modifierKey {
	case "is_electric":
		return "#16A34A", "Automatisch bei elektrischen Radfahrten"
	case "is_indoor":
		return "#3B82F6", "Automatisch bei Indoor-Aktivitäten (Sync mit is_indoor)"
	case "is_virtual":
		return "#8B5CF6", "Automatisch bei virtuellen Aktivitäten (Sync mit is_virtual)"
	case "is_commute":
		return "#6B7280", "Automatisch bei Pendel-Aktivitäten (Sync mit is_commute)"
	default:
		return "#6B7280", "Automatisch synchronisiert"
	}
}
