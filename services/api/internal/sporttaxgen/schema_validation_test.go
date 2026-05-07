package sporttaxgen

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"gopkg.in/yaml.v2"
)

func TestSportTaxonomyRepositoryFixturesValidateAgainstSchemas(t *testing.T) {
	cases := []struct {
		name       string
		fixture    string
		schemaPath string
	}{
		{
			name:       "activity filters",
			fixture:    "config/sport-taxonomy/projections/activity_filters.yaml",
			schemaPath: "config/sport-taxonomy/schema/projections.schema.json",
		},
		{
			name:       "segments",
			fixture:    "config/sport-taxonomy/projections/segments.yaml",
			schemaPath: "config/sport-taxonomy/schema/projections.schema.json",
		},
		{
			name:       "records",
			fixture:    "config/sport-taxonomy/projections/records.yaml",
			schemaPath: "config/sport-taxonomy/schema/projections.schema.json",
		},
		{
			name:       "rules",
			fixture:    "config/sport-taxonomy/projections/rules.yaml",
			schemaPath: "config/sport-taxonomy/schema/projections.schema.json",
		},
		{
			name:       "modifiers",
			fixture:    "config/sport-taxonomy/modifiers.yaml",
			schemaPath: "config/sport-taxonomy/schema/modifiers.schema.json",
		},
		{
			name:       "sports",
			fixture:    "config/sport-taxonomy/sports.yaml",
			schemaPath: "config/sport-taxonomy/schema/taxonomy.schema.json",
		},
	}
	aliasFiles, err := filepath.Glob(repoPathForSchemaTest(t, "config/sport-taxonomy/aliases/*.yaml"))
	if err != nil {
		t.Fatalf("glob aliases: %v", err)
	}
	if len(aliasFiles) == 0 {
		t.Fatal("alias schema validation found no alias fixtures")
	}
	for _, aliasFile := range aliasFiles {
		rel, err := filepath.Rel(repoRootForSchemaTest(t), aliasFile)
		if err != nil {
			t.Fatalf("rel alias path: %v", err)
		}
		cases = append(cases, struct {
			name       string
			fixture    string
			schemaPath string
		}{
			name:       "alias " + strings.TrimSuffix(filepath.Base(aliasFile), filepath.Ext(aliasFile)),
			fixture:    rel,
			schemaPath: "config/sport-taxonomy/schema/aliases.schema.json",
		})
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			schema := compileJSONSchemaForTest(t, tc.schemaPath)
			instance := loadYAMLInstanceForSchemaTest(t, tc.fixture)
			if err := schema.Validate(instance); err != nil {
				t.Fatalf("%s does not validate against %s: %v", tc.fixture, tc.schemaPath, err)
			}
		})
	}
}

func TestProjectionSchemaRejectsLocalRuleModifiers(t *testing.T) {
	schema := compileJSONSchemaForTest(t, "config/sport-taxonomy/schema/projections.schema.json")
	instance := yamlStringInstanceForSchemaTest(t, validRuleProjectionYAMLForTest+`
modifiers:
  - key: is_indoor
    labels: {de: Indoor}
    condition_applicable_from: modifier_applicability.is_indoor
    action_applicable_from: modifier_applicability.is_indoor
    render_existing: true
`)
	if err := schema.Validate(instance); err == nil {
		t.Fatal("projection schema accepted local rules modifiers, but LoadRuleProjection derives them from modifiers.yaml")
	}
}

