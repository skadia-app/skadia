package applicablemodifiers_test

import (
	"testing"

	"github.com/skadia-app/skadia/services/api/internal/applicablemodifiers"
)

func TestAllSubsystemReferencedModifierKeysExistInRegistry(t *testing.T) {
	subsystems := []struct {
		Name string
		Path string
	}{
		{"gear", "../../../../config/gear/applicable_modifiers.yaml"},
		{"rules", "../../../../config/rules/applicable_modifiers.yaml"},
		{"dataimport", "../../../../config/dataimport/applicable_modifiers.yaml"},
		{"shadowfs", "../../../../config/shadowfs/applicable_modifiers.yaml"},
		{"routing", "../../../../config/routing/applicable_modifiers.yaml"},
		{"records", "../../../../config/records/applicable_modifiers.yaml"},
		{"heatmap", "../../../../config/heatmap/applicable_modifiers.yaml"},
		{"frontend", "../../../../config/frontend/applicable_modifiers.yaml"},
	}
	for _, ss := range subsystems {
		t.Run(ss.Name, func(t *testing.T) {
			keys, err := applicablemodifiers.Load(ss.Path)
			if err != nil {
				t.Fatalf("load: %v", err)
			}
			if err := applicablemodifiers.ValidateKeys(keys); err != nil {
				t.Fatalf("subsystem %s yaml references unknown modifier key: %v", ss.Name, err)
			}
		})
	}
}
