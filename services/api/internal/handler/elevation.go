package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/exec"

	"github.com/go-chi/chi/v5"

	"github.com/skadia-app/skadia/services/api/internal/httputil"
)

// ResetActivityElevation restores the GPS-recorded altitude values for an activity
// by re-parsing the original file (streams + metrics scope) and then explicitly
// setting elevation_source = 'gps'. Runs as a background job.
//
// POST /api/v1/activities/{id}/reset-elevation
//
// Response: 202 Accepted  { "job_id": "<uuid>", "status": "started" }
func (h *Handler) ResetActivityElevation(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	activity, activityID, ok := h.requireActivity(w, r, userID)
	if !ok {
		return
	}
	h.startResetJob(w, r, userID, activityID, activity.Title, "elevation_reset", 2, h.runElevationResetJob)
}

func (h *Handler) runElevationResetJob(jobID, activityID, userID string) {
	ctx := context.Background()

	defer func() {
		if rec := recover(); rec != nil {
			slog.Error("elevation reset job panicked", "panic", rec, "job_id", jobID)
			h.DB.CompleteBackgroundJob(ctx, jobID, true, "")
		}
	}()

	slog.Info("elevation reset: starting", "job_id", jobID, "activity_id", activityID)

	// Phase 1: Re-parse original file (streams + metrics scope) to restore GPS altitude.
	scope := map[string]bool{"streams": true, "metrics": true}
	if err := h.reprocessActivity(ctx, activityID, userID, scope, 0, false, nil); err != nil {
		slog.Error("elevation reset: reprocess failed", "error", err, "activity_id", activityID)
		h.DB.CompleteBackgroundJob(ctx, jobID, true, "")
		return
	}
	h.DB.UpdateBackgroundJobProgress(ctx, jobID, 1)

	// Phase 2: Explicitly mark elevation_source = 'gps'.
	if err := h.DB.UpdateEnrichmentFields(ctx, activityID, map[string]any{"elevation_source": "gps"}); err != nil {
		slog.Error("elevation reset: failed to update elevation_source", "error", err, "activity_id", activityID)
		h.DB.CompleteBackgroundJob(ctx, jobID, true, "")
		return
	}
	h.DB.UpdateBackgroundJobProgress(ctx, jobID, 2)

	slog.Info("elevation reset: complete", "job_id", jobID, "activity_id", activityID)
	h.DB.CompleteBackgroundJob(ctx, jobID, false, "")
}

// QueryElevation returns DEM elevations for a list of [lng, lat] coordinate pairs.
//
// POST /api/v1/elevation/query
//
//	{
//	  "points": [[lng, lat], ...]
//	}
//
// Response:
//
//	{
//	  "elevations": [float64, ...],
//	  "source":     "srtm30" | "copernicus30" | "none" | ...
//	}
func (h *Handler) QueryElevation(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}

	var req struct {
		Points [][2]float64 `json:"points"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "invalid JSON body", r.URL.Path)
		return
	}
	if len(req.Points) == 0 {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "points must not be empty", r.URL.Path)
		return
	}
	if len(req.Points) > 10000 {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "too many points (max 10000)", r.URL.Path)
		return
	}

	elevations, source, err := h.DB.GetElevation(r.Context(), req.Points)
	if err != nil {
		slog.Error("elevation query failed", "error", err, "user_id", userID)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "elevation query failed", r.URL.Path)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]any{
		"elevations": elevations,
		"source":     source,
	}); err != nil {
		slog.Error("elevation query: encode response", "error", err)
	}
}

// ElevationProfile returns a sampled elevation profile along a GeoJSON geometry.
//
// POST /api/v1/elevation/profile
//
//	{
//	  "geometry":   <GeoJSON geometry object>,
//	  "num_points": 100
//	}
//
// Response:
//
//	{
//	  "profile": [
//	    {"fraction": 0.0, "elevation": 1234.5, "lat": 47.1, "lng": 11.2},
//	    ...
//	  ]
//	}
func (h *Handler) ElevationProfile(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}

	var req struct {
		Geometry  json.RawMessage `json:"geometry"`
		NumPoints int             `json:"num_points"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "invalid JSON body", r.URL.Path)
		return
	}
	if len(req.Geometry) == 0 {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "geometry is required", r.URL.Path)
		return
	}

	profile, err := h.DB.GetElevationProfile(r.Context(), string(req.Geometry), req.NumPoints)
	if err != nil {
		slog.Error("elevation profile failed", "error", err, "user_id", userID)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "elevation profile failed", r.URL.Path)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]any{
		"profile": profile,
	}); err != nil {
		slog.Error("elevation profile: encode response", "error", err)
	}
}

