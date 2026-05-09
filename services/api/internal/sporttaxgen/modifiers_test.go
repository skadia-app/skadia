package sporttaxgen

import (
	"encoding/json"
	"os"
	"reflect"
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

	assertCapabilityKeys(t, registry, "modifier_filter", []string{"is_electric", "is_indoor", "is_virtual", "is_commute"})
	assertCapabilityKeys(t, registry, "activity_state", []string{"is_electric", "is_indoor", "is_virtual", "is_commute"})
	assertCapabilityKeys(t, registry, "modifier_patch", []string{"is_electric", "is_indoor", "is_virtual", "is_commute"})
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
      activity_state: true
      modifier_filter: true
      modifier_patch: true
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
      activity_state: true
`))
	if err == nil {
		t.Fatal("LoadModifiers accepted missing capability fields")
	}
}

func TestLoadModifiersRejectsTaxonomyDefaultWithoutModifierPatch(t *testing.T) {
	_, err := LoadModifiers(strings.NewReader(`
modifiers:
  - key: is_indoor
    labels: {de: Indoor}
    go_field: IsIndoor
    ts_field: isIndoor
    json_key: is_indoor
    activity_column: is_indoor
    action_name: set_indoor
    action_param: indoor
    shadowfs_token: indoor
    capabilities:
      activity_state: true
      modifier_filter: true
      modifier_patch: false
      taxonomy_default: true
`))
	if err == nil {
		t.Fatal("LoadModifiers accepted taxonomy_default without modifier_patch")
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

func TestModifierCapabilitiesAreSportInternalOnly(t *testing.T) {
	file, err := os.Open("../../../../config/sport-taxonomy/modifiers.yaml")
	if err != nil {
		t.Fatalf("open modifiers.yaml: %v", err)
	}
	defer file.Close()

	registry, err := LoadModifiers(file)
	if err != nil {
		t.Fatalf("LoadModifiers() error = %v", err)
	}

	var buf strings.Builder
	rendered, err := renderModifierRegistryForTest(registry)
	if err != nil {
		t.Fatalf("render modifiers: %v", err)
	}
	buf.Write(rendered)
	out := buf.String()

	decoupledCapabilities := []string{
		"gear_default", "route_filter", "rule_condition", "rule_action",
		"shadowfs_label", "record_grouping", "heatmap_filter",
		"source_alias", "quick_edit", "tag_sync", "TagSync",
	}
	for _, cap := range decoupledCapabilities {
		if strings.Contains(out, `"`+cap+`"`) || strings.Contains(out, cap+":") {
			t.Errorf("rendered output still contains decoupled capability %q", cap)
		}
	}

	sportInternalCapabilities := []string{
		"activity_state", "modifier_filter", "modifier_patch", "taxonomy_default",
	}
	for _, cap := range sportInternalCapabilities {
		if !strings.Contains(out, cap) {
			t.Errorf("rendered output missing sport-internal capability %q", cap)
		}
	}
}

func assertCapabilityKeys(t *testing.T, registry ModifierRegistry, capability string, want []string) {
	t.Helper()
	if got := registry.ModifierKeysWithCapability(capability); !stringSlicesEqual(got, want) {
		t.Fatalf("%s keys = %v, want %v", capability, got, want)
	}
}

func renderModifierRegistryForTest(registry ModifierRegistry) ([]byte, error) {
	return RenderGoModifiers(registry)
}

func TestLoadModifiersRejectsUnknownSubsystem(t *testing.T) {
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
      activity_state: true
      modifier_filter: true
      modifier_patch: true
      taxonomy_default: false
    subsystems: [unknown_subsystem]
`))
	if err == nil {
		t.Fatal("LoadModifiers accepted unknown subsystem")
	}
	if !strings.Contains(err.Error(), "unknown subsystem") {
		t.Fatalf("error should mention unknown subsystem, got: %v", err)
	}
}

func TestModifierHasSubsystemsArray(t *testing.T) {
	file, err := os.Open("../../../../config/sport-taxonomy/modifiers.yaml")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()

	registry, err := LoadModifiers(file)
	if err != nil {
		t.Fatal(err)
	}

	// subsystem names match the --subsystem flag values used by the tool scripts
	expectedBySubsystem := map[string][]string{
		"gear":        {"is_electric", "is_indoor", "is_virtual"},
		"rules":       {"is_electric", "is_indoor", "is_virtual", "is_commute"},
		"dataimport":  {"is_electric", "is_indoor", "is_virtual"},
		"shadowfs":    {"is_electric", "is_indoor", "is_virtual", "is_commute"},
		"routefilter": {"is_electric", "is_indoor", "is_virtual", "is_commute"},
		"bestefforts": {"is_electric", "is_indoor", "is_virtual"},
		"heatmap":     {"is_electric", "is_commute"},
		"quick-edit":  {"is_electric", "is_indoor", "is_virtual"},
	}
	gotBySubsystem := map[string][]string{}
	for _, m := range registry.Modifiers {
		for _, s := range m.Subsystems {
			gotBySubsystem[s] = append(gotBySubsystem[s], m.Key)
		}
	}
	if !reflect.DeepEqual(gotBySubsystem, expectedBySubsystem) {
		t.Errorf("got %v, want %v", gotBySubsystem, expectedBySubsystem)
	}
}

func validModifierRegistryFixture() string {
	data, err := os.ReadFile("../../../../config/sport-taxonomy/modifiers.yaml")
	if err != nil {
		panic(err)
	}
	return string(data)
}
