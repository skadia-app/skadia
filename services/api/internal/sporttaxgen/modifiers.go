package sporttaxgen

import (
	"fmt"
	"io"
	"regexp"
	"strings"

	"gopkg.in/yaml.v2"
)

type ModifierRegistry struct {
	Modifiers []ModifierDefinition `yaml:"modifiers"`
}

type ModifierDefinition struct {
	Key                 string               `yaml:"key"`
	Labels              LocaleMap            `yaml:"labels"`
	GoField             string               `yaml:"go_field"`
	TSField             string               `yaml:"ts_field"`
	JSONKey             string               `yaml:"json_key"`
	ActivityColumn      string               `yaml:"activity_column"`
	ApplicabilitySource string               `yaml:"applicability_source"`
	ActionName          string               `yaml:"action_name"`
	ActionParam         string               `yaml:"action_param"`
	ShadowFSToken       string               `yaml:"shadowfs_token"`
	Capabilities        ModifierCapabilities `yaml:"capabilities"`
	TagSync             *ModifierTagSync     `yaml:"tag_sync"`
}

type ModifierCapabilities struct {
	SourceAlias    *bool `yaml:"source_alias"`
	ModifierPatch  *bool `yaml:"modifier_patch"`
	ActivityState  *bool `yaml:"activity_state"`
	ModifierFilter *bool `yaml:"modifier_filter"`
	RuleCondition  *bool `yaml:"rule_condition"`
	RuleAction     *bool `yaml:"rule_action"`
	RouteFilter    *bool `yaml:"route_filter"`
	HeatmapFilter  *bool `yaml:"heatmap_filter"`
	GearDefault    *bool `yaml:"gear_default"`
	RecordGrouping *bool `yaml:"record_grouping"`
	ShadowFSLabel  *bool `yaml:"shadowfs_label"`
	QuickEdit      *bool `yaml:"quick_edit"`
}

type ModifierTagSync struct {
	SystemTag     string `yaml:"system_tag"`
	OnlySportType string `yaml:"only_sport_type"`
	LabelContext  string `yaml:"label_context"`
}

var shadowFSTokenPattern = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

func modifierCapabilityNames() []string {
	return []string{
		"source_alias",
		"modifier_patch",
		"activity_state",
		"modifier_filter",
		"rule_condition",
		"rule_action",
		"route_filter",
		"heatmap_filter",
		"gear_default",
		"record_grouping",
		"shadowfs_label",
		"quick_edit",
	}
}

func LoadModifiers(r io.Reader) (ModifierRegistry, error) {
	var registry ModifierRegistry
	dec := yaml.NewDecoder(r)
	dec.SetStrict(true)
	if err := dec.Decode(&registry); err != nil {
		return ModifierRegistry{}, err
	}
	if err := validateModifierRegistry(registry); err != nil {
		return ModifierRegistry{}, err
	}
	return registry, nil
}

func (r ModifierRegistry) ModifierKeys() []string {
	keys := make([]string, 0, len(r.Modifiers))
	for _, modifier := range r.Modifiers {
		keys = append(keys, modifier.Key)
	}
	return keys
}

func (r ModifierRegistry) ModifierByKey(key string) (ModifierDefinition, bool) {
	for _, modifier := range r.Modifiers {
		if modifier.Key == key {
			return modifier, true
		}
	}
	return ModifierDefinition{}, false
}

func (r ModifierRegistry) ModifierKeysWithCapability(capability string) []string {
	keys := make([]string, 0, len(r.Modifiers))
	for _, modifier := range r.Modifiers {
		if modifier.Capabilities.Enabled(capability) {
			keys = append(keys, modifier.Key)
		}
	}
	return keys
}

