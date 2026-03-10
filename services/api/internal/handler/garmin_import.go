package handler

import (
	"archive/zip"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"

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
	r.Register(&garmin.MonitoringFITImporter{})
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
	if !strings.HasPrefix(filepath.Clean(req.SourcePath), os.TempDir()) {
		writeError(w, http.StatusBadRequest, "Bad Request", "invalid source path", r.URL.Path)
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
	if !strings.HasPrefix(filepath.Clean(req.SourcePath), os.TempDir()) {
		writeError(w, http.StatusBadRequest, "Bad Request", "invalid source path", r.URL.Path)
		return
	}

	// Create job record in DB
	jobID, err := h.DB.CreateGarminImportJob(r.Context(), userID, req.SourcePath)
	if err != nil {
		slog.Error("create garmin import job failed", "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to create import job", r.URL.Path)
		return
	}

	// Run import asynchronously; clean up temp dir afterwards if ZIP was uploaded
	isTempPath := strings.HasPrefix(req.SourcePath, os.TempDir())
	go func() {
		if isTempPath {
			defer os.RemoveAll(req.SourcePath)
		}
		bgCtx := context.Background()
		src := garmin.NewDirSource(req.SourcePath)
		registry := h.buildGarminRegistry()

		params := garmin.ImportParams{
			Source:  src,
			UserID:  userID,
			DB:      h.DB,
			Storage: h.Storage,
			JobID:   jobID,
			OnProgress: func(current, total int, msg string) {
				if err := h.DB.UpdateGarminImportCurrentModule(bgCtx, jobID, msg); err != nil {
					slog.Warn("garmin import: update current module failed", "error", err)
				}
			},
			OnModuleComplete: func(completed, total int, name string) {
				if err := h.DB.UpdateGarminImportModuleProgress(bgCtx, jobID, completed, total); err != nil {
					slog.Warn("garmin import: update module progress failed", "error", err)
				}
			},
		}
		if req.Options != nil {
			params.Options = *req.Options
		}

		allErrors, err := registry.RunModules(bgCtx, req.Modules, params)
		if err != nil {
			slog.Error("garmin import failed", "job_id", jobID, "error", err)
			_ = h.DB.FailGarminImportJob(bgCtx, jobID, err.Error())
			return
		}

		if err := h.DB.CompleteGarminImportJob(bgCtx, jobID, allErrors); err != nil {
			slog.Warn("garmin import: complete job failed", "job_id", jobID, "error", err)
		}
		slog.Info("garmin import completed", "job_id", jobID, "errors", len(allErrors))
	}()

	writeJSON(w, http.StatusAccepted, map[string]string{"job_id": jobID})
}

// UploadGarminZIP accepts a multipart ZIP upload of a Garmin GDPR export, extracts it to
// a temporary directory and immediately starts an async import job for all modules.
//
// POST /api/v1/import/garmin/upload-zip
// Body: multipart/form-data { file: <export.zip> }
func (h *Handler) UploadGarminZIP(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "Unauthorized", "no user ID in context", r.URL.Path)
		return
	}

	tmpDir, ok := parseUploadedZIP(w, r, r.URL.Path)
	if !ok {
		return
	}

	jobID, err := h.DB.CreateGarminImportJob(r.Context(), userID, tmpDir)
	if err != nil {
		slog.Error("create garmin import job (zip) failed", "error", err)
		os.RemoveAll(tmpDir)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to create import job", r.URL.Path)
		return
	}

	go func() {
		defer os.RemoveAll(tmpDir)

		bgCtx := context.Background()
		src := garmin.NewDirSource(tmpDir)
		registry := h.buildGarminRegistry()
		allModules := registry.Detect(src)
		moduleNames := make([]string, 0, len(allModules))
		for _, m := range allModules {
			if m.Available {
				moduleNames = append(moduleNames, m.Name)
			}
		}

		params := garmin.ImportParams{
			Source:  src,
			UserID:  userID,
			DB:      h.DB,
			Storage: h.Storage,
			JobID:   jobID,
			OnProgress: func(current, total int, msg string) {
				if err := h.DB.UpdateGarminImportCurrentModule(bgCtx, jobID, msg); err != nil {
					slog.Warn("garmin import (zip): update current module failed", "error", err)
				}
			},
			OnModuleComplete: func(completed, total int, name string) {
				if err := h.DB.UpdateGarminImportModuleProgress(bgCtx, jobID, completed, total); err != nil {
					slog.Warn("garmin import (zip): update module progress failed", "error", err)
				}
			},
		}

		allErrors, err := registry.RunModules(bgCtx, moduleNames, params)
		if err != nil {
			slog.Error("garmin import (zip) failed", "job_id", jobID, "error", err)
			_ = h.DB.FailGarminImportJob(bgCtx, jobID, err.Error())
			return
		}
		if err := h.DB.CompleteGarminImportJob(bgCtx, jobID, allErrors); err != nil {
			slog.Warn("garmin import (zip): complete job failed", "job_id", jobID, "error", err)
		}
		slog.Info("garmin import (zip) completed", "job_id", jobID, "errors", len(allErrors))
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

// ScanGarminExport analyses a Garmin export directory without writing to the DB.
// For each import module it returns how many items were found, how many already
// exist in the database, and how many would be new imports.
//
// POST /api/v1/import/garmin/scan
// Body: {"source_path": "/path/to/garmin/export"}
func (h *Handler) ScanGarminExport(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "Unauthorized", "no user ID in context", r.URL.Path)
		return
	}

	var req struct {
		SourcePath string `json:"source_path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.SourcePath == "" {
		writeError(w, http.StatusBadRequest, "Bad Request", "source_path required", r.URL.Path)
		return
	}
	if !strings.HasPrefix(filepath.Clean(req.SourcePath), os.TempDir()) {
		writeError(w, http.StatusBadRequest, "Bad Request", "invalid source path", r.URL.Path)
		return
	}

	src := garmin.NewDirSource(req.SourcePath)
	registry := h.buildGarminRegistry()

	type categoryStats struct {
		Found    int `json:"found"`
		Existing int `json:"existing"`
		New      int `json:"new"`
	}
	result := map[string]categoryStats{}

	// Detect all modules to get item counts
	modules := registry.Detect(src)
	for _, m := range modules {
		if !m.Available {
			continue
		}
		stats := categoryStats{Found: m.ItemCount}
		// For activities: check how many Garmin activity IDs already exist in the DB.
		if m.Name == "activities" {
			existing, err := scanExistingGarminActivities(r.Context(), h, userID, src)
			if err != nil {
				slog.Warn("garmin scan: activity existence check failed", "error", err)
			} else {
				stats.Existing = existing
			}
		}
		stats.New = stats.Found - stats.Existing
		if stats.New < 0 {
			stats.New = 0
		}
		result[m.Name] = stats
	}

	writeJSON(w, http.StatusOK, result)
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
	userID := middleware.GetUserID(r.Context())
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "Unauthorized", "no user ID in context", r.URL.Path)
		return
	}

	tmpDir, ok := parseUploadedZIP(w, r, r.URL.Path)
	if !ok {
		return
	}

	type categoryStats struct {
		Found    int `json:"found"`
		Existing int `json:"existing"`
		New      int `json:"new"`
	}
	scanResult := map[string]categoryStats{}

	src := garmin.NewDirSource(tmpDir)
	registry := h.buildGarminRegistry()
	modules := registry.Detect(src)
	for _, m := range modules {
		if !m.Available {
			continue
		}
		stats := categoryStats{Found: m.ItemCount}
		if m.Name == "activities" {
			existing, err := scanExistingGarminActivities(r.Context(), h, userID, src)
			if err != nil {
				slog.Warn("garmin zip scan: activity existence check failed", "error", err)
			} else {
				stats.Existing = existing
			}
		}
		stats.New = stats.Found - stats.Existing
		if stats.New < 0 {
			stats.New = 0
		}
		scanResult[m.Name] = stats
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"source_path": tmpDir,
		"scan_result": scanResult,
	})
}
