package handler

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/skadia-app/skadia/services/api/internal/httputil"
	"github.com/skadia-app/skadia/services/api/internal/importer"
	"github.com/skadia-app/skadia/services/api/internal/importutil"
	"github.com/skadia-app/skadia/services/api/internal/strava"
)

// buildStravaRegistry returns a registry with all available Strava importers.
func (h *Handler) buildStravaRegistry() *strava.ImportRegistry {
	r := strava.NewRegistry()
	r.Register(&strava.ProfileImporter{})
	r.Register(&strava.GearImporter{})
	r.Register(&strava.ActivitiesImporter{})
	r.Register(&strava.PrivacyZonesImporter{})
	r.Register(&strava.MediaImporter{})
	r.Register(&strava.RoutesImporter{})
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
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "invalid JSON body", r.URL.Path)
		return
	}
	if _, err := importutil.ValidateImportSourcePath(req.SourcePath); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", err.Error(), r.URL.Path)
		return
	}

	src := strava.NewDirSource(req.SourcePath)
	registry := h.buildStravaRegistry()
	modules := registry.Detect(src)

	httputil.WriteJSON(w, http.StatusOK, map[string]any{"modules": modules})
}

// StartStravaImport starts an async Strava export import job.
//
// POST /api/v1/import/strava/start
// Body: {"source_path": "/path/to/strava/export", "modules": ["activities", "profile"]}
func (h *Handler) StartStravaImport(w http.ResponseWriter, r *http.Request) {
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

	// Create job record in DB
	jobID, err := h.DB.CreateStravaImportJob(r.Context(), userID, req.SourcePath)
	if err != nil {
		slog.Error("create strava import job failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to create import job", r.URL.Path)
		return
	}

	// Run import asynchronously; clean up temp dir afterwards if ZIP was uploaded
	isTempPath := strings.HasPrefix(req.SourcePath, os.TempDir())
	go func() {
		if isTempPath {
			defer os.RemoveAll(req.SourcePath)
		}
		bgCtx := context.Background()
		src := strava.NewDirSource(req.SourcePath)
		registry := h.buildStravaRegistry()

		params := strava.ImportParams{
			Source:        src,
			UserID:        userID,
			DB:            h.DB,
			ShadowFS:      h.ShadowFS,
			ImportService: h.ImportService,
			JobID:         jobID,
			OnProgress: func(current, total int, msg string) {
				if err := h.DB.UpdateStravaImportCurrentModule(bgCtx, jobID, msg); err != nil {
					slog.Warn("strava import: update current module failed", "error", err)
				}
			},
			OnModuleComplete: func(completed, total int, name string) {
				if err := h.DB.UpdateStravaImportModuleProgress(bgCtx, jobID, completed, total); err != nil {
					slog.Warn("strava import: update module progress failed", "error", err)
				}
			},
		}
		if req.Options != nil {
			params.Options = *req.Options
		}

		allErrors, err := registry.RunModules(bgCtx, req.Modules, params)
		if err != nil {
			slog.Error("strava import failed", "job_id", jobID, "error", err)
			_ = h.DB.FailStravaImportJob(bgCtx, jobID, err.Error())
			return
		}

		if err := h.DB.CompleteStravaImportJob(bgCtx, jobID, allErrors); err != nil {
			slog.Warn("strava import: complete job failed", "job_id", jobID, "error", err)
		}
		slog.Info("strava import completed", "job_id", jobID, "errors", len(allErrors))
	}()

	httputil.WriteJSON(w, http.StatusAccepted, map[string]string{"job_id": jobID})
}

