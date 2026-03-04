package handler

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/skadia-app/skadia/services/api/internal/importer"
	"github.com/skadia-app/skadia/services/api/internal/middleware"
	"github.com/skadia-app/skadia/services/api/internal/strava"
)

// buildStravaRegistry returns a registry with all available Strava importers.
func (h *Handler) buildStravaRegistry() *strava.ImportRegistry {
	r := strava.NewRegistry()
	r.Register(&strava.ProfileImporter{})
	r.Register(&strava.GearImporter{})
	r.Register(&strava.ActivitiesImporter{})
	r.Register(&strava.PrivacyZonesImporter{})
	return r
}

// DetectStravaModules probes a Strava export directory and returns available import modules.
//
// POST /api/v1/import/strava/detect
// Body: {"source_path": "/path/to/strava/export"}
func (h *Handler) DetectStravaModules(w http.ResponseWriter, r *http.Request) {
	var req struct {
		SourcePath string `json:"source_path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Bad Request", "invalid JSON body", r.URL.Path)
		return
	}
	if req.SourcePath == "" {
		writeError(w, http.StatusBadRequest, "Bad Request", "source_path is required", r.URL.Path)
		return
	}

	src := strava.NewDirSource(req.SourcePath)
	registry := h.buildStravaRegistry()
	modules := registry.Detect(src)

	writeJSON(w, http.StatusOK, map[string]interface{}{"modules": modules})
}

// StartStravaImport starts an async Strava export import job.
//
// POST /api/v1/import/strava/start
// Body: {"source_path": "/path/to/strava/export", "modules": ["activities", "profile"]}
func (h *Handler) StartStravaImport(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "Unauthorized", "no user ID in context", r.URL.Path)
		return
	}

	var req struct {
		SourcePath string                  `json:"source_path"`
		Modules    []string                `json:"modules"`
		Options    *importer.ImportOptions `json:"options,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Bad Request", "invalid JSON body", r.URL.Path)
		return
	}
	if req.SourcePath == "" {
		writeError(w, http.StatusBadRequest, "Bad Request", "source_path is required", r.URL.Path)
		return
	}

	// Create job record in DB
	jobID, err := h.DB.CreateStravaImportJob(r.Context(), userID, req.SourcePath)
	if err != nil {
		slog.Error("create strava import job failed", "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to create import job", r.URL.Path)
		return
	}

	// Run import asynchronously
	go func() {
		bgCtx := context.Background()
		src := strava.NewDirSource(req.SourcePath)
		registry := h.buildStravaRegistry()

		moduleIndex := 0
		params := strava.ImportParams{
			Source:  src,
			UserID:  userID,
			DB:      h.DB,
			Storage: h.Storage,
			JobID:   jobID,
			OnProgress: func(current, total int, msg string) {
				if err := h.DB.UpdateStravaImportJob(bgCtx, jobID, moduleIndex*100+current, total, msg); err != nil {
					slog.Warn("strava import: update progress failed", "error", err)
				}
			},
		}
		if req.Options != nil {
			params.Options = *req.Options
		}

		if err := registry.RunModules(bgCtx, req.Modules, params); err != nil {
			slog.Error("strava import failed", "job_id", jobID, "error", err)
			_ = h.DB.FailStravaImportJob(bgCtx, jobID, err.Error())
			return
		}

		if err := h.DB.CompleteStravaImportJob(bgCtx, jobID); err != nil {
			slog.Warn("strava import: complete job failed", "job_id", jobID, "error", err)
		}
		slog.Info("strava import completed", "job_id", jobID)
	}()

	writeJSON(w, http.StatusAccepted, map[string]string{"job_id": jobID})
}

// GetStravaImportJob returns the current status of an import job.
//
// GET /api/v1/import/strava/jobs/{id}
func (h *Handler) GetStravaImportJob(w http.ResponseWriter, r *http.Request) {
	jobID := chi.URLParam(r, "id")
	userID := middleware.GetUserID(r.Context())

	job, err := h.DB.GetStravaImportJob(r.Context(), userID, jobID)
	if err != nil {
		writeError(w, http.StatusNotFound, "Not Found", "import job not found", r.URL.Path)
		return
	}

	writeJSON(w, http.StatusOK, job)
}
