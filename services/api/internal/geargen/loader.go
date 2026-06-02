package geargen

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

var compiledSchema *jsonschema.Schema

func init() {
	var schemaDoc any
	if err := json.Unmarshal(schemaBytes, &schemaDoc); err != nil {
		panic(fmt.Sprintf("geargen: parse embedded gear schema JSON schema: %v", err))
	}
	const schemaURL = "geargen/schema.json"
	compiler := jsonschema.NewCompiler()
	if err := compiler.AddResource(schemaURL, schemaDoc); err != nil {
		panic(fmt.Sprintf("geargen: add gear schema resource: %v", err))
	}
	schema, err := compiler.Compile(schemaURL)
	if err != nil {
		panic(fmt.Sprintf("geargen: compile gear schema: %v", err))
	}
	compiledSchema = schema
}

// AssignmentRole describes a role a gear item can play relative to an activity.
type AssignmentRole struct {
	Key               string    `yaml:"key"`
	LabelDE           string    `yaml:"label_de"`
	Labels            LocaleMap `yaml:"labels"`
	UniquePerActivity bool      `yaml:"unique_per_activity"`
}

type LocaleMap map[string]string

// OptionValue is a single selectable value within an OptionSet.
type OptionValue struct {
	Key     string    `yaml:"key"`
	LabelDE string    `yaml:"label_de"`
	Labels  LocaleMap `yaml:"labels"`
}

// OptionSet is a named list of enumerable option values referenced by attributes.
type OptionSet struct {
	Key     string        `yaml:"key"`
	Options []OptionValue `yaml:"options"`
}

// UnitGroup is a named set of allowed units for a physical quantity.
// Attributes referencing this group via `unit_group:` may only declare
// `allowed_units` that are a subset of AllowedUnits here.
type UnitGroup struct {
	Key           string    `yaml:"key"`
	LabelDE       string    `yaml:"label_de"`
	Labels        LocaleMap `yaml:"labels"`
	AllowedUnits  []string  `yaml:"allowed_units"`
	CanonicalUnit string    `yaml:"canonical_unit,omitempty"`
}

// ShowWhenSpec describes the condition under which a field is visible and validated.
// The field named by Field must equal Value for this attribute to be active.
type ShowWhenSpec struct {
	Field string `yaml:"field"`
	Value string `yaml:"value"`
}

// Condition is a single soft-UX visibility predicate. Multiple conditions in
// an array are AND-combined. Op selects the comparison:
//
//	eq, neq      compare Ref's value against Value (scalar)
//	in, not_in   compare Ref's value against Values (array)
//	is_set       Ref has a non-null/non-empty value (Value/Values unused)
//	is_unset     Ref is absent or empty (Value/Values unused)
//
// Ref is a dotted key. Bare keys reference siblings in the same scope.
// "gear.<attr>" and "component.<attr>" are reserved namespaces for
// cross-item references resolved at UI render time.
type Condition struct {
	Ref    string `yaml:"ref"`
	Op     string `yaml:"op"`
	Value  any    `yaml:"value,omitempty"`
	Values []any  `yaml:"values,omitempty"`
}

// Attribute describes a typed metadata field for a gear type.
// Subfields enables recursive composition for complex value types (value_type=object).
//
// VisibleWhen lists same-scope visibility conditions (AND-combined). It is
// a UX hint — the backend does not gate writes on it.
// AppliesToGearWhen lists cross-item visibility conditions used by
// component-attribute UIs that read the parent gear's attributes. Same
// AND-combination, same soft semantics.
//
// KombiGroup tags two attributes that should render side-by-side in the
// editor (e.g. shock eye-to-eye + stroke). Within a single scope
// (gear_type.attribute_keys or component_type.attribute_keys) the group
// must contain exactly two members.
type Attribute struct {
	Key               string        `yaml:"key"`
	LabelDE           string        `yaml:"label_de"`
	Labels            LocaleMap     `yaml:"labels"`
	ValueType         string        `yaml:"value_type"`
	OptionSetKey      string        `yaml:"option_set_key,omitempty"`
	Subfields         []Attribute   `yaml:"subfields,omitempty"`
	UnitGroup         string        `yaml:"unit_group,omitempty"`
	AllowedUnits      []string      `yaml:"allowed_units,omitempty"`
	Required          bool          `yaml:"required,omitempty"`
	ShowWhen          *ShowWhenSpec `yaml:"show_when,omitempty"`
	VisibleWhen       []Condition   `yaml:"visible_when,omitempty"`
	AppliesToGearWhen []Condition   `yaml:"applies_to_gear_when,omitempty"`
	KombiGroup        string        `yaml:"kombi_group,omitempty"`
}

