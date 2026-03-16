package osm

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const brouterSegmentsBaseURL = "https://brouter.de/brouter/segments4"

// BRouterTile represents a 5x5 degree tile in BRouter's naming convention.
// Example: E005N45.rd5 covers lon 5..10, lat 45..50.
type BRouterTile struct {
	LonStart int // Western edge in degrees (can be negative for W tiles)
	LatStart int // Southern edge in degrees (can be negative for S tiles)
}

// Filename returns the BRouter rd5 filename for this tile.
// Convention: E/W prefix + 3-digit abs(lon) + N/S prefix + 2-digit abs(lat)
func (t BRouterTile) Filename() string {
	var lonPrefix string
	lonAbs := t.LonStart
	if lonAbs >= 0 {
		lonPrefix = "E"
	} else {
		lonPrefix = "W"
		lonAbs = -lonAbs
	}
	var latPrefix string
	latAbs := t.LatStart
	if latAbs >= 0 {
		latPrefix = "N"
	} else {
		latPrefix = "S"
		latAbs = -latAbs
	}
	return fmt.Sprintf("%s%03d%s%02d.rd5", lonPrefix, lonAbs, latPrefix, latAbs)
}

// URL returns the full download URL for this tile.
func (t BRouterTile) URL() string {
	return brouterSegmentsBaseURL + "/" + t.Filename()
}

// floorTo5 rounds down to the nearest multiple of 5.
func floorTo5(v float64) int {
	return int(math.Floor(v/5)) * 5
}

// ComputeBRouterTiles returns the set of BRouter 5x5-degree tiles that cover
// the bounding boxes of the given regions. Duplicates are removed.
func ComputeBRouterTiles(regions []Region) []BRouterTile {
	seen := make(map[BRouterTile]bool)
	var tiles []BRouterTile

	for _, reg := range regions {
		// Skip regions without bbox (should not happen, but be safe)
		if reg.MinLon == 0 && reg.MaxLon == 0 && reg.MinLat == 0 && reg.MaxLat == 0 {
			continue
		}

		lonStart := floorTo5(reg.MinLon)
		lonEnd := floorTo5(reg.MaxLon)
		latStart := floorTo5(reg.MinLat)
		latEnd := floorTo5(reg.MaxLat)

		for lon := lonStart; lon <= lonEnd; lon += 5 {
			for lat := latStart; lat <= latEnd; lat += 5 {
				tile := BRouterTile{LonStart: lon, LatStart: lat}
				if !seen[tile] {
					seen[tile] = true
					tiles = append(tiles, tile)
				}
			}
		}
	}

	// Sort for deterministic output
	sort.Slice(tiles, func(i, j int) bool {
		if tiles[i].LonStart != tiles[j].LonStart {
			return tiles[i].LonStart < tiles[j].LonStart
		}
		return tiles[i].LatStart < tiles[j].LatStart
	})

	return tiles
}

// DownloadBRouterSegments downloads pre-built rd5 segment files from brouter.de
// for the given regions. Already existing files are skipped. Non-fatal: individual
// tile failures are logged but don't abort the process (ocean tiles don't exist).
// Returns (downloaded count, skipped count, error count).
func (p *Pipeline) DownloadBRouterSegments(ctx context.Context, regions []Region, segmentsDir string, reporter ProgressReporter) (int, int, int) {
	tiles := ComputeBRouterTiles(regions)
	if len(tiles) == 0 {
		slog.Warn("brouter: no tiles computed for regions (missing bbox data)")
		return 0, 0, 0
	}

	if err := os.MkdirAll(segmentsDir, 0755); err != nil {
		slog.Error("brouter: failed to create segments dir", "dir", segmentsDir, "error", err)
		return 0, 0, 0
	}

	var tileNames []string
	for _, t := range tiles {
		tileNames = append(tileNames, t.Filename())
	}
	slog.Info("brouter: downloading segments", "count", len(tiles), "tiles", strings.Join(tileNames, ", "), "dir", segmentsDir)

	downloaded, skipped, errors := 0, 0, 0

	for i, tile := range tiles {
		if ctx.Err() != nil {
			slog.Warn("brouter: download cancelled")
			break
		}

		reporter.SetProgress(int64(i))
		destPath := filepath.Join(segmentsDir, tile.Filename())

		// Skip if already exists and has nonzero size
		if info, err := os.Stat(destPath); err == nil && info.Size() > 0 {
			slog.Debug("brouter: segment already exists", "file", tile.Filename(), "size", info.Size())
			skipped++
			continue
		}

		if err := downloadBRouterTile(ctx, tile, destPath); err != nil {
			slog.Warn("brouter: tile download failed (non-fatal, may be ocean tile)",
				"tile", tile.Filename(), "error", err)
			errors++
			// Remove partial download
			os.Remove(destPath)
			continue
		}

		downloaded++
	}

	reporter.SetProgress(int64(len(tiles)))
	slog.Info("brouter: segment download complete",
		"downloaded", downloaded, "skipped", skipped, "errors", errors)

	return downloaded, skipped, errors
}

// downloadBRouterTile downloads a single rd5 tile from brouter.de.
func downloadBRouterTile(ctx context.Context, tile BRouterTile, destPath string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, tile.URL(), nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("download: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return fmt.Errorf("not found (ocean/empty tile)")
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	// Write to temp file then rename for atomicity
	tmpPath := destPath + ".tmp"
	f, err := os.Create(tmpPath)
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}

	n, err := io.Copy(f, resp.Body)
	if closeErr := f.Close(); closeErr != nil && err == nil {
		err = closeErr
	}
	if err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("write: %w", err)
	}

	if err := os.Rename(tmpPath, destPath); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("rename: %w", err)
	}

	slog.Info("brouter: downloaded segment", "file", tile.Filename(), "size_mb", n/1024/1024)
	return nil
}
