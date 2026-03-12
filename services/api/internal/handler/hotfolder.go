package handler

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/skadia-app/skadia/services/api/internal/enrichment"
	"github.com/skadia-app/skadia/services/api/internal/event"
	"github.com/skadia-app/skadia/services/api/internal/fitimport"
	"github.com/skadia-app/skadia/services/api/internal/gpximport"
	"github.com/skadia-app/skadia/services/api/internal/importer"
	"github.com/skadia-app/skadia/services/api/internal/model"
	"github.com/skadia-app/skadia/services/api/internal/sidecar"
	"github.com/skadia-app/skadia/services/api/internal/tcximport"
)

// UploadFromFile imports a single activity file from disk on behalf of userID.
// This is used by the hotfolder watcher.
func (h *Handler) UploadFromFile(ctx context.Context, userID, filePath string) error {
	fileBytes, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("read file: %w", err)
	}

	filename := filepath.Base(filePath)
	format := detectFormat(fileBytes, filename)
	if format == "" {
		return fmt.Errorf("unsupported format: %s", filename)
	}

	var data *model.ActivityData
	switch format {
	case "fit":
		data, err = fitimport.ParseFIT(bytes.NewReader(fileBytes), filename)
	case "gpx":
		data, err = gpximport.ParseGPX(bytes.NewReader(fileBytes), filename)
	case "tcx":
		data, err = tcximport.ParseTCX(bytes.NewReader(fileBytes), filename)
	}
	if err != nil {
		return fmt.Errorf("parse %s: %w", format, err)
	}

	// Duplicate check
	match, err := importer.CheckTier1(ctx, h.DB, userID, fileBytes, data, 0, 0, 0)
	if err != nil {
		slog.Warn("hotfolder: tier-1 check failed", "file", filename, "error", err)
	} else if match != nil {
		settings, _ := h.DB.GetImportSettings(ctx, userID)
		strategy := importer.StrategyEnrichMissing
		if settings != nil {
			strategy = importer.ConflictStrategy(settings.ConflictStrategy)
		}
		switch strategy {
		case importer.StrategyReplaceAll:
			_ = importer.ReplaceActivity(ctx, h.DB, match.ExistingActivityID, data)
			slog.Info("hotfolder: activity replaced", "id", match.ExistingActivityID, "file", filename)
		case importer.StrategySkip:
			slog.Info("hotfolder: duplicate skipped", "id", match.ExistingActivityID, "file", filename)
		default:
			existing, _ := h.DB.GetActivity(ctx, match.ExistingActivityID)
			n, _ := importer.EnrichActivity(ctx, h.DB, match.ExistingActivityID, existing, data)
			slog.Info("hotfolder: activity enriched", "id", match.ExistingActivityID, "fields", n, "file", filename)
		}
		return nil
	}

	// SHA256 hash
	hash := sha256.Sum256(fileBytes)
	hashStr := hex.EncodeToString(hash[:])
	data.FileHash = hashStr

	s3Key := fmt.Sprintf("%s/%s.%s", userID, hashStr, format)
	contentType := "application/octet-stream"
	if format == "gpx" || format == "tcx" {
		contentType = "application/xml"
	}

	if err := h.Storage.Upload(ctx, s3Key, fileBytes, contentType); err != nil {
		return fmt.Errorf("S3 upload: %w", err)
	}

	activity, err := h.DB.InsertActivity(ctx, userID, data)
	if err != nil {
		return fmt.Errorf("insert activity: %w", err)
	}

	if data.HasGPS && data.StartLat != 0 {
		enrichment.RunStage1(ctx, h.DB, userID, activity.ID, data, h.Events)
	}
	_ = h.DB.InsertStreams(ctx, activity.ID, data.Streams)
	if len(data.RRIntervals) > 0 {
		_ = h.DB.InsertRRIntervals(ctx, activity.ID, data.RRIntervals)
	}
	_ = h.DB.InsertTrack(ctx, activity.ID, data.TrackPoints)

	if len(data.TrackPoints) >= 2 {
		var subTypePtr *string
		if data.SubType != "" {
			subTypePtr = &data.SubType
		}
		_ = h.DB.InsertHeatmapLine(ctx, userID, activity.ID, data.SportType, subTypePtr, data.StartedAt)
		h.DB.InvalidateTerraIncognitaCache(ctx, userID)
		interval, _ := h.DB.GetUserHeatmapInterval(ctx, userID)
		_ = h.DB.InsertHeatmapPoints(ctx, userID, activity.ID, data.SportType, subTypePtr, data.StartedAt, interval)
	}

	_ = h.DB.InsertSensors(ctx, activity.ID, data.Sensors)
	_ = h.DB.InsertLaps(ctx, activity.ID, data.Laps)
	_ = h.DB.InsertSplits(ctx, activity.ID, data.Splits)
	_ = h.DB.InsertSplitSummaries(ctx, activity.ID, data.SplitSummaries)
	_ = h.DB.InsertLengths(ctx, activity.ID, data.Lengths)
	_ = h.DB.InsertClimbProEvents(ctx, activity.ID, data.ClimbProEvents)

	if len(data.TimeInZoneData) > 0 {
		_ = h.DB.SaveTimeInZones(ctx, activity.ID, data.TimeInZoneData)
	}

	metaKey := s3Key + ".meta"
	_ = h.DB.UpdateActivityFileInfo(ctx, activity.ID, "skadia-originals", s3Key, format, filename, metaKey)

	meta := sidecar.GenerateMeta(activity.ID, data)
	if metaBytes, err := meta.Marshal(); err == nil {
		_ = h.Storage.Upload(ctx, metaKey, metaBytes, "application/json")
	}

	if h.Events != nil {
		_ = h.Events.PublishActivityUploaded(event.ActivityUploadedEvent{
			ActivityID: activity.ID,
			UserID:     userID,
			SportType:  data.SportType,
			HasGPS:     data.HasGPS,
		})
	}

	if h.ShadowFS != nil {
		if err := h.WriteShadowForActivity(ctx, userID, activity.ID, data, fileBytes); err != nil {
			slog.Warn("hotfolder: shadow fs write failed", "id", activity.ID, "error", err)
		}
	}

	if data.HasGPS {
		var gearIDPtr *string
		if activity.GearID != nil {
			gearIDPtr = activity.GearID
		}
		go func() {
			bgCtx := context.Background()
			MatchActivitySegments(bgCtx, h.DB, activity.ID, userID, data.SportType, data.SubType, gearIDPtr, activity.StartedAt)
			if h.DB.IsLapDetectionEnabled(bgCtx, userID) {
				gpsStream, err := h.DB.GetActivityGPSStream(bgCtx, activity.ID)
				if err == nil && len(gpsStream) >= 20 {
					if laps := DetectLaps(bgCtx, h.DB, activity.ID, gpsStream); len(laps) > 0 {
						_ = h.DB.InsertDetectedLaps(bgCtx, activity.ID, laps)
					}
				}
			}
		}()
	}

	slog.Info("hotfolder: activity imported",
		"id", activity.ID,
		"sport", data.SportType,
		"format", format,
		"file", filename,
	)
	return nil
}
