package main

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/skadia-app/skadia/services/api/internal/applicablemodifiers"
	"github.com/skadia-app/skadia/services/api/internal/sporttaxgen"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	// Per-subsystem flags (single-subsystem mode).
	subsystem := flag.String("subsystem", "", "subsystem name (snake_case, e.g. gear)")
	exportPrefix := flag.String("export-prefix", "", "PascalCase export prefix for generated symbols (e.g. Gear)")
	goOut := flag.String("go-out", "", "path to write generated Go file (optional)")
	tsOut := flag.String("ts-out", "", "path to write generated TypeScript file (optional)")

	// Shared flags.
	source := flag.String("source", "", "path to modifiers.yaml (sport-taxonomy)")
	check := flag.Bool("check", false, "verify generated outputs are current instead of writing them")

	// Registry/bulk flags (--all mode).
	all := flag.Bool("all", false, "generate all subsystems from the registry")
	registry := flag.String("registry", "", "path to subsystems.yaml registry (required with --all)")
	repoRoot := flag.String("repo-root", "", "repo root path; go_out/ts_out in registry are relative to this (required with --all)")
	knownSubsystemsOut := flag.String("known-subsystems-out", "", "path to write generated knownSubsystems Go file")
	schemaEnumOut := flag.String("schema-enum-out", "", "path to write generated subsystems enum JSONSchema fragment")

	flag.Parse()

	if *source == "" {
		return fmt.Errorf("--source is required")
	}

	if *all {
		return runAll(*source, *registry, *repoRoot, *knownSubsystemsOut, *schemaEnumOut, *check)
	}
	return runSingle(*source, *subsystem, *exportPrefix, *goOut, *tsOut, *check)
}

// runSingle is the original single-subsystem generation mode.
func runSingle(source, subsystem, exportPrefix, goOut, tsOut string, check bool) error {
	if subsystem == "" {
		return fmt.Errorf("--subsystem is required")
	}
	if exportPrefix == "" {
		return fmt.Errorf("--export-prefix is required")
	}

	keys, err := applicablemodifiers.LoadForSubsystem(source, subsystem)
	if err != nil {
		return fmt.Errorf("load: %w", err)
	}

	var goRendered []byte
	if goOut != "" {
		rendered, err := applicablemodifiers.RenderGo(subsystem, exportPrefix, keys)
		if err != nil {
			return fmt.Errorf("render Go: %w", err)
		}
		goRendered = []byte(rendered)
	}

	var tsRendered []byte
	if tsOut != "" {
		rendered, err := applicablemodifiers.RenderTS(exportPrefix, keys)
		if err != nil {
			return fmt.Errorf("render TS: %w", err)
		}
		tsRendered = []byte(rendered)
	}

	if check {
		var failures []string
		if goOut != "" {
			if err := checkFile(goOut, goRendered); err != nil {
				failures = append(failures, err.Error())
			}
		}
		if tsOut != "" {
			if err := checkFile(tsOut, tsRendered); err != nil {
				failures = append(failures, err.Error())
			}
		}
		if len(failures) > 0 {
			return fmt.Errorf("generated files are stale:\n\n%s", strings.Join(failures, "\n\n"))
		}
		return nil
	}

	if goOut != "" {
		if err := os.WriteFile(goOut, goRendered, 0o644); err != nil {
			return fmt.Errorf("write Go output: %w", err)
		}
	}
	if tsOut != "" {
		if err := os.WriteFile(tsOut, tsRendered, 0o644); err != nil {
			return fmt.Errorf("write TS output: %w", err)
		}
	}
	return nil
}

// runAll generates all subsystems from the registry YAML, and optionally writes
// the knownSubsystems Go file and the subsystems enum JSONSchema fragment.
// repoRoot is prepended to repo-relative go_out/ts_out paths from the registry.
func runAll(source, registryPath, repoRoot, knownSubsystemsOut, schemaEnumOut string, check bool) error {
	if registryPath == "" {
		return fmt.Errorf("--registry is required with --all")
	}
	if repoRoot == "" {
		return fmt.Errorf("--repo-root is required with --all")
	}

	subs, err := applicablemodifiers.LoadSubsystemRegistry(registryPath)
	if err != nil {
		return fmt.Errorf("load registry: %w", err)
	}

	var failures []string

	// Generate per-subsystem files.
	for _, sub := range subs {
		goOut := absPath(repoRoot, sub.GoOut)
		tsOut := absPath(repoRoot, sub.TSOut)
		if err := runSingle(source, sub.Name, sub.TSExportPrefix, goOut, tsOut, check); err != nil {
			failures = append(failures, fmt.Sprintf("subsystem %s: %v", sub.Name, err))
		}
	}

	// Generate knownSubsystems Go file.
	if knownSubsystemsOut != "" {
		rendered, err := sporttaxgen.RenderKnownSubsystems(subs)
		if err != nil {
			failures = append(failures, fmt.Sprintf("render knownSubsystems: %v", err))
		} else if check {
			if err := checkFile(knownSubsystemsOut, []byte(rendered)); err != nil {
				failures = append(failures, err.Error())
			}
		} else {
			if err := os.WriteFile(knownSubsystemsOut, []byte(rendered), 0o644); err != nil {
				failures = append(failures, fmt.Sprintf("write knownSubsystems: %v", err))
			}
		}
	}

	// Generate subsystems enum JSONSchema fragment.
	if schemaEnumOut != "" {
		rendered, err := sporttaxgen.RenderSubsystemsEnumFragment(subs)
		if err != nil {
			failures = append(failures, fmt.Sprintf("render schema enum fragment: %v", err))
		} else if check {
			if err := checkFile(schemaEnumOut, []byte(rendered)); err != nil {
				failures = append(failures, err.Error())
			}
		} else {
			if err := os.WriteFile(schemaEnumOut, []byte(rendered), 0o644); err != nil {
				failures = append(failures, fmt.Sprintf("write schema enum fragment: %v", err))
			}
		}
	}

	if len(failures) > 0 {
		return fmt.Errorf("generated files are stale:\n\n%s", strings.Join(failures, "\n\n"))
	}
	return nil
}

// absPath returns filepath.Join(root, rel) when rel is non-empty, or "" when rel is empty.
func absPath(root, rel string) string {
	if rel == "" {
		return ""
	}
	return root + "/" + rel
}

// checkFile compares the on-disk content of path against want.
// Returns an error with a unified diff if they differ.
func checkFile(path string, want []byte) error {
	got, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read generated file %s: %w", path, err)
	}
	if !bytes.Equal(got, want) {
		return fmt.Errorf("%s is stale; run applicablemodifiersgen to regenerate\n%s", path, unifiedDiff(path, got, want))
	}
	return nil
}

func unifiedDiff(path string, got, want []byte) string {
	gotLines := splitLines(got)
	wantLines := splitLines(want)

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

func splitLines(data []byte) []string {
	text := strings.TrimSuffix(string(data), "\n")
	if text == "" {
		return nil
	}
	return strings.Split(text, "\n")
}