// ImportDEM startet einen Background-Job der nach einem manuellen raster2pgsql-Import
// ANALYZE + VACUUM auf der dem.raster_tiles-Tabelle ausführt und optional einen
// PostGIS-Raster-Overview erstellt.
//
// POST /api/v1/dem/import
//
// Optional request body (all fields optional):
//
//	{
//	  "source": "eea10"        // Label für Log-Ausgaben
//	}
//
// Response: 202 Accepted  { "job_id": "<uuid>" }
//
// Typischer Workflow:
//  1. scripts/dem-import.sh <source> <dir>   → lädt GeoTIFFs per raster2pgsql in DB
//  2. POST /api/v1/dem/import                → ANALYZE + Overview-Raster im Hintergrund
func (h *Handler) ImportDEM(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}

	var req struct {
		Source string `json:"source"`
	}
	// Body is optional — ignore decode errors gracefully
	if r.ContentLength > 0 {
		_ = json.NewDecoder(r.Body).Decode(&req)
	}
	if req.Source == "" {
		req.Source = "unknown"
	}

	jobName := fmt.Sprintf("DEM-Import (%s)", req.Source)
	// 3 phases: analyze → overview → vacuum
	jobID, err := h.DB.InsertBackgroundJob(r.Context(), userID, "dem_import", nil, &jobName, 3)
	if err != nil {
		slog.Error("dem import: failed to insert background job", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "could not create background job", r.URL.Path)
		return
	}

	go h.runDEMImportJob(jobID, req.Source)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(map[string]string{"job_id": jobID}) //nolint:errcheck
}

func (h *Handler) runDEMImportJob(jobID, source string) {
	ctx := context.Background()

	defer func() {
		if rec := recover(); rec != nil {
			slog.Error("dem import job panicked", "panic", rec, "job_id", jobID)
			h.DB.CompleteBackgroundJob(ctx, jobID, true, "")
		}
	}()

	slog.Info("dem import: starting post-import maintenance", "job_id", jobID, "source", source)

	// Phase 1: ANALYZE — aktualisiert Query-Planer-Statistiken nach dem Import
	if err := h.DB.UpdateBackgroundJobPhase(ctx, jobID, "analyze"); err != nil {
		slog.Warn("dem import: failed to update phase", "error", err)
	}
	if _, err := h.DB.Pool.Exec(ctx, `ANALYZE dem.raster_tiles`); err != nil {
		slog.Error("dem import: ANALYZE failed", "error", err, "job_id", jobID)
		h.DB.CompleteBackgroundJob(ctx, jobID, true, "")
		return
	}
	h.DB.UpdateBackgroundJobProgress(ctx, jobID, 1)

	// Phase 2: Overview-Raster (optional, für schnelle Zoom-out-Abfragen)
	// AddRasterConstraints stellt sicher, dass Metadaten korrekt sind.
	if err := h.DB.UpdateBackgroundJobPhase(ctx, jobID, "overview"); err != nil {
		slog.Warn("dem import: failed to update phase", "error", err)
	}
	_, err := h.DB.Pool.Exec(ctx, `
		SELECT AddRasterConstraints('dem', 'raster_tiles', 'rast', TRUE, TRUE, TRUE, TRUE, TRUE, TRUE, FALSE, TRUE, TRUE, TRUE, TRUE, TRUE)
	`)
	if err != nil {
		// Nicht fatal — Constraints können bereits existieren oder Tabelle hat noch keine Daten
		slog.Warn("dem import: AddRasterConstraints skipped", "error", err, "job_id", jobID)
	}
	h.DB.UpdateBackgroundJobProgress(ctx, jobID, 2)

	// Phase 3: VACUUM ANALYZE — gibt Bloat nach upserts frei
	if err := h.DB.UpdateBackgroundJobPhase(ctx, jobID, "vacuum"); err != nil {
		slog.Warn("dem import: failed to update phase", "error", err)
	}
	// VACUUM kann nicht innerhalb einer Transaktion laufen → exec über psql
	if err := runVacuumAnalyzeDEM(ctx); err != nil {
		// Nicht fatal — VACUUM ist eine Optimierung, kein Fehler
		slog.Warn("dem import: VACUUM ANALYZE skipped", "error", err, "job_id", jobID)
	}
	h.DB.UpdateBackgroundJobProgress(ctx, jobID, 3)

	slog.Info("dem import: post-import maintenance complete", "job_id", jobID, "source", source)
	h.DB.CompleteBackgroundJob(ctx, jobID, false, "")
}

