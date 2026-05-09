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

//go:embed subsystem_registry_schema.json
var subsystemRegistrySchemaBytes []byte

var compiledSubsystemRegistrySchema *jsonschema.Schema

func init() {
	var schemaDoc any
	if err := json.Unmarshal(subsystemRegistrySchemaBytes, &schemaDoc); err != nil {
		panic(fmt.Sprintf("applicablemodifiers: parse embedded subsystem registry schema: %v", err))
	}
	const schemaURL = "applicablemodifiers/subsystem_registry_schema.json"
	compiler := jsonschema.NewCompiler()
	if err := compiler.AddResource(schemaURL, schemaDoc); err != nil {
		panic(fmt.Sprintf("applicablemodifiers: add subsystem registry schema resource: %v", err))
	}
	schema, err := compiler.Compile(schemaURL)
	if err != nil {
		panic(fmt.Sprintf("applicablemodifiers: compile subsystem registry schema: %v", err))
	}
	compiledSubsystemRegistrySchema = schema
}

// Subsystem describes a single consumer subsystem entry from the registry YAML.
type Subsystem struct {
	// Name is the subsystem identifier, matching the --subsystem flag value in applicablemodifiersgen.
	Name string
	// GoPackage is the Go package name for the generated Go file. Empty when no Go output.
	GoPackage string
	// GoOut is the repo-relative path for the generated Go file. Empty when no Go output.
	GoOut string
	// TSExportPrefix is the PascalCase prefix for generated TypeScript symbols.
	TSExportPrefix string
	// TSOut is the repo-relative path for the generated TypeScript file. Empty when no TS output.
	TSOut string
}

type subsystemYAML struct {
	Name           string `yaml:"name"`
	GoPackage      string `yaml:"go_package"`
	GoOut          string `yaml:"go_out"`
	TSExportPrefix string `yaml:"ts_export_prefix"`
	TSOut          string `yaml:"ts_out"`
}

type subsystemRegistryYAML struct {
	Subsystems []subsystemYAML `yaml:"subsystems"`
}

// LoadSubsystemRegistry reads and validates the subsystem registry YAML file,
// returning the ordered list of subsystem definitions.
func LoadSubsystemRegistry(path string) ([]Subsystem, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	if err := validateSubsystemRegistryYAML(path, data); err != nil {
		return nil, err
	}
	var doc subsystemRegistryYAML
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	out := make([]Subsystem, 0, len(doc.Subsystems))
	for _, s := range doc.Subsystems {
		out = append(out, Subsystem{
			Name:           s.Name,
			GoPackage:      s.GoPackage,
			GoOut:          s.GoOut,
			TSExportPrefix: s.TSExportPrefix,
			TSOut:          s.TSOut,
		})
	}
	return out, nil
}

func validateSubsystemRegistryYAML(path string, data []byte) error {
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
	if err := compiledSubsystemRegistrySchema.Validate(instance); err != nil {
		return fmt.Errorf("validate %s: %w", path, err)
	}
	return nil
}
