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

	"github.com/google/uuid"

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
			// Multisport: create parent activity + one child per session
			parentAct := &model.Activity{
				ID:        uuid.New().String(),
				UserID:    userID,
				SportType: "multisport",
				Title:     strPtr("Multisport"),
				StartedAt: activities[0].StartedAt,
			}
			if insertErr := h.DB.InsertMultisportParent(r.Context(), parentAct); insertErr != nil {
				slog.Error("insert multisport parent failed", "error", insertErr)
				writeError(w, http.StatusInternalServerError, "Database Error", "failed to save multisport parent", r.URL.Path)
				return
			}

			// Upload original file once (shared by all children)
			s3Key := fmt.Sprintf("%s/%s.fit", userID, hashStr)
			if uploadErr := h.Storage.Upload(r.Context(), s3Key, fileBytes, "application/octet-stream"); uploadErr != nil {
				slog.Error("S3 upload failed", "error", uploadErr)
			}

			for i, childData := range activities {
				childData.FileHash = hashStr
				childActivity, childErr := h.DB.InsertActivityWithParent(r.Context(), userID, childData, parentAct.ID)
				if childErr != nil {
					slog.Warn("insert child activity failed", "session", i, "err", childErr)
					continue
				}
				h.postInsertActivity(r.Context(), userID, childActivity, childData, fileBytes, s3Key, format, filename)
			}

			if aggrErr := h.DB.AggregateParentMetrics(r.Context(), parentAct.ID); aggrErr != nil {
				slog.Warn("aggregate parent metrics failed", "id", parentAct.ID, "err", aggrErr)
			}

			if h.Events != nil {
				h.Events.PublishAnalyticsInvalidate(userID)
			}

			writeJSON(w, http.StatusCreated, parentAct)
			return
		}

		// Single-session FIT — fall through with the parsed data
		data := activities[0]

		// Duplicate check
		match, checkErr := importer.CheckTier1(r.Context(), h.DB, userID, fileBytes, data, 0, 0)
		if checkErr != nil {
			slog.Error("duplicate check failed", "error", checkErr)
		} else if match != nil {
			h.handleDuplicateMatch(w, r, userID, match, data)
			return
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

		data.FileHash = hashStr
		if data.SportType == "unknown" {
			slog.Warn("sport type unknown", "filename", filename, "format", format)
		}

		s3Key := fmt.Sprintf("%s/%s.fit", userID, hashStr)
		if uploadErr := h.Storage.Upload(r.Context(), s3Key, fileBytes, "application/octet-stream"); uploadErr != nil {
			slog.Error("S3 upload failed", "error", uploadErr)
			writeError(w, http.StatusInternalServerError, "Storage Error", "failed to store original file", r.URL.Path)
			return
		}

		activity, insertErr := h.DB.InsertActivity(r.Context(), userID, data)
		if insertErr != nil {
			slog.Error("insert activity failed", "error", insertErr)
			writeError(w, http.StatusInternalServerError, "Database Error", "failed to save activity", r.URL.Path)
			return
		}

		// FIT physiological profile import
		if data.UserProfileImport != nil {
			if profErr := h.DB.SetUserProfileFieldsIfNull(r.Context(), userID, data.UserProfileImport); profErr != nil {
				slog.Warn("set user profile fields failed", "error", profErr)
			}
		}

		h.postInsertActivity(r.Context(), userID, activity, data, fileBytes, s3Key, format, filename)

		full, _ := h.DB.GetActivity(r.Context(), activity.ID)
		if full == nil {
			full = activity
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

	// Duplicate check via central DuplicateDetector
	match, err := importer.CheckTier1(r.Context(), h.DB, userID, fileBytes, data, 0, 0)
	if err != nil {
		slog.Error("duplicate check failed", "error", err)
		// Not fatal — continue with normal import on error
	} else if match != nil {
		h.handleDuplicateMatch(w, r, userID, match, data)
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

	contentType := "application/xml"

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

	h.postInsertActivity(r.Context(), userID, activity, data, fileBytes, s3Key, format, filename)

	// Re-fetch full activity for response
	full, err := h.DB.GetActivity(r.Context(), activity.ID)
	if err != nil || full == nil {
		full = activity
	}

	resp := model.UploadResponse{
		Activity: full,
		Warnings: warnings,
	}
	writeJSON(w, http.StatusCreated, resp)
}

// strPtr returns a pointer to a string literal (for model fields that require *string).
func strPtr(s string) *string { return &s }

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

// postInsertActivity runs all post-insert steps: Stage 1, streams, track, heatmap,
// sensors, laps/splits, time zones, file info, sidecar, NATS events, shadow FS,
// and background segment matching + lap detection.
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
	// Stage 1 Enrichment (synchronous, ~50ms)
	if data.HasGPS && data.StartLat != 0 {
		enrichment.RunStage1(ctx, h.DB, userID, activity.ID, data, h.Events)
	}

	// Insert streams
	if err := h.DB.InsertStreams(ctx, activity.ID, data.Streams); err != nil {
		slog.Error("insert streams failed", "error", err)
	}

	// Insert R-R intervals (FIT only; empty for GPX/TCX)
	if len(data.RRIntervals) > 0 {
		if err := h.DB.InsertRRIntervals(ctx, activity.ID, data.RRIntervals); err != nil {
			slog.Error("insert rr intervals failed", "error", err)
		}
	}

	// Insert track
	if err := h.DB.InsertTrack(ctx, activity.ID, data.TrackPoints); err != nil {
		slog.Error("insert track failed", "error", err)
	}

	// Insert heatmap line (uses track_full or track_public if privacy zones exist)
	if len(data.TrackPoints) >= 2 {
		var subTypePtr *string
		if data.SubType != "" {
			subTypePtr = &data.SubType
		}
		if err := h.DB.InsertHeatmapLine(ctx, userID, activity.ID, data.SportType, subTypePtr, data.StartedAt); err != nil {
			slog.Error("insert heatmap line failed", "error", err)
		}
		interval, _ := h.DB.GetUserHeatmapInterval(ctx, userID)
		if err := h.DB.InsertHeatmapPoints(ctx, userID, activity.ID, data.SportType, subTypePtr, data.StartedAt, interval); err != nil {
			slog.Error("insert heatmap points failed", "error", err)
		}
	}

	// Insert sensors
	if err := h.DB.InsertSensors(ctx, activity.ID, data.Sensors); err != nil {
		slog.Error("insert sensors failed", "error", err)
	}

	// Insert activity details (laps, splits, lengths, climb pro)
	if err := h.DB.InsertLaps(ctx, activity.ID, data.Laps); err != nil {
		slog.Error("insert laps failed", "error", err)
	}
	if err := h.DB.InsertSplits(ctx, activity.ID, data.Splits); err != nil {
		slog.Error("insert splits failed", "error", err)
	}
	if err := h.DB.InsertSplitSummaries(ctx, activity.ID, data.SplitSummaries); err != nil {
		slog.Error("insert split summaries failed", "error", err)
	}
	if err := h.DB.InsertLengths(ctx, activity.ID, data.Lengths); err != nil {
		slog.Error("insert lengths failed", "error", err)
	}
	if err := h.DB.InsertClimbProEvents(ctx, activity.ID, data.ClimbProEvents); err != nil {
		slog.Error("insert climb pro events failed", "error", err)
	}

	// Save time-in-zone data (from FIT time_in_zone messages or session arrays)
	if len(data.TimeInZoneData) > 0 {
		if err := h.DB.SaveTimeInZones(ctx, activity.ID, data.TimeInZoneData); err != nil {
			slog.Error("save time in zones failed", "error", err)
		}
	}

	// Update file info
	metaKey := s3Key + ".meta"
	bucket := "skadia-originals"
	if err := h.DB.UpdateActivityFileInfo(ctx, activity.ID, bucket, s3Key, format, filename, metaKey); err != nil {
		slog.Error("update file info failed", "error", err)
	}

	// Write .meta sidecar
	meta := sidecar.GenerateMeta(activity.ID, data)
	metaBytes, err := meta.Marshal()
	if err == nil {
		if err := h.Storage.Upload(ctx, metaKey, metaBytes, "application/json"); err != nil {
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
		if err := h.WriteShadowForActivity(ctx, userID, activity.ID, data, fileBytes); err != nil {
			slog.Error("shadow fs write failed", "id", activity.ID, "error", err)
		}
	}

	// Segment matching + lap detection (non-blocking background goroutine)
	if data.HasGPS {
		var gearIDPtr *string
		if activity.GearID != nil {
			gearIDPtr = activity.GearID
		}
		activityID := activity.ID
		sportType := data.SportType
		subType := data.SubType
		startedAt := activity.StartedAt
		go func() {
			bgCtx := context.Background()
			MatchActivitySegments(bgCtx, h.DB, activityID, userID, sportType, subType, gearIDPtr, startedAt)

			// Lap detection (after segment matching, same goroutine)
			if h.DB.IsLapDetectionEnabled(bgCtx, userID) {
				gpsStream, err := h.DB.GetActivityGPSStream(bgCtx, activityID)
				if err == nil && len(gpsStream) >= 20 {
					if laps := DetectLaps(bgCtx, h.DB, activityID, gpsStream); len(laps) > 0 {
						if err := h.DB.InsertDetectedLaps(bgCtx, activityID, laps); err != nil {
							slog.Error("upload: insert detected laps failed", "id", activityID, "error", err)
						} else {
							slog.Info("upload: detected laps", "id", activityID, "count", len(laps))
						}
					}
				}
			}
		}()
	}

	slog.Info("activity inserted",
		"id", activity.ID,
		"sport", data.SportType,
		"format", format,
		"streams", len(data.Streams),
		"sensors", len(data.Sensors),
		"has_gps", data.HasGPS,
	)
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
