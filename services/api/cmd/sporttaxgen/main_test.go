package main

import (
	"bytes"
	"flag"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/skadia-app/skadia/services/api/internal/sporttaxgen"
)

func TestCheckFileReportsDiffForStaleGeneratedOutput(t *testing.T) {
	path := filepath.Join(t.TempDir(), "generated.go")
	if err := os.WriteFile(path, []byte("package old\n"), 0o644); err != nil {
		t.Fatalf("write generated fixture: %v", err)
	}

	err := checkFile(path, []byte("package new\n"))
	if err == nil {
		t.Fatal("checkFile returned nil for stale output")
	}
	message := err.Error()
	for _, want := range []string{
		"is stale",
		"--- " + path,
		"+++ expected",
		"-package old",
		"+package new",
	} {
		if !strings.Contains(message, want) {
			t.Fatalf("checkFile error missing %q:\n%s", want, message)
		}
	}
}

func TestCheckGeneratedFilesReportsEveryStaleFile(t *testing.T) {
	dir := t.TempDir()
	first := filepath.Join(dir, "first.go")
	second := filepath.Join(dir, "second.go")
	if err := os.WriteFile(first, []byte("old first\n"), 0o644); err != nil {
		t.Fatalf("write first fixture: %v", err)
	}
	if err := os.WriteFile(second, []byte("old second\n"), 0o644); err != nil {
		t.Fatalf("write second fixture: %v", err)
	}

	err := checkGeneratedFiles([]generatedOutput{
		{path: first, want: []byte("new first\n")},
		{path: second, want: []byte("new second\n")},
	})
	if err == nil {
		t.Fatal("checkGeneratedFiles returned nil for stale outputs")
	}
	message := err.Error()
	for _, want := range []string{
		first + " is stale",
		second + " is stale",
		"-old first",
		"+new first",
		"-old second",
		"+new second",
	} {
		if !strings.Contains(message, want) {
			t.Fatalf("checkGeneratedFiles error missing %q:\n%s", want, message)
		}
	}
}

func TestRenderAliasesFromDirUsesGearDefaultProjectionSource(t *testing.T) {
	taxonomy := loadMainTestTaxonomy(t)
	modifiers := loadMainTestModifiers(t)
	configPath := filepath.Join(t.TempDir(), "gear_defaults.yaml")
	if err := os.WriteFile(configPath, []byte("projection_sources: [garmin]\n"), 0o644); err != nil {
		t.Fatalf("write gear defaults config: %v", err)
	}

	goAliases, _, _, aliasesExist, err := renderAliasesFromDir(repoPathForMainTest(t, "config/sport-taxonomy/aliases"), configPath, taxonomy, modifiers)
	if err != nil {
		t.Fatalf("renderAliasesFromDir() error = %v", err)
	}
	if !aliasesExist {
		t.Fatal("renderAliasesFromDir() aliasesExist = false")
	}
	if !bytes.Contains(goAliases, []byte("var generatedGearDefaultSportsBySource = map[Source]map[string]string{\n\tSourceGarminCourse:")) {
		t.Fatalf("generated gear default source map does not include configured garmin source:\n%s", goAliases)
	}
	if bytes.Contains(goAliases, []byte("var generatedGearDefaultSportsBySource = map[Source]map[string]string{\n\tSourceStrava:")) {
		t.Fatalf("generated gear default source map includes unconfigured strava source:\n%s", goAliases)
	}
}

func TestCheckModeReportsGearDefaultProjectionConfigDrift(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "gear_defaults.yaml")
	if err := os.WriteFile(configPath, []byte("projection_sources: [garmin]\n"), 0o644); err != nil {
		t.Fatalf("write gear defaults config: %v", err)
	}

	oldArgs := os.Args
	oldCommandLine := flag.CommandLine
	t.Cleanup(func() {
		os.Args = oldArgs
		flag.CommandLine = oldCommandLine
	})
	flag.CommandLine = flag.NewFlagSet("sporttaxgen", flag.ContinueOnError)
	flag.CommandLine.SetOutput(io.Discard)
	os.Args = []string{
		"sporttaxgen",
		"--repo-root", repoRootForMainTest(t),
		"--gear-default-projection-source", configPath,
		"--check",
	}

	err := run()
	if err == nil {
		t.Fatal("run --check returned nil for changed gear default projection source")
	}
	message := err.Error()
	for _, want := range []string{
		"aliases_generated.go is stale",
		"generatedGearDefaultSportsBySource",
		"SourceGarminCourse",
	} {
		if !strings.Contains(message, want) {
			t.Fatalf("run --check error missing %q:\n%s", want, message)
		}
	}
}

func loadMainTestTaxonomy(t *testing.T) sporttaxgen.Taxonomy {
	t.Helper()
	file, err := os.Open(repoPathForMainTest(t, "config/sport-taxonomy/sports.yaml"))
	if err != nil {
		t.Fatalf("open sports.yaml: %v", err)
	}
	defer file.Close()
	taxonomy, err := sporttaxgen.LoadTaxonomy(file)
	if err != nil {
		t.Fatalf("load taxonomy: %v", err)
	}
	return taxonomy
}

func loadMainTestModifiers(t *testing.T) sporttaxgen.ModifierRegistry {
	t.Helper()
	file, err := os.Open(repoPathForMainTest(t, "config/sport-taxonomy/modifiers.yaml"))
	if err != nil {
		t.Fatalf("open modifiers.yaml: %v", err)
	}
	defer file.Close()
	modifiers, err := sporttaxgen.LoadModifiers(file)
	if err != nil {
		t.Fatalf("load modifiers: %v", err)
	}
	return modifiers
}

func repoRootForMainTest(t *testing.T) string {
	t.Helper()
	root, err := findRepoRoot(".")
	if err != nil {
		t.Fatalf("repo root: %v", err)
	}
	return root
}

func repoPathForMainTest(t *testing.T, rel string) string {
	t.Helper()
	return filepath.Join(repoRootForMainTest(t), rel)
}