func (r ModifierRegistry) ValidateModifierKeyForCapability(key, capability, surface string) error {
	modifier, ok := r.ModifierByKey(key)
	if !ok {
		return fmt.Errorf("%s: unknown modifier key %q", surface, key)
	}
	if !modifier.Capabilities.Enabled(capability) {
		return fmt.Errorf("%s: modifier %q does not support capability %q", surface, key, capability)
	}
	return nil
}

func (c ModifierCapabilities) Enabled(capability string) bool {
	switch capability {
	case "source_alias":
		return boolValue(c.SourceAlias)
	case "modifier_patch":
		return boolValue(c.ModifierPatch)
	case "activity_state":
		return boolValue(c.ActivityState)
	case "modifier_filter":
		return boolValue(c.ModifierFilter)
	case "rule_condition":
		return boolValue(c.RuleCondition)
	case "rule_action":
		return boolValue(c.RuleAction)
	case "route_filter":
		return boolValue(c.RouteFilter)
	case "heatmap_filter":
		return boolValue(c.HeatmapFilter)
	case "gear_default":
		return boolValue(c.GearDefault)
	case "record_grouping":
		return boolValue(c.RecordGrouping)
	case "shadowfs_label":
		return boolValue(c.ShadowFSLabel)
	case "quick_edit":
		return boolValue(c.QuickEdit)
	default:
		return false
	}
}

func validateModifierRegistry(registry ModifierRegistry) error {
	if len(registry.Modifiers) == 0 {
		return fmt.Errorf("missing modifiers")
	}
	keys := map[string]struct{}{}
	actionNames := map[string]string{}
	actionParams := map[string]string{}
	shadowTokens := map[string]string{}
	for i, modifier := range registry.Modifiers {
		context := fmt.Sprintf("modifiers[%d]", i)
		if modifier.Key == "" {
			return fmt.Errorf("%s: missing key", context)
		}
		if _, ok := keys[modifier.Key]; ok {
			return fmt.Errorf("duplicate modifier key %q", modifier.Key)
		}
		keys[modifier.Key] = struct{}{}
		if modifier.Labels["de"] == "" {
			return fmt.Errorf("modifier %q: missing labels.de", modifier.Key)
		}
		if modifier.GoField == "" {
			return fmt.Errorf("modifier %q: missing go_field", modifier.Key)
		}
		if modifier.TSField == "" {
			return fmt.Errorf("modifier %q: missing ts_field", modifier.Key)
		}
		if modifier.JSONKey == "" {
			return fmt.Errorf("modifier %q: missing json_key", modifier.Key)
		}
		if modifier.ActivityColumn == "" {
			return fmt.Errorf("modifier %q: missing activity_column", modifier.Key)
		}
		if modifier.ApplicabilitySource == "" {
			return fmt.Errorf("modifier %q: missing applicability_source", modifier.Key)
		}
		if want := "modifier_applicability." + modifier.Key; modifier.ApplicabilitySource != want {
			return fmt.Errorf("modifier %q: applicability_source = %q, want %q", modifier.Key, modifier.ApplicabilitySource, want)
		}
		if modifier.ActionName == "" {
			return fmt.Errorf("modifier %q: missing action_name", modifier.Key)
		}
		if !strings.HasPrefix(modifier.ActionName, "set_") {
			return fmt.Errorf("modifier %q: action_name must start with set_", modifier.Key)
		}
		if previous, ok := actionNames[modifier.ActionName]; ok {
			return fmt.Errorf("modifier %q: duplicate action_name %q already used by %q", modifier.Key, modifier.ActionName, previous)
		}
		actionNames[modifier.ActionName] = modifier.Key
		if modifier.ActionParam == "" {
			return fmt.Errorf("modifier %q: missing action_param", modifier.Key)
		}
		if previous, ok := actionParams[modifier.ActionParam]; ok {
			return fmt.Errorf("modifier %q: duplicate action_param %q already used by %q", modifier.Key, modifier.ActionParam, previous)
		}
		actionParams[modifier.ActionParam] = modifier.Key
		if !shadowFSTokenPattern.MatchString(modifier.ShadowFSToken) {
			return fmt.Errorf("modifier %q: invalid shadowfs_token %q", modifier.Key, modifier.ShadowFSToken)
		}
		if previous, ok := shadowTokens[modifier.ShadowFSToken]; ok {
			return fmt.Errorf("modifier %q: duplicate shadowfs_token %q already used by %q", modifier.Key, modifier.ShadowFSToken, previous)
		}
		shadowTokens[modifier.ShadowFSToken] = modifier.Key
		if err := validateModifierCapabilities(modifier); err != nil {
			return err
		}
		if err := validateModifierTagSync(modifier); err != nil {
			return err
		}
	}
	return nil
}

