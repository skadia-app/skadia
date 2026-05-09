package applicablemodifiers

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// LoadForSubsystem reads a modifiers.yaml file (the canonical sport-taxonomy
// modifiers registry) and returns the keys of modifiers that declare the given
// subsystem in their `subsystems:` audience array, in YAML order.
//
// LoadForSubsystem returns an error when subsystem is not declared by any
// modifier in modifiersPath. Empty-on-unknown was a drift hazard: a caller
// asking for "heatmaps" (typo of "heatmap") or "gear-defaults" (a stale name
// after a rename) would receive an empty slice and behave as if no modifier
// matched the subsystem. Failing loudly forces the caller to fix the name.
func LoadForSubsystem(modifiersPath, subsystem string) ([]string, error) {
	data, err := os.ReadFile(modifiersPath)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", modifiersPath, err)
	}
	var doc struct {
		Modifiers []struct {
			Key        string   `yaml:"key"`
			Subsystems []string `yaml:"subsystems"`
		} `yaml:"modifiers"`
	}
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("parse %s: %w", modifiersPath, err)
	}
	var (
		keys  []string
		known bool
	)
	for _, m := range doc.Modifiers {
		for _, s := range m.Subsystems {
			if s == subsystem {
				keys = append(keys, m.Key)
				known = true
				break
			}
		}
	}
	if !known {
		return nil, fmt.Errorf("subsystem %q not declared by any modifier in %s; "+
			"check for a typo or a stale subsystem name", subsystem, modifiersPath)
	}
	return keys, nil
}

// toJSONCompatible recursively converts yaml.v3 map types (map[string]any and
// map[any]any) to map[string]any so json.Marshal produces valid JSON.
func toJSONCompatible(value any) (any, error) {
	switch typed := value.(type) {
	case map[any]any:
		out := make(map[string]any, len(typed))
		for k, v := range typed {
			ks, ok := k.(string)
			if !ok {
				return nil, fmt.Errorf("non-string YAML key %#v", k)
			}
			converted, err := toJSONCompatible(v)
			if err != nil {
				return nil, err
			}
			out[ks] = converted
		}
		return out, nil
	case map[string]any:
		out := make(map[string]any, len(typed))
		for k, v := range typed {
			converted, err := toJSONCompatible(v)
			if err != nil {
				return nil, err
			}
			out[k] = converted
		}
		return out, nil
	case []any:
		out := make([]any, 0, len(typed))
		for _, v := range typed {
			converted, err := toJSONCompatible(v)
			if err != nil {
				return nil, err
			}
			out = append(out, converted)
		}
		return out, nil
	default:
		return value, nil
	}
}
