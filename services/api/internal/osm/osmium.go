// services/api/internal/osm/osmium.go
package osm

import (
	"bufio"
	"context"
	_ "embed"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
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

// ErrTooOld is returned by patchPBF when the PBF is more than 90 diffs behind
// the replication server, making incremental update impractical.
var ErrTooOld = fmt.Errorf("PBF too old for incremental update (>90 days)")

// extractReplicationInfo reads the replication base URL and sequence number
// from a PBF file's header using osmium fileinfo.
// Returns empty url and seq=0 (no error) if the PBF has no replication info.
func extractReplicationInfo(pbfPath string) (url string, seq int, err error) {
	cmd := exec.Command("osmium", "fileinfo", "-e", "-g",
		"header.option.osmosis_replication_base_url", pbfPath)
	out, cmdErr := cmd.Output()
	if cmdErr != nil {
		return "", 0, nil // PBF without replication info — not an error
	}
	url = strings.TrimSpace(string(out))

	cmd = exec.Command("osmium", "fileinfo", "-e", "-g",
		"header.option.osmosis_replication_sequence_number", pbfPath)
	out, cmdErr = cmd.Output()
	if cmdErr != nil {
		return url, 0, nil
	}
	seq, _ = strconv.Atoi(strings.TrimSpace(string(out)))
	return url, seq, nil
}

// extractReplicationTimestamp reads the OSM data date from a PBF header.
// Returns nil (no error) if the timestamp is absent or unparseable.
func extractReplicationTimestamp(pbfPath string) (*time.Time, error) {
	cmd := exec.Command("osmium", "fileinfo", "-e", "-g",
		"header.option.osmosis_replication_timestamp", pbfPath)
	out, err := cmd.Output()
	if err != nil {
		return nil, nil
	}
	ts := strings.TrimSpace(string(out))
	if ts == "" {
		return nil, nil
	}
	t, parseErr := time.Parse(time.RFC3339, ts)
	if parseErr != nil {
		t, parseErr = time.Parse("2006-01-02T15:04:05Z", ts)
		if parseErr != nil {
			return nil, nil
		}
	}
	return &t, nil
}

// formatSequence converts an integer sequence number to GeoFabrik's
// three-level directory path format, e.g. 4722 → "000/004/722".
func formatSequence(seq int) string {
	a := seq / 1_000_000
	b := (seq / 1_000) % 1_000
	c := seq % 1_000
	return fmt.Sprintf("%03d/%03d/%03d", a, b, c)
}

// fetchCurrentSequence retrieves the latest sequence number from a
// replication server by fetching its state.txt file.
func fetchCurrentSequence(ctx context.Context, replicationURL string) (int, error) {
	stateURL := strings.TrimRight(replicationURL, "/") + "/state.txt"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, stateURL, nil)
	if err != nil {
		return 0, fmt.Errorf("fetchCurrentSequence: build request: %w", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, fmt.Errorf("fetchCurrentSequence: GET %s: %w", stateURL, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("fetchCurrentSequence: HTTP %d for %s", resp.StatusCode, stateURL)
	}

	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if after, ok := strings.CutPrefix(line, "sequenceNumber="); ok {
			val := after
			seq, err := strconv.Atoi(strings.TrimSpace(val))
			if err != nil {
				return 0, fmt.Errorf("fetchCurrentSequence: parse sequenceNumber %q: %w", val, err)
			}
			return seq, nil
		}
	}
	if err := scanner.Err(); err != nil {
		return 0, fmt.Errorf("fetchCurrentSequence: read body: %w", err)
	}
	return 0, fmt.Errorf("fetchCurrentSequence: sequenceNumber not found in state.txt")
}

// patchPBF applies incremental OSM diffs (.osc.gz) to pbfPath, bringing it
// up to date with the replication server at replicationURL.
//
// lastSeq is the sequence number already embedded in the PBF header.
// Returns the new sequence number on success.
// Returns ErrTooOld if the gap exceeds 90 diffs (replication servers typically
// keep ~90 days of daily diffs).
func patchPBF(ctx context.Context, pbfPath, replicationURL string, lastSeq int) (int, error) {
	currentSeq, err := fetchCurrentSequence(ctx, replicationURL)
	if err != nil {
		return 0, fmt.Errorf("patchPBF: fetch current sequence: %w", err)
	}

	if currentSeq <= lastSeq {
		slog.Info("osm patch: PBF already up to date", "seq", lastSeq)
		return lastSeq, nil
	}

	gap := currentSeq - lastSeq
	if gap > 90 {
		return 0, ErrTooOld
	}

	slog.Info("osm patch: applying diffs", "from_seq", lastSeq, "to_seq", currentSeq, "count", gap)

	// Download all required .osc.gz diffs into a temp directory
	tmpDir, err := os.MkdirTemp("", "skadia-osm-patch-*")
	if err != nil {
		return 0, fmt.Errorf("patchPBF: create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	var diffPaths []string
	for seq := lastSeq + 1; seq <= currentSeq; seq++ {
		if err := ctx.Err(); err != nil {
			return 0, err
		}
		diffURL := strings.TrimRight(replicationURL, "/") + "/" + formatSequence(seq) + ".osc.gz"
		diffPath := filepath.Join(tmpDir, fmt.Sprintf("%09d.osc.gz", seq))
		slog.Debug("osm patch: downloading diff", "seq", seq, "url", diffURL)
		if err := downloadWithProgress(ctx, diffURL, diffPath, func(_, _ int64) {}); err != nil {
			return 0, fmt.Errorf("patchPBF: download diff seq %d: %w", seq, err)
		}
		diffPaths = append(diffPaths, diffPath)
	}

	// Build the output path alongside the original PBF
	dir := filepath.Dir(pbfPath)
	base := strings.TrimSuffix(filepath.Base(pbfPath), ".osm.pbf")
	newPBFPath := filepath.Join(dir, base+"-patched.osm.pbf")

	// osmium apply-changes: pbfPath + all diffs → newPBFPath
	args := []string{"apply-changes",
		pbfPath,
		"-o", newPBFPath,
		"--overwrite",
		"-f", "pbf,compress=lz4",
	}
	args = append(args, diffPaths...)

	cmd := exec.CommandContext(ctx, "osmium", args...)
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return 0, fmt.Errorf("patchPBF: osmium apply-changes: %w", err)
	}

	// Atomically replace the original PBF
	if err := os.Rename(newPBFPath, pbfPath); err != nil {
		return 0, fmt.Errorf("patchPBF: rename patched PBF: %w", err)
	}

	slog.Info("osm patch: PBF updated", "path", filepath.Base(pbfPath), "new_seq", currentSeq)
	return currentSeq, nil
}
