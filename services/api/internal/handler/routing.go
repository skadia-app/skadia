package handler

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/middleware"
	osmregions "github.com/skadia-app/skadia/services/api/internal/osm"
	"github.com/skadia-app/skadia/services/api/internal/routing"
	"github.com/skadia-app/skadia/services/api/internal/shutdown"
)

// RebuildRoutingGraph triggers a background graph rebuild.
// Must be placed under an admin-only route prefix.
func (h *Handler) RebuildRoutingGraph(w http.ResponseWriter, r *http.Request) {
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

	graphPath := h.Config.RoutingGraphPath
	if graphPath == "" {
		graphPath = "/data/routing/graph.bin"
	}

	// Create background job.
	entityName := "Routing Graph"
	jobID, err := h.DB.InsertBackgroundJob(r.Context(), userID,
		"routing_graph_build", nil, &entityName, 100)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{
			"error": "failed to create job",
		})
		return
	}

	shutdown.Go(jobID, func(ctx context.Context) {
		err := h.GraphManager.Rebuild(ctx, h.DB.Pool, graphPath,
			func(step string, pct int) {
				_ = h.DB.UpdateBackgroundJobProgress(ctx, jobID, int64(pct))
				_ = h.DB.UpdateBackgroundJobPhase(ctx, jobID, step)
			})
		if err != nil {
			slog.Error("routing graph build failed", "error", err, "job_id", jobID)
			_ = h.DB.CompleteBackgroundJob(context.Background(), jobID, true)
		} else {
			_ = h.DB.CompleteBackgroundJob(context.Background(), jobID, false)
		}
	})

	writeJSON(w, http.StatusAccepted, map[string]any{
		"status": "started",
		"job_id": jobID,
	})
}

// RoutingGraphStatus returns the current graph status.
func (h *Handler) RoutingGraphStatus(w http.ResponseWriter, r *http.Request) {
	if h.GraphManager == nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"graph_loaded": false,
		})
		return
	}

	g := h.GraphManager.Load()
	if g == nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"graph_loaded": false,
		})
		return
	}

	// File size from disk.
	var fileSizeMB float64
	graphPath := h.Config.RoutingGraphPath
	if graphPath == "" {
		graphPath = "/data/routing/graph.bin"
	}
	if info, err := os.Stat(graphPath); err == nil {
		fileSizeMB = float64(info.Size()) / (1024 * 1024)
	}

	resp := map[string]any{
		"graph_loaded": true,
		"node_count":   g.NodeCount,
		"edge_count":   g.EdgeCount,
		"build_time":   g.BuildTime,
		"bbox":         g.BBox,
		"version":      g.Version,
		"file_size_mb": int(fileSizeMB),
		"tile_mode":    h.GraphManager.IsTileMode(),
	}

	if tm := h.GraphManager.TileManager(); tm != nil {
		resp["tile_count"] = tm.TileCount()
		resp["tile_cached"] = tm.CachedCount()
	}

	writeJSON(w, http.StatusOK, resp)
}

