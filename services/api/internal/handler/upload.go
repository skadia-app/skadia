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
	"time"

	"github.com/skadia-app/skadia/services/api/internal/fitimport"
	"github.com/skadia-app/skadia/services/api/internal/garmin"
	"github.com/skadia-app/skadia/services/api/internal/gpximport"
	"github.com/skadia-app/skadia/services/api/internal/importer"
	"github.com/skadia-app/skadia/services/api/internal/middleware"
	"github.com/skadia-app/skadia/services/api/internal/model"
	"github.com/skadia-app/skadia/services/api/internal/tcximport"
)

func (h *Handler) Upload(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "Unauthorized", "no user ID in context", r.URL.Path)
		return
	}

	maxUploadSize := h.Config.MaxUploadSize
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

	// Detect format + decompress .gz if needed
	maxSize := h.Config.MaxUploadSize
	if maxSize <= 0 {
		maxSize = 50 << 20
	}
	decompressed, format, decompErr := detectFormatAndDecompress(fileBytes, header.Filename, maxSize*10)
	if decompErr != nil {
		writeError(w, http.StatusBadRequest, "Bad Request", fmt.Sprintf("unsupported or unreadable file: %s", decompErr), r.URL.Path)
		return
	}

	// Calculate SHA256 hash on decompressed bytes
	hash := sha256.Sum256(decompressed)
	hashStr := hex.EncodeToString(hash[:])

	// Resolve user's conflict strategy for duplicate handling
	var conflictStrategy importer.ConflictStrategy
	if importSettings, settErr := h.DB.GetImportSettings(r.Context(), userID); settErr == nil && importSettings != nil {
		conflictStrategy = importer.ConflictStrategy(importSettings.ConflictStrategy)
	}

	filename := header.Filename

	// Route monitoring FIT files (health data, no activity created)
	if format == "fit" {
		ftype := garmin.ReadFITFileType(decompressed)
		if ftype == 68 || ftype == 44 {
			start := time.Now()
			logID := ""
			sz := int64(len(decompressed))
			logEntry := model.ImportLogEntry{
				UserID:         userID,
				SourcePlatform: "upload",
				Filename:       header.Filename,
				FileSizeBytes:  &sz,
				Status:         "processing",
			}
			if id, logErr := h.DB.InsertImportLog(r.Context(), logEntry); logErr == nil {
				logID = id
			}
			monErr := garmin.ProcessMonitoringFIT(r.Context(), h.DB, decompressed, userID)
			if logID != "" {
				elapsed := int(time.Since(start).Milliseconds())
				if monErr != nil {
					msg := monErr.Error()
					h.DB.UpdateImportLogStatus(r.Context(), logID, "error", &msg, nil, &elapsed)
				} else {
					h.DB.UpdateImportLogStatus(r.Context(), logID, "completed", nil, nil, &elapsed)
				}
			}
			if monErr != nil {
				writeError(w, http.StatusUnprocessableEntity, "Process Error",
					fmt.Sprintf("failed to process monitoring FIT: %s", monErr), r.URL.Path)
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{
				"status":    "completed",
				"data_type": "monitoring",
				"fit_type":  ftype,
			})
			return
		}
		// ftype == 4 or unknown → continue to activity import below
	}

	// For FIT files try multi-session parsing to detect multisport activities.
	// GPX and TCX always produce a single activity.
	if format == "fit" {
		activities, parseErr := fitimport.ParseMultiSessionFIT(decompressed, filename)
		if parseErr != nil || len(activities) == 0 {
			// Fall back to single-session parse for error reporting
			singleData, singleErr := fitimport.ParseFIT(bytes.NewReader(decompressed), filename)
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
			if uploadErr := h.Storage.Upload(r.Context(), s3Key, decompressed, "application/octet-stream"); uploadErr != nil {
				slog.Warn("s3 upload for multisport failed", "error", uploadErr)
			}

			msResult, msErr := h.ImportService.ProcessMultisport(
				r.Context(), userID, activities, decompressed, hashStr,
				s3Key, format, filename,
				importer.ImportMetadata{SourcePlatform: "webui", ConflictStrategy: conflictStrategy},
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

		// Import log for single-session FIT activity
		start := time.Now()
		sz := int64(len(decompressed))
		logEntry := model.ImportLogEntry{
			UserID:         userID,
			SourcePlatform: "upload",
			Filename:       filename,
			FileSizeBytes:  &sz,
			Status:         "processing",
		}
		logID := ""
		if id, logErr := h.DB.InsertImportLog(r.Context(), logEntry); logErr == nil {
			logID = id
		}

		fitResult, processErr := h.ImportService.ProcessActivity(r.Context(), userID, data, decompressed, importer.ImportMetadata{SourcePlatform: "webui", ConflictStrategy: conflictStrategy})
		if processErr != nil {
			slog.Error("process activity failed", "error", processErr)
			if logID != "" {
				elapsed := int(time.Since(start).Milliseconds())
				msg := processErr.Error()
				h.DB.UpdateImportLogStatus(r.Context(), logID, "error", &msg, nil, &elapsed)
			}
			writeError(w, http.StatusInternalServerError, "Internal Error", "failed to process activity", r.URL.Path)
			return
		}

		if fitResult.Status == "skipped" {
			if logID != "" {
				elapsed := int(time.Since(start).Milliseconds())
				h.DB.UpdateImportLogStatus(r.Context(), logID, "skipped", nil, nil, &elapsed)
			}
			writeJSON(w, http.StatusOK, map[string]any{
				"status":      "duplicate",
				"activity_id": fitResult.ActivityID,
			})
			return
		}

		// For created/enriched/replaced: upload to S3 + PostProcess
		s3Key := fmt.Sprintf("%s/%s.fit", userID, hashStr)
		if uploadErr := h.Storage.Upload(r.Context(), s3Key, decompressed, "application/octet-stream"); uploadErr != nil {
			slog.Error("S3 upload failed", "error", uploadErr)
			writeError(w, http.StatusInternalServerError, "Storage Error", "failed to store original file", r.URL.Path)
			return
		}

		fitActivity, _ := h.DB.GetActivity(r.Context(), fitResult.ActivityID)
		if fitActivity == nil {
			writeError(w, http.StatusInternalServerError, "Internal Error", "failed to fetch inserted activity", r.URL.Path)
			return
		}

		h.postInsertActivity(r.Context(), userID, fitActivity, data, decompressed, s3Key, format, filename)

		if logID != "" {
			elapsed := int(time.Since(start).Milliseconds())
			switch fitResult.Status {
			case "skipped":
				h.DB.UpdateImportLogStatus(r.Context(), logID, "skipped", nil, nil, &elapsed)
			default:
				actID := fitResult.ActivityID
				h.DB.UpdateImportLogStatus(r.Context(), logID, "completed", nil, &actID, &elapsed)
			}
		}

		switch fitResult.Status {
		case "enriched", "replaced":
			writeJSON(w, http.StatusOK, map[string]any{
				"status":      fitResult.Status,
				"activity_id": fitResult.ActivityID,
			})
			return
		}

		// "created": physiological profile + full response
		if data.UserProfileImport != nil {
			if profErr := h.DB.SetUserProfileFieldsIfNull(r.Context(), userID, data.UserProfileImport); profErr != nil {
				slog.Warn("set user profile fields failed", "error", profErr)
			}
		}

		full, _ := h.DB.GetActivity(r.Context(), fitResult.ActivityID)
		if full == nil {
			full = fitActivity
		}
		writeJSON(w, http.StatusCreated, model.UploadResponse{Activity: full, Warnings: warnings})
		return
	}

	// GPX / TCX path
	var data *model.ActivityData
	switch format {
	case "gpx":
		data, err = gpximport.ParseGPX(bytes.NewReader(decompressed), filename)
	case "tcx":
		data, err = tcximport.ParseTCX(bytes.NewReader(decompressed), filename)
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

	// Import log for GPX/TCX activity
	start := time.Now()
	sz := int64(len(decompressed))
	gpxLogEntry := model.ImportLogEntry{
		UserID:         userID,
		SourcePlatform: "upload",
		Filename:       filename,
		FileSizeBytes:  &sz,
		Status:         "processing",
	}
	gpxLogID := ""
	if id, logErr := h.DB.InsertImportLog(r.Context(), gpxLogEntry); logErr == nil {
		gpxLogID = id
	}

	gpxResult, processErr := h.ImportService.ProcessActivity(r.Context(), userID, data, decompressed, importer.ImportMetadata{SourcePlatform: "webui", ConflictStrategy: conflictStrategy})
	if processErr != nil {
		slog.Error("process activity failed", "error", processErr)
		if gpxLogID != "" {
			elapsed := int(time.Since(start).Milliseconds())
			msg := processErr.Error()
			h.DB.UpdateImportLogStatus(r.Context(), gpxLogID, "error", &msg, nil, &elapsed)
		}
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to process activity", r.URL.Path)
		return
	}

	if gpxResult.Status == "skipped" {
		if gpxLogID != "" {
			elapsed := int(time.Since(start).Milliseconds())
			h.DB.UpdateImportLogStatus(r.Context(), gpxLogID, "skipped", nil, nil, &elapsed)
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"status":      "duplicate",
			"activity_id": gpxResult.ActivityID,
		})
		return
	}

	// For created/enriched/replaced: upload to Garage + PostProcess
	ext := format
	if ext == "" {
		ext = strings.TrimPrefix(filepath.Ext(filename), ".")
	}
	s3Key := fmt.Sprintf("%s/%s.%s", userID, hashStr, ext)

	contentType := "application/xml"
	if err := h.Storage.Upload(r.Context(), s3Key, decompressed, contentType); err != nil {
		slog.Error("S3 upload failed", "error", err)
		writeError(w, http.StatusInternalServerError, "Storage Error", "failed to store original file", r.URL.Path)
		return
	}

	gpxActivity, _ := h.DB.GetActivity(r.Context(), gpxResult.ActivityID)
	if gpxActivity == nil {
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to fetch inserted activity", r.URL.Path)
		return
	}

	h.postInsertActivity(r.Context(), userID, gpxActivity, data, decompressed, s3Key, format, filename)

	if gpxLogID != "" {
		elapsed := int(time.Since(start).Milliseconds())
		switch gpxResult.Status {
		case "skipped":
			h.DB.UpdateImportLogStatus(r.Context(), gpxLogID, "skipped", nil, nil, &elapsed)
		default:
			actID := gpxResult.ActivityID
			h.DB.UpdateImportLogStatus(r.Context(), gpxLogID, "completed", nil, &actID, &elapsed)
		}
	}

	if gpxResult.Status == "enriched" || gpxResult.Status == "replaced" {
		writeJSON(w, http.StatusOK, map[string]any{
			"status":      gpxResult.Status,
			"activity_id": gpxResult.ActivityID,
		})
		return
	}

	// "created": re-fetch full activity for response
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
