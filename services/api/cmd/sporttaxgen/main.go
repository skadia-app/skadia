package main

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/skadia-app/skadia/services/api/internal/sporttaxgen"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	defaultRoot, err := findRepoRoot(".")
	if err != nil {
		return err
	}

	repoRoot := flag.String("repo-root", defaultRoot, "repository root")
	source := flag.String("source", "", "taxonomy YAML source")
	goOut := flag.String("go-out", "", "generated Go output path")
	tsOut := flag.String("ts-out", "", "generated TypeScript output path")
	check := flag.Bool("check", false, "verify generated outputs are current")
	flag.Parse()

	if *source == "" {
		*source = filepath.Join(*repoRoot, "config", "sport-taxonomy", "sports.yaml")
	}
	if *goOut == "" {
		*goOut = filepath.Join(*repoRoot, "services", "api", "internal", "sport", "taxonomy_generated.go")
	}
	if *tsOut == "" {
		*tsOut = filepath.Join(*repoRoot, "frontend", "src", "lib", "generatedSportTaxonomy.ts")
	}

	sourceBytes, err := os.ReadFile(*source)
	if err != nil {
		return fmt.Errorf("read taxonomy source: %w", err)
	}
	taxonomy, err := sporttaxgen.LoadTaxonomy(bytes.NewReader(sourceBytes))
	if err != nil {
		return fmt.Errorf("load taxonomy: %w", err)
	}
	renderedGo, err := sporttaxgen.RenderGo(taxonomy)
	if err != nil {
		return fmt.Errorf("render Go taxonomy: %w", err)
	}
	renderedTS, err := sporttaxgen.RenderTS(taxonomy, sourceBytes)
	if err != nil {
		return fmt.Errorf("render TypeScript taxonomy: %w", err)
	}

	if *check {
		if err := checkFile(*goOut, renderedGo); err != nil {
			return err
		}
		return checkFile(*tsOut, renderedTS)
	}
	if err := os.WriteFile(*goOut, renderedGo, 0o644); err != nil {
		return fmt.Errorf("write Go taxonomy: %w", err)
	}
	if err := os.WriteFile(*tsOut, renderedTS, 0o644); err != nil {
		return fmt.Errorf("write TypeScript taxonomy: %w", err)
	}
	return nil
}

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

func checkFile(path string, want []byte) error {
	got, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read generated file %s: %w", path, err)
	}
	if !bytes.Equal(got, want) {
		return fmt.Errorf("%s is stale; run scripts/generate-sport-taxonomy.sh", path)
	}
	return nil
}