func validateModifierCapabilities(modifier ModifierDefinition) error {
	for _, capability := range modifierCapabilityNames() {
		if modifier.Capabilities.pointer(capability) == nil {
			return fmt.Errorf("modifier %q: missing capabilities.%s", modifier.Key, capability)
		}
	}
	if boolValue(modifier.Capabilities.RuleCondition) != boolValue(modifier.Capabilities.RuleAction) {
		return fmt.Errorf("modifier %q: rule_condition and rule_action must match", modifier.Key)
	}
	if boolValue(modifier.Capabilities.QuickEdit) && !boolValue(modifier.Capabilities.ModifierPatch) {
		return fmt.Errorf("modifier %q: quick_edit requires modifier_patch", modifier.Key)
	}
	if boolValue(modifier.Capabilities.GearDefault) && !boolValue(modifier.Capabilities.ModifierPatch) {
		return fmt.Errorf("modifier %q: gear_default requires modifier_patch", modifier.Key)
	}
	if boolValue(modifier.Capabilities.RecordGrouping) && !boolValue(modifier.Capabilities.ActivityState) {
		return fmt.Errorf("modifier %q: record_grouping requires activity_state", modifier.Key)
	}
	if boolValue(modifier.Capabilities.HeatmapFilter) && !boolValue(modifier.Capabilities.ModifierFilter) {
		return fmt.Errorf("modifier %q: heatmap_filter requires modifier_filter", modifier.Key)
	}
	return nil
}

func (c ModifierCapabilities) pointer(capability string) *bool {
	switch capability {
	case "source_alias":
		return c.SourceAlias
	case "modifier_patch":
		return c.ModifierPatch
	case "activity_state":
		return c.ActivityState
	case "modifier_filter":
		return c.ModifierFilter
	case "rule_condition":
		return c.RuleCondition
	case "rule_action":
		return c.RuleAction
	case "route_filter":
		return c.RouteFilter
	case "heatmap_filter":
		return c.HeatmapFilter
	case "gear_default":
		return c.GearDefault
	case "record_grouping":
		return c.RecordGrouping
	case "shadowfs_label":
		return c.ShadowFSLabel
	case "quick_edit":
		return c.QuickEdit
	default:
		return nil
	}
}

func validateModifierTagSync(modifier ModifierDefinition) error {
	if modifier.TagSync == nil {
		return nil
	}
	if modifier.TagSync.SystemTag == "" {
		return fmt.Errorf("modifier %q: tag_sync.system_tag is required", modifier.Key)
	}
	if modifier.Key == "is_electric" {
		if modifier.TagSync.SystemTag != "E-Bike" || modifier.TagSync.OnlySportType != "cycling" || modifier.TagSync.LabelContext != "cycling" {
			return fmt.Errorf("modifier %q: tag_sync must keep cycling-scoped E-Bike semantics", modifier.Key)
		}
	}
	if modifier.TagSync.OnlySportType != "" && modifier.TagSync.LabelContext == "" {
		return fmt.Errorf("modifier %q: tag_sync.only_sport_type requires label_context", modifier.Key)
	}
	return nil
}

func boolValue(value *bool) bool {
	return value != nil && *value
}
