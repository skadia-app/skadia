package applicablemodifiers

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

type document struct {
	ApplicableModifiers []string `yaml:"applicable_modifiers"`
}

// Load reads a subsystem applicable_modifiers YAML file and returns the list of modifier keys.
func Load(path string) ([]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	var doc document
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return doc.ApplicableModifiers, nil
}
