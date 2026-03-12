package handler

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/skadia-app/skadia/services/api/internal/fitimport"
	"github.com/skadia-app/skadia/services/api/internal/gpximport"
	"github.com/skadia-app/skadia/services/api/internal/importer"
	"github.com/skadia-app/skadia/services/api/internal/middleware"
	"github.com/skadia-app/skadia/services/api/internal/model"
	"github.com/skadia-app/skadia/services/api/internal/tcximport"
)

const maxUploadSize = 50 << 20 // 50 MB

func (h *Handler) Upload(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "Unauthorized", "no user ID in context", r.URL.Path)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxUploadSize)
	if err := r.ParseMultipartForm(maxUploadSize); err != nil {
		writeError(w, http.StatusBadRequest, "Bad Request", "file too large or invalid multipart form", r.URL.Path)
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "Bad Request", "missing 'file' field", r.URL.Path)
		return
	}
	defer file.Close()

	fileBytes, err := io.ReadAll(file)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to read file", r.URL.Path)
		return
	}

	// Detect format
	format := detectFormat(fileBytes, header.Filename)
	if format == "" {
		writeError(w, http.StatusBadRequest, "Bad Request", "unsupported file format (expected FIT, GPX, or TCX)", r.URL.Path)
		return
	}

	// Calculate SHA256 hash
	hash := sha256.Sum256(fileBytes)
	hashStr := hex.EncodeToString(hash[:])

	filename := header.Filename

	// For FIT files try multi-session parsing to detect multisport activities.
	// GPX and TCX always produce a single activity.
	if format == "fit" {
		activities, parseErr := fitimport.ParseMultiSessionFIT(fileBytes, filename)
		if parseErr != nil || len(activities) == 0 {
			// Fall back to single-session parse for error reporting
			singleData, singleErr := fitimport.ParseFIT(bytes.NewReader(fileBytes), filename)
			if singleErr != nil {
				slog.Error("parse FIT failed", "error", singleErr)
				writeError(w, http.StatusUnprocessableEntity, "Parse Error",
					fmt.Sprintf("failed to parse FIT file: %s", singleErr), r.URL.Path)
				return
			}
			activities = []*model.ActivityData{singleData}
		}

		if len(activities) > 1 {
			// S3 upload first (shared by all children)
			s3Key := fmt.Sprintf("%s/%s.fit", userID, hashStr)
			if uploadErr := h.Storage.Upload(r.Context(), s3Key, fileBytes, "application/octet-stream"); uploadErr != nil {
				slog.Warn("s3 upload for multisport failed", "error", uploadErr)
			}

			msResult, msErr := h.ImportService.ProcessMultisport(
				r.Context(), userID, activities, fileBytes, hashStr,
				s3Key, format, filename,
				importer.ImportMetadata{SourcePlatform: "webui"},
			)
			if msErr != nil {
				writeError(w, http.StatusInternalServerError, "Import Error", msErr.Error(), r.URL.Path)
				return
			}

			// Return parent activity
			parentAct, _ := h.DB.GetActivity(r.Context(), msResult.ParentID)
			if parentAct != nil {
				writeJSON(w, http.StatusCreated, parentAct)
			} else {
				writeJSON(w, http.StatusCreated, map[string]string{"parent_id": msResult.ParentID})
			}
			return
		}

		// Single-session FIT — fall through with the parsed data
		data := activities[0]

		data.FileHash = hashStr
		if data.SportType == "unknown" {
			slog.Warn("sport type unknown", "filename", filename, "format", format)
		}

		// Time overlap check
		var warnings []string
		if !data.StartedAt.IsZero() && !data.EndedAt.IsZero() {
			if overlap, oErr := h.DB.CheckTimeOverlap(r.Context(), userID, data.StartedAt, data.EndedAt); oErr != nil {
				slog.Error("time overlap check failed", "error", oErr)
			} else if overlap != nil {
				warnings = append(warnings, "time_overlap:"+overlap.ID)
			}
		}

		fitResult, processErr := h.ImportService.ProcessActivity(r.Context(), userID, data, fileBytes, importer.ImportMetadata{SourcePlatform: "webui"})
		if processErr != nil {
			slog.Error("process activity failed", "error", processErr)
			writeError(w, http.StatusInternalServerError, "Internal Error", "failed to process activity", r.URL.Path)
			return
		}

		switch fitResult.Status {
		case "skipped":
			writeJSON(w, http.StatusOK, map[string]any{
				"status":      "duplicate",
				"activity_id": fitResult.ActivityID,
			})
			return
		case "enriched", "replaced":
			writeJSON(w, http.StatusOK, map[string]any{
				"status":      fitResult.Status,
				"activity_id": fitResult.ActivityID,
			})
			return
		}

		// "created": upload original to S3, run post-processing
		s3Key := fmt.Sprintf("%s/%s.fit", userID, hashStr)
		if uploadErr := h.Storage.Upload(r.Context(), s3Key, fileBytes, "application/octet-stream"); uploadErr != nil {
			slog.Error("S3 upload failed", "error", uploadErr)
			writeError(w, http.StatusInternalServerError, "Storage Error", "failed to store original file", r.URL.Path)
			return
		}

		// FIT physiological profile import
		if data.UserProfileImport != nil {
			if profErr := h.DB.SetUserProfileFieldsIfNull(r.Context(), userID, data.UserProfileImport); profErr != nil {
				slog.Warn("set user profile fields failed", "error", profErr)
			}
		}

		fitActivity, _ := h.DB.GetActivity(r.Context(), fitResult.ActivityID)
		if fitActivity == nil {
			writeError(w, http.StatusInternalServerError, "Internal Error", "failed to fetch inserted activity", r.URL.Path)
			return
		}

		h.postInsertActivity(r.Context(), userID, fitActivity, data, fileBytes, s3Key, format, filename)

		full, _ := h.DB.GetActivity(r.Context(), fitResult.ActivityID)
		if full == nil {
			full = fitActivity
		}
		writeJSON(w, http.StatusCreated, model.UploadResponse{Activity: full, Warnings: warnings})
		return
	}

	// GPX / TCX path (unchanged)
	var data *model.ActivityData
	switch format {
	case "gpx":
		data, err = gpximport.ParseGPX(bytes.NewReader(fileBytes), filename)
	case "tcx":
		data, err = tcximport.ParseTCX(bytes.NewReader(fileBytes), filename)
	}
	if err != nil {
		slog.Error("parse failed", "format", format, "error", err)
		writeError(w, http.StatusUnprocessableEntity, "Parse Error",
			fmt.Sprintf("failed to parse %s file: %s", format, err), r.URL.Path)
		return
	}

	data.FileHash = hashStr

	// Warn if sport type could not be determined
	if data.SportType == "unknown" {
		slog.Warn("sport type unknown, manual classification required",
			"filename", filename, "format", format)
	}

	// Time overlap check
	var warnings []string
	if !data.StartedAt.IsZero() && !data.EndedAt.IsZero() {
		overlap, overlapErr := h.DB.CheckTimeOverlap(r.Context(), userID, data.StartedAt, data.EndedAt)
		if overlapErr != nil {
			slog.Error("time overlap check failed", "error", overlapErr)
		} else if overlap != nil {
			warnings = append(warnings, "time_overlap:"+overlap.ID)
			slog.Warn("activity time overlaps with existing",
				"existing_id", overlap.ID, "filename", filename)
		}
	}

	gpxResult, processErr := h.ImportService.ProcessActivity(r.Context(), userID, data, fileBytes, importer.ImportMetadata{SourcePlatform: "webui"})
	if processErr != nil {
		slog.Error("process activity failed", "error", processErr)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to process activity", r.URL.Path)
		return
	}

	switch gpxResult.Status {
	case "skipped":
		writeJSON(w, http.StatusOK, map[string]any{
			"status":      "duplicate",
			"activity_id": gpxResult.ActivityID,
		})
		return
	case "enriched", "replaced":
		writeJSON(w, http.StatusOK, map[string]any{
			"status":      gpxResult.Status,
			"activity_id": gpxResult.ActivityID,
		})
		return
	}

	// "created": upload original to Garage, run post-processing
	ext := format
	if ext == "" {
		ext = strings.TrimPrefix(filepath.Ext(filename), ".")
	}
	s3Key := fmt.Sprintf("%s/%s.%s", userID, hashStr, ext)

	contentType := "application/xml"
	if err := h.Storage.Upload(r.Context(), s3Key, fileBytes, contentType); err != nil {
		slog.Error("S3 upload failed", "error", err)
		writeError(w, http.StatusInternalServerError, "Storage Error", "failed to store original file", r.URL.Path)
		return
	}

	gpxActivity, _ := h.DB.GetActivity(r.Context(), gpxResult.ActivityID)
	if gpxActivity == nil {
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to fetch inserted activity", r.URL.Path)
		return
	}

	h.postInsertActivity(r.Context(), userID, gpxActivity, data, fileBytes, s3Key, format, filename)

	// Re-fetch full activity for response
	full, err := h.DB.GetActivity(r.Context(), gpxResult.ActivityID)
	if err != nil || full == nil {
		full = gpxActivity
	}

	resp := model.UploadResponse{
		Activity: full,
		Warnings: warnings,
	}
	writeJSON(w, http.StatusCreated, resp)
}

