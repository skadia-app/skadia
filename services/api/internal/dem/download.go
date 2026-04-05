package dem

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sync/atomic"
	"syscall"

	"golang.org/x/sync/errgroup"
)

// DEMSourceConfig describes a downloadable DEM source.
type DEMSourceConfig struct {
	ID          string // "glo30"
	Label       string // "Copernicus GLO-30"
	ResolutionM int    // 30
	BaseURL     string // S3 base
	OutputDir   string // "/data/dem/glo30"
	TempDir     string // "/data/dem/glo30_tiles" (GeoTIFF cache, cleaned after convert)
}

// DefaultGLO30 returns the Copernicus GLO-30 configuration.
func DefaultGLO30(dataDir string) DEMSourceConfig {
	return DEMSourceConfig{
		ID:          "glo30",
		Label:       "Copernicus GLO-30",
		ResolutionM: 30,
		BaseURL:     "https://copernicus-dem-30m.s3.eu-central-1.amazonaws.com",
		OutputDir:   filepath.Join(dataDir, "glo30"),
		TempDir:     filepath.Join(dataDir, "glo30_tiles"),
	}
}

// CopernicusURL builds the S3 URL for a given HGT cell.
func CopernicusURL(baseURL string, cell HGTCell) string {
	latPrefix := "N"
	latAbs := cell.Lat
	if cell.Lat < 0 {
		latPrefix = "S"
		latAbs = -cell.Lat
	}
	lonPrefix := "E"
	lonAbs := cell.Lon
	if cell.Lon < 0 {
		lonPrefix = "W"
		lonAbs = -cell.Lon
	}
	name := fmt.Sprintf("Copernicus_DSM_COG_10_%s%02d_00_%s%03d_00_DEM",
		latPrefix, latAbs, lonPrefix, lonAbs)
	return fmt.Sprintf("%s/%s/%s.tif", baseURL, name, name)
}

// DownloadResult summarizes a DEM download job.
type DownloadResult struct {
	Downloaded int
	Skipped    int // already existed
	OceanSkip  int // 404 = ocean tile
	Failed     int
}

// DownloadMissingTiles downloads and converts missing HGT tiles for the given cells.
// progress is called after each tile with (completed, total).
func DownloadMissingTiles(ctx context.Context, cfg DEMSourceConfig, cells []HGTCell,
	progress func(completed, total int64)) (DownloadResult, error) {

	if err := os.MkdirAll(cfg.OutputDir, 0755); err != nil {
		return DownloadResult{}, fmt.Errorf("create output dir: %w", err)
	}
	if err := os.MkdirAll(cfg.TempDir, 0755); err != nil {
		return DownloadResult{}, fmt.Errorf("create temp dir: %w", err)
	}

	// Filter to missing tiles only
	var missing []HGTCell
	skipped := 0
	for _, c := range cells {
		hgtPath := filepath.Join(cfg.OutputDir, c.FileName())
		if _, err := os.Stat(hgtPath); err == nil {
			skipped++
			continue
		}
		missing = append(missing, c)
	}

	if len(missing) == 0 {
		return DownloadResult{Skipped: skipped}, nil
	}

	// Disk space check: missing × 25 MB (HGT result)
	requiredBytes := int64(len(missing)) * 25 * 1024 * 1024
	var stat syscall.Statfs_t
	if err := syscall.Statfs(cfg.OutputDir, &stat); err == nil {
		freeBytes := int64(stat.Bavail) * int64(stat.Bsize)
		if freeBytes < requiredBytes {
			return DownloadResult{}, fmt.Errorf(
				"insufficient disk space: %d MB free, %d MB needed for %d tiles",
				freeBytes/1024/1024, requiredBytes/1024/1024, len(missing))
		}
	}

	var completed atomic.Int64
	total := int64(len(missing))
	var result DownloadResult
	result.Skipped = skipped

	var oceanCount, failCount, dlCount atomic.Int64

	g, gCtx := errgroup.WithContext(ctx)
	g.SetLimit(4)

	for _, cell := range missing {
		c := cell
		g.Go(func() error {
			if gCtx.Err() != nil {
				return gCtx.Err()
			}

			hgtPath := filepath.Join(cfg.OutputDir, c.FileName())
			tifPath := filepath.Join(cfg.TempDir, fmt.Sprintf("Copernicus_DSM_COG_10_%s.tif", tileAWSName(c)))
			url := CopernicusURL(cfg.BaseURL, c)

			// Download GeoTIFF
			err := downloadFile(gCtx, url, tifPath)
			if err != nil {
				// Retry once for transient errors
				if !isHTTP404(err) {
					slog.Debug("dem download: retrying", "cell", c.FileName(), "error", err)
					err = downloadFile(gCtx, url, tifPath)
				}
			}
			if err != nil {
				os.Remove(tifPath)
				if isHTTP404(err) {
					// Ocean tile — expected for coastal/island regions
					oceanCount.Add(1)
					slog.Debug("dem download: ocean tile skipped", "cell", c.FileName())
				} else {
					failCount.Add(1)
					slog.Warn("dem download: failed", "cell", c.FileName(), "error", err)
				}
				done := completed.Add(1)
				progress(done, total)
				return nil // continue with other tiles
			}

			// Convert GeoTIFF → SRTMHGT
			cmd := exec.CommandContext(gCtx, "gdal_translate",
				"-of", "SRTMHGT",
				"-outsize", "3601", "3601",
				"-r", "bilinear",
				"-q",
				tifPath, hgtPath)
			if err := cmd.Run(); err != nil {
				os.Remove(hgtPath)
				os.Remove(tifPath)
				failCount.Add(1)
				slog.Warn("dem download: gdal_translate failed", "cell", c.FileName(), "error", err)
				done := completed.Add(1)
				progress(done, total)
				return nil
			}

			// Cleanup GeoTIFF immediately
			os.Remove(tifPath)
			dlCount.Add(1)
			done := completed.Add(1)
			progress(done, total)
			return nil
		})
	}

	if err := g.Wait(); err != nil {
		return DownloadResult{}, err
	}

	result.Downloaded = int(dlCount.Load())
	result.OceanSkip = int(oceanCount.Load())
	result.Failed = int(failCount.Load())
	return result, nil
}

