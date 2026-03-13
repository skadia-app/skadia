package handler

import (
	"context"
	"net/http"
	"os"
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

	userID := r.Context().Value("user_id").(string)

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