// CorrectActivityElevation applies DEM-based elevation correction to an existing
// activity in the background.
//
// POST /api/v1/activities/{id}/correct-elevation
//
// Response: 202 Accepted  { "job_id": "<uuid>", "status": "started" }
//
// The job queries dem.get_elevation() from the PostGIS DEM raster for every
// GPS-bearing stream point, updates altitude_m in activity_streams where the
// DEM value deviates by less than 50 m from the recorded value, and sets
// activities.elevation_source = 'dem'.
func (h *Handler) CorrectActivityElevation(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}

	activityID := chi.URLParam(r, "id")

	// Ownership check.
	activity, err := h.DB.GetActivity(r.Context(), activityID)
	if err != nil || activity == nil || activity.UserID != userID {
		httputil.WriteError(w, http.StatusNotFound, "Not Found", "activity not found", r.URL.Path)
		return
	}

	activityName := activity.Title
	jobID, err := h.DB.InsertBackgroundJob(r.Context(), userID, "elevation_correction", &activityID, activityName, 1)
	if err != nil {
		slog.Error("correct-elevation: failed to insert background job", "activity_id", activityID, "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "could not create background job", r.URL.Path)
		return
	}

	go h.runElevationCorrectionJob(jobID, activityID)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(map[string]string{ //nolint:errcheck
		"job_id": jobID,
		"status": "started",
	})
}

func (h *Handler) runElevationCorrectionJob(jobID, activityID string) {
	ctx := context.Background()

	defer func() {
		if rec := recover(); rec != nil {
			slog.Error("elevation correction job panicked", "panic", rec, "job_id", jobID)
			h.DB.CompleteBackgroundJob(ctx, jobID, true, "")
		}
	}()

	slog.Info("elevation correction: starting", "job_id", jobID, "activity_id", activityID)

	corrected, source, err := h.DB.CorrectActivityElevationFromDEM(ctx, activityID)
	if err != nil {
		slog.Error("elevation correction: DEM query failed", "error", err, "activity_id", activityID)
		h.DB.CompleteBackgroundJob(ctx, jobID, true, "")
		return
	}

	if corrected == 0 {
		slog.Info("elevation correction: no points corrected (no DEM data or no GPS)", "activity_id", activityID)
		h.DB.CompleteBackgroundJob(ctx, jobID, false, "")
		return
	}

	// Update elevation_source on the activity.
	fields := map[string]any{"elevation_source": "dem"}
	if err := h.DB.UpdateEnrichmentFields(ctx, activityID, fields); err != nil {
		slog.Error("elevation correction: failed to update elevation_source", "error", err, "activity_id", activityID)
		h.DB.CompleteBackgroundJob(ctx, jobID, true, "")
		return
	}

	h.DB.UpdateBackgroundJobProgress(ctx, jobID, 1)
	slog.Info("elevation correction: complete", "job_id", jobID, "activity_id", activityID,
		"corrected", corrected, "source", source)
	h.DB.CompleteBackgroundJob(ctx, jobID, false, "")
}

// runVacuumAnalyzeDEM führt VACUUM ANALYZE über ein psql-Subprocess aus,
// da VACUUM nicht innerhalb einer pgx-Transaktion ausgeführt werden kann.
func runVacuumAnalyzeDEM(ctx context.Context) error {
	// DATABASE_URL aus Umgebung lesen (wird beim API-Start gesetzt).
	// Fallback: leerer String → psql nutzt PGDATABASE etc.
	dbURL := os.Getenv("DATABASE_URL")

	args := []string{"-c", "VACUUM ANALYZE dem.raster_tiles;"}
	if dbURL != "" {
		args = append([]string{dbURL}, args...)
	}
	cmd := exec.CommandContext(ctx, "psql", args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("psql VACUUM ANALYZE: %w (output: %s)", err, string(out))
	}
	return nil
}