// Predicate restricts a gear type to specific sport / sub-type combinations.
type Predicate struct {
	SportType string `yaml:"sport_type"`
	SubType   string `yaml:"sub_type,omitempty"`
}

// GearTypeSlot is a pre-defined component slot on a gear type.
type GearTypeSlot struct {
	Key           string    `yaml:"key"`
	LabelDE       string    `yaml:"label_de"`
	Labels        LocaleMap `yaml:"labels"`
	ComponentType string    `yaml:"component_type"`
	Optional      bool      `yaml:"optional,omitempty"`
}

// RecommendedAttribute lists discrete soft-default values for one attribute
// on a gear type. Values are UX hints — typical "common" choices — and never
// reject existing items that fall outside them.
type RecommendedAttribute struct {
	Key    string `yaml:"key"`
	Values []any  `yaml:"values,omitempty"`
}

// RecommendedConstraints is a per-gear_type bundle of soft UX defaults. All
// fields are optional. The backend ignores these — they exist purely to let
// the frontend highlight typical choices and warn ("Ungewöhnlich für X")
// on outliers without ever rejecting a write.
type RecommendedConstraints struct {
	Attributes []RecommendedAttribute `yaml:"attributes,omitempty"`
}

// GearType is a top-level equipment category with optional hierarchy, sport scope,
// and modifier audience.
type GearType struct {
	Key                      string                  `yaml:"key"`
	LabelDE                  string                  `yaml:"label_de"`
	Labels                   LocaleMap               `yaml:"labels"`
	Scope                    string                  `yaml:"scope"`
	ParentKey                string                  `yaml:"parent_key,omitempty"`
	DefaultAssignmentRoleKey string                  `yaml:"default_assignment_role_key"`
	AttributeKeys            []string                `yaml:"attribute_keys,omitempty"`
	ModifierKeys             []string                `yaml:"modifier_keys,omitempty"`
	ApplicableSports         []Predicate             `yaml:"applicable_sports,omitempty"`
	Slots                    []GearTypeSlot          `yaml:"slots,omitempty"`
	Recommended              *RecommendedConstraints `yaml:"recommended,omitempty"`
}

// ComponentType describes a reusable component kind (e.g. bike fork, shock)
// with optional subtypes and associated attribute keys.
type ComponentType struct {
	Key           string    `yaml:"key"`
	LabelDE       string    `yaml:"label_de"`
	Labels        LocaleMap `yaml:"labels"`
	ParentKey     string    `yaml:"parent_key,omitempty"`
	SubtypeKeys   []string  `yaml:"subtype_keys,omitempty"`
	AttributeKeys []string  `yaml:"attribute_keys,omitempty"`
}

// Schema is the parsed, validated representation of a gear-schema.yaml file.
type Schema struct {
	Version         int              `yaml:"version"`
	AssignmentRoles []AssignmentRole `yaml:"assignment_roles"`
	OptionSets      []OptionSet      `yaml:"option_sets"`
	UnitGroups      []UnitGroup      `yaml:"unit_groups,omitempty"`
	Attributes      []Attribute      `yaml:"attributes"`
	GearTypes       []GearType       `yaml:"gear_types"`
	ComponentTypes  []ComponentType  `yaml:"component_types,omitempty"`
}

// Load reads and validates path against the embedded JSONSchema, then unmarshals
// the result into a Schema.
func Load(path string) (*Schema, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	if err := validateAgainstSchema(path, data); err != nil {
		return nil, err
	}
	var s Schema
	if err := yaml.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if err := checkSubfieldDepth(s.Attributes); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return &s, nil
}

func validateAgainstSchema(path string, data []byte) error {
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

// toJSONCompatible recursively converts yaml.v3 map types to map[string]any so
// json.Marshal produces valid JSON.
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