// handleDuplicateMatch processes a duplicate activity match according to user conflict strategy.
func (h *Handler) handleDuplicateMatch(w http.ResponseWriter, r *http.Request, userID string, match *importer.DuplicateMatch, data *model.ActivityData) {
	settings, _ := h.DB.GetImportSettings(r.Context(), userID)
	var strategy importer.ConflictStrategy
	if settings != nil {
		strategy = importer.ConflictStrategy(settings.ConflictStrategy)
	} else {
		strategy = importer.StrategyEnrichMissing
	}
	switch strategy {
	case importer.StrategyReplaceAll:
		if replaceErr := importer.ReplaceActivity(r.Context(), h.DB, match.ExistingActivityID, data); replaceErr != nil {
			slog.Error("replace activity failed", "error", replaceErr)
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"status":      "replaced",
			"activity_id": match.ExistingActivityID,
		})
	case importer.StrategySkip:
		writeJSON(w, http.StatusOK, map[string]any{
			"status":      "skipped",
			"activity_id": match.ExistingActivityID,
		})
	default: // enrich_missing
		existing, getErr := h.DB.GetActivity(r.Context(), match.ExistingActivityID)
		if getErr != nil {
			slog.Warn("upload: cannot fetch existing activity for enrich", "id", match.ExistingActivityID, "error", getErr)
		}
		n, enrichErr := importer.EnrichActivity(r.Context(), h.DB, match.ExistingActivityID, existing, data)
		if enrichErr != nil {
			slog.Error("enrich activity failed", "error", enrichErr)
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"status":          "enriched",
			"activity_id":     match.ExistingActivityID,
			"fields_enriched": n,
		})
	}
}

