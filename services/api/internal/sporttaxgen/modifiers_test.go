package sporttaxgen

import (
	"os"
	"strings"
	"testing"
)

func TestLoadModifiersAcceptsCanonicalRegistry(t *testing.T) {
	file, err := os.Open("../../../../config/sport-taxonomy/modifiers.yaml")
	if err != nil {
		t.Fatalf("open modifiers.yaml: %v", err)
	}
	defer file.Close()

	registry, err := LoadModifiers(file)
	if err != nil {
		t.Fatalf("LoadModifiers() error = %v", err)
	}

	wantOrder := []string{"is_electric", "is_indoor", "is_virtual", "is_commute"}
	if !stringSlicesEqual(registry.ModifierKeys(), wantOrder) {
		t.Fatalf("modifier order = %v, want %v", registry.ModifierKeys(), wantOrder)
	}

	for _, key := range wantOrder {
		modifier, ok := registry.ModifierByKey(key)
		if !ok {
			t.Fatalf("ModifierByKey(%q) missing", key)
		}
		if modifier.Labels["de"] == "" ||
			modifier.GoField == "" ||
			modifier.TSField == "" ||
			modifier.JSONKey == "" ||
			modifier.ActivityColumn == "" ||
			modifier.ApplicabilitySource == "" ||
			modifier.ActionName == "" ||
			modifier.ActionParam == "" ||
			modifier.ShadowFSToken == "" {
			t.Fatalf("modifier %q has missing required fields: %#v", key, modifier)
		}
		if want := "modifier_applicability." + key; modifier.ApplicabilitySource != want {
			t.Fatalf("modifier %q applicability_source = %q, want %q", key, modifier.ApplicabilitySource, want)
		}
		if !strings.HasPrefix(modifier.ActionName, "set_") {
			t.Fatalf("modifier %q action_name = %q, want set_*", key, modifier.ActionName)
		}
	}

	electric, _ := registry.ModifierByKey("is_electric")
	if electric.TagSync == nil || electric.TagSync.SystemTag != "E-Bike" || electric.TagSync.OnlySportType != "cycling" {
		t.Fatalf("is_electric tag_sync = %#v, want cycling-scoped E-Bike", electric.TagSync)
	}

	assertCapabilityKeys(t, registry, "source_alias", []string{"is_electric", "is_indoor", "is_virtual"})
	assertCapabilityKeys(t, registry, "gear_default", []string{"is_electric", "is_indoor", "is_virtual"})
	assertCapabilityKeys(t, registry, "record_grouping", []string{"is_electric", "is_indoor", "is_virtual"})
	assertCapabilityKeys(t, registry, "heatmap_filter", []string{"is_electric", "is_commute"})
}

func TestLoadModifiersRejectsDuplicateKeys(t *testing.T) {
	_, err := LoadModifiers(strings.NewReader(`
modifiers:
  - key: is_electric
    labels: {de: E-Antrieb}
    go_field: IsElectric
    ts_field: isElectric
    json_key: is_electric
    activity_column: is_electric
    applicability_source: modifier_applicability.is_electric
    action_name: set_electric
    action_param: electric
    shadowfs_token: electric
    capabilities: &all
      source_alias: true
      modifier_patch: true
      activity_state: true
      modifier_filter: true
      rule_condition: true
      rule_action: true
      route_filter: true
      heatmap_filter: true
      gear_default: true
      record_grouping: true
      shadowfs_label: true
      quick_edit: true
  - key: is_electric
    labels: {de: E-Antrieb 2}
    go_field: IsElectric
    ts_field: isElectric
    json_key: is_electric
    activity_column: is_electric
    applicability_source: modifier_applicability.is_electric
    action_name: set_electric_2
    action_param: electric_2
    shadowfs_token: electric2
    capabilities: *all
`))
	if err == nil {
		t.Fatal("LoadModifiers accepted duplicate key")
	}
}

func TestLoadModifiersRejectsMissingCapabilities(t *testing.T) {
	_, err := LoadModifiers(strings.NewReader(`
modifiers:
  - key: is_electric
    labels: {de: E-Antrieb}
    go_field: IsElectric
    ts_field: isElectric
    json_key: is_electric
    activity_column: is_electric
    applicability_source: modifier_applicability.is_electric
    action_name: set_electric
    action_param: electric
    shadowfs_token: electric
    capabilities:
      source_alias: true
`))
	if err == nil {
		t.Fatal("LoadModifiers accepted missing capability fields")
	}
}

func TestLoadModifiersRejectsInvalidTagSync(t *testing.T) {
	_, err := LoadModifiers(strings.NewReader(strings.Replace(validModifierRegistryFixture(), "only_sport_type: cycling", "only_sport_type: running", 1)))
	if err == nil {
		t.Fatal("LoadModifiers accepted unscoped is_electric tag_sync drift")
	}
}

func TestLoadModifiersRejectsUnknownCapabilityCombination(t *testing.T) {
	_, err := LoadModifiers(strings.NewReader(strings.Replace(validModifierRegistryFixture(), "rule_action: true", "rule_action: false", 1)))
	if err == nil {
		t.Fatal("LoadModifiers accepted rule_condition without rule_action")
	}
}

func assertCapabilityKeys(t *testing.T, registry ModifierRegistry, capability string, want []string) {
	t.Helper()
	if got := registry.ModifierKeysWithCapability(capability); !stringSlicesEqual(got, want) {
		t.Fatalf("%s keys = %v, want %v", capability, got, want)
	}
}

func validModifierRegistryFixture() string {
	data, err := os.ReadFile("../../../../config/sport-taxonomy/modifiers.yaml")
	if err != nil {
		panic(err)
	}
	return string(data)
}
