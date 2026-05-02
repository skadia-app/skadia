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
	activityFilterSource := flag.String("activity-filter-source", "", "activity filter projection YAML source")
	goProjectionsOut := flag.String("go-projections-out", "", "generated Go activity filter projection output path")
	tsProjectionsOut := flag.String("ts-projections-out", "", "generated TypeScript activity filter projection output path")
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
	if *activityFilterSource == "" {
		*activityFilterSource = filepath.Join(*repoRoot, "config", "sport-taxonomy", "projections", "activity_filters.yaml")
	}
	if *goProjectionsOut == "" {
		*goProjectionsOut = filepath.Join(*repoRoot, "services", "api", "internal", "sport", "projections_generated.go")
	}
	if *tsProjectionsOut == "" {
		*tsProjectionsOut = filepath.Join(*repoRoot, "frontend", "src", "lib", "generatedSportFilterProjections.ts")
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
	activityFilterBytes, err := os.ReadFile(*activityFilterSource)
	if err != nil {
		return fmt.Errorf("read activity filter projection source: %w", err)
	}
	projections, err := sporttaxgen.LoadActivityFilterProjections(bytes.NewReader(activityFilterBytes), taxonomy)
	if err != nil {
		return fmt.Errorf("load activity filter projection: %w", err)
	}
	expandedProjections, err := sporttaxgen.ExpandActivityFilterPresets(taxonomy, projections)
	if err != nil {
		return fmt.Errorf("expand activity filter projection: %w", err)
	}
	renderedGoProjections, err := sporttaxgen.RenderGoActivityFilterProjections(expandedProjections)
	if err != nil {
		return fmt.Errorf("render Go activity filter projection: %w", err)
	}
	renderedTSProjections, err := sporttaxgen.RenderTSActivityFilterProjections(expandedProjections)
	if err != nil {
		return fmt.Errorf("render TypeScript activity filter projection: %w", err)
	}

	if *check {
		if err := checkFile(*goOut, renderedGo); err != nil {
			return err
		}
		if err := checkFile(*tsOut, renderedTS); err != nil {
			return err
		}
		if err := checkFile(*goProjectionsOut, renderedGoProjections); err != nil {
			return err
		}
		return checkFile(*tsProjectionsOut, renderedTSProjections)
	}
	if err := os.WriteFile(*goOut, renderedGo, 0o644); err != nil {
		return fmt.Errorf("write Go taxonomy: %w", err)
	}
	if err := os.WriteFile(*tsOut, renderedTS, 0o644); err != nil {
		return fmt.Errorf("write TypeScript taxonomy: %w", err)
	}
	if err := os.WriteFile(*goProjectionsOut, renderedGoProjections, 0o644); err != nil {
		return fmt.Errorf("write Go activity filter projection: %w", err)
	}
	if err := os.WriteFile(*tsProjectionsOut, renderedTSProjections, 0o644); err != nil {
		return fmt.Errorf("write TypeScript activity filter projection: %w", err)
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