func TestProjectionSchemaRejectsLoaderRejectedFields(t *testing.T) {
	schema := compileJSONSchemaForTest(t, "config/sport-taxonomy/schema/projections.schema.json")
	for _, tc := range []struct {
		name string
		src  string
	}{
		{
			name: "activity preset extra field",
			src:  strings.Replace(validProjectionFixture(), "labels: {de: Aktivitätsfilter}", "labels: {de: Aktivitätsfilter}\n    extra: nope", 1),
		},
		{
			name: "activity entry label_ref",
			src:  strings.Replace(validProjectionFixture(), "category_key: run", "category_key: run\n            label_ref: category:run", 1),
		},
		{
			name: "segment eligibility extra field",
			src:  strings.Replace(validSegmentProjectionFixture(), "sport_group: foot", "sport_group: foot\n    extra: nope", 1),
		},
		{
			name: "rule trait extra field",
			src:  strings.Replace(validRuleProjectionYAMLForTest, "source: traits.environments", "source: traits.environments\n    extra: nope", 1),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := schema.Validate(yamlStringInstanceForSchemaTest(t, tc.src)); err == nil {
				t.Fatalf("projection schema accepted %s", tc.name)
			}
		})
	}
}

func TestProjectionSchemaStrictNodesAndRequiredFields(t *testing.T) {
	doc := loadJSONSchemaDocumentForTest(t, "config/sport-taxonomy/schema/projections.schema.json")

	for _, path := range [][]string{
		{"$defs", "activityFilterProjection"},
		{"$defs", "activityFilterProjection", "properties", "presets", "items"},
		{"$defs", "activityFilterProjection", "properties", "presets", "items", "properties", "groups", "items"},
		{"$defs", "activityEntry"},
		{"$defs", "segmentProjection"},
		{"$defs", "segmentProjection", "properties", "domains", "items"},
		{"$defs", "segmentProjection", "properties", "segment_types", "items"},
		{"$defs", "segmentProjection", "properties", "route_options", "items"},
		{"$defs", "segmentProjection", "properties", "eligible", "items"},
		{"$defs", "ruleProjection"},
		{"$defs", "ruleProjection", "properties", "tuple_selectors", "items"},
		{"$defs", "ruleProjection", "properties", "trait_conditions", "items"},
		{"$defs", "recordsProjection"},
		{"$defs", "recordsProjection", "properties", "category_sets", "items"},
		{"$defs", "recordsProjection", "properties", "category_profiles", "items"},
		{"$defs", "recordsProjection", "properties", "record_groups", "items"},
		{"$defs", "recordsProjection", "properties", "main_categories", "items"},
		{"$defs", "recordCategory"},
		{"$defs", "recordMatchRule"},
		{"$defs", "recordGroupCollection"},
	} {
		node := schemaMapAtPathForTest(t, doc, path...)
		if got, ok := node["additionalProperties"].(bool); !ok || got {
			t.Fatalf("schema node %s additionalProperties = %#v, want false", strings.Join(path, "."), node["additionalProperties"])
		}
	}

	for _, tc := range []struct {
		path string
		want []string
	}{
		{path: "$defs.activityFilterProjection", want: []string{"presets"}},
		{path: "$defs.segmentProjection", want: []string{"domains", "segment_types", "eligible"}},
		{path: "$defs.ruleProjection", want: []string{"tuple_selectors", "trait_conditions"}},
		{path: "$defs.recordsProjection", want: []string{"category_sets", "category_profiles", "record_groups", "main_categories"}},
	} {
		node := schemaMapAtPathForTest(t, doc, strings.Split(tc.path, ".")...)
		got := schemaStringSliceForTest(t, node["required"])
		for _, want := range tc.want {
			if !slices.Contains(got, want) {
				t.Fatalf("%s required = %v, missing %q", tc.path, got, want)
			}
		}
	}
}

func compileJSONSchemaForTest(t *testing.T, schemaPath string) *jsonschema.Schema {
	t.Helper()
	doc := loadJSONSchemaDocumentForTest(t, schemaPath)
	compiler := jsonschema.NewCompiler()
	if err := compiler.AddResource(schemaPath, doc); err != nil {
		t.Fatalf("add schema resource %s: %v", schemaPath, err)
	}
	schema, err := compiler.Compile(schemaPath)
	if err != nil {
		t.Fatalf("compile schema %s: %v", schemaPath, err)
	}
	return schema
}

