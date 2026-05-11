package geargen_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/skadia-app/skadia/services/api/internal/geargen"
)

func TestLoadSupportsRecursiveSubfields(t *testing.T) {
	yaml := `version: 1
assignment_roles: []
option_sets: []
attributes:
  - key: bike.geometry
    label_de: Geometrie
    value_type: geometry
    subfields:
      - key: bike.geometry.reach
        label_de: Reach (mm)
        value_type: number
      - key: bike.geometry.stack
        label_de: Stack (mm)
        value_type: number
        subfields:
          - key: bike.geometry.stack.detail
            label_de: Detail
            value_type: string
gear_types: []
`
	path := filepath.Join(t.TempDir(), "gear-schema.yaml")
	if err := os.WriteFile(path, []byte(yaml), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	schema, err := geargen.Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if len(schema.Attributes) != 1 {
		t.Fatalf("Attributes: got %d, want 1", len(schema.Attributes))
	}
	root := schema.Attributes[0]
	if root.Key != "bike.geometry" {
		t.Errorf("root.Key = %q, want %q", root.Key, "bike.geometry")
	}
	if len(root.Subfields) != 2 {
		t.Fatalf("root.Subfields: got %d, want 2", len(root.Subfields))
	}
	if root.Subfields[0].Key != "bike.geometry.reach" {
		t.Errorf("Subfields[0].Key = %q, want %q", root.Subfields[0].Key, "bike.geometry.reach")
	}
	nested := root.Subfields[1]
	if len(nested.Subfields) != 1 {
		t.Fatalf("nested.Subfields: got %d, want 1", len(nested.Subfields))
	}
	if nested.Subfields[0].Key != "bike.geometry.stack.detail" {
		t.Errorf("nested.Subfields[0].Key = %q, want %q", nested.Subfields[0].Key, "bike.geometry.stack.detail")
	}
}

// sixLevelDeepSchemaYAML builds a YAML with 6 levels of subfield nesting
// using only valid key characters (all lowercase, dots, digits).
// Structure: root → l1 → l2 → l3 → l4 → l5 → l6 (6 nesting levels below root).
const sixLevelDeepSchemaYAML = `version: 1
assignment_roles: []
option_sets: []
attributes:
  - key: a.root
    label_de: Root
    value_type: geometry
    subfields:
      - key: a.l1
        label_de: L1
        value_type: geometry
        subfields:
          - key: a.l2
            label_de: L2
            value_type: geometry
            subfields:
              - key: a.l3
                label_de: L3
                value_type: geometry
                subfields:
                  - key: a.l4
                    label_de: L4
                    value_type: geometry
                    subfields:
                      - key: a.l5
                        label_de: L5
                        value_type: geometry
                        subfields:
                          - key: a.l6
                            label_de: L6
                            value_type: number
gear_types: []
`

func TestLoadRejectsTooDeepSubfieldNesting(t *testing.T) {
	path := filepath.Join(t.TempDir(), "too-deep.yaml")
	if err := os.WriteFile(path, []byte(sixLevelDeepSchemaYAML), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	_, err := geargen.Load(path)
	if err == nil {
		t.Fatal("Load() returned nil for 6-level deep subfields")
	}
	msg := err.Error()
	if !strings.Contains(msg, "subfield") && !strings.Contains(msg, "depth") && !strings.Contains(msg, "nesting") {
		t.Errorf("error message should mention subfield depth, got: %v", err)
	}
}

// TestLoadAcceptsFiveLevelSubfieldNesting verifies the boundary: 5 levels is valid.
const fiveLevelDeepSchemaYAML = `version: 1
assignment_roles: []
option_sets: []
attributes:
  - key: a.root
    label_de: Root
    value_type: geometry
    subfields:
      - key: a.l1
        label_de: L1
        value_type: geometry
        subfields:
          - key: a.l2
            label_de: L2
            value_type: geometry
            subfields:
              - key: a.l3
                label_de: L3
                value_type: geometry
                subfields:
                  - key: a.l4
                    label_de: L4
                    value_type: geometry
                    subfields:
                      - key: a.l5
                        label_de: L5
                        value_type: number
gear_types: []
`

func TestLoadAcceptsFiveLevelSubfieldNesting(t *testing.T) {
	path := filepath.Join(t.TempDir(), "five-deep.yaml")
	if err := os.WriteFile(path, []byte(fiveLevelDeepSchemaYAML), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	_, err := geargen.Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v for 5-level deep subfields (should be valid)", err)
	}
}