// postInsertActivity runs all post-insert steps by delegating to the shared
// ImportService.PostProcess pipeline. This ensures that the upload handler and
// the cross-platform import pipeline use identical post-processing logic.
func (h *Handler) postInsertActivity(
	ctx context.Context,
	userID string,
	activity *model.Activity,
	data *model.ActivityData,
	fileBytes []byte,
	s3Key string,
	format string,
	filename string,
) {
	if err := h.ImportService.PostProcess(ctx, userID, activity.ID, data, fileBytes, s3Key, format, filename); err != nil {
		slog.Error("post-process activity failed", "id", activity.ID, "error", err)
	}
}


func detectFormat(data []byte, filename string) string {
	// FIT: Check for ".FIT" signature
	if len(data) >= 14 {
		headerSize := int(data[0])
		if (headerSize == 12 || headerSize == 14) && headerSize <= len(data) {
			sig := string(data[headerSize-4 : headerSize])
			if sig == ".FIT" {
				return "fit"
			}
		}
	}

	// XML-based formats
	content := string(data[:min(len(data), 1024)])
	lower := strings.ToLower(content)

	if strings.Contains(lower, "<gpx") {
		return "gpx"
	}
	if strings.Contains(lower, "<trainingcenterdatabase") {
		return "tcx"
	}

	// Fallback: check extension
	ext := strings.ToLower(filepath.Ext(filename))
	switch ext {
	case ".fit":
		return "fit"
	case ".gpx":
		return "gpx"
	case ".tcx":
		return "tcx"
	}

	return ""
}