func loadJSONSchemaDocumentForTest(t *testing.T, schemaPath string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(repoPathForSchemaTest(t, schemaPath))
	if err != nil {
		t.Fatalf("read schema %s: %v", schemaPath, err)
	}
	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatalf("parse schema %s: %v", schemaPath, err)
	}
	return doc
}

func loadYAMLInstanceForSchemaTest(t *testing.T, fixturePath string) any {
	t.Helper()
	data, err := os.ReadFile(repoPathForSchemaTest(t, fixturePath))
	if err != nil {
		t.Fatalf("read fixture %s: %v", fixturePath, err)
	}
	return yamlBytesInstanceForSchemaTest(t, data)
}

func yamlStringInstanceForSchemaTest(t *testing.T, src string) any {
	t.Helper()
	return yamlBytesInstanceForSchemaTest(t, []byte(src))
}

func yamlBytesInstanceForSchemaTest(t *testing.T, src []byte) any {
	t.Helper()
	var decoded any
	if err := yaml.Unmarshal(src, &decoded); err != nil {
		t.Fatalf("parse yaml instance: %v", err)
	}
	compatible, err := jsonCompatibleYAMLValueForTest(decoded)
	if err != nil {
		t.Fatalf("convert yaml instance: %v", err)
	}
	buf, err := json.Marshal(compatible)
	if err != nil {
		t.Fatalf("marshal yaml instance to json: %v", err)
	}
	instance, err := jsonschema.UnmarshalJSON(bytes.NewReader(buf))
	if err != nil {
		t.Fatalf("unmarshal json-compatible instance: %v", err)
	}
	return instance
}

func jsonCompatibleYAMLValueForTest(value any) (any, error) {
	switch typed := value.(type) {
	case map[any]any:
		out := make(map[string]any, len(typed))
		for key, nested := range typed {
			keyString, ok := key.(string)
			if !ok {
				return nil, fmt.Errorf("non-string yaml key %#v", key)
			}
			converted, err := jsonCompatibleYAMLValueForTest(nested)
			if err != nil {
				return nil, err
			}
			out[keyString] = converted
		}
		return out, nil
	case map[string]any:
		out := make(map[string]any, len(typed))
		for key, nested := range typed {
			converted, err := jsonCompatibleYAMLValueForTest(nested)
			if err != nil {
				return nil, err
			}
			out[key] = converted
		}
		return out, nil
	case []any:
		out := make([]any, 0, len(typed))
		for _, nested := range typed {
			converted, err := jsonCompatibleYAMLValueForTest(nested)
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

func schemaMapAtPathForTest(t *testing.T, doc map[string]any, path ...string) map[string]any {
	t.Helper()
	var current any = doc
	for _, part := range path {
		asMap, ok := current.(map[string]any)
		if !ok {
			t.Fatalf("schema path %s parent is %T", strings.Join(path, "."), current)
		}
		current = asMap[part]
	}
	asMap, ok := current.(map[string]any)
	if !ok {
		t.Fatalf("schema path %s is %T", strings.Join(path, "."), current)
	}
	return asMap
}

func schemaStringSliceForTest(t *testing.T, value any) []string {
	t.Helper()
	raw, ok := value.([]any)
	if !ok {
		t.Fatalf("schema string slice value is %T", value)
	}
	out := make([]string, 0, len(raw))
	for _, item := range raw {
		text, ok := item.(string)
		if !ok {
			t.Fatalf("schema string slice item is %T", item)
		}
		out = append(out, text)
	}
	return out
}

func repoRootForSchemaTest(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", "..", "..", ".."))
	if err != nil {
		t.Fatalf("resolve repo root: %v", err)
	}
	return root
}

func repoPathForSchemaTest(t *testing.T, rel string) string {
	t.Helper()
	return filepath.Join(repoRootForSchemaTest(t), rel)
}
