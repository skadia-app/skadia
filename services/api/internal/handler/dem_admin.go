package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"syscall"

	"github.com/skadia-app/skadia/services/api/internal/dem"
	"github.com/skadia-app/skadia/services/api/internal/httputil"
	osmregions "github.com/skadia-app/skadia/services/api/internal/osm"
	"github.com/skadia-app/skadia/services/api/internal/shutdown"
)

// DownloadDEM triggers a background DEM download for specified or all imported regions.
// POST /api/v1/admin/dem/download
func (h *Handler) DownloadDEM(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}

	var req struct {
		Regions []string `json:"regions"`
	}
	// Empty body = all imported regions (ignore decode error for empty body)
	json.NewDecoder(r.Body).Decode(&req)

	// Guard
	if running, _ := h.DB.HasRunningJobGlobal(r.Context(), "dem_download"); running {
		httputil.WriteJSON(w, http.StatusConflict, map[string]string{"error": "DEM download already running"})
		return
	}

	// Determine regions
	type regionBBox struct {
		ID   string
		BBox [4]float64 // [minLat, minLon, maxLat, maxLon]
	}
	var regions []regionBBox

	if len(req.Regions) > 0 {
		// Specific regions from request
		for _, id := range req.Regions {
			reg, ok := osmregions.RegionByID(id)
			if !ok {
				httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "unknown region: "+id, r.URL.Path)
				return
			}
			regions = append(regions, regionBBox{
				ID:   reg.ID,
				BBox: [4]float64{reg.MinLat, reg.MinLon, reg.MaxLat, reg.MaxLon},
			})
		}
	} else {
		// All imported regions
		rows, err := h.DB.Pool.Query(r.Context(), `SELECT region_id FROM osm_import_regions WHERE status = 'imported'`)
		if err != nil {
			httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", err.Error(), r.URL.Path)
			return
		}
		defer rows.Close()
		for rows.Next() {
			var id string
			rows.Scan(&id)
			if reg, ok := osmregions.RegionByID(id); ok {
				regions = append(regions, regionBBox{
					ID:   reg.ID,
					BBox: [4]float64{reg.MinLat, reg.MinLon, reg.MaxLat, reg.MaxLon},
				})
			}
		}
	}

	if len(regions) == 0 {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "no regions to download DEM for", r.URL.Path)
		return
	}

	// Compute cells
	demDir := h.demDataDir()
	cfg := dem.DefaultGLO30(demDir)
	cellSet := make(map[dem.HGTCell]bool)
	for _, r := range regions {
		for _, c := range dem.HGTCellsForBBox(r.BBox) {
			cellSet[c] = true
		}
	}
	var allCells []dem.HGTCell
	for c := range cellSet {
		allCells = append(allCells, c)
	}

	entityName := fmt.Sprintf("DEM Download (%d Kacheln)", len(allCells))
	jobID, created, err := h.DB.InsertBackgroundJobGuarded(r.Context(), userID, "dem_download", nil, &entityName, int64(len(allCells)))
	if err != nil || !created {
		httputil.WriteJSON(w, http.StatusConflict, map[string]string{"error": "DEM download already running"})
		return
	}

	shutdown.Go(jobID, func(ctx context.Context) {
		result, err := dem.DownloadMissingTiles(ctx, cfg, allCells, func(completed, total int64) {
			h.DB.UpdateBackgroundJobProgress(ctx, jobID, completed)
		})
		if err != nil {
			slog.Error("dem download failed", "error", err)
			h.DB.CompleteBackgroundJob(ctx, jobID, true, err.Error())
			return
		}

		msg := ""
		if result.Failed > 0 {
			msg = fmt.Sprintf("%d heruntergeladen, %d übersprungen (vorhanden), %d Ocean-Tiles, %d fehlgeschlagen",
				result.Downloaded, result.Skipped, result.OceanSkip, result.Failed)
			// More than 50% non-ocean failures = mark as failed
			nonOcean := len(allCells) - result.OceanSkip - result.Skipped
			if nonOcean > 0 && result.Failed > nonOcean/2 {
				h.DB.CompleteBackgroundJob(ctx, jobID, true, msg)
				return
			}
		}
		slog.Info("dem download complete", "downloaded", result.Downloaded, "skipped", result.Skipped,
			"ocean", result.OceanSkip, "failed", result.Failed)
		h.DB.CompleteBackgroundJob(ctx, jobID, false, msg)
	})

	httputil.WriteJSON(w, http.StatusAccepted, map[string]string{"job_id": jobID})
}

