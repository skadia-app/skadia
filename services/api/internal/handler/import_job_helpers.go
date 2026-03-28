package handler

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"strings"

	"github.com/skadia-app/skadia/services/api/internal/httputil"
	"github.com/skadia-app/skadia/services/api/internal/importutil"
)

// importJobCallbacks bundles the DB operations that differ between Garmin and Strava.
type importJobCallbacks struct {
	// label is used in log messages, e.g. "garmin" or "strava".
	label string
	// createJob inserts a new import job row and returns the job UUID.
	createJob func(ctx context.Context, userID, sourcePath string) (string, error)
	// failJob marks a job as failed with the given error message.
	failJob func(ctx context.Context, jobID, errMsg string) error
	// completeJob marks a job as completed with the collected non-fatal errors.
	completeJob func(ctx context.Context, jobID string, errs []string) error
	// updateCurrentModule updates the "current_module" field of the job row.
	updateCurrentModule func(ctx context.Context, jobID, module string) error
	// updateModuleProgress updates the completed/total counters of the job row.
	updateModuleProgress func(ctx context.Context, jobID string, completed, total int) error
}

// categoryStats is the per-module result returned by the scan endpoints.
type categoryStats struct {
	Found    int `json:"found"`
	Existing int `json:"existing"`
	New      int `json:"new"`
}

// startImportAsync creates the job row, starts the async goroutine via runFn and writes
// 202 Accepted with the job ID. It is the shared core of StartGarminImport /
// StartStravaImport and UploadGarminZIP / UploadStravaZIP.
//
// runFn receives the job ID and must execute the import synchronously (it runs inside a
// goroutine started by this helper).  When isTempPath is true the goroutine defers
// os.RemoveAll(sourcePath) so the caller does not need to clean up.
func (h *Handler) startImportAsync(
	w http.ResponseWriter,
	r *http.Request,
	userID, sourcePath string,
	isTempPath bool,
	cb importJobCallbacks,
	runFn func(ctx context.Context, jobID string),
) {
	jobID, err := cb.createJob(r.Context(), userID, sourcePath)
	if err != nil {
		slog.Error("create "+cb.label+" import job failed", "error", err)
		if isTempPath {
			os.RemoveAll(sourcePath)
		}
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to create import job", r.URL.Path)
		return
	}

	go func() {
		if isTempPath {
			defer os.RemoveAll(sourcePath)
		}
		runFn(context.Background(), jobID)
	}()

	httputil.WriteJSON(w, http.StatusAccepted, map[string]string{"job_id": jobID})
}

// makeProgressCallbacks builds the OnProgress / OnModuleComplete funcs used inside
// ImportParams, wired to the correct DB update methods via cb.
func makeProgressCallbacks(cb importJobCallbacks, logTag, jobID string) (
	onProgress func(current, total int, msg string),
	onModuleComplete func(completed, total int, name string),
) {
	onProgress = func(_, _ int, msg string) {
		if err := cb.updateCurrentModule(context.Background(), jobID, msg); err != nil {
			slog.Warn(logTag+": update current module failed", "error", err)
		}
	}
	onModuleComplete = func(completed, total int, _ string) {
		if err := cb.updateModuleProgress(context.Background(), jobID, completed, total); err != nil {
			slog.Warn(logTag+": update module progress failed", "error", err)
		}
	}
	return
}

// buildScanResult iterates detected modules, fills per-module categoryStats, and calls
// existingFn for any module that has an existence check defined.
//
// existingFn receives the module name and returns (count, error).  Return (0, nil) for
// modules that don't need an existence check.
func buildScanResult(
	ctx context.Context,
	modules []importutil.DetectedModule,
	existingFn func(ctx context.Context, moduleName string) (int, error),
) map[string]categoryStats {
	result := make(map[string]categoryStats, len(modules))
	for _, m := range modules {
		if !m.Available {
			continue
		}
		stats := categoryStats{Found: m.ItemCount}
		existing, err := existingFn(ctx, m.Name)
		if err != nil {
			slog.Warn("import scan: existence check failed", "module", m.Name, "error", err)
		} else {
			stats.Existing = existing
		}
		stats.New = max(stats.Found-stats.Existing, 0)
		result[m.Name] = stats
	}
	return result
}

// isTempSourcePath reports whether sourcePath is inside the OS temp directory and
// therefore should be cleaned up after the import finishes.
func isTempSourcePath(sourcePath string) bool {
	return strings.HasPrefix(sourcePath, os.TempDir())
}
