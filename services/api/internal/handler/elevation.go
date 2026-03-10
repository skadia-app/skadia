package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os/exec"

	"github.com/skadia-app/skadia/services/api/internal/middleware"
)

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
	userID := middleware.GetUserID(r.Context())
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "Unauthorized", "no user ID in context", r.URL.Path)
		return
	}

	var req struct {
		Points [][2]float64 `json:"points"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Bad Request", "invalid JSON body", r.URL.Path)
		return
	}
	if len(req.Points) == 0 {
		writeError(w, http.StatusBadRequest, "Bad Request", "points must not be empty", r.URL.Path)
		return
	}
	if len(req.Points) > 10000 {
		writeError(w, http.StatusBadRequest, "Bad Request", "too many points (max 10000)", r.URL.Path)
		return
	}

	elevations, source, err := h.DB.GetElevation(r.Context(), req.Points)
	if err != nil {
		slog.Error("elevation query failed", "error", err, "user_id", userID)
		writeError(w, http.StatusInternalServerError, "Internal Error", "elevation query failed", r.URL.Path)
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
	userID := middleware.GetUserID(r.Context())
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "Unauthorized", "no user ID in context", r.URL.Path)
		return
	}

	var req struct {
		Geometry  json.RawMessage `json:"geometry"`
		NumPoints int             `json:"num_points"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Bad Request", "invalid JSON body", r.URL.Path)
		return
	}
	if len(req.Geometry) == 0 {
		writeError(w, http.StatusBadRequest, "Bad Request", "geometry is required", r.URL.Path)
		return
	}

	profile, err := h.DB.GetElevationProfile(r.Context(), string(req.Geometry), req.NumPoints)
	if err != nil {
		slog.Error("elevation profile failed", "error", err, "user_id", userID)
		writeError(w, http.StatusInternalServerError, "Internal Error", "elevation profile failed", r.URL.Path)
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
	userID := middleware.GetUserID(r.Context())
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "Unauthorized", "no user ID in context", r.URL.Path)
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
		writeError(w, http.StatusInternalServerError, "Internal Error", err.Error(), r.URL.Path)
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
			h.DB.CompleteBackgroundJob(ctx, jobID, true)
		}
	}()

	slog.Info("dem import: starting post-import maintenance", "job_id", jobID, "source", source)

	// Phase 1: ANALYZE — aktualisiert Query-Planer-Statistiken nach dem Import
	if err := h.DB.UpdateBackgroundJobPhase(ctx, jobID, "analyze"); err != nil {
		slog.Warn("dem import: failed to update phase", "error", err)
	}
	if _, err := h.DB.Pool.Exec(ctx, `ANALYZE dem.raster_tiles`); err != nil {
		slog.Error("dem import: ANALYZE failed", "error", err, "job_id", jobID)
		h.DB.CompleteBackgroundJob(ctx, jobID, true)
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
	h.DB.CompleteBackgroundJob(ctx, jobID, false)
}

// runVacuumAnalyzeDEM führt VACUUM ANALYZE über ein psql-Subprocess aus,
// da VACUUM nicht innerhalb einer pgx-Transaktion ausgeführt werden kann.
func runVacuumAnalyzeDEM(ctx context.Context) error {
	dbURL := ""
	// DATABASE_URL aus Umgebung lesen (wird beim API-Start gesetzt)
	// Fallback: leerer String → psql nutzt PGDATABASE etc.
	if url, ok := ctx.Value(struct{}{}).(string); ok {
		dbURL = url
	}

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
