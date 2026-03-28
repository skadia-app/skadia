package handler

import (
	"archive/zip"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/skadia-app/skadia/services/api/internal/garmin"
	"github.com/skadia-app/skadia/services/api/internal/httputil"
	"github.com/skadia-app/skadia/services/api/internal/importer"
	"github.com/skadia-app/skadia/services/api/internal/importutil"
)

// buildGarminRegistry returns a registry with all available importers.
func (h *Handler) buildGarminRegistry() *garmin.ImportRegistry {
	r := garmin.NewRegistry()
	r.Register(&garmin.ProfileImporter{})
	r.Register(&garmin.GearImporter{})
	r.Register(&garmin.ActivitiesImporter{})
	r.Register(&garmin.StepsImporter{})
	r.Register(&garmin.MonitoringFITImporter{})
	r.Register(&garmin.CoursesImporter{})
	return r
}

// garminJobCallbacks returns the importJobCallbacks wired to the Garmin DB methods.
func (h *Handler) garminJobCallbacks() importJobCallbacks {
	return importJobCallbacks{
		label:     "garmin",
		createJob: h.DB.CreateGarminImportJob,
		failJob: func(ctx context.Context, jobID, errMsg string) error {
			return h.DB.FailGarminImportJob(ctx, jobID, errMsg)
		},
		completeJob: func(ctx context.Context, jobID string, errs []string) error {
			return h.DB.CompleteGarminImportJob(ctx, jobID, errs)
		},
		updateCurrentModule: func(ctx context.Context, jobID, module string) error {
			return h.DB.UpdateGarminImportCurrentModule(ctx, jobID, module)
		},
		updateModuleProgress: func(ctx context.Context, jobID string, completed, total int) error {
			return h.DB.UpdateGarminImportModuleProgress(ctx, jobID, completed, total)
		},
	}
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
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "invalid JSON body", r.URL.Path)
		return
	}
	if _, err := importutil.ValidateImportSourcePath(req.SourcePath); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", err.Error(), r.URL.Path)
		return
	}

	src := garmin.NewDirSource(req.SourcePath)
	registry := h.buildGarminRegistry()
	modules := registry.Detect(src)

	httputil.WriteJSON(w, http.StatusOK, map[string]any{"modules": modules})
}

// StartGarminImport starts an async Garmin export import job.
//
// POST /api/v1/import/garmin/start
// Body: {"source_path": "/path/to/garmin/export", "modules": ["activities", "profile"]}
func (h *Handler) StartGarminImport(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}

	var req struct {
		SourcePath string                  `json:"source_path"`
		Modules    []string                `json:"modules"`
		Options    *importer.ImportOptions `json:"options,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "invalid JSON body", r.URL.Path)
		return
	}
	if _, err := importutil.ValidateImportSourcePath(req.SourcePath); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", err.Error(), r.URL.Path)
		return
	}

	cb := h.garminJobCallbacks()
	h.startImportAsync(w, r, userID, req.SourcePath, isTempSourcePath(req.SourcePath), cb,
		func(ctx context.Context, jobID string) {
			src := garmin.NewDirSource(req.SourcePath)
			registry := h.buildGarminRegistry()
			onProgress, onModuleComplete := makeProgressCallbacks(cb, "garmin import", jobID)

			params := garmin.ImportParams{
				Source:           src,
				UserID:           userID,
				DB:               h.DB,
				ShadowFS:         h.ShadowFS,
				ImportService:    h.ImportService,
				JobID:            jobID,
				OnProgress:       onProgress,
				OnModuleComplete: onModuleComplete,
			}
			if req.Options != nil {
				params.Options = *req.Options
			}

			allErrors, err := registry.RunModules(ctx, req.Modules, params)
			if err != nil {
				slog.Error("garmin import failed", "job_id", jobID, "error", err)
				_ = cb.failJob(ctx, jobID, err.Error())
				return
			}
			if err := cb.completeJob(ctx, jobID, allErrors); err != nil {
				slog.Warn("garmin import: complete job failed", "job_id", jobID, "error", err)
			}
			slog.Info("garmin import completed", "job_id", jobID, "errors", len(allErrors))
		},
	)
}

// UploadGarminZIP accepts a multipart ZIP upload of a Garmin GDPR export, extracts it to
// a temporary directory and immediately starts an async import job for all modules.
//
// POST /api/v1/import/garmin/upload-zip
// Body: multipart/form-data { file: <export.zip> }
func (h *Handler) UploadGarminZIP(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}

	tmpDir, ok := parseUploadedZIP(w, r, r.URL.Path)
	if !ok {
		return
	}

	cb := h.garminJobCallbacks()
	h.startImportAsync(w, r, userID, tmpDir, true, cb,
		func(ctx context.Context, jobID string) {
			src := garmin.NewDirSource(tmpDir)
			registry := h.buildGarminRegistry()
			allModules := registry.Detect(src)
			moduleNames := make([]string, 0, len(allModules))
			for _, m := range allModules {
				if m.Available {
					moduleNames = append(moduleNames, m.Name)
				}
			}

			onProgress, onModuleComplete := makeProgressCallbacks(cb, "garmin import (zip)", jobID)
			params := garmin.ImportParams{
				Source:           src,
				UserID:           userID,
				DB:               h.DB,
				ShadowFS:         h.ShadowFS,
				ImportService:    h.ImportService,
				JobID:            jobID,
				OnProgress:       onProgress,
				OnModuleComplete: onModuleComplete,
			}

			allErrors, err := registry.RunModules(ctx, moduleNames, params)
			if err != nil {
				slog.Error("garmin import (zip) failed", "job_id", jobID, "error", err)
				_ = cb.failJob(ctx, jobID, err.Error())
				return
			}
			if err := cb.completeJob(ctx, jobID, allErrors); err != nil {
				slog.Warn("garmin import (zip): complete job failed", "job_id", jobID, "error", err)
			}
			slog.Info("garmin import (zip) completed", "job_id", jobID, "errors", len(allErrors))
		},
	)
}

// GetGarminImportJob returns the current status of an import job.
//
// GET /api/v1/import/garmin/jobs/{id}
func (h *Handler) GetGarminImportJob(w http.ResponseWriter, r *http.Request) {
	jobID := chi.URLParam(r, "id")
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}

	job, err := h.DB.GetGarminImportJob(r.Context(), userID, jobID)
	if err != nil {
		httputil.WriteError(w, http.StatusNotFound, "Not Found", "import job not found", r.URL.Path)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, job)
}

// ScanGarminExport analyses a Garmin export directory without writing to the DB.
// For each import module it returns how many items were found, how many already
// exist in the database, and how many would be new imports.
//
// POST /api/v1/import/garmin/scan
// Body: {"source_path": "/path/to/garmin/export"}
func (h *Handler) ScanGarminExport(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}

	var req struct {
		SourcePath string `json:"source_path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "invalid JSON body", r.URL.Path)
		return
	}
	if _, err := importutil.ValidateImportSourcePath(req.SourcePath); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", err.Error(), r.URL.Path)
		return
	}

	src := garmin.NewDirSource(req.SourcePath)
	registry := h.buildGarminRegistry()
	modules := registry.Detect(src)

	result := buildScanResult(r.Context(), modules, func(ctx context.Context, name string) (int, error) {
		if name == "activities" {
			return scanExistingGarminActivities(ctx, h, userID, src)
		}
		return 0, nil
	})

	httputil.WriteJSON(w, http.StatusOK, result)
}

