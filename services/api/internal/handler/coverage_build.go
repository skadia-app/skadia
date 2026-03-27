package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/skadia-app/skadia/services/api/internal/httputil"
	osmpkg "github.com/skadia-app/skadia/services/api/internal/osm"
	"github.com/skadia-app/skadia/services/api/internal/osm/coverage"
	"github.com/skadia-app/skadia/services/api/internal/shutdown"
)

func (h *Handler) BuildCoverage(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}

	var req struct {
		Layer   string `json:"layer"`
		Tile    string `json:"tile"`
		Region  string `json:"region"`
		Workers int    `json:"workers"`
	}
	json.NewDecoder(r.Body).Decode(&req)

	entityName := "Coverage-Grids"
	jobID, created, err := h.DB.InsertBackgroundJobGuarded(r.Context(), userID,
		"coverage_build", nil, &entityName, 0)
	if err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, "Job creation failed", err.Error(), r.URL.Path)
		return
	}
	if !created {
		httputil.WriteJSON(w, http.StatusOK, map[string]string{"status": "already_running"})
		return
	}

	opts := coverage.BuildCoverageOptions{
		BaseDir: h.CoverageBaseDir,
		Layer:   req.Layer,
		Region:  req.Region,
		Workers: req.Workers,
	}

	// Resolve region to bounding box
	if req.Region != "" {
		reg, ok := osmpkg.RegionByID(req.Region)
		if !ok {
			_ = h.DB.CompleteBackgroundJob(r.Context(), jobID, true, fmt.Sprintf("unknown region: %q", req.Region))
			httputil.WriteError(w, http.StatusBadRequest, "Unknown region", fmt.Sprintf("unknown region: %q", req.Region), r.URL.Path)
			return
		}
		opts.RegionBBox = [4]float64{reg.MinLat, reg.MinLon, reg.MaxLat, reg.MaxLon}
	}

	if req.Tile != "" {
		var lat, lon int
		if _, err := fmt.Sscanf(req.Tile, "%d,%d", &lat, &lon); err == nil {
			opts.TileLat = lat
			opts.TileLon = lon
			opts.SingleTile = true
		}
	}

	shutdown.Go(jobID, func(ctx context.Context) {
		err := coverage.RunBuildCoverage(ctx, h.DB.Pool, opts, func(current, total int64) {
			if current == 0 && total > 0 {
				_ = h.DB.UpdateBackgroundJobTotal(ctx, jobID, total)
			} else {
				_ = h.DB.UpdateBackgroundJobProgress(ctx, jobID, current)
			}
		})
		if err != nil {
			_ = h.DB.CompleteBackgroundJob(context.Background(), jobID, true, err.Error())
		} else {
			_ = h.DB.CompleteBackgroundJob(context.Background(), jobID, false, "")
		}
	})

	httputil.WriteJSON(w, http.StatusAccepted, map[string]any{
		"status": "started",
		"job_id": jobID,
	})
}
