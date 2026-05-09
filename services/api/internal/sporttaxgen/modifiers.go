package sporttaxgen

import (
	"fmt"
	"io"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
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
	ApplicabilitySource string               `yaml:"-"`
	ActionName          string               `yaml:"action_name"`
	ActionParam         string               `yaml:"action_param"`
	ShadowFSToken       string               `yaml:"shadowfs_token"`
	Capabilities        ModifierCapabilities `yaml:"capabilities"`
	// Subsystems lists the consumer subsystems that include this modifier in their applicable set.
	// Values are the --subsystem flag values used by applicablemodifiersgen.
	Subsystems []string `yaml:"subsystems"`
}

// ModifierCapabilities holds the four sport-internal capabilities.
// Subsystem-specific capabilities (gear_default, route_filter, rule_condition/action,
// heatmap_filter, record_grouping, shadowfs_label, source_alias, quick_edit) are
// now owned by per-subsystem applicable_modifiers_generated.go files.
type ModifierCapabilities struct {
	ActivityState   *bool `yaml:"activity_state"`
	ModifierFilter  *bool `yaml:"modifier_filter"`
	ModifierPatch   *bool `yaml:"modifier_patch"`
	TaxonomyDefault *bool `yaml:"taxonomy_default"`
}

var modifierIdentifierPattern = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)
var shadowFSTokenPattern = modifierIdentifierPattern

func modifierCapabilityNames() []string {
	return []string{
		"activity_state",
		"modifier_filter",
		"modifier_patch",
		"taxonomy_default",
	}
}

func LoadModifiers(r io.Reader) (ModifierRegistry, error) {
	var registry ModifierRegistry
	dec := yaml.NewDecoder(r)
	dec.KnownFields(true)
	if err := dec.Decode(&registry); err != nil {
		return ModifierRegistry{}, err
	}
	for i := range registry.Modifiers {
		registry.Modifiers[i].ApplicabilitySource = "modifier_applicability." + registry.Modifiers[i].Key
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

// HasCapability reports whether the modifier with the given key supports the named capability.
// Returns false if the key is unknown.
func (r ModifierRegistry) HasCapability(key, capability string) bool {
	modifier, ok := r.ModifierByKey(key)
	if !ok {
		return false
	}
	return modifier.Capabilities.Enabled(capability)
}

// ValidateModifierKeyForCapability checks that key is a known modifier and that isApplicable(key) is true.
// isApplicable encodes the capability predicate for the target subsystem.
func (r ModifierRegistry) ValidateModifierKeyForCapability(key string, isApplicable func(string) bool, surface string) error {
	if _, ok := r.ModifierByKey(key); !ok {
		return fmt.Errorf("%s: unknown modifier key %q", surface, key)
	}
	if !isApplicable(key) {
		return fmt.Errorf("%s: modifier %q is not applicable", surface, key)
	}
	return nil
}

func (c ModifierCapabilities) Enabled(capability string) bool {
	switch capability {
	case "activity_state":
		return boolValue(c.ActivityState)
	case "modifier_filter":
		return boolValue(c.ModifierFilter)
	case "modifier_patch":
		return boolValue(c.ModifierPatch)
	case "taxonomy_default":
		return boolValue(c.TaxonomyDefault)
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
		if !modifierIdentifierPattern.MatchString(modifier.Key) {
			return fmt.Errorf("%s: invalid key %q", context, modifier.Key)
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
		if !modifierIdentifierPattern.MatchString(modifier.JSONKey) {
			return fmt.Errorf("modifier %q: invalid json_key %q", modifier.Key, modifier.JSONKey)
		}
		if modifier.ActivityColumn == "" {
			return fmt.Errorf("modifier %q: missing activity_column", modifier.Key)
		}
		if !modifierIdentifierPattern.MatchString(modifier.ActivityColumn) {
			return fmt.Errorf("modifier %q: invalid activity_column %q", modifier.Key, modifier.ActivityColumn)
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
	}
	return nil
}

func validateModifierCapabilities(modifier ModifierDefinition) error {
	for _, capability := range modifierCapabilityNames() {
		if modifier.Capabilities.pointer(capability) == nil {
			return fmt.Errorf("modifier %q: missing capabilities.%s", modifier.Key, capability)
		}
	}
	if boolValue(modifier.Capabilities.TaxonomyDefault) && !boolValue(modifier.Capabilities.ModifierPatch) {
		return fmt.Errorf("modifier %q: taxonomy_default requires modifier_patch", modifier.Key)
	}
	return nil
}

func (c ModifierCapabilities) pointer(capability string) *bool {
	switch capability {
	case "activity_state":
		return c.ActivityState
	case "modifier_filter":
		return c.ModifierFilter
	case "modifier_patch":
		return c.ModifierPatch
	case "taxonomy_default":
		return c.TaxonomyDefault
	default:
		return nil
	}
}

func boolValue(value *bool) bool {
	return value != nil && *value
}
