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
	modifierSource := flag.String("modifier-source", "", "modifier registry YAML source")
	goOut := flag.String("go-out", "", "generated Go output path")
	tsOut := flag.String("ts-out", "", "generated TypeScript output path")
	goModifiersOut := flag.String("go-modifiers-out", "", "generated Go modifier output path")
	tsModifiersOut := flag.String("ts-modifiers-out", "", "generated TypeScript modifier output path")
	activityFilterSource := flag.String("activity-filter-source", "", "activity filter projection YAML source")
	goProjectionsOut := flag.String("go-projections-out", "", "generated Go activity filter projection output path")
	tsProjectionsOut := flag.String("ts-projections-out", "", "generated TypeScript activity filter projection output path")
	segmentProjectionSource := flag.String("segment-projection-source", "", "segment eligibility projection YAML source")
	tsSegmentProjectionsOut := flag.String("ts-segment-projections-out", "", "generated TypeScript segment projection output path")
	ruleProjectionSource := flag.String("rule-projection-source", "", "rule projection YAML source")
	tsRuleProjectionsOut := flag.String("ts-rule-projections-out", "", "generated TypeScript rule projection output path")
	recordsProjectionSource := flag.String("records-projection-source", "", "records projection YAML source")
	goRecordsProjectionOut := flag.String("go-records-projection-out", "", "generated Go records projection output path")
	tsRecordsProjectionOut := flag.String("ts-records-projection-out", "", "generated TypeScript records projection output path")
	aliasSourceDir := flag.String("alias-source-dir", "", "source-specific sport alias YAML directory")
	goAliasesOut := flag.String("go-aliases-out", "", "generated Go source aliases output path")
	tsAliasesOut := flag.String("ts-aliases-out", "", "generated TypeScript source aliases output path")
	tsGearDefaultsOut := flag.String("ts-gear-defaults-out", "", "generated TypeScript gear default projection output path")
	predicateValidationSQLOut := flag.String("predicate-validation-sql-out", "", "generated SQL predicate validator output path")
	check := flag.Bool("check", false, "verify generated outputs are current")
	flag.Parse()

	if *source == "" {
		*source = filepath.Join(*repoRoot, "config", "sport-taxonomy", "sports.yaml")
	}
	if *modifierSource == "" {
		*modifierSource = filepath.Join(*repoRoot, "config", "sport-taxonomy", "modifiers.yaml")
	}
	if *goOut == "" {
		*goOut = filepath.Join(*repoRoot, "services", "api", "internal", "sport", "taxonomy_generated.go")
	}
	if *tsOut == "" {
		*tsOut = filepath.Join(*repoRoot, "frontend", "src", "lib", "generatedSportTaxonomy.ts")
	}
	if *goModifiersOut == "" {
		*goModifiersOut = filepath.Join(*repoRoot, "services", "api", "internal", "sport", "modifiers_generated.go")
	}
	if *tsModifiersOut == "" {
		*tsModifiersOut = filepath.Join(*repoRoot, "frontend", "src", "lib", "generatedSportModifiers.ts")
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
	if *segmentProjectionSource == "" {
		*segmentProjectionSource = filepath.Join(*repoRoot, "config", "sport-taxonomy", "projections", "segments.yaml")
	}
	if *tsSegmentProjectionsOut == "" {
		*tsSegmentProjectionsOut = filepath.Join(*repoRoot, "frontend", "src", "lib", "generatedSegmentProjections.ts")
	}
	if *ruleProjectionSource == "" {
		*ruleProjectionSource = filepath.Join(*repoRoot, "config", "sport-taxonomy", "projections", "rules.yaml")
	}
	if *tsRuleProjectionsOut == "" {
		*tsRuleProjectionsOut = filepath.Join(*repoRoot, "frontend", "src", "lib", "generatedRuleProjections.ts")
	}
	if *recordsProjectionSource == "" {
		*recordsProjectionSource = filepath.Join(*repoRoot, "config", "sport-taxonomy", "projections", "records.yaml")
	}
	if *goRecordsProjectionOut == "" {
		*goRecordsProjectionOut = filepath.Join(*repoRoot, "services", "api", "internal", "bestefforts", "records_projection_generated.go")
	}
	if *tsRecordsProjectionOut == "" {
		*tsRecordsProjectionOut = filepath.Join(*repoRoot, "frontend", "src", "lib", "generatedRecordProjections.ts")
	}
	if *aliasSourceDir == "" {
		*aliasSourceDir = filepath.Join(*repoRoot, "config", "sport-taxonomy", "aliases")
	}
	if *goAliasesOut == "" {
		*goAliasesOut = filepath.Join(*repoRoot, "services", "api", "internal", "sport", "aliases_generated.go")
	}
	if *tsAliasesOut == "" {
		*tsAliasesOut = filepath.Join(*repoRoot, "frontend", "src", "lib", "generatedSportAliases.ts")
	}
	if *tsGearDefaultsOut == "" {
		*tsGearDefaultsOut = filepath.Join(*repoRoot, "frontend", "src", "lib", "generatedGearDefaultProjections.ts")
	}
	if *predicateValidationSQLOut == "" {
		*predicateValidationSQLOut = filepath.Join(*repoRoot, "services", "api", "internal", "sport", "predicate_validation_generated.sql")
	}

	sourceBytes, err := os.ReadFile(*source)
	if err != nil {
		return fmt.Errorf("read taxonomy source: %w", err)
	}
	taxonomy, err := sporttaxgen.LoadTaxonomy(bytes.NewReader(sourceBytes))
	if err != nil {
		return fmt.Errorf("load taxonomy: %w", err)
	}
	modifierBytes, err := os.ReadFile(*modifierSource)
	if err != nil {
		return fmt.Errorf("read modifier registry source: %w", err)
	}
	modifierRegistry, err := sporttaxgen.LoadModifiers(bytes.NewReader(modifierBytes))
	if err != nil {
		return fmt.Errorf("load modifier registry: %w", err)
	}
	renderedGoModifiers, err := sporttaxgen.RenderGoModifiers(modifierRegistry)
	if err != nil {
		return fmt.Errorf("render Go modifiers: %w", err)
	}
	renderedTSModifiers, err := sporttaxgen.RenderTSModifiers(modifierRegistry)
	if err != nil {
		return fmt.Errorf("render TypeScript modifiers: %w", err)
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
	segmentProjectionBytes, err := os.ReadFile(*segmentProjectionSource)
	if err != nil {
		return fmt.Errorf("read segment projection source: %w", err)
	}
	segmentProjection, err := sporttaxgen.LoadSegmentProjection(bytes.NewReader(segmentProjectionBytes), taxonomy)
	if err != nil {
		return fmt.Errorf("load segment projection: %w", err)
	}
	expandedSegmentProjection, err := sporttaxgen.ExpandSegmentProjection(taxonomy, segmentProjection)
	if err != nil {
		return fmt.Errorf("expand segment projection: %w", err)
	}
	ruleProjectionBytes, err := os.ReadFile(*ruleProjectionSource)
	if err != nil {
		return fmt.Errorf("read rule projection source: %w", err)
	}
	ruleProjection, err := sporttaxgen.LoadRuleProjection(bytes.NewReader(ruleProjectionBytes), taxonomy)
	if err != nil {
		return fmt.Errorf("load rule projection: %w", err)
	}
	expandedRuleProjection, err := sporttaxgen.ExpandRuleProjection(taxonomy, ruleProjection)
	if err != nil {
		return fmt.Errorf("expand rule projection: %w", err)
	}
	recordsProjectionBytes, err := os.ReadFile(*recordsProjectionSource)
	if err != nil {
		return fmt.Errorf("read records projection source: %w", err)
	}
	recordsProjection, err := sporttaxgen.LoadRecordsProjection(bytes.NewReader(recordsProjectionBytes), taxonomy)
	if err != nil {
		return fmt.Errorf("load records projection: %w", err)
	}
	expandedRecordsProjection, err := sporttaxgen.ExpandRecordsProjection(taxonomy, recordsProjection)
	if err != nil {
		return fmt.Errorf("expand records projection: %w", err)
	}
	renderedGoProjections, err := sporttaxgen.RenderGoProjections(expandedProjections, expandedSegmentProjection, expandedRuleProjection)
	if err != nil {
		return fmt.Errorf("render Go projections: %w", err)
	}
	renderedTSProjections, err := sporttaxgen.RenderTSActivityFilterProjections(expandedProjections)
	if err != nil {
		return fmt.Errorf("render TypeScript activity filter projection: %w", err)
	}
	renderedTSSegmentProjections, err := sporttaxgen.RenderTSSegmentProjection(expandedSegmentProjection)
	if err != nil {
		return fmt.Errorf("render TypeScript segment projection: %w", err)
	}
	renderedTSRuleProjections, err := sporttaxgen.RenderTSRuleProjection(expandedRuleProjection)
	if err != nil {
		return fmt.Errorf("render TypeScript rule projection: %w", err)
	}
	renderedGoRecordsProjection, err := sporttaxgen.RenderGoRecordsProjection(expandedRecordsProjection)
	if err != nil {
		return fmt.Errorf("render Go records projection: %w", err)
	}
	renderedTSRecordsProjection, err := sporttaxgen.RenderTSRecordsProjection(expandedRecordsProjection)
	if err != nil {
		return fmt.Errorf("render TypeScript records projection: %w", err)
	}
	renderedGoAliases, renderedTSAliases, renderedTSGearDefaults, aliasesExist, err := renderAliasesFromDir(*aliasSourceDir, taxonomy)
	if err != nil {
		return err
	}
	renderedPredicateValidationSQL, err := sporttaxgen.RenderPredicateValidationSQL(taxonomy, expandedSegmentProjection)
	if err != nil {
		return fmt.Errorf("render predicate validation SQL: %w", err)
	}

	if *check {
		outputs := []generatedOutput{
			{path: *goOut, want: renderedGo},
			{path: *tsOut, want: renderedTS},
			{path: *goModifiersOut, want: renderedGoModifiers},
			{path: *tsModifiersOut, want: renderedTSModifiers},
			{path: *goProjectionsOut, want: renderedGoProjections},
			{path: *tsProjectionsOut, want: renderedTSProjections},
			{path: *tsSegmentProjectionsOut, want: renderedTSSegmentProjections},
			{path: *tsRuleProjectionsOut, want: renderedTSRuleProjections},
			{path: *goRecordsProjectionOut, want: renderedGoRecordsProjection},
			{path: *tsRecordsProjectionOut, want: renderedTSRecordsProjection},
		}
		if !aliasesExist {
			return fmt.Errorf("source alias directory %s does not exist; generated alias artifacts cannot be checked", *aliasSourceDir)
		}
		outputs = append(outputs,
			generatedOutput{path: *goAliasesOut, want: renderedGoAliases},
			generatedOutput{path: *tsAliasesOut, want: renderedTSAliases},
			generatedOutput{path: *tsGearDefaultsOut, want: renderedTSGearDefaults},
			generatedOutput{path: *predicateValidationSQLOut, want: renderedPredicateValidationSQL},
		)
		if err := checkGeneratedFiles(outputs); err != nil {
			return err
		}
		return nil
	}
	if err := os.WriteFile(*goOut, renderedGo, 0o644); err != nil {
		return fmt.Errorf("write Go taxonomy: %w", err)
	}
	if err := os.WriteFile(*tsOut, renderedTS, 0o644); err != nil {
		return fmt.Errorf("write TypeScript taxonomy: %w", err)
	}
	if err := os.WriteFile(*goModifiersOut, renderedGoModifiers, 0o644); err != nil {
		return fmt.Errorf("write Go modifiers: %w", err)
	}
	if err := os.WriteFile(*tsModifiersOut, renderedTSModifiers, 0o644); err != nil {
		return fmt.Errorf("write TypeScript modifiers: %w", err)
	}
	if err := os.WriteFile(*goProjectionsOut, renderedGoProjections, 0o644); err != nil {
		return fmt.Errorf("write Go activity filter projection: %w", err)
	}
	if err := os.WriteFile(*tsProjectionsOut, renderedTSProjections, 0o644); err != nil {
		return fmt.Errorf("write TypeScript activity filter projection: %w", err)
	}
	if err := os.WriteFile(*tsSegmentProjectionsOut, renderedTSSegmentProjections, 0o644); err != nil {
		return fmt.Errorf("write TypeScript segment projection: %w", err)
	}
	if err := os.WriteFile(*tsRuleProjectionsOut, renderedTSRuleProjections, 0o644); err != nil {
		return fmt.Errorf("write TypeScript rule projection: %w", err)
	}
	if err := os.WriteFile(*goRecordsProjectionOut, renderedGoRecordsProjection, 0o644); err != nil {
		return fmt.Errorf("write Go records projection: %w", err)
	}
	if err := os.WriteFile(*tsRecordsProjectionOut, renderedTSRecordsProjection, 0o644); err != nil {
		return fmt.Errorf("write TypeScript records projection: %w", err)
	}
	if !aliasesExist {
		return fmt.Errorf("source alias directory %s does not exist; generated alias artifacts cannot be written", *aliasSourceDir)
	}
	if err := os.WriteFile(*goAliasesOut, renderedGoAliases, 0o644); err != nil {
		return fmt.Errorf("write Go source aliases: %w", err)
	}
	if err := os.WriteFile(*tsAliasesOut, renderedTSAliases, 0o644); err != nil {
		return fmt.Errorf("write TypeScript source aliases: %w", err)
	}
	if err := os.WriteFile(*tsGearDefaultsOut, renderedTSGearDefaults, 0o644); err != nil {
		return fmt.Errorf("write TypeScript gear default projections: %w", err)
	}
	if err := os.WriteFile(*predicateValidationSQLOut, renderedPredicateValidationSQL, 0o644); err != nil {
		return fmt.Errorf("write predicate validation SQL: %w", err)
	}
	return nil
}

func renderAliasesFromDir(dir string, taxonomy sporttaxgen.Taxonomy) ([]byte, []byte, []byte, bool, error) {
	if _, err := os.Stat(dir); err != nil {
		if os.IsNotExist(err) {
			return nil, nil, nil, false, nil
		}
		return nil, nil, nil, false, fmt.Errorf("stat source alias directory: %w", err)
	}
	matches, err := filepath.Glob(filepath.Join(dir, "*.yaml"))
	if err != nil {
		return nil, nil, nil, false, fmt.Errorf("glob source alias files: %w", err)
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
			return nil, nil, nil, false, fmt.Errorf("open source alias file %s: %w", match, err)
		}
		opened = append(opened, file)
		stem := strings.TrimSuffix(filepath.Base(match), filepath.Ext(match))
		files[stem] = file
	}
	sources, err := sporttaxgen.LoadSourceAliasFiles(files, taxonomy)
	if err != nil {
		return nil, nil, nil, false, fmt.Errorf("load source aliases: %w", err)
	}
	gearDefaults, err := sporttaxgen.BuildGearDefaultProjection(taxonomy, sources)
	if err != nil {
		return nil, nil, nil, false, fmt.Errorf("build gear default projection: %w", err)
	}
	renderedGo, err := sporttaxgen.RenderGoSourceAliasesWithGearDefaults(sources, gearDefaults)
	if err != nil {
		return nil, nil, nil, false, fmt.Errorf("render Go source aliases: %w", err)
	}
	renderedTSAliases, err := sporttaxgen.RenderTSSourceAliases(sources)
	if err != nil {
		return nil, nil, nil, false, fmt.Errorf("render TypeScript source aliases: %w", err)
	}
	renderedTSGearDefaults, err := sporttaxgen.RenderTSGearDefaultProjection(gearDefaults)
	if err != nil {
		return nil, nil, nil, false, fmt.Errorf("render TypeScript gear default projection: %w", err)
	}
	return renderedGo, renderedTSAliases, renderedTSGearDefaults, true, nil
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
		return fmt.Errorf("%s is stale; run scripts/generate-sport-taxonomy.sh\n%s", path, unifiedGeneratedDiff(path, got, want))
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
