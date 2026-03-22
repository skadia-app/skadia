package handler

import (
	"context"
	"encoding/json"
	"math"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/middleware"
	osmregions "github.com/skadia-app/skadia/services/api/internal/osm"
	"github.com/skadia-app/skadia/services/api/internal/routing"
	"github.com/skadia-app/skadia/services/api/internal/shutdown"
)

// RoutingGraphStatus returns the current routing status based on tile availability.
func (h *Handler) RoutingGraphStatus(w http.ResponseWriter, r *http.Request) {
	if h.GraphManager == nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"routing_available": false,
		})
		return
	}

	resp := map[string]any{
		"routing_available": h.GraphManager.UnifiedRouter() != nil,
	}

	if tm := h.GraphManager.TileManager(); tm != nil {
		resp["tile_count"] = tm.TileCount()
		resp["tile_cached"] = tm.CachedCount()
	}

	writeJSON(w, http.StatusOK, resp)
}

// RouteCalculate computes a route using the UnifiedRouter (TiledGraphReader).
// Returns 503 if no routing engine is available.
func (h *Handler) RouteCalculate(w http.ResponseWriter, r *http.Request) {
	if h.GraphManager == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{
			"error": "routing not configured",
		})
		return
	}

	unifiedRouter := h.GraphManager.UnifiedRouter()
	if unifiedRouter == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{
			"error": "routing not available",
		})
		return
	}

	var req struct {
		From   [2]float64            `json:"from"`   // [lat, lon]
		To     [2]float64            `json:"to"`     // [lat, lon]
		Via    [][2]float64          `json:"via"`    // optional [[lat, lon], ...]
		Config *routing.ProfileConfig `json:"config"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"error": "invalid request body",
		})
		return
	}

	// Input validation — system boundary, validate lat/lon ranges
	validCoord := func(lat, lon float64) bool {
		return lat >= -90 && lat <= 90 && lon >= -180 && lon <= 180
	}
	if !validCoord(req.From[0], req.From[1]) || !validCoord(req.To[0], req.To[1]) {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"error": "lat must be [-90,90], lon must be [-180,180]",
		})
		return
	}
	if len(req.Via) > 25 {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"error": "maximum 25 via points allowed",
		})
		return
	}
	for _, v := range req.Via {
		if !validCoord(v[0], v[1]) {
			writeJSON(w, http.StatusBadRequest, map[string]any{
				"error": "via point: lat must be [-90,90], lon must be [-180,180]",
			})
			return
		}
	}

	var cfg *routing.CostConfig
	if req.Config != nil && req.Config.Profile != "" {
		var err error
		cfg, err = routing.BuildCostConfig(*req.Config)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{
				"error": err.Error(),
			})
			return
		}
	} else if req.Config != nil {
		// Legacy mode: no profile name, apply individual overrides to default.
		cfg = routing.DefaultCostConfig()
		if req.Config.UphillCost != nil {
			cfg.UphillCost = *req.Config.UphillCost
		}
		if req.Config.DownhillCost != nil {
			cfg.DownhillCost = *req.Config.DownhillCost
		}
		if req.Config.Pass0Coefficient != nil {
			cfg.Pass0Coefficient = *req.Config.Pass0Coefficient
		}
		if req.Config.ElevPenaltyBuffer != nil {
			cfg.ElevPenaltyBuffer = *req.Config.ElevPenaltyBuffer
		}
		if req.Config.ElevMaxBuffer != nil {
			cfg.ElevMaxBuffer = *req.Config.ElevMaxBuffer
		}
		cfg.MaxLatentCost = cfg.ElevMaxBuffer * max(cfg.UphillCost, cfg.DownhillCost)
	} else {
		cfg = routing.DefaultCostConfig()
	}

	timeout := 30 * time.Second
	if h.Config.RoutingTimeout > 0 {
		timeout = h.Config.RoutingTimeout
	}
	ctx, cancel := context.WithTimeout(r.Context(), timeout)
	defer cancel()

	start := time.Now()
	var result *routing.RouteResult
	var err error

	if len(req.Via) > 0 {
		wp := make([]routing.LatLon, 0, len(req.Via)+2)
		wp = append(wp, routing.LatLon{Lat: req.From[0], Lon: req.From[1]})
		for _, v := range req.Via {
			wp = append(wp, routing.LatLon{Lat: v[0], Lon: v[1]})
		}
		wp = append(wp, routing.LatLon{Lat: req.To[0], Lon: req.To[1]})
		result, err = unifiedRouter.RouteVia(ctx, wp, cfg)
	} else {
		result, err = unifiedRouter.Route(ctx, routing.RouteRequest{
			FromLat: req.From[0], FromLon: req.From[1],
			ToLat: req.To[0], ToLon: req.To[1],
		}, cfg)
	}

	duration := time.Since(start)

	if err != nil {
		status := "no_route"
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
			status = "timeout"
		}
		slog.Warn("route failed",
			"from", req.From, "to", req.To,
			"via_count", len(req.Via),
			"duration_ms", duration.Milliseconds(),
			"status", status,
			"error", err)
		writeJSON(w, http.StatusOK, map[string]any{
			"status": status,
			"error":  err.Error(),
		})
		return
	}
	slog.Info("route computed",
		"mode", "unified",
		"from", req.From, "to", req.To,
		"via_count", len(req.Via),
		"distance_m", result.DistanceM,
		"ascent_m", result.AscentM,
		"duration_ms", duration.Milliseconds(),
		"pass0_expansions", result.PassStats.Pass0Expansions,
		"pass1_expansions", result.PassStats.Pass1Expansions)

	w.Header().Set("Content-Type", "application/geo+json")
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write(result.ToGeoJSON()); err != nil {
		slog.Warn("route response write failed", "error", err)
	}
}

// RebuildRoutingTiles triggers a background tile build from PostGIS data.
// POST /admin/routing/rebuild-tiles
func (h *Handler) RebuildRoutingTiles(w http.ResponseWriter, r *http.Request) {
	if h.GraphManager == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{
			"error": "routing not configured",
		})
		return
	}

	userID := middleware.GetUserID(r.Context())
	if userID == "" {
		writeJSON(w, http.StatusUnauthorized, map[string]any{
			"error": "unauthorized",
		})
		return
	}

	var req struct {
		Regions []string `json:"regions"`
		Force   bool     `json:"force"`
	}
	json.NewDecoder(r.Body).Decode(&req) // empty body = all tiles

	// Concurrent-Guard
	var running int
	h.DB.Pool.QueryRow(r.Context(),
		`SELECT count(*) FROM background_jobs WHERE job_type = 'tile_build' AND status = 'running'`).Scan(&running)
	if running > 0 {
		writeJSON(w, http.StatusOK, map[string]string{"status": "already_running"})
		return
	}

	// BBox-Union aus Regionen berechnen
	var filterBBox [4]float64
	if len(req.Regions) > 0 {
		filterBBox = [4]float64{180, 90, -180, -90} // inverted start
		for _, id := range req.Regions {
			reg, ok := osmregions.RegionByID(id)
			if !ok {
				continue
			}
			filterBBox[0] = min(filterBBox[0], reg.MinLon)
			filterBBox[1] = min(filterBBox[1], reg.MinLat)
			filterBBox[2] = max(filterBBox[2], reg.MaxLon)
			filterBBox[3] = max(filterBBox[3], reg.MaxLat)
		}
	}

	graphPath := h.Config.RoutingGraphPath
	if graphPath == "" {
		graphPath = "/data/routing/graph.bin"
	}
	tileDir := filepath.Join(filepath.Dir(graphPath), "tiles")

	entityName := "Routing Tiles"
	// progress_total = 0 initially; updated to tile count after discovery.
	jobID, err := h.DB.InsertBackgroundJob(r.Context(), userID,
		"tile_build", nil, &entityName, 0)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{
			"error": "failed to create job",
		})
		return
	}

	const tileCacheBytes = 4_000_000_000 // 4 GB — consistent with startup default

	shutdown.Go(jobID, func(ctx context.Context) {
		hgtDirs := []string{"/data/dem/sonny-05", "/data/dem/sonny-1", "/data/dem/glo30"}
		workers := 2
		var tilesBuilt atomic.Int64
		var countedTiles sync.Map // tileID → true, prevents double-counting
		err := h.GraphManager.RebuildTiles(ctx, h.DB.Pool, tileDir, tileCacheBytes,
			hgtDirs, workers, filterBBox, req.Force,
			func(tileID uint32, step string, pct int) {
				if step == "discovering" {
					_ = h.DB.UpdateBackgroundJobTotal(ctx, jobID, int64(pct))
				} else if tileID == math.MaxUint32 {
					// Landmark preprocessing phase — update phase column for UI display
					_ = h.DB.UpdateBackgroundJobPhase(ctx, jobID, step)
				} else if pct == 100 {
					// Only count once per tile — sync.Map handles concurrent workers.
					if _, loaded := countedTiles.LoadOrStore(tileID, true); !loaded {
						n := tilesBuilt.Add(1)
						_ = h.DB.UpdateBackgroundJobProgress(ctx, jobID, n)
					}
				}
			})
		if err != nil {
			slog.Error("tile build failed", "error", err, "job_id", jobID)
			_ = h.DB.CompleteBackgroundJob(context.Background(), jobID, true, "")
		} else {
			slog.Info("tile build completed", "job_id", jobID)
			_ = h.DB.CompleteBackgroundJob(context.Background(), jobID, false, "")
		}
	})

	writeJSON(w, http.StatusAccepted, map[string]any{
		"status":   "started",
		"job_id":   jobID,
		"tile_dir": tileDir,
	})
}

// RebuildLandmarks runs ONLY the landmark preprocessing on existing tiles.
// Does NOT rebuild tiles. Use when tiles exist but landmarks need recomputing.
// POST /admin/routing/rebuild-landmarks
func (h *Handler) RebuildLandmarks(w http.ResponseWriter, r *http.Request) {
	if h.GraphManager == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "routing not configured"})
		return
	}
	userID := middleware.GetUserID(r.Context())
	if userID == "" {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "unauthorized"})
		return
	}

	graphPath := h.Config.RoutingGraphPath
	if graphPath == "" {
		graphPath = "/data/routing/graph.bin"
	}
	tileDir := filepath.Join(filepath.Dir(graphPath), "tiles")

	entityName := "Landmarks"
	jobID, err := h.DB.InsertBackgroundJob(r.Context(), userID, "tile_build", nil, &entityName, 0)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "failed to create job"})
		return
	}

	shutdown.Go(jobID, func(ctx context.Context) {
		meta, landmarkErr := routing.RunUnifiedLandmarkPreprocessing(ctx, tileDir,
			func(step string, current, total int) {
				_ = h.DB.UpdateBackgroundJobPhase(ctx, jobID, step)
			})
		if landmarkErr != nil {
			slog.Error("landmark rebuild failed", "error", landmarkErr, "job_id", jobID)
			_ = h.DB.CompleteBackgroundJob(context.Background(), jobID, true, landmarkErr.Error())
		} else {
			slog.Info("landmark rebuild completed", "job_id", jobID, "landmarks", meta.LandmarkCount)
			_ = h.DB.CompleteBackgroundJob(context.Background(), jobID, false, "")
		}
	})

	writeJSON(w, http.StatusAccepted, map[string]any{"status": "started", "job_id": jobID})
}

// BuildBRouterSegments builds BRouter rd5 segment files from local PBF data.
// This is a standalone endpoint — does NOT trigger OSM reimport or enrichment.
// POST /admin/routing/build-brouter-segments
func (h *Handler) BuildBRouterSegments(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	if userID == "" {
		writeJSON(w, http.StatusUnauthorized, map[string]any{
			"error": "unauthorized",
		})
		return
	}

	var req struct {
		Regions []string `json:"regions"` // optional — if empty, uses all regions that have PBF files
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		// No body is fine — build from all available PBFs
		req.Regions = nil
	}

	// Resolve regions
	var regions []osmregions.Region
	if len(req.Regions) > 0 {
		for _, id := range req.Regions {
			reg, ok := osmregions.RegionByID(id)
			if !ok {
				writeJSON(w, http.StatusBadRequest, map[string]any{
					"error": "unknown region: " + id,
				})
				return
			}
			regions = append(regions, reg)
		}
	} else {
		// Auto-detect: find all regions that have PBF files on disk
		osmDataDir := os.Getenv("OSM_DATA_DIR")
		if osmDataDir == "" {
			osmDataDir = "/data/osm"
		}
		for _, reg := range osmregions.Regions {
			pbfPath := filepath.Join(osmDataDir, reg.ID+"-latest.osm.pbf")
			if info, err := os.Stat(pbfPath); err == nil && info.Size() > 0 {
				regions = append(regions, reg)
			}
		}
	}

	if len(regions) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"error": "no PBF files found for any region",
		})
		return
	}

	var labels []string
	for _, reg := range regions {
		labels = append(labels, reg.Label)
	}

	// Concurrent-Guard
	var running int
	h.DB.Pool.QueryRow(r.Context(),
		`SELECT count(*) FROM background_jobs WHERE job_type = 'brouter_segment_build' AND status = 'running'`).Scan(&running)
	if running > 0 {
		writeJSON(w, http.StatusOK, map[string]string{"status": "already_running"})
		return
	}

	entityName := "BRouter Segments: " + strings.Join(labels, ", ")
	jobID, err := h.DB.InsertBackgroundJob(r.Context(), userID,
		"brouter_segment_build", nil, &entityName, 100)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{
			"error": "failed to create job",
		})
		return
	}

	shutdown.Go(jobID, func(ctx context.Context) {
		osmDataDir := os.Getenv("OSM_DATA_DIR")
		if osmDataDir == "" {
			osmDataDir = "/data/osm"
		}
		segmentsDir := os.Getenv("BROUTER_SEGMENTS_DIR")
		if segmentsDir == "" {
			segmentsDir = "/data/brouter/segments"
		}

		pipeline := &osmregions.Pipeline{
			Pool:    h.DB.Pool,
			DataDir: osmDataDir,
			DBURL:   os.Getenv("DATABASE_URL"),
		}
		reporter := &jobReporter{db: h.DB, jobID: jobID}

		if err := pipeline.BuildBRouterSegments(ctx, regions, osmDataDir, segmentsDir, reporter); err != nil {
			slog.Error("BRouter segment build failed", "error", err, "job_id", jobID)
			_ = h.DB.CompleteBackgroundJob(context.Background(), jobID, true, "")
		} else {
			slog.Info("BRouter segment build completed", "job_id", jobID)
			_ = h.DB.CompleteBackgroundJob(context.Background(), jobID, false, "")
		}
	})

	writeJSON(w, http.StatusAccepted, map[string]any{
		"status":   "started",
		"job_id":   jobID,
		"regions":  labels,
	})
}

// DownloadBRouterSegments downloads pre-built BRouter rd5 segments from brouter.de.
// POST /admin/routing/download-brouter-segments
func (h *Handler) DownloadBRouterSegments(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	if userID == "" {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "unauthorized"})
		return
	}

	var req struct {
		Regions []string `json:"regions"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || len(req.Regions) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "regions required"})
		return
	}

	var regions []osmregions.Region
	for _, id := range req.Regions {
		reg, ok := osmregions.RegionByID(id)
		if !ok {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "unknown region: " + id})
			return
		}
		regions = append(regions, reg)
	}

	tiles := osmregions.BRouterTileIDs(regions)
	if len(tiles) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "no tiles for selected regions"})
		return
	}

	var running int
	h.DB.Pool.QueryRow(r.Context(),
		`SELECT count(*) FROM background_jobs WHERE job_type = 'brouter_segment_download' AND status = 'running'`).Scan(&running)
	if running > 0 {
		writeJSON(w, http.StatusOK, map[string]string{"status": "already_running"})
		return
	}

	entityName := fmt.Sprintf("BRouter Download: %d Tiles", len(tiles))
	jobID, err := h.DB.InsertBackgroundJob(r.Context(), userID,
		"brouter_segment_download", nil, &entityName, int64(len(tiles)))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "failed to create job"})
		return
	}

	segmentsDir := os.Getenv("BROUTER_SEGMENTS_DIR")
	if segmentsDir == "" {
		segmentsDir = "/data/brouter/segments"
	}

	shutdown.Go(jobID, func(ctx context.Context) {
		res, err := osmregions.DownloadBRouterSegments(ctx, tiles, segmentsDir,
			func(downloaded, total int) {
				_ = h.DB.UpdateBackgroundJobProgress(ctx, jobID, int64(downloaded))
			})
		bgCtx := context.Background()
		if err != nil {
			slog.Error("brouter download failed", "error", err, "job_id", jobID)
			_ = h.DB.CompleteBackgroundJob(bgCtx, jobID, true, "")
		} else {
			// Update entity_name with actual counts so notification is informative.
			summary := fmt.Sprintf("%d heruntergeladen", res.Downloaded)
			if res.Skipped > 0 {
				summary += fmt.Sprintf(", %d übersprungen (bereits vorhanden)", res.Skipped)
			}
			if res.Failed > 0 {
				summary += fmt.Sprintf(", %d fehlgeschlagen", res.Failed)
			}
			_, _ = h.DB.Pool.Exec(bgCtx,
				"UPDATE background_jobs SET entity_name = $1 WHERE id = $2", summary, jobID)
			_ = h.DB.CompleteBackgroundJob(bgCtx, jobID, false, "")
		}
	})

	writeJSON(w, http.StatusAccepted, map[string]any{
		"status": "started",
		"job_id": jobID,
		"tiles":  len(tiles),
	})
}