// UploadStravaZIP accepts a multipart ZIP upload of a Strava data export, extracts it to
// a temporary directory and immediately starts an async import job for all modules.
//
// POST /api/v1/import/strava/upload-zip
// Body: multipart/form-data { file: <export.zip> }
func (h *Handler) UploadStravaZIP(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}

	tmpDir, ok := parseUploadedZIP(w, r, r.URL.Path)
	if !ok {
		return
	}

	jobID, err := h.DB.CreateStravaImportJob(r.Context(), userID, tmpDir)
	if err != nil {
		slog.Error("create strava import job (zip) failed", "error", err)
		os.RemoveAll(tmpDir)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to create import job", r.URL.Path)
		return
	}

	go func() {
		defer os.RemoveAll(tmpDir)

		bgCtx := context.Background()
		src := strava.NewDirSource(tmpDir)
		registry := h.buildStravaRegistry()
		allModules := registry.Detect(src)
		moduleNames := make([]string, 0, len(allModules))
		for _, m := range allModules {
			if m.Available {
				moduleNames = append(moduleNames, m.Name)
			}
		}

		params := strava.ImportParams{
			Source:        src,
			UserID:        userID,
			DB:            h.DB,
			ShadowFS:      h.ShadowFS,
			ImportService: h.ImportService,
			JobID:         jobID,
			OnProgress: func(current, total int, msg string) {
				if err := h.DB.UpdateStravaImportCurrentModule(bgCtx, jobID, msg); err != nil {
					slog.Warn("strava import (zip): update current module failed", "error", err)
				}
			},
			OnModuleComplete: func(completed, total int, name string) {
				if err := h.DB.UpdateStravaImportModuleProgress(bgCtx, jobID, completed, total); err != nil {
					slog.Warn("strava import (zip): update module progress failed", "error", err)
				}
			},
		}

		allErrors, err := registry.RunModules(bgCtx, moduleNames, params)
		if err != nil {
			slog.Error("strava import (zip) failed", "job_id", jobID, "error", err)
			_ = h.DB.FailStravaImportJob(bgCtx, jobID, err.Error())
			return
		}
		if err := h.DB.CompleteStravaImportJob(bgCtx, jobID, allErrors); err != nil {
			slog.Warn("strava import (zip): complete job failed", "job_id", jobID, "error", err)
		}
		slog.Info("strava import (zip) completed", "job_id", jobID, "errors", len(allErrors))
	}()

	httputil.WriteJSON(w, http.StatusAccepted, map[string]string{"job_id": jobID})
}

// GetStravaImportJob returns the current status of an import job.
//
// GET /api/v1/import/strava/jobs/{id}
func (h *Handler) GetStravaImportJob(w http.ResponseWriter, r *http.Request) {
	jobID := chi.URLParam(r, "id")
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}

	job, err := h.DB.GetStravaImportJob(r.Context(), userID, jobID)
	if err != nil {
		httputil.WriteError(w, http.StatusNotFound, "Not Found", "import job not found", r.URL.Path)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, job)
}

// ScanStravaExport analyses a Strava export directory without writing to the DB.
// For each import module it returns how many items were found, how many already
// exist in the database, and how many would be new imports.
//
// POST /api/v1/import/strava/scan
// Body: {"source_path": "/path/to/strava/export"}
func (h *Handler) ScanStravaExport(w http.ResponseWriter, r *http.Request) {
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

	src := strava.NewDirSource(req.SourcePath)
	registry := h.buildStravaRegistry()

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
		// For activities: check how many Strava activity IDs already exist in the DB.
		if m.Name == "activities" {
			existing, err := scanExistingStravaActivities(r.Context(), h, userID, src)
			if err != nil {
				slog.Warn("strava scan: activity existence check failed", "error", err)
			} else {
				stats.Existing = existing
			}
		} else if m.Name == "media" {
			existing, err := scanExistingStravaMedia(r.Context(), h, userID, src)
			if err != nil {
				slog.Warn("strava scan: media existence check failed", "error", err)
			} else {
				stats.Existing = existing
			}
		}
		stats.New = max(stats.Found-stats.Existing, 0)
		result[m.Name] = stats
	}

	httputil.WriteJSON(w, http.StatusOK, result)
}

