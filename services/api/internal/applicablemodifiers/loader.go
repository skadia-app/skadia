package applicablemodifiers

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"os"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"gopkg.in/yaml.v3"
)

//go:embed schema.json
var schemaBytes []byte

// compiledSchema is the compiled JSONSchema for applicable_modifiers YAML files.
// It is initialised once at package load time.
var compiledSchema *jsonschema.Schema

func init() {
	var schemaDoc any
	if err := json.Unmarshal(schemaBytes, &schemaDoc); err != nil {
		panic(fmt.Sprintf("applicablemodifiers: parse embedded schema: %v", err))
	}
	const schemaURL = "applicablemodifiers/schema.json"
	compiler := jsonschema.NewCompiler()
	if err := compiler.AddResource(schemaURL, schemaDoc); err != nil {
		panic(fmt.Sprintf("applicablemodifiers: add schema resource: %v", err))
	}
	schema, err := compiler.Compile(schemaURL)
	if err != nil {
		panic(fmt.Sprintf("applicablemodifiers: compile schema: %v", err))
	}
	compiledSchema = schema
}

type document struct {
	ApplicableModifiers []string `yaml:"applicable_modifiers"`
}

// Load reads a subsystem applicable_modifiers YAML file, validates it against the
// embedded JSONSchema, and returns the list of modifier keys.
func Load(path string) ([]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	if err := validateYAML(path, data); err != nil {
		return nil, err
	}
	var doc document
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return doc.ApplicableModifiers, nil
}

// validateYAML converts the YAML to a JSON-compatible value and validates it
// against the compiled schema. The round-trip through JSON is required because
// the jsonschema/v6 library validates against JSON-typed values.
func validateYAML(path string, data []byte) error {
	var decoded any
	if err := yaml.Unmarshal(data, &decoded); err != nil {
		return fmt.Errorf("parse %s: %w", path, err)
	}
	compatible, err := toJSONCompatible(decoded)
	if err != nil {
		return fmt.Errorf("convert %s for schema validation: %w", path, err)
	}
	buf, err := json.Marshal(compatible)
	if err != nil {
		return fmt.Errorf("marshal %s for schema validation: %w", path, err)
	}
	instance, err := jsonschema.UnmarshalJSON(bytes.NewReader(buf))
	if err != nil {
		return fmt.Errorf("unmarshal %s for schema validation: %w", path, err)
	}
	if err := compiledSchema.Validate(instance); err != nil {
		return fmt.Errorf("validate %s: %w", path, err)
	}
	return nil
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
