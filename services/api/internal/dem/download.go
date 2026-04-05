package dem

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"

	"golang.org/x/sync/errgroup"
)

// TileStatus tracks whether an HGT cell is land or ocean on Copernicus.
// Persisted as .tile_status.json in the output directory.
type TileStatus struct {
	mu     sync.Mutex
	status map[string]string // cell filename (e.g. "N47E011.hgt") → "land" | "ocean"
	path   string
}

const (
	tileStatusLand  = "land"
	tileStatusOcean = "ocean"
)

// LoadTileStatus loads or creates the tile status manifest.
func LoadTileStatus(outputDir string) *TileStatus {
	ts := &TileStatus{
		status: make(map[string]string),
		path:   filepath.Join(outputDir, ".tile_status.json"),
	}
	data, err := os.ReadFile(ts.path)
	if err == nil {
		if jsonErr := json.Unmarshal(data, &ts.status); jsonErr != nil {
			slog.Warn("dem: corrupt tile status manifest, starting fresh", "path", ts.path, "error", jsonErr)
		}
	}
	return ts
}

func (ts *TileStatus) Get(cellFile string) (string, bool) {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	s, ok := ts.status[cellFile]
	return s, ok
}

func (ts *TileStatus) Set(cellFile, status string) {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	ts.status[cellFile] = status
}

// Save persists the manifest to disk (atomic write).
func (ts *TileStatus) Save() error {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	data, err := json.MarshalIndent(ts.status, "", "  ")
	if err != nil {
		return err
	}
	tmp := ts.path + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	f.Close()
	return os.Rename(tmp, ts.path)
}

// LandTilesForCells returns the subset of cells known to be land tiles.
// Unknown cells (not in manifest) are NOT counted as land.
func (ts *TileStatus) LandTilesForCells(cells []HGTCell) []HGTCell {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	var land []HGTCell
	for _, c := range cells {
		if ts.status[c.FileName()] == tileStatusLand {
			land = append(land, c)
		}
	}
	return land
}

// HasManifest returns true if the manifest exists and has entries.
func (ts *TileStatus) HasManifest() bool {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	return len(ts.status) > 0
}

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
// Uses a tile status manifest (.tile_status.json) to track land vs ocean tiles.
// Known ocean tiles are skipped without HTTP requests. New discoveries are persisted.
// progress is called after each tile with (completed, total).
func DownloadMissingTiles(ctx context.Context, cfg DEMSourceConfig, cells []HGTCell,
	progress func(completed, total int64)) (DownloadResult, error) {

	if err := os.MkdirAll(cfg.OutputDir, 0755); err != nil {
		return DownloadResult{}, fmt.Errorf("create output dir: %w", err)
	}
	if err := os.MkdirAll(cfg.TempDir, 0755); err != nil {
		return DownloadResult{}, fmt.Errorf("create temp dir: %w", err)
	}

	// Cleanup GeoTIFF temp files on exit (success or cancel)
	defer func() {
		entries, _ := os.ReadDir(cfg.TempDir)
		for _, e := range entries {
			if strings.HasSuffix(e.Name(), ".tif") {
				os.Remove(filepath.Join(cfg.TempDir, e.Name()))
			}
		}
	}()

	// Load tile status manifest (land vs ocean knowledge from previous downloads)
	manifest := LoadTileStatus(cfg.OutputDir)
	defer manifest.Save() // persist any new discoveries

	// Filter: skip existing HGT files + known ocean tiles
	var missing []HGTCell
	skipped := 0
	oceanSkippedFromManifest := 0
	for _, c := range cells {
		hgtPath := filepath.Join(cfg.OutputDir, c.FileName())
		if _, err := os.Stat(hgtPath); err == nil {
			skipped++
			manifest.Set(c.FileName(), tileStatusLand) // retroactively mark existing tiles as land
			continue
		}
		if status, ok := manifest.Get(c.FileName()); ok && status == tileStatusOcean {
			oceanSkippedFromManifest++
			continue // known ocean tile — skip without HTTP request
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
				if !isHTTPNotFound(err) {
					slog.Debug("dem download: retrying", "cell", c.FileName(), "error", err)
					err = downloadFile(gCtx, url, tifPath)
				}
			}
			if err != nil {
				os.Remove(tifPath)
				if isHTTPNotFound(err) {
					// Ocean tile — expected for coastal/island regions
					manifest.Set(c.FileName(), tileStatusOcean) // persist for future runs
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
			manifest.Set(c.FileName(), tileStatusLand) // persist: this tile exists
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
	result.OceanSkip = int(oceanCount.Load()) + oceanSkippedFromManifest
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
	if resp.StatusCode == 404 {
		return &httpError{StatusCode: 404}
	}
	if resp.StatusCode != 200 {
		return fmt.Errorf("HTTP %d for %s", resp.StatusCode, url)
	}
	f, err := os.Create(dest)
	if err != nil {
		return err
	}
	if _, cpErr := io.Copy(f, resp.Body); cpErr != nil {
		f.Close()
		os.Remove(dest)
		return cpErr
	}
	if err := f.Sync(); err != nil {
		f.Close()
		os.Remove(dest)
		return err
	}
	return f.Close()
}

type httpError struct {
	StatusCode int
}

func (e *httpError) Error() string { return fmt.Sprintf("HTTP %d", e.StatusCode) }

// isHTTPNotFound returns true only for 404 (tile does not exist on server).
// 403 is NOT treated as "not found" — it could be transient (rate-limiting, auth).
func isHTTPNotFound(err error) bool {
	var he *httpError
	return errors.As(err, &he) && he.StatusCode == 404
}

// RegionDEMStatus describes DEM coverage for one region.
type RegionDEMStatus struct {
	ID           string `json:"id"`
	Label        string `json:"label"`
	TilesNeeded  int    `json:"tiles_needed"`
	TilesPresent int    `json:"tiles_present"`
	Complete     bool   `json:"complete"`
}

