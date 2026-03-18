package osm

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"sync/atomic"
	"time"

	"golang.org/x/sync/errgroup"
)

var errNotFound = errors.New("not found")

const brouterSegmentsURL = "https://brouter.de/brouter/segments4"

// BRouterTileIDs computes the 5x5 degree BRouter tile IDs that cover the given regions.
func BRouterTileIDs(regions []Region) []string {
	seen := make(map[string]bool)
	var tiles []string
	for _, reg := range regions {
		minLon := int(math.Floor(reg.MinLon/5)) * 5
		minLat := int(math.Floor(reg.MinLat/5)) * 5
		maxLon := int(math.Ceil(reg.MaxLon/5)) * 5
		maxLat := int(math.Ceil(reg.MaxLat/5)) * 5
		for lon := minLon; lon < maxLon; lon += 5 {
			for lat := minLat; lat < maxLat; lat += 5 {
				id := formatBRouterTileID(lon, lat)
				if !seen[id] {
					seen[id] = true
					tiles = append(tiles, id)
				}
			}
		}
	}
	return tiles
}

func formatBRouterTileID(lon, lat int) string {
	lonPrefix := "E"
	if lon < 0 {
		lonPrefix = "W"
		lon = -lon
	}
	latPrefix := "N"
	if lat < 0 {
		latPrefix = "S"
		lat = -lat
	}
	return fmt.Sprintf("%s%d_%s%d", lonPrefix, lon, latPrefix, lat)
}

// DownloadBRouterSegments downloads rd5 segment files from brouter.de.
func DownloadBRouterSegments(ctx context.Context, tiles []string, outputDir string,
	progressFn func(downloaded, total int)) error {

	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", outputDir, err)
	}

	total := len(tiles)
	var downloaded, skipped, failed atomic.Int32

	g, gCtx := errgroup.WithContext(ctx)
	g.SetLimit(3)

	for _, tileID := range tiles {
		g.Go(func() error {
			if gCtx.Err() != nil {
				return gCtx.Err()
			}
			outPath := filepath.Join(outputDir, tileID+".rd5")
			if info, err := os.Stat(outPath); err == nil && info.Size() > 0 {
				skipped.Add(1)
				if progressFn != nil {
					progressFn(int(downloaded.Load()+skipped.Load()), total)
				}
				return nil
			}
			url := fmt.Sprintf("%s/%s.rd5", brouterSegmentsURL, tileID)
			if err := downloadFile(gCtx, url, outPath); err != nil {
				slog.Warn("brouter download: tile failed", "tile", tileID, "error", err)
				failed.Add(1)
				if progressFn != nil {
					progressFn(int(downloaded.Load()+skipped.Load()+failed.Load()), total)
				}
				return nil
			}
			downloaded.Add(1)
			if progressFn != nil {
				progressFn(int(downloaded.Load()+skipped.Load()), total)
			}
			return nil
		})
	}

	if err := g.Wait(); err != nil {
		return err
	}
	slog.Info("brouter download: complete",
		"downloaded", downloaded.Load(), "skipped", skipped.Load(), "failed", failed.Load(), "total", total)
	return nil
}

func downloadFile(ctx context.Context, url, outPath string) error {
	const maxRetries = 2
	var lastErr error
	for range maxRetries {
		if err := downloadFileOnce(ctx, url, outPath); err != nil {
			lastErr = err
			if errors.Is(err, errNotFound) {
				return err // don't retry 404
			}
			time.Sleep(5 * time.Second)
			continue
		}
		return nil
	}
	return lastErr
}

func downloadFileOnce(ctx context.Context, url, outPath string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return fmt.Errorf("%w: %s", errNotFound, url)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, url)
	}
	tmpPath := outPath + ".tmp"
	f, err := os.Create(tmpPath)
	if err != nil {
		return err
	}
	if _, err := io.Copy(f, resp.Body); err != nil {
		f.Close()
		os.Remove(tmpPath)
		return err
	}
	f.Close()
	return os.Rename(tmpPath, outPath)
}
