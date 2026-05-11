package main

import (
	"flag"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// repoRootForMainTest walks up from the package directory to find the repo root.
func repoRootForMainTest(t *testing.T) string {
	t.Helper()
	root, err := findRepoRoot(".")
	if err != nil {
		t.Fatalf("findRepoRoot: %v", err)
	}
	return root
}

// minimalTestSchemaYAML is a minimal valid gear-schema.yaml for test fixtures.
const minimalTestSchemaYAML = `version: 1
assignment_roles:
  - key: primary
    label_de: Hauptausrüstung
    unique_per_activity: true
option_sets: []
attributes:
  - key: bike.frame_size_cm
    label_de: Rahmengröße (cm)
    value_type: number
gear_types:
  - key: bike
    label_de: Fahrrad
    scope: bike
    default_assignment_role_key: primary
    attribute_keys: [bike.frame_size_cm]
`

func resetFlags(t *testing.T) {
	t.Helper()
	old := os.Args
	oldCommandLine := flag.CommandLine
	t.Cleanup(func() {
		os.Args = old
		flag.CommandLine = oldCommandLine
	})
	flag.CommandLine = flag.NewFlagSet("geargen", flag.ContinueOnError)
	flag.CommandLine.SetOutput(io.Discard)
}

func TestCheckFileReportsStale(t *testing.T) {
	path := filepath.Join(t.TempDir(), "generated.go")
	if err := os.WriteFile(path, []byte("package old\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	err := checkFile(path, []byte("package new\n"))
	if err == nil {
		t.Fatal("checkFile returned nil for stale output")
	}
	for _, want := range []string{"is stale", "--- " + path, "+++ expected", "-package old", "+package new"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("checkFile error missing %q:\n%s", want, err.Error())
		}
	}
}

func TestGenerateModeWritesFiles(t *testing.T) {
	root := repoRootForMainTest(t)
	dir := t.TempDir()
	srcPath := filepath.Join(dir, "gear-schema.yaml")
	goOut := filepath.Join(dir, "schema_generated.go")
	tsOut := filepath.Join(dir, "generatedGearSchema.ts")

	if err := os.WriteFile(srcPath, []byte(minimalTestSchemaYAML), 0o644); err != nil {
		t.Fatalf("write source: %v", err)
	}

	resetFlags(t)
	os.Args = []string{"geargen",
		"--source", srcPath,
		"--go-out", goOut,
		"--ts-out", tsOut,
		"--repo-root", root,
	}
	if err := run(); err != nil {
		t.Fatalf("run() error = %v", err)
	}

	goContent, err := os.ReadFile(goOut)
	if err != nil {
		t.Fatalf("read go-out: %v", err)
	}
	if !strings.Contains(string(goContent), "package gear") {
		t.Errorf("go-out missing 'package gear'")
	}

	tsContent, err := os.ReadFile(tsOut)
	if err != nil {
		t.Fatalf("read ts-out: %v", err)
	}
	if !strings.Contains(string(tsContent), "export const gearSchema") {
		t.Errorf("ts-out missing 'export const gearSchema'")
	}
}

func TestCheckModePassesForCurrentFiles(t *testing.T) {
	root := repoRootForMainTest(t)
	dir := t.TempDir()
	srcPath := filepath.Join(dir, "gear-schema.yaml")
	goOut := filepath.Join(dir, "schema_generated.go")
	tsOut := filepath.Join(dir, "generatedGearSchema.ts")

	if err := os.WriteFile(srcPath, []byte(minimalTestSchemaYAML), 0o644); err != nil {
		t.Fatalf("write source: %v", err)
	}

	// First generate the files.
	resetFlags(t)
	os.Args = []string{"geargen",
		"--source", srcPath,
		"--go-out", goOut,
		"--ts-out", tsOut,
		"--repo-root", root,
	}
	if err := run(); err != nil {
		t.Fatalf("generate run() error = %v", err)
	}

	// Then check them — should pass.
	resetFlags(t)
	os.Args = []string{"geargen",
		"--source", srcPath,
		"--go-out", goOut,
		"--ts-out", tsOut,
		"--repo-root", root,
		"--check",
	}
	if err := run(); err != nil {
		t.Fatalf("check run() unexpected error = %v", err)
	}
}

func TestCheckModeReportsStale(t *testing.T) {
	root := repoRootForMainTest(t)
	dir := t.TempDir()
	srcPath := filepath.Join(dir, "gear-schema.yaml")
	goOut := filepath.Join(dir, "schema_generated.go")
	tsOut := filepath.Join(dir, "generatedGearSchema.ts")

	if err := os.WriteFile(srcPath, []byte(minimalTestSchemaYAML), 0o644); err != nil {
		t.Fatalf("write source: %v", err)
	}
	// Write stale content.
	if err := os.WriteFile(goOut, []byte("package stale\n"), 0o644); err != nil {
		t.Fatalf("write stale go: %v", err)
	}
	if err := os.WriteFile(tsOut, []byte("// stale\n"), 0o644); err != nil {
		t.Fatalf("write stale ts: %v", err)
	}

	resetFlags(t)
	os.Args = []string{"geargen",
		"--source", srcPath,
		"--go-out", goOut,
		"--ts-out", tsOut,
		"--repo-root", root,
		"--check",
	}
	err := run()
	if err == nil {
		t.Fatal("run --check returned nil for stale files")
	}
	if !strings.Contains(err.Error(), "stale") {
		t.Errorf("error should mention 'stale', got: %v", err)
	}
}

// step1FixtureYAML exercises the Step 1 schema additions: per-gear_type
// recommended block plus per-attribute visible_when / applies_to_gear_when
// condition DSL. Run end-to-end through the generator to confirm the
// emitted Go and TS contain literal traces of the new fields.
const step1FixtureYAML = `version: 1
assignment_roles:
  - key: primary
    label_de: Haupt
    unique_per_activity: true
option_sets: []
attributes:
  - key: bike.frame_design
    label_de: Rahmenkonzept
    value_type: string
  - key: bike.shock_eye_to_eye_mm
    label_de: Dämpfer-Einbaulänge
    value_type: number
    applies_to_gear_when:
      - {ref: gear.bike.frame_design, op: eq, value: full_suspension}
  - key: bike.shock_stroke_mm
    label_de: Dämpfer-Hub
    value_type: number
    visible_when:
      - {ref: bike.shock_eye_to_eye_mm, op: is_set}
gear_types:
  - key: bike.mtb.enduro
    label_de: Enduro
    scope: bike
    default_assignment_role_key: primary
    attribute_keys: [bike.frame_design, bike.shock_eye_to_eye_mm, bike.shock_stroke_mm]
    recommended:
      attributes:
        - {key: bike.frame_design, values: [full_suspension]}
`

func TestStep1FixtureRoundTripsThroughGenerator(t *testing.T) {
	root := repoRootForMainTest(t)
	dir := t.TempDir()
	srcPath := filepath.Join(dir, "gear-schema.yaml")
	goOut := filepath.Join(dir, "schema_generated.go")
	tsOut := filepath.Join(dir, "generatedGearSchema.ts")

	if err := os.WriteFile(srcPath, []byte(step1FixtureYAML), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	resetFlags(t)
	os.Args = []string{"geargen",
		"--source", srcPath,
		"--go-out", goOut,
		"--ts-out", tsOut,
		"--repo-root", root,
	}
	if err := run(); err != nil {
		t.Fatalf("run() error = %v", err)
	}

	goContent, err := os.ReadFile(goOut)
	if err != nil {
		t.Fatalf("read go-out: %v", err)
	}
	for _, want := range []string{
		"type Condition struct",
		"type RecommendedConstraints struct",
		`AppliesToGearWhen: []Condition{{Ref: "gear.bike.frame_design", Op: "eq", Value: "full_suspension"}}`,
		`VisibleWhen: []Condition{{Ref: "bike.shock_eye_to_eye_mm", Op: "is_set"}}`,
		`Recommended: &RecommendedConstraints{`,
	} {
		if !strings.Contains(string(goContent), want) {
			t.Errorf("generated Go missing %q\n--- output ---\n%s", want, goContent)
		}
	}

	tsContent, err := os.ReadFile(tsOut)
	if err != nil {
		t.Fatalf("read ts-out: %v", err)
	}
	for _, want := range []string{
		"export type ConditionOp",
		"export interface Condition",
		"export interface RecommendedConstraints",
		`"appliesToGearWhen": [`,
		`"visibleWhen": [`,
		`"recommended": {`,
		`"op": "eq"`,
		`"op": "is_set"`,
	} {
		if !strings.Contains(string(tsContent), want) {
			t.Errorf("generated TS missing %q\n--- output ---\n%s", want, tsContent)
		}
	}
}

func TestRepoRootModeResolvesCanonicalPaths(t *testing.T) {
	root := repoRootForMainTest(t)

	// Verify canonical paths exist (generated files must exist for check mode to work).
	goOutPath := filepath.Join(root, "services", "api", "internal", "gear", "schema_generated.go")
	tsOutPath := filepath.Join(root, "frontend", "src", "lib", "generatedGearSchema.ts")
	srcPath := filepath.Join(root, "config", "gear-schema.yaml")

	for _, p := range []string{srcPath, goOutPath, tsOutPath} {
		if _, err := os.Stat(p); err != nil {
			t.Skipf("canonical path %s not present — skipping repo-root check mode test", p)
		}
	}

	resetFlags(t)
	os.Args = []string{"geargen", "--repo-root", root, "--check"}
	if err := run(); err != nil {
		t.Fatalf("run --repo-root --check error = %v", err)
	}
}
