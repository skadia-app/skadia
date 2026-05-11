package main

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/skadia-app/skadia/services/api/internal/geargen"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	repoRoot := flag.String("repo-root", "", "repository root (resolves canonical source and output paths)")
	source := flag.String("source", "", "gear-schema.yaml source path (overrides --repo-root)")
	goOut := flag.String("go-out", "", "generated Go output path (overrides --repo-root)")
	tsOut := flag.String("ts-out", "", "generated TypeScript output path (overrides --repo-root)")
	modifiersPathFlag := flag.String("modifiers-path", "", "path to config/sport-taxonomy/modifiers.yaml (auto-resolved from --repo-root when omitted)")
	check := flag.Bool("check", false, "verify generated outputs are current; exit 1 on drift")
	flag.Parse()

	// Auto-detect repo root from CWD when not explicitly supplied.
	if *repoRoot == "" {
		detected, err := findRepoRoot(".")
		if err == nil {
			*repoRoot = detected
		}
	}

	// Resolve source and output paths: explicit flags override --repo-root.
	if *source == "" {
		if *repoRoot == "" {
			return fmt.Errorf("either --source or --repo-root must be provided")
		}
		*source = filepath.Join(*repoRoot, "config", "gear-schema.yaml")
	}
	if *goOut == "" {
		if *repoRoot == "" {
			return fmt.Errorf("either --go-out or --repo-root must be provided")
		}
		*goOut = filepath.Join(*repoRoot, "services", "api", "internal", "gear", "schema_generated.go")
	}
	if *tsOut == "" {
		if *repoRoot == "" {
			return fmt.Errorf("either --ts-out or --repo-root must be provided")
		}
		*tsOut = filepath.Join(*repoRoot, "frontend", "src", "lib", "generatedGearSchema.ts")
	}

	// Resolve modifiers and taxonomy dir.
	modifiersPath := *modifiersPathFlag
	taxonomyDir := ""
	if modifiersPath == "" {
		if *repoRoot == "" {
			return fmt.Errorf("either --modifiers-path or --repo-root must be provided for validation")
		}
		modifiersPath = filepath.Join(*repoRoot, "config", "sport-taxonomy", "modifiers.yaml")
		taxonomyDir = filepath.Join(*repoRoot, "config", "sport-taxonomy")
	} else {
		taxonomyDir = filepath.Dir(modifiersPath)
	}

	// Load and validate schema.
	schema, err := geargen.Load(*source)
	if err != nil {
		return fmt.Errorf("load gear schema: %w", err)
	}
	if err := geargen.Validate(schema, taxonomyDir, modifiersPath); err != nil {
		return fmt.Errorf("validate gear schema: %w", err)
	}

	// Render outputs.
	renderedGo, err := geargen.RenderGo(schema)
	if err != nil {
		return fmt.Errorf("render Go: %w", err)
	}
	renderedTS, err := geargen.RenderTS(schema)
	if err != nil {
		return fmt.Errorf("render TypeScript: %w", err)
	}

	if *check {
		outputs := []generatedOutput{
			{path: *goOut, want: renderedGo},
			{path: *tsOut, want: []byte(renderedTS)},
		}
		return checkGeneratedFiles(outputs)
	}

	if err := os.WriteFile(*goOut, renderedGo, 0o644); err != nil {
		return fmt.Errorf("write Go output: %w", err)
	}
	if err := os.WriteFile(*tsOut, []byte(renderedTS), 0o644); err != nil {
		return fmt.Errorf("write TypeScript output: %w", err)
	}
	return nil
}

// findRepoRoot walks up from start until it finds a directory containing
// config/sport-taxonomy/sports.yaml, which marks the repo root.
func findRepoRoot(start string) (string, error) {
	dir, err := filepath.Abs(start)
	if err != nil {
		return "", fmt.Errorf("resolve start dir: %w", err)
	}
	for {
		candidate := filepath.Join(dir, "config", "sport-taxonomy", "sports.yaml")
		if _, err := os.Stat(candidate); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("could not find repository root from %s", start)
		}
		dir = parent
	}
}

// checkFile compares the file at path against want, returning an error with
// a unified diff fragment if they differ.
func checkFile(path string, want []byte) error {
	got, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read generated file %s: %w", path, err)
	}
	if !bytes.Equal(got, want) {
		return fmt.Errorf("%s is stale; run scripts/generate-gear-schema.sh\n%s",
			path, unifiedGeneratedDiff(path, got, want))
	}
	return nil
}

type generatedOutput struct {
	path string
	want []byte
}

func checkGeneratedFiles(outputs []generatedOutput) error {
	var failures []string
	for _, output := range outputs {
		if err := checkFile(output.path, output.want); err != nil {
			failures = append(failures, err.Error())
		}
	}
	if len(failures) > 0 {
		return fmt.Errorf("generated files are stale:\n\n%s", strings.Join(failures, "\n\n"))
	}
	return nil
}

func unifiedGeneratedDiff(path string, got, want []byte) string {
	gotLines := diffLines(got)
	wantLines := diffLines(want)

	prefix := 0
	for prefix < len(gotLines) && prefix < len(wantLines) && gotLines[prefix] == wantLines[prefix] {
		prefix++
	}
	suffix := 0
	for suffix < len(gotLines)-prefix &&
		suffix < len(wantLines)-prefix &&
		gotLines[len(gotLines)-1-suffix] == wantLines[len(wantLines)-1-suffix] {
		suffix++
	}

	gotChangeEnd := len(gotLines) - suffix
	wantChangeEnd := len(wantLines) - suffix
	contextStart := max(0, prefix-3)
	contextEnd := min(len(wantLines), wantChangeEnd+3)

	var diff strings.Builder
	fmt.Fprintf(&diff, "--- %s\n+++ expected\n@@\n", path)
	for _, line := range gotLines[contextStart:prefix] {
		fmt.Fprintf(&diff, " %s\n", line)
	}
	for _, line := range gotLines[prefix:gotChangeEnd] {
		fmt.Fprintf(&diff, "-%s\n", line)
	}
	for _, line := range wantLines[prefix:wantChangeEnd] {
		fmt.Fprintf(&diff, "+%s\n", line)
	}
	for _, line := range wantLines[wantChangeEnd:contextEnd] {
		fmt.Fprintf(&diff, " %s\n", line)
	}
	return strings.TrimRight(diff.String(), "\n")
}

func diffLines(data []byte) []string {
	text := strings.TrimSuffix(string(data), "\n")
	if text == "" {
		return nil
	}
	return strings.Split(text, "\n")
}
