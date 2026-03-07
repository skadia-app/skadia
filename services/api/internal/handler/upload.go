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

	"github.com/skadia-app/skadia/services/api/internal/enrichment"
	"github.com/skadia-app/skadia/services/api/internal/event"
	"github.com/skadia-app/skadia/services/api/internal/fitimport"
	"github.com/skadia-app/skadia/services/api/internal/gpximport"
	"github.com/skadia-app/skadia/services/api/internal/importer"
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

	// Duplicate check via central DuplicateDetector
	match, err := importer.CheckTier1(r.Context(), h.DB, userID, fileBytes, data, 0, 0)
	if err != nil {
		slog.Error("duplicate check failed", "error", err)
		// Not fatal — continue with normal import on error
	} else if match != nil {
		// Conflict handling based on user settings
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

	// Stage 1 Enrichment (synchronous, ~50ms)
	if data.HasGPS && data.StartLat != 0 {
		enrichment.RunStage1(r.Context(), h.DB, userID, activity.ID, data, h.Events)
	}

	// Insert streams
	if err := h.DB.InsertStreams(r.Context(), activity.ID, data.Streams); err != nil {
		slog.Error("insert streams failed", "error", err)
	}

	// Insert R-R intervals (FIT only; empty for GPX/TCX)
	if len(data.RRIntervals) > 0 {
		if err := h.DB.InsertRRIntervals(r.Context(), activity.ID, data.RRIntervals); err != nil {
			slog.Error("insert rr intervals failed", "error", err)
		}
	}

	// Insert track
	if err := h.DB.InsertTrack(r.Context(), activity.ID, data.TrackPoints); err != nil {
		slog.Error("insert track failed", "error", err)
	}

	// Insert heatmap line (uses track_full or track_public if privacy zones exist)
	if len(data.TrackPoints) >= 2 {
		var subTypePtr *string
		if data.SubType != "" {
			subTypePtr = &data.SubType
		}
		if err := h.DB.InsertHeatmapLine(r.Context(), userID, activity.ID, data.SportType, subTypePtr, data.StartedAt); err != nil {
			slog.Error("insert heatmap line failed", "error", err)
		}
		interval, _ := h.DB.GetUserHeatmapInterval(r.Context(), userID)
		if err := h.DB.InsertHeatmapPoints(r.Context(), userID, activity.ID, data.SportType, subTypePtr, data.StartedAt, interval); err != nil {
			slog.Error("insert heatmap points failed", "error", err)
		}
	}

	// Insert sensors
	if err := h.DB.InsertSensors(r.Context(), activity.ID, data.Sensors); err != nil {
		slog.Error("insert sensors failed", "error", err)
	}

	// Insert activity details (laps, splits, lengths, climb pro)
	if err := h.DB.InsertLaps(r.Context(), activity.ID, data.Laps); err != nil {
		slog.Error("insert laps failed", "error", err)
	}
	if err := h.DB.InsertSplits(r.Context(), activity.ID, data.Splits); err != nil {
		slog.Error("insert splits failed", "error", err)
	}
	if err := h.DB.InsertSplitSummaries(r.Context(), activity.ID, data.SplitSummaries); err != nil {
		slog.Error("insert split summaries failed", "error", err)
	}
	if err := h.DB.InsertLengths(r.Context(), activity.ID, data.Lengths); err != nil {
		slog.Error("insert lengths failed", "error", err)
	}
	if err := h.DB.InsertClimbProEvents(r.Context(), activity.ID, data.ClimbProEvents); err != nil {
		slog.Error("insert climb pro events failed", "error", err)
	}

	// Save time-in-zone data (from FIT time_in_zone messages or session arrays)
	if len(data.TimeInZoneData) > 0 {
		if err := h.DB.SaveTimeInZones(r.Context(), activity.ID, data.TimeInZoneData); err != nil {
			slog.Error("save time in zones failed", "error", err)
		}
	}

	// Import physiological profile fields from FIT user_profile / zones_target (only if not yet set)
	if format == "fit" && data.UserProfileImport != nil {
		if err := h.DB.SetUserProfileFieldsIfNull(r.Context(), userID, data.UserProfileImport); err != nil {
			slog.Warn("set user profile fields failed", "error", err)
		}
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
		// Trigger analytics recompute for the user (graceful — never blocks upload)
		h.Events.PublishAnalyticsInvalidate(userID)
	}

	// Write to Shadow FS (best-effort, don't fail upload)
	if h.ShadowFS != nil {
		if err := h.WriteShadowForActivity(r.Context(), userID, activity.ID, data, fileBytes); err != nil {
			slog.Error("shadow fs write failed", "id", activity.ID, "error", err)
		}
	}

	// Segment matching + lap detection (non-blocking background goroutine)
	if data.HasGPS {
		var gearIDPtr *string
		if activity.GearID != nil {
			gearIDPtr = activity.GearID
		}
		go func() {
			bgCtx := context.Background()
			MatchActivitySegments(bgCtx, h.DB, activity.ID, userID, data.SportType, data.SubType, gearIDPtr, activity.StartedAt)

			// Lap detection (after segment matching, same goroutine)
			if h.DB.IsLapDetectionEnabled(bgCtx, userID) {
				gpsStream, err := h.DB.GetActivityGPSStream(bgCtx, activity.ID)
				if err == nil && len(gpsStream) >= 20 {
					if laps := DetectLaps(bgCtx, h.DB, activity.ID, gpsStream); len(laps) > 0 {
						if err := h.DB.InsertDetectedLaps(bgCtx, activity.ID, laps); err != nil {
							slog.Error("upload: insert detected laps failed", "id", activity.ID, "error", err)
						} else {
							slog.Info("upload: detected laps", "id", activity.ID, "count", len(laps))
						}
					}
				}
			}
		}()
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
