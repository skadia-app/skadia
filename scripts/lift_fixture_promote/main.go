package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/enrichment/lifts"
	"github.com/skadia-app/skadia/services/api/internal/fitimport"
	"github.com/skadia-app/skadia/services/api/internal/gpximport"
	"gopkg.in/yaml.v3"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("subcommand required: extract or resynthesize")
	}
	switch args[0] {
	case "extract":
		return runExtract(args[1:])
	case "resynthesize":
		return runResynthesize(args[1:])
	default:
		return fmt.Errorf("unknown subcommand %q", args[0])
	}
}

func runExtract(args []string) error {
	fs := flag.NewFlagSet("extract", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	input := fs.String("input", "", "source activity file")
	windowFrom := fs.String("window-from", "", "window start")
	windowTo := fs.String("window-to", "", "window end")
	output := fs.String("output", "", "raw window output")
	localRoot := fs.String("local-root", "", "local-only output root")
	gtLifts := fs.String("gt-lifts", "", "local ground-truth lift coordinates")
	gtFile := fs.String("gt-file", "", "ground-truth case file key")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *input == "" || *windowFrom == "" || *windowTo == "" || *output == "" {
		return fmt.Errorf("extract requires --input, --window-from, --window-to, and --output")
	}
	roots := configuredLocalRoots(*localRoot)
	if err := ValidateExtractOutputPath(*output, roots); err != nil {
		return err
	}
	from, err := time.Parse(time.RFC3339, *windowFrom)
	if err != nil {
		return fmt.Errorf("parse --window-from: %w", err)
	}
	to, err := time.Parse(time.RFC3339, *windowTo)
	if err != nil {
		return fmt.Errorf("parse --window-to: %w", err)
	}
	if to.Before(from) {
		return fmt.Errorf("--window-to must be after --window-from")
	}
	rows, err := loadSourcePoints(*input)
	if err != nil {
		return err
	}
	raw, err := extractRawWindow(rows, from, to)
	if err != nil {
		return err
	}
	if *gtLifts != "" {
		key := *gtFile
		if key == "" {
			key = filepath.Base(*input)
		}
		anchors, err := loadStationAnchors(*gtLifts, key)
		if err != nil {
			return err
		}
		raw.StationAnchors = anchors
	}
	data, err := json.MarshalIndent(raw, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(*output), 0o700); err != nil {
		return err
	}
	if err := os.WriteFile(*output, append(data, '\n'), 0o600); err != nil {
		return err
	}
	return writeSourceMapping(*output, *input)
}

func runResynthesize(args []string) error {
	fs := flag.NewFlagSet("resynthesize", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	input := fs.String("input", "", "raw window input")
	outputCanonical := fs.String("output-canonical", "", "canonical fixture output")
	outputGT := fs.String("output-gt", "", "ground-truth output")
	scenarioKind := fs.String("scenario-kind", "", "scenario kind")
	description := fs.String("description", "", "generic description")
	seed := fs.String("seed", "", "repo seed hex")
	privacyList := fs.String("privacy-list", "", "privacy replacement list")
	toleranceSpec := fs.String("tolerance-spec", "", "tolerance spec")
	traceDiff := fs.Bool("trace-diff", false, "show debug trace diff")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *input == "" || *outputCanonical == "" || *outputGT == "" || *scenarioKind == "" {
		return fmt.Errorf("resynthesize requires --input, --output-canonical, --output-gt, and --scenario-kind")
	}
	repoSeed := strings.TrimSpace(*seed)
	if repoSeed == "" {
		var err error
		repoSeed, err = defaultRepoSeed()
		if err != nil {
			return err
		}
	}
	data, err := os.ReadFile(*input)
	if err != nil {
		return err
	}
	var raw RawWindow
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	var tolerance RideTolerance
	if *toleranceSpec != "" {
		tolerance, err = LoadToleranceSpec(*toleranceSpec)
		if err != nil {
			return err
		}
	}
	var traceWriter *os.File
	if *traceDiff {
		traceWriter = os.Stderr
	}
	fixture, gt, err := ResynthesizeWindow(raw, ResynthesizeOptions{
		RepoSeedHex:  repoSeed,
		ScenarioKind: *scenarioKind,
		Description:  *description,
		Tolerance:    tolerance,
		TraceWriter:  traceWriter,
	})
	if err != nil {
		return err
	}
	if err := PrivacySelfCheck(*privacyList, fixture, gt); err != nil {
		return err
	}
	canonicalBytes, err := json.MarshalIndent(fixture, "", "  ")
	if err != nil {
		return err
	}
	gtBytes, err := yaml.Marshal(gt)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(*outputCanonical), 0o755); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(*outputGT), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(*outputCanonical, append(canonicalBytes, '\n'), 0o644); err != nil {
		return err
	}
	if err := os.WriteFile(*outputGT, gtBytes, 0o644); err != nil {
		return err
	}
	return nil
}

func loadSourcePoints(path string) ([]lifts.StreamPoint, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	switch strings.ToLower(filepath.Ext(path)) {
	case ".gpx":
		data, err := gpximport.ParseGPX(f, filepath.Base(path))
		if err != nil {
			return nil, fmt.Errorf("parse gpx: %w", err)
		}
		return lifts.ConvertActivityDataToStream(data), nil
	case ".fit":
		data, err := fitimport.ParseFIT(f, filepath.Base(path))
		if err != nil {
			return nil, fmt.Errorf("parse fit: %w", err)
		}
		return lifts.ConvertActivityDataToStream(data), nil
	default:
		return nil, fmt.Errorf("unsupported source extension %q", filepath.Ext(path))
	}
}

func extractRawWindow(rows []lifts.StreamPoint, from, to time.Time) (RawWindow, error) {
	var samples []RawSample
	var first time.Time
	for _, row := range rows {
		if row.Time.Before(from) || row.Time.After(to) {
			continue
		}
		if first.IsZero() {
			first = row.Time
		}
		samples = append(samples, RawSample{
			DtMS:      int64(row.Time.Sub(first) / time.Millisecond),
			Lat:       copyFloat(row.Lat),
			Lng:       copyFloat(row.Lng),
			AltM:      copyFloat(row.AltitudeM),
			SpeedMS:   copyFloat(row.SpeedMs),
			DistanceM: copyFloat(row.DistanceM),
		})
	}
	if len(samples) == 0 {
		return RawWindow{}, fmt.Errorf("window contains no samples")
	}
	return RawWindow{FormatVersion: FormatVersion, Samples: samples}, nil
}

type gtLiftFile struct {
	Cases []gtCase `yaml:"cases"`
}

type gtCase struct {
	File  string   `yaml:"file"`
	Lifts []gtLift `yaml:"lifts"`
}

type gtLift struct {
	ID string     `yaml:"id"`
	P1 [2]float64 `yaml:"p1"`
	P2 [2]float64 `yaml:"p2"`
}

func loadStationAnchors(path, fileKey string) ([]StationAnchor, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var gt gtLiftFile
	if err := yaml.Unmarshal(data, &gt); err != nil {
		return nil, err
	}
	for _, c := range gt.Cases {
		if c.File != fileKey {
			continue
		}
		if len(c.Lifts) == 0 {
			return nil, fmt.Errorf("ground-truth case %q has no lift coordinates", fileKey)
		}
		anchors := make([]StationAnchor, 0, len(c.Lifts)*2)
		for i, lift := range c.Lifts {
			if lift.ID == "" {
				lift.ID = fmt.Sprintf("%d", i)
			}
			anchors = append(anchors,
				StationAnchor{Name: fmt.Sprintf("lift_%02d_p1", i), Lat: lift.P1[0], Lng: lift.P1[1]},
				StationAnchor{Name: fmt.Sprintf("lift_%02d_p2", i), Lat: lift.P2[0], Lng: lift.P2[1]},
			)
		}
		return anchors, nil
	}
	return nil, fmt.Errorf("ground-truth case %q not found", fileKey)
}

func configuredLocalRoots(flagRoot string) []string {
	var roots []string
	if flagRoot != "" {
		roots = append(roots, flagRoot)
	}
	if env := os.Getenv("SKADIA_LIFT_PROMOTE_LOCAL"); env != "" {
		roots = append(roots, filepath.SplitList(env)...)
	}
	return roots
}

func writeSourceMapping(outputPath, inputPath string) error {
	mappingPath := filepath.Join(filepath.Dir(outputPath), "source_mapping.json")
	mapping := map[string]map[string]string{}
	if data, err := os.ReadFile(mappingPath); err == nil {
		_ = json.Unmarshal(data, &mapping)
	}
	mapping[filepath.Base(outputPath)] = map[string]string{
		"source_fingerprint_hash": sourceFingerprint(inputPath),
	}
	data, err := json.MarshalIndent(mapping, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(mappingPath, append(data, '\n'), 0o600)
}

func sourceFingerprint(inputPath string) string {
	seed := os.Getenv("SKADIA_ANON_SOURCE_SEED")
	if seed == "" {
		seedPath := filepath.Join(os.Getenv("HOME"), ".config", "skadia", "anon-source-seed")
		if data, err := os.ReadFile(seedPath); err == nil {
			seed = strings.TrimSpace(string(data))
		}
	}
	if seed == "" {
		return ""
	}
	info, err := os.Stat(inputPath)
	if err != nil {
		return ""
	}
	h := sha256.New()
	h.Write([]byte(seed))
	h.Write([]byte(filepath.Clean(inputPath)))
	h.Write([]byte(fmt.Sprintf("%d", info.ModTime().UnixNano())))
	sum := h.Sum(nil)
	return hex.EncodeToString(sum)
}

func defaultRepoSeed() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		candidate := filepath.Join(dir, "services", "api", "internal", "enrichment", "lifts", "testdata", ".fixture-seed")
		if data, err := os.ReadFile(candidate); err == nil {
			return strings.TrimSpace(string(data)), nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("repo seed not found; pass --seed")
		}
		dir = parent
	}
}