// scanExistingStravaActivities reads activities.csv, collects all Strava activity IDs,
// and returns a count of how many already exist in the DB. Read-only — no writes.
func scanExistingStravaActivities(ctx context.Context, h *Handler, userID string, src *strava.DirSource) (int, error) {
	rows, err := src.ReadCSV("activities.csv")
	if err != nil || len(rows) < 2 {
		return 0, nil
	}

	hdr := stravaCSVHeader(rows[0])
	idColIdx, ok := hdr["Aktivitäts-ID"]
	if !ok {
		// Fallback: column 0 is always the activity ID
		idColIdx = 0
	}

	var stravaIDs []int64
	seen := make(map[int64]struct{})

	for _, row := range rows[1:] {
		if idColIdx >= len(row) {
			continue
		}
		idStr := row[idColIdx]
		id, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil || id <= 0 {
			continue
		}
		if _, dup := seen[id]; dup {
			continue
		}
		seen[id] = struct{}{}
		stravaIDs = append(stravaIDs, id)
	}

	if len(stravaIDs) == 0 {
		return 0, nil
	}

	return h.DB.CountExistingStravaActivityIDs(ctx, userID, stravaIDs)
}

// scanExistingStravaMedia reads media.csv, opens each referenced photo file, computes its
// SHA256 hash and checks whether a photo with that hash already exists for the user.
// Returns the count of photos that already exist in the DB. Read-only — no writes.
func scanExistingStravaMedia(ctx context.Context, h *Handler, userID string, src *strava.DirSource) (int, error) {
	rows, err := src.ReadCSV("media.csv")
	if err != nil || len(rows) < 2 {
		return 0, nil
	}

	hdr := stravaCSVHeader(rows[0])
	fileColIdx, ok := hdr["Medien-Dateiname"]
	if !ok {
		// Fallback: try English column name
		fileColIdx, ok = hdr["Media Filename"]
		if !ok {
			return 0, nil
		}
	}

	existing := 0
	for _, row := range rows[1:] {
		if ctx.Err() != nil {
			break
		}
		if fileColIdx >= len(row) {
			continue
		}
		relPath := strings.TrimSpace(row[fileColIdx])
		if relPath == "" {
			continue
		}

		data, err := src.ReadFile(relPath)
		if err != nil {
			// File not present in the export — skip
			continue
		}

		hashBytes := sha256.Sum256(data)
		hash := fmt.Sprintf("%x", hashBytes)

		exists, err := h.DB.ActivityPhotoExistsByHash(ctx, userID, hash)
		if err != nil {
			slog.Warn("strava scan: media hash check failed", "file", relPath, "error", err)
			continue
		}
		if exists {
			existing++
		}
	}

	return existing, nil
}

// stravaCSVHeader builds a column-name → column-index map from a Strava CSV header row.
func stravaCSVHeader(row []string) map[string]int {
	h := make(map[string]int, len(row))
	for i, col := range row {
		h[col] = i
	}
	return h
}

// UploadStravaZIPScan accepts a ZIP upload, extracts it to a temp directory, runs a scan
// (without starting the import) and returns the scan result + the temp dir path so the
// frontend can show the config wizard before the actual import begins.
//
// POST /api/v1/import/strava/upload-zip-scan
// Body: multipart/form-data { file: <export.zip> }
func (h *Handler) UploadStravaZIPScan(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
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

	src := strava.NewDirSource(tmpDir)
	registry := h.buildStravaRegistry()
	modules := registry.Detect(src)
	for _, m := range modules {
		if !m.Available {
			continue
		}
		stats := categoryStats{Found: m.ItemCount}
		if m.Name == "activities" {
			existing, err := scanExistingStravaActivities(r.Context(), h, userID, src)
			if err != nil {
				slog.Warn("strava zip scan: activity existence check failed", "error", err)
			} else {
				stats.Existing = existing
			}
		} else if m.Name == "media" {
			existing, err := scanExistingStravaMedia(r.Context(), h, userID, src)
			if err != nil {
				slog.Warn("strava zip scan: media existence check failed", "error", err)
			} else {
				stats.Existing = existing
			}
		}
		stats.New = max(stats.Found-stats.Existing, 0)
		scanResult[m.Name] = stats
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]any{
		"source_path": tmpDir,
		"scan_result": scanResult,
	})
}