// scanExistingGarminActivities iterates all activity files inside the Garmin UploadedFiles ZIPs,
// collects their Garmin activity IDs, and returns a count of how many already exist in the DB.
// This is read-only: no data is written.
func scanExistingGarminActivities(ctx context.Context, h *Handler, userID string, src *garmin.DirSource) (int, error) {
	zips, err := src.FindFiles("DI_CONNECT/DI-Connect-Uploaded-Files/UploadedFiles_*.zip")
	if err != nil || len(zips) == 0 {
		return 0, nil
	}

	var garminIDs []int64
	seen := make(map[int64]struct{})

	for _, z := range zips {
		zr, err := zip.OpenReader(filepath.Join(src.BaseDir(), z))
		if err != nil {
			slog.Warn("garmin scan: cannot open zip", "zip", z, "error", err)
			continue
		}
		for _, f := range zr.File {
			if !isGarminActivityFile(f.Name) {
				continue
			}
			id := garmin.ExtractActivityID(f.Name)
			if id <= 0 {
				continue
			}
			if _, dup := seen[id]; dup {
				continue
			}
			seen[id] = struct{}{}
			garminIDs = append(garminIDs, id)
		}
		zr.Close()
	}

	if len(garminIDs) == 0 {
		return 0, nil
	}

	return h.DB.CountExistingGarminActivityIDs(ctx, userID, garminIDs)
}

// isGarminActivityFile returns true for FIT, GPX, and TCX files.
func isGarminActivityFile(name string) bool {
	lower := strings.ToLower(name)
	return strings.HasSuffix(lower, ".fit") ||
		strings.HasSuffix(lower, ".gpx") ||
		strings.HasSuffix(lower, ".tcx")
}

// UploadGarminZIPScan accepts a ZIP upload, extracts it to a temp directory, runs a scan
// (without starting the import) and returns the scan result + the temp dir path so the
// frontend can show the config wizard before the actual import begins.
//
// POST /api/v1/import/garmin/upload-zip-scan
// Body: multipart/form-data { file: <export.zip> }
func (h *Handler) UploadGarminZIPScan(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}

	tmpDir, ok := parseUploadedZIP(w, r, r.URL.Path)
	if !ok {
		return
	}

	src := garmin.NewDirSource(tmpDir)
	registry := h.buildGarminRegistry()
	modules := registry.Detect(src)

	scanResult := buildScanResult(r.Context(), modules, func(ctx context.Context, name string) (int, error) {
		if name == "activities" {
			return scanExistingGarminActivities(ctx, h, userID, src)
		}
		return 0, nil
	})

	httputil.WriteJSON(w, http.StatusOK, map[string]any{
		"source_path": tmpDir,
		"scan_result": scanResult,
	})
}