// GetDEMRegionStatus returns DEM coverage per imported region + disk space info.
// GET /api/v1/admin/dem/status
func (h *Handler) GetDEMRegionStatus(w http.ResponseWriter, r *http.Request) {
	// Existing source info
	type demSource struct {
		Source      string  `json:"source"`
		ResolutionM float64 `json:"resolution_m"`
		TileCount   int     `json:"tile_count"`
	}
	rows, err := h.DB.Pool.Query(r.Context(), `
		SELECT COALESCE(source, 'unknown'), COALESCE(resolution_m, 0), COUNT(*)
		FROM dem.raster_tiles GROUP BY source, resolution_m ORDER BY resolution_m ASC`)

	var sources []demSource
	var totalTiles int
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var s demSource
			if rows.Scan(&s.Source, &s.ResolutionM, &s.TileCount) == nil {
				sources = append(sources, s)
				totalTiles += s.TileCount
			}
		}
	}
	if sources == nil {
		sources = []demSource{}
	}

	// Region coverage from HGT files on disk
	demDir := h.demDataDir()
	hgtDir := filepath.Join(demDir, "glo30")

	regRows, err := h.DB.Pool.Query(r.Context(), `SELECT region_id FROM osm_import_regions WHERE status IN ('imported', 'needs_redownload')`)
	type rBBox struct {
		ID   string
		BBox [4]float64
	}
	var regs []rBBox
	if err == nil {
		defer regRows.Close()
		for regRows.Next() {
			var id string
			regRows.Scan(&id)
			if reg, ok := osmregions.RegionByID(id); ok {
				regs = append(regs, rBBox{ID: reg.ID, BBox: [4]float64{reg.MinLat, reg.MinLon, reg.MaxLat, reg.MaxLon}})
			}
		}
	}

	var regionStatus []dem.RegionDEMStatus
	var missingTiles int
	if len(regs) > 0 {
		cellSet := make(map[dem.HGTCell]bool)
		regionCells := make(map[string][]dem.HGTCell)
		for _, r := range regs {
			cells := dem.HGTCellsForBBox(r.BBox)
			regionCells[r.ID] = cells
			for _, c := range cells {
				cellSet[c] = true
			}
		}
		present := make(map[dem.HGTCell]bool)
		for c := range cellSet {
			if _, err := os.Stat(filepath.Join(hgtDir, c.FileName())); err == nil {
				present[c] = true
			}
		}
		for _, r := range regs {
			cells := regionCells[r.ID]
			p := 0
			for _, c := range cells {
				if present[c] {
					p++
				}
			}
			// A region is "complete" if at least 50% of BBox tiles are present.
			// Many BBox tiles are ocean (e.g. Portugal Azores) — 100% is unreachable.
			reg, _ := osmregions.RegionByID(r.ID)
			label := r.ID
			if reg.Label != "" {
				label = reg.Label
			}
			regionStatus = append(regionStatus, dem.RegionDEMStatus{
				ID: r.ID, Label: label, TilesNeeded: len(cells), TilesPresent: p,
				Complete: len(cells) == 0 || p >= len(cells)/2,
			})
		}
		// Count globally missing tiles (deduplicated across regions)
		for c := range cellSet {
			if !present[c] {
				missingTiles++
			}
		}
	}
	if regionStatus == nil {
		regionStatus = []dem.RegionDEMStatus{}
	}

	// Disk space
	var diskFreeBytes int64
	var stat syscall.Statfs_t
	if syscall.Statfs(demDir, &stat) == nil {
		diskFreeBytes = int64(stat.Bavail) * int64(stat.Bsize)
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]any{
		"sources":         sources,
		"total_tiles":     totalTiles,
		"regions":         regionStatus,
		"missing_tiles":   missingTiles,
		"disk_free_bytes": diskFreeBytes,
	})
}

func (h *Handler) demDataDir() string {
	dir := os.Getenv("DEM_DATA_DIR")
	if dir == "" {
		dir = "/data/dem"
	}
	return dir
}