// tileAWSName formats a cell for the Copernicus S3 key (with _00 suffix).
func tileAWSName(c HGTCell) string {
	latPrefix := "N"
	latAbs := c.Lat
	if c.Lat < 0 {
		latPrefix = "S"
		latAbs = -c.Lat
	}
	lonPrefix := "E"
	lonAbs := c.Lon
	if c.Lon < 0 {
		lonPrefix = "W"
		lonAbs = -c.Lon
	}
	return fmt.Sprintf("%s%02d_00_%s%03d_00_DEM", latPrefix, latAbs, lonPrefix, lonAbs)
}

// downloadFile downloads a URL to a local path.
func downloadFile(ctx context.Context, url, dest string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == 404 || resp.StatusCode == 403 {
		return &httpError{StatusCode: resp.StatusCode}
	}
	if resp.StatusCode != 200 {
		return fmt.Errorf("HTTP %d for %s", resp.StatusCode, url)
	}
	f, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(f, resp.Body)
	return err
}

type httpError struct {
	StatusCode int
}

func (e *httpError) Error() string { return fmt.Sprintf("HTTP %d", e.StatusCode) }

func isHTTP404(err error) bool {
	var he *httpError
	return errors.As(err, &he) && (he.StatusCode == 404 || he.StatusCode == 403)
}

// RegionDEMStatus describes DEM coverage for one region.
type RegionDEMStatus struct {
	ID           string `json:"id"`
	TilesNeeded  int    `json:"tiles_needed"`
	TilesPresent int    `json:"tiles_present"`
	Complete     bool   `json:"complete"`
}

// CheckRegionCoverage computes DEM tile coverage for imported regions.
func CheckRegionCoverage(hgtDir string, regions []struct {
	ID   string
	BBox [4]float64
}) []RegionDEMStatus {
	// Deduplicate cells across regions
	allCellSet := make(map[HGTCell]bool)
	regionCells := make(map[string][]HGTCell)
	for _, r := range regions {
		cells := HGTCellsForBBox(r.BBox)
		regionCells[r.ID] = cells
		for _, c := range cells {
			allCellSet[c] = true
		}
	}

	// Check presence once
	present := make(map[HGTCell]bool)
	for c := range allCellSet {
		path := filepath.Join(hgtDir, c.FileName())
		if _, err := os.Stat(path); err == nil {
			present[c] = true
		}
	}

	var results []RegionDEMStatus
	for _, r := range regions {
		cells := regionCells[r.ID]
		p := 0
		for _, c := range cells {
			if present[c] {
				p++
			}
		}
		results = append(results, RegionDEMStatus{
			ID:           r.ID,
			TilesNeeded:  len(cells),
			TilesPresent: p,
			Complete:     p >= len(cells),
		})
	}
	return results
}
