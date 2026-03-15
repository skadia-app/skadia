// services/api/internal/osm/osmium.go
package osm

import (
	"context"
	_ "embed"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

//go:embed osmium_filter.txt
var osmiumFilterExpressions string

// OsmiumPreprocess runs osmium tags-filter on each PBF, then merges them.
// Returns the path to the merged+filtered PBF, or "" if osmium is not available (fallback).
// Filtered PBFs are cached alongside raw PBFs — re-filtering is skipped if newer.
func OsmiumPreprocess(ctx context.Context, dataDir string, rawPBFs []string, reporter ProgressReporter) (string, error) {
	if _, err := exec.LookPath("osmium"); err != nil {
		slog.Warn("osm import: osmium not found, skipping preprocessing")
		return "", nil
	}

	filterFile := filepath.Join(dataDir, "skadia-filter.txt")
	if err := os.WriteFile(filterFile, []byte(osmiumFilterExpressions), 0644); err != nil {
		return "", fmt.Errorf("osmium: write filter file: %w", err)
	}

	reporter.SetPhase("filter")
	reporter.SetProgress(0)

	var filteredPBFs []string
	for i, rawPBF := range rawPBFs {
		if err := ctx.Err(); err != nil {
			return "", err
		}

		filteredPBF := strings.TrimSuffix(rawPBF, ".osm.pbf") + "-filtered.osm.pbf"

		rawInfo, _ := os.Stat(rawPBF)
		filtInfo, filtErr := os.Stat(filteredPBF)
		if filtErr == nil && rawInfo != nil && filtInfo.ModTime().After(rawInfo.ModTime()) {
			slog.Info("osm import: reusing filtered PBF", "path", filepath.Base(filteredPBF))
			filteredPBFs = append(filteredPBFs, filteredPBF)
			reporter.SetProgress(int64(i + 1))
			continue
		}

		slog.Info("osm import: filtering PBF", "input", filepath.Base(rawPBF))
		cmd := exec.CommandContext(ctx, "osmium", "tags-filter",
			rawPBF,
			"--expressions", filterFile,
			"-o", filteredPBF,
			"--overwrite",
			"-f", "pbf,compress=lz4",
		)
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return "", fmt.Errorf("osmium tags-filter %s: %w", filepath.Base(rawPBF), err)
		}

		filteredPBFs = append(filteredPBFs, filteredPBF)
		reporter.SetProgress(int64(i + 1))
	}

	if len(filteredPBFs) == 1 {
		return filteredPBFs[0], nil
	}

	reporter.SetPhase("merge")
	reporter.SetProgress(0)
	mergedPBF := filepath.Join(dataDir, "import-ready.osm.pbf")
	slog.Info("osm import: merging filtered PBFs", "count", len(filteredPBFs))

	args := []string{"merge"}
	args = append(args, filteredPBFs...)
	args = append(args, "-o", mergedPBF, "--overwrite", "-f", "pbf,compress=lz4")

	cmd := exec.CommandContext(ctx, "osmium", args...)
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("osmium merge: %w", err)
	}

	reporter.SetProgress(1)
	return mergedPBF, nil
}

// CleanupMergedPBF removes the ephemeral merged PBF after import.
func CleanupMergedPBF(dataDir string) {
	os.Remove(filepath.Join(dataDir, "import-ready.osm.pbf"))
}
