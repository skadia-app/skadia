package handler

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/routing"
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

	userID, _ := r.Context().Value("user_id").(string)
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

	// Use detached context — request context is cancelled after 202 response.
	bgCtx := context.Background()

	go func() {
		err := h.GraphManager.Rebuild(bgCtx, h.DB.Pool, graphPath,
			func(step string, pct int) {
				_ = h.DB.UpdateBackgroundJobProgress(bgCtx, jobID, int64(pct))
				_ = h.DB.UpdateBackgroundJobPhase(bgCtx, jobID, step)
			})
		if err != nil {
			_ = h.DB.CompleteBackgroundJob(bgCtx, jobID, true)
		} else {
			_ = h.DB.CompleteBackgroundJob(bgCtx, jobID, false)
		}
	}()

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

	writeJSON(w, http.StatusOK, map[string]any{
		"graph_loaded": true,
		"node_count":   g.NodeCount,
		"edge_count":   g.EdgeCount,
		"build_time":   g.BuildTime,
		"bbox":         g.BBox,
		"version":      g.Version,
		"file_size_mb": int(fileSizeMB),
	})
}

// RouteCalculate computes a route using the internal routing engine.
func (h *Handler) RouteCalculate(w http.ResponseWriter, r *http.Request) {
	if h.GraphManager == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{
			"error": "routing not configured",
		})
		return
	}

	router := h.GraphManager.Router()
	if router == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{
			"error": "no routing graph loaded",
		})
		return
	}

	var req struct {
		From   [2]float64   `json:"from"`   // [lat, lon]
		To     [2]float64   `json:"to"`     // [lat, lon]
		Via    [][2]float64 `json:"via"`    // optional [[lat, lon], ...]
		Config *struct {
			UphillCostFactor   *float32 `json:"uphill_cost_factor"`
			DownhillCostFactor *float32 `json:"downhill_cost_factor"`
			Pass0Coefficient   *float32 `json:"pass0_coefficient"`
			ElevPenaltyBuffer  *float32 `json:"elev_penalty_buffer"`
			ElevMaxBuffer      *float32 `json:"elev_max_buffer"`
		} `json:"config"`
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

	cfg := routing.DefaultTrekkingConfig()
	if req.Config != nil {
		if req.Config.UphillCostFactor != nil {
			if *req.Config.UphillCostFactor < 0 {
				writeJSON(w, http.StatusBadRequest, map[string]any{
					"error": "uphill_cost_factor must be >= 0",
				})
				return
			}
			cfg.UphillCostFactor = *req.Config.UphillCostFactor
		}
		if req.Config.DownhillCostFactor != nil {
			if *req.Config.DownhillCostFactor < 0 {
				writeJSON(w, http.StatusBadRequest, map[string]any{
					"error": "downhill_cost_factor must be >= 0",
				})
				return
			}
			cfg.DownhillCostFactor = *req.Config.DownhillCostFactor
		}
		if req.Config.Pass0Coefficient != nil {
			if *req.Config.Pass0Coefficient <= 0 || *req.Config.Pass0Coefficient > 10 {
				writeJSON(w, http.StatusBadRequest, map[string]any{
					"error": "pass0_coefficient must be in (0, 10]",
				})
				return
			}
			cfg.Pass0Coefficient = *req.Config.Pass0Coefficient
		}
		if req.Config.ElevPenaltyBuffer != nil {
			if *req.Config.ElevPenaltyBuffer < 0 {
				writeJSON(w, http.StatusBadRequest, map[string]any{
					"error": "elev_penalty_buffer must be >= 0",
				})
				return
			}
			cfg.ElevPenaltyBuffer = *req.Config.ElevPenaltyBuffer
		}
		if req.Config.ElevMaxBuffer != nil {
			if *req.Config.ElevMaxBuffer < 0 {
				writeJSON(w, http.StatusBadRequest, map[string]any{
					"error": "elev_max_buffer must be >= 0",
				})
				return
			}
			cfg.ElevMaxBuffer = *req.Config.ElevMaxBuffer
		}
		if cfg.ElevPenaltyBuffer > cfg.ElevMaxBuffer {
			writeJSON(w, http.StatusBadRequest, map[string]any{
				"error": "elev_penalty_buffer must be <= elev_max_buffer",
			})
			return
		}
		cfg.MaxLatentCost = cfg.ElevMaxBuffer * max(cfg.UphillCostFactor, cfg.DownhillCostFactor)
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
