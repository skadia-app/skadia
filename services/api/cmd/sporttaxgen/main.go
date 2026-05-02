package main

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

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
	aliasSourceDir := flag.String("alias-source-dir", "", "source-specific sport alias YAML directory")
	goAliasesOut := flag.String("go-aliases-out", "", "generated Go source aliases output path")
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
	if *aliasSourceDir == "" {
		*aliasSourceDir = filepath.Join(*repoRoot, "config", "sport-taxonomy", "aliases")
	}
	if *goAliasesOut == "" {
		*goAliasesOut = filepath.Join(*repoRoot, "services", "api", "internal", "sport", "aliases_generated.go")
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
	renderedGoAliases, aliasesExist, err := renderGoAliasesFromDir(*aliasSourceDir, taxonomy)
	if err != nil {
		return err
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
		if err := checkFile(*tsProjectionsOut, renderedTSProjections); err != nil {
			return err
		}
		if aliasesExist {
			return checkFile(*goAliasesOut, renderedGoAliases)
		}
		return nil
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
	if aliasesExist {
		if err := os.WriteFile(*goAliasesOut, renderedGoAliases, 0o644); err != nil {
			return fmt.Errorf("write Go source aliases: %w", err)
		}
	}
	return nil
}

func renderGoAliasesFromDir(dir string, taxonomy sporttaxgen.Taxonomy) ([]byte, bool, error) {
	if _, err := os.Stat(dir); err != nil {
		if os.IsNotExist(err) {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("stat source alias directory: %w", err)
	}
	matches, err := filepath.Glob(filepath.Join(dir, "*.yaml"))
	if err != nil {
		return nil, false, fmt.Errorf("glob source alias files: %w", err)
	}
	files := make(map[string]io.Reader, len(matches))
	opened := make([]*os.File, 0, len(matches))
	defer func() {
		for _, file := range opened {
			_ = file.Close()
		}
	}()
	for _, match := range matches {
		file, err := os.Open(match)
		if err != nil {
			return nil, false, fmt.Errorf("open source alias file %s: %w", match, err)
		}
		opened = append(opened, file)
		stem := strings.TrimSuffix(filepath.Base(match), filepath.Ext(match))
		files[stem] = file
	}
	sources, err := sporttaxgen.LoadSourceAliasFiles(files, taxonomy)
	if err != nil {
		return nil, false, fmt.Errorf("load source aliases: %w", err)
	}
	rendered, err := sporttaxgen.RenderGoSourceAliases(sources)
	if err != nil {
		return nil, false, fmt.Errorf("render Go source aliases: %w", err)
	}
	return rendered, true, nil
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
