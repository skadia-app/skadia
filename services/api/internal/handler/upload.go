package handler

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/skadia-app/skadia/services/api/internal/event"
	"github.com/skadia-app/skadia/services/api/internal/fitimport"
	"github.com/skadia-app/skadia/services/api/internal/gpximport"
	"github.com/skadia-app/skadia/services/api/internal/middleware"
	"github.com/skadia-app/skadia/services/api/internal/model"
	"github.com/skadia-app/skadia/services/api/internal/sidecar"
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

	// Duplicate check
	existing, err := h.DB.CheckDuplicate(r.Context(), userID, hashStr)
	if err != nil {
		slog.Error("duplicate check failed", "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", "duplicate check failed", r.URL.Path)
		return
	}
	if existing != nil {
		writeError(w, http.StatusConflict, "Duplicate",
			fmt.Sprintf("file already imported as activity %s", existing.ID), r.URL.Path)
		return
	}

	// Parse file
	var data *model.ActivityData
	filename := header.Filename

	switch format {
	case "fit":
		data, err = fitimport.ParseFIT(bytes.NewReader(fileBytes), filename)
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

	// Time overlap check
	var warnings []string
	if !data.StartedAt.IsZero() && !data.EndedAt.IsZero() {
		overlap, err := h.DB.CheckTimeOverlap(r.Context(), userID, data.StartedAt, data.EndedAt)
		if err != nil {
			slog.Error("time overlap check failed", "error", err)
		} else if overlap != nil {
			warnings = append(warnings, "time_overlap:"+overlap.ID)
			slog.Warn("activity time overlaps with existing",
				"existing_id", overlap.ID, "filename", filename)
		}
	}

	data.FileHash = hashStr

	// Warn if sport type could not be determined
	if data.SportType == "unknown" {
		slog.Warn("sport type unknown, manual classification required",
			"filename", filename, "format", format)
	}

	// Upload original to Garage
	ext := format
	if ext == "" {
		ext = strings.TrimPrefix(filepath.Ext(filename), ".")
	}
	s3Key := fmt.Sprintf("%s/%s.%s", userID, hashStr, ext)

	contentType := "application/octet-stream"
	if format == "gpx" || format == "tcx" {
		contentType = "application/xml"
	}

	if err := h.Storage.Upload(r.Context(), s3Key, fileBytes, contentType); err != nil {
		slog.Error("S3 upload failed", "error", err)
		writeError(w, http.StatusInternalServerError, "Storage Error", "failed to store original file", r.URL.Path)
		return
	}

	// Insert into database
	activity, err := h.DB.InsertActivity(r.Context(), userID, data)
	if err != nil {
		slog.Error("insert activity failed", "error", err)
		writeError(w, http.StatusInternalServerError, "Database Error", "failed to save activity", r.URL.Path)
		return
	}

	// Insert streams
	if err := h.DB.InsertStreams(r.Context(), activity.ID, data.Streams); err != nil {
		slog.Error("insert streams failed", "error", err)
	}

	// Insert track
	if err := h.DB.InsertTrack(r.Context(), activity.ID, data.TrackPoints); err != nil {
		slog.Error("insert track failed", "error", err)
	}

	// Insert heatmap line (uses track_full or track_public if privacy zones exist)
	if len(data.TrackPoints) >= 2 {
		if err := h.DB.InsertHeatmapLine(r.Context(), userID, activity.ID, data.SportType, data.StartedAt); err != nil {
			slog.Error("insert heatmap line failed", "error", err)
		}
	}

	// Insert sensors
	if err := h.DB.InsertSensors(r.Context(), activity.ID, data.Sensors); err != nil {
		slog.Error("insert sensors failed", "error", err)
	}

	// Update file info
	metaKey := s3Key + ".meta"
	bucket := "skadia-originals"
	if err := h.DB.UpdateActivityFileInfo(r.Context(), activity.ID, bucket, s3Key, format, filename, metaKey); err != nil {
		slog.Error("update file info failed", "error", err)
	}

	// Write .meta sidecar
	meta := sidecar.GenerateMeta(activity.ID, data)
	metaBytes, err := meta.Marshal()
	if err == nil {
		if err := h.Storage.Upload(r.Context(), metaKey, metaBytes, "application/json"); err != nil {
			slog.Error("meta upload failed", "error", err)
		}
	}

	// Publish NATS event
	if h.Events != nil {
		evt := event.ActivityUploadedEvent{
			ActivityID: activity.ID,
			UserID:     userID,
			SportType:  data.SportType,
			HasGPS:     data.HasGPS,
		}
		if err := h.Events.PublishActivityUploaded(evt); err != nil {
			slog.Error("event publish failed", "error", err)
		}
	}

	// Write to Shadow FS (best-effort, don't fail upload)
	if h.ShadowFS != nil {
		if err := h.WriteShadowForActivity(r.Context(), userID, activity.ID, data, fileBytes); err != nil {
			slog.Error("shadow fs write failed", "id", activity.ID, "error", err)
		}
	}

	// Re-fetch full activity for response
	full, err := h.DB.GetActivity(r.Context(), activity.ID)
	if err != nil || full == nil {
		full = activity
	}

	slog.Info("activity uploaded",
		"id", activity.ID,
		"sport", data.SportType,
		"format", format,
		"streams", len(data.Streams),
		"sensors", len(data.Sensors),
		"has_gps", data.HasGPS,
	)

	resp := model.UploadResponse{
		Activity: full,
		Warnings: warnings,
	}
	writeJSON(w, http.StatusCreated, resp)
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
