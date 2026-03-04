package handler

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/skadia-app/skadia/services/api/internal/garmin"
	"github.com/skadia-app/skadia/services/api/internal/importer"
	"github.com/skadia-app/skadia/services/api/internal/middleware"
)

// buildGarminRegistry returns a registry with all available importers.
func (h *Handler) buildGarminRegistry() *garmin.ImportRegistry {
	r := garmin.NewRegistry()
	r.Register(&garmin.ProfileImporter{})
	r.Register(&garmin.GearImporter{})
	r.Register(&garmin.ActivitiesImporter{})
	r.Register(&garmin.StepsImporter{})
	return r
}

// DetectGarminModules probes a Garmin export directory and returns available import modules.
//
// POST /api/v1/import/garmin/detect
// Body: {"source_path": "/path/to/garmin/export"}
func (h *Handler) DetectGarminModules(w http.ResponseWriter, r *http.Request) {
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

	src := garmin.NewDirSource(req.SourcePath)
	registry := h.buildGarminRegistry()
	modules := registry.Detect(src)

	writeJSON(w, http.StatusOK, map[string]interface{}{"modules": modules})
}

// StartGarminImport starts an async Garmin export import job.
//
// POST /api/v1/import/garmin/start
// Body: {"source_path": "/path/to/garmin/export", "modules": ["activities", "profile"]}
func (h *Handler) StartGarminImport(w http.ResponseWriter, r *http.Request) {
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
	jobID, err := h.DB.CreateGarminImportJob(r.Context(), userID, req.SourcePath)
	if err != nil {
		slog.Error("create garmin import job failed", "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to create import job", r.URL.Path)
		return
	}

	// Run import asynchronously
	go func() {
		bgCtx := context.Background()
		src := garmin.NewDirSource(req.SourcePath)
		registry := h.buildGarminRegistry()

		moduleIndex := 0
		params := garmin.ImportParams{
			Source:  src,
			UserID:  userID,
			DB:      h.DB,
			Storage: h.Storage,
			JobID:   jobID,
			OnProgress: func(current, total int, msg string) {
				if err := h.DB.UpdateGarminImportJob(bgCtx, jobID, moduleIndex*100+current, total, msg); err != nil {
					slog.Warn("garmin import: update progress failed", "error", err)
				}
			},
		}
		if req.Options != nil {
			params.Options = *req.Options
		}

		if err := registry.RunModules(bgCtx, req.Modules, params); err != nil {
			slog.Error("garmin import failed", "job_id", jobID, "error", err)
			_ = h.DB.FailGarminImportJob(bgCtx, jobID, err.Error())
			return
		}

		if err := h.DB.CompleteGarminImportJob(bgCtx, jobID); err != nil {
			slog.Warn("garmin import: complete job failed", "job_id", jobID, "error", err)
		}
		slog.Info("garmin import completed", "job_id", jobID)
	}()

	writeJSON(w, http.StatusAccepted, map[string]string{"job_id": jobID})
}

// GetGarminImportJob returns the current status of an import job.
//
// GET /api/v1/import/garmin/jobs/{id}
func (h *Handler) GetGarminImportJob(w http.ResponseWriter, r *http.Request) {
	jobID := chi.URLParam(r, "id")
	userID := middleware.GetUserID(r.Context())

	job, err := h.DB.GetGarminImportJob(r.Context(), userID, jobID)
	if err != nil {
		writeError(w, http.StatusNotFound, "Not Found", "import job not found", r.URL.Path)
		return
	}

	writeJSON(w, http.StatusOK, job)
}
