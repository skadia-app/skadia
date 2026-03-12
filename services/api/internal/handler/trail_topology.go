package handler

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/skadia-app/skadia/services/api/internal/middleware"
)

// BuildTrailTopology startet einen Background-Job der die pgRouting-Topologie
// für osm_ways aufbaut und Lift-Daten aus osm_lifts importiert.
// POST /api/v1/trails/build-topology
func (h *Handler) BuildTrailTopology(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "Unauthorized", "no user ID", r.URL.Path)
		return
	}

	jobName := "Trail-Topologie"
	jobID, err := h.DB.InsertBackgroundJob(r.Context(), userID, "trail_topology_build", nil, &jobName, 4)
	if err != nil {
		slog.Error("failed to insert trail topology job", "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", "could not create background job", r.URL.Path)
		return
	}

	go h.runTrailTopologyBuild(jobID)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(map[string]string{"job_id": jobID})
}

func (h *Handler) runTrailTopologyBuild(jobID string) {
	ctx := context.Background()

	defer func() {
		if rec := recover(); rec != nil {
			slog.Error("trail topology build panicked", "panic", rec, "job_id", jobID)
			h.DB.CompleteBackgroundJob(ctx, jobID, true)
		}
	}()

	// Phase 1: pgr_createTopology für alle osm_ways
	if err := h.DB.UpdateBackgroundJobPhase(ctx, jobID, "topology"); err != nil {
		slog.Warn("trail topology: failed to update phase", "error", err)
	}
	_, err := h.DB.Pool.Exec(ctx, `
		SELECT pgr_createTopology('osm_ways', 0.00001, 'geom', 'id', 'source', 'target')
	`)
	if err != nil {
		slog.Error("trail topology: pgr_createTopology failed", "error", err, "job_id", jobID)
		h.DB.CompleteBackgroundJob(ctx, jobID, true)
		return
	}
	h.DB.UpdateBackgroundJobProgress(ctx, jobID, 1)

	// Phase 2: cost/reverse_cost aus Tobler-Geschwindigkeit berechnen
	if err := h.DB.UpdateBackgroundJobPhase(ctx, jobID, "cost_compute"); err != nil {
		slog.Warn("trail topology: failed to update phase", "error", err)
	}
	_, err = h.DB.Pool.Exec(ctx, `
		UPDATE osm_ways SET
			cost = CASE
				WHEN is_lift THEN 0.001
				WHEN oneway  THEN -1
				ELSE length_m / NULLIF(tobler_speed(COALESCE(avg_slope, 0)) * 1000.0 / 60.0, 0)
				     * sac_cost_factor(sac_scale)
				     * surface_cost_factor(surface)
			END,
			reverse_cost = CASE
				WHEN is_lift THEN 60.0
				WHEN oneway  THEN -1
				ELSE length_m / NULLIF(tobler_speed(-COALESCE(avg_slope, 0)) * 1000.0 / 60.0, 0)
				     * sac_cost_factor(sac_scale)
				     * surface_cost_factor(surface)
			END
		WHERE cost IS NULL OR reverse_cost IS NULL
	`)
	if err != nil {
		slog.Error("trail topology: cost computation failed", "error", err, "job_id", jobID)
		h.DB.CompleteBackgroundJob(ctx, jobID, true)
		return
	}
	h.DB.UpdateBackgroundJobProgress(ctx, jobID, 2)

	// Phase 3: Lifte aus osm_lifts in osm_ways importieren (E12: WHERE NOT EXISTS)
	if err := h.DB.UpdateBackgroundJobPhase(ctx, jobID, "lift_merge"); err != nil {
		slog.Warn("trail topology: failed to update phase", "error", err)
	}
	_, err = h.DB.Pool.Exec(ctx, `
		INSERT INTO osm_ways (osm_id, highway, name, length_m, avg_slope, is_lift, geom)
		SELECT l.osm_id, 'lift', l.name, ST_Length(l.geom::geography), 0.0, TRUE, l.geom
		FROM osm_lifts l
		WHERE NOT EXISTS (
			SELECT 1 FROM osm_ways t
			WHERE t.osm_id = l.osm_id AND t.is_lift = TRUE
		)
	`)
	if err != nil {
		slog.Error("trail topology: lift merge failed", "error", err, "job_id", jobID)
		h.DB.CompleteBackgroundJob(ctx, jobID, true)
		return
	}
	h.DB.UpdateBackgroundJobProgress(ctx, jobID, 3)

	// Phase 4: Topologie für neu eingefügte Lifte (source/target noch NULL)
	if err := h.DB.UpdateBackgroundJobPhase(ctx, jobID, "lift_topology"); err != nil {
		slog.Warn("trail topology: failed to update phase", "error", err)
	}
	_, err = h.DB.Pool.Exec(ctx, `
		SELECT pgr_createTopology('osm_ways', 0.00001, 'geom', 'id', 'source', 'target')
	`)
	if err != nil {
		// Nicht fatal — Topologie läuft auf bereits vorhandenen Knoten; nur warnen
		slog.Warn("trail topology: second pgr_createTopology (lifts) failed", "error", err, "job_id", jobID)
	}
	h.DB.UpdateBackgroundJobProgress(ctx, jobID, 4)

	slog.Info("trail topology build complete", "job_id", jobID)
	h.DB.CompleteBackgroundJob(ctx, jobID, false)
}