// RouteCalculate computes a route using the internal routing engine.
// When tile mode is active, routes are computed via the cross-tile A*;
// otherwise the monolithic in-memory graph is used.
func (h *Handler) RouteCalculate(w http.ResponseWriter, r *http.Request) {
	if h.GraphManager == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{
			"error": "routing not configured",
		})
		return
	}

	useTileMode := h.GraphManager.IsTileMode()

	// In monolithic mode we need a loaded graph; in tile mode we don't.
	var router *routing.Router
	if !useTileMode {
		router = h.GraphManager.Router()
		if router == nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]any{
				"error": "no routing graph loaded",
			})
			return
		}
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

	if useTileMode {
		// Tile mode: segment-by-segment for via points, single call otherwise.
		if len(req.Via) > 0 {
			waypoints := make([]routing.LatLon, 0, len(req.Via)+2)
			waypoints = append(waypoints, routing.LatLon{Lat: req.From[0], Lon: req.From[1]})
			for _, v := range req.Via {
				waypoints = append(waypoints, routing.LatLon{Lat: v[0], Lon: v[1]})
			}
			waypoints = append(waypoints, routing.LatLon{Lat: req.To[0], Lon: req.To[1]})
			result, err = h.tileRouteVia(ctx, waypoints, cfg)
		} else {
			result, err = h.GraphManager.TileRoute(ctx, routing.RouteRequest{
				FromLat: req.From[0], FromLon: req.From[1],
				ToLat: req.To[0], ToLon: req.To[1],
			}, cfg)
		}
	} else {
		// Monolithic mode.
		if len(req.Via) > 0 {
			waypoints := make([]routing.LatLon, 0, len(req.Via)+2)
			waypoints = append(waypoints, routing.LatLon{Lat: req.From[0], Lon: req.From[1]})
			for _, v := range req.Via {
				waypoints = append(waypoints, routing.LatLon{Lat: v[0], Lon: v[1]})
			}
			waypoints = append(waypoints, routing.LatLon{Lat: req.To[0], Lon: req.To[1]})
			result, err = router.RouteVia(ctx, waypoints, cfg)
		} else {
			result, err = router.Route(ctx, routing.RouteRequest{
				FromLat: req.From[0], FromLon: req.From[1],
				ToLat: req.To[0], ToLon: req.To[1],
			}, cfg)
		}
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

	mode := "monolithic"
	if useTileMode {
		mode = "tiled"
	}
	slog.Info("route computed",
		"mode", mode,
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

// tileRouteVia routes through multiple waypoints in tile mode by calling
// TileRoute for each consecutive pair and concatenating the results.
func (h *Handler) tileRouteVia(ctx context.Context, waypoints []routing.LatLon,
	cfg *routing.CostConfig) (*routing.RouteResult, error) {

	if len(waypoints) < 2 {
		return nil, errors.New("need at least 2 waypoints")
	}

	combined := &routing.RouteResult{}
	for i := 0; i < len(waypoints)-1; i++ {
		seg, err := h.GraphManager.TileRoute(ctx, routing.RouteRequest{
			FromLat: waypoints[i].Lat, FromLon: waypoints[i].Lon,
			ToLat: waypoints[i+1].Lat, ToLon: waypoints[i+1].Lon,
		}, cfg)
		if err != nil {
			return nil, err
		}

		// Concatenate, avoiding duplicate junction points.
		if i > 0 && len(combined.Coordinates) > 0 && len(seg.Coordinates) > 0 {
			seg.Coordinates = seg.Coordinates[1:]
		}
		combined.Coordinates = append(combined.Coordinates, seg.Coordinates...)
		combined.DistanceM += seg.DistanceM
		combined.AscentM += seg.AscentM
		combined.DescentM += seg.DescentM
		combined.Cost += seg.Cost
		combined.PassStats.Pass0Expansions += seg.PassStats.Pass0Expansions
		combined.PassStats.TotalDurationMs += seg.PassStats.TotalDurationMs
	}
	return combined, nil
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

	const tileCacheBytes = 1_000_000_000 // 1 GB — keeps ~3-4 tiles hot, avoids OOM on 32 GB systems

	shutdown.Go(jobID, func(ctx context.Context) {
		hgtDirs := []string{"/data/dem/sonny-05", "/data/dem/sonny-1", "/data/dem/glo30"}
		workers := 2
		var tilesBuilt atomic.Int64
		var lastReportedTile atomic.Uint32
		err := h.GraphManager.RebuildTiles(ctx, h.DB.Pool, tileDir, tileCacheBytes,
			hgtDirs, workers,
			func(tileID uint32, step string, pct int) {
				if step == "discovering" {
					_ = h.DB.UpdateBackgroundJobTotal(ctx, jobID, int64(pct))
				} else if pct == 100 {
					// Only count once per tile (multiple steps report pct=100).
					if lastReportedTile.Swap(tileID) != tileID {
						n := tilesBuilt.Add(1)
						_ = h.DB.UpdateBackgroundJobProgress(ctx, jobID, n)
					}
				}
			})
		if err != nil {
			slog.Error("tile build failed", "error", err, "job_id", jobID)
			_ = h.DB.CompleteBackgroundJob(context.Background(), jobID, true)
		} else {
			slog.Info("tile build completed", "job_id", jobID)
			_ = h.DB.CompleteBackgroundJob(context.Background(), jobID, false)
		}
	})

	writeJSON(w, http.StatusAccepted, map[string]any{
		"status":   "started",
		"job_id":   jobID,
		"tile_dir": tileDir,
	})
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
			_ = h.DB.CompleteBackgroundJob(context.Background(), jobID, true)
		} else {
			slog.Info("BRouter segment build completed", "job_id", jobID)
			_ = h.DB.CompleteBackgroundJob(context.Background(), jobID, false)
		}
	})

	writeJSON(w, http.StatusAccepted, map[string]any{
		"status":   "started",
		"job_id":   jobID,
		"regions":  labels,
	})
}
