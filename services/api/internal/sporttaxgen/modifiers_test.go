package sporttaxgen

import (
	"encoding/json"
	"os"
	"sort"
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
	assertCapabilityKeys(t, registry, "taxonomy_default", []string{"is_indoor"})
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
	      taxonomy_default: false
  - key: is_electric
    labels: {de: E-Antrieb 2}
    go_field: IsElectric
    ts_field: isElectric
    json_key: is_electric
    activity_column: is_electric
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

func TestLoadModifiersRejectsScopedTagSyncWithoutLabelContext(t *testing.T) {
	_, err := LoadModifiers(strings.NewReader(strings.Replace(validModifierRegistryFixture(), "      label_context: cycling\n", "", 1)))
	if err == nil {
		t.Fatal("LoadModifiers accepted scoped tag_sync without label_context")
	}
}

func TestLoadModifiersRejectsInvalidTagSyncColor(t *testing.T) {
	_, err := LoadModifiers(strings.NewReader(strings.Replace(validModifierRegistryFixture(), `      color: "#16A34A"`, `      color: orange`, 1)))
	if err == nil {
		t.Fatal("LoadModifiers accepted invalid tag_sync.color")
	}
}

func TestLoadModifiersRejectsEmptyTagSyncDescription(t *testing.T) {
	_, err := LoadModifiers(strings.NewReader(strings.Replace(validModifierRegistryFixture(), "      description: Automatisch bei elektrischen Radfahrten\n", "      description: \"\"\n", 1)))
	if err == nil {
		t.Fatal("LoadModifiers accepted empty tag_sync.description")
	}
}

func TestLoadModifiersRejectsMissingTagSyncReadonlySync(t *testing.T) {
	_, err := LoadModifiers(strings.NewReader(strings.Replace(validModifierRegistryFixture(), "      readonly_sync: true\n", "", 1)))
	if err == nil {
		t.Fatal("LoadModifiers accepted missing tag_sync.readonly_sync")
	}
}

func TestLoadModifiersRejectsUnknownCapabilityCombination(t *testing.T) {
	_, err := LoadModifiers(strings.NewReader(strings.Replace(validModifierRegistryFixture(), "rule_action: true", "rule_action: false", 1)))
	if err == nil {
		t.Fatal("LoadModifiers accepted rule_condition without rule_action")
	}
}

func TestModifiersSchemaListsSameCapabilitiesAsLoader(t *testing.T) {
	data, err := os.ReadFile("../../../../config/sport-taxonomy/schema/modifiers.schema.json")
	if err != nil {
		t.Fatalf("read modifiers schema: %v", err)
	}
	var schema struct {
		Defs map[string]struct {
			Required   []string       `json:"required"`
			Properties map[string]any `json:"properties"`
		} `json:"$defs"`
	}
	if err := json.Unmarshal(data, &schema); err != nil {
		t.Fatalf("parse modifiers schema: %v", err)
	}
	capabilities, ok := schema.Defs["capabilities"]
	if !ok {
		t.Fatal("schema missing $defs.capabilities")
	}
	required := append([]string(nil), capabilities.Required...)
	properties := make([]string, 0, len(capabilities.Properties))
	for key := range capabilities.Properties {
		properties = append(properties, key)
	}
	sort.Strings(required)
	sort.Strings(properties)
	want := modifierCapabilityNames()
	sort.Strings(want)
	if !stringSlicesEqual(required, want) {
		t.Fatalf("schema required capabilities = %v, want %v", required, want)
	}
	if !stringSlicesEqual(properties, want) {
		t.Fatalf("schema capability properties = %v, want %v", properties, want)
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
