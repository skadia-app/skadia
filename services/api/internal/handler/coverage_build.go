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
		Layer   string   `json:"layer"`
		Tile    string   `json:"tile"`
		Regions []string `json:"regions"` // region names (e.g. ["austria", "germany"])
		Workers int      `json:"workers"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && err.Error() != "EOF" {
		httputil.WriteError(w, http.StatusBadRequest, "Invalid JSON", err.Error(), r.URL.Path)
		return
	}

	opts := coverage.BuildCoverageOptions{
		BaseDir: h.CoverageBaseDir,
		Layer:   req.Layer,
		Workers: req.Workers,
	}

	// Resolve regions to union bounding box — validate BEFORE creating background job
	if len(req.Regions) > 0 {
		filterBBox := [4]float64{90, 180, -90, -180} // minLat, minLon, maxLat, maxLon
		for _, regionID := range req.Regions {
			reg, ok := osmpkg.RegionByID(regionID)
			if !ok {
				httputil.WriteError(w, http.StatusBadRequest, "Unknown region", fmt.Sprintf("unknown region: %q", regionID), r.URL.Path)
				return
			}
			filterBBox[0] = min(filterBBox[0], reg.MinLat)
			filterBBox[1] = min(filterBBox[1], reg.MinLon)
			filterBBox[2] = max(filterBBox[2], reg.MaxLat)
			filterBBox[3] = max(filterBBox[3], reg.MaxLon)
		}
		opts.Region = req.Regions[0] // informational
		opts.RegionBBox = filterBBox
	}

	if req.Tile != "" {
		var lat, lon int
		if _, err := fmt.Sscanf(req.Tile, "%d,%d", &lat, &lon); err == nil {
			opts.TileLat = lat
			opts.TileLon = lon
			opts.SingleTile = true
		}
	}

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
