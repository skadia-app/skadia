package importer

import (
	"context"
	"log/slog"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/enrichment"
	"github.com/skadia-app/skadia/services/api/internal/event"
	"github.com/skadia-app/skadia/services/api/internal/model"
	"github.com/skadia-app/skadia/services/api/internal/sidecar"
	"github.com/skadia-app/skadia/services/api/internal/storage"
)

// ShadowWriterFunc is a callback that writes the shadow file for an activity.
// The handler package provides this via (*Handler).WriteShadowForActivity.
type ShadowWriterFunc func(ctx context.Context, userID, activityID string, data *model.ActivityData, fileBytes []byte) error

// SegmentMatcherFunc is a callback that runs background segment matching and lap detection.
// The handler package provides the concrete implementation.
type SegmentMatcherFunc func(activityID, userID, sportType, subType string, gearID *string, activityDate time.Time)

// Service holds shared dependencies needed to run post-insert processing steps
// after an activity has been written to the database. It is used by the upload
// handler and will be reused by the cross-platform import pipeline so that all
// import paths go through the same post-processing logic.
type Service struct {
	DB      *database.DB
	Storage *storage.S3Client
	Events  *event.Publisher

	// Callbacks for operations that live in the handler package to avoid
	// circular imports. Both are optional (nil is safe – the step is skipped).
	ShadowWriter   ShadowWriterFunc
	SegmentMatcher SegmentMatcherFunc
}

// NewService constructs a Service with the provided dependencies.
// shadowWriter and segmentMatcher may be nil; those steps will be skipped.
func NewService(
	db *database.DB,
	s3 *storage.S3Client,
	events *event.Publisher,
	shadowWriter ShadowWriterFunc,
	segmentMatcher SegmentMatcherFunc,
) *Service {
	return &Service{
		DB:             db,
		Storage:        s3,
		Events:         events,
		ShadowWriter:   shadowWriter,
		SegmentMatcher: segmentMatcher,
	}
}

// PostProcess runs all post-insert steps for a newly stored activity:
//  1. Stage 1 Enrichment (RunStage1)
//  2. Streams, R-R intervals, Track
//  3. Heatmap + InvalidateTerraIncognitaCache
//  4. Sensors, Laps, Splits, SplitSummaries, Lengths, ClimbPro, TimeInZones
//  5. File Info + S3 Meta-Sidecar
//  6. NATS Events (ActivityUploaded, AnalyticsInvalidate)
//  7. Shadow FS (via ShadowWriter callback)
//  8. Background Segment-Matching + Lap-Detection (via SegmentMatcher callback)
func (s *Service) PostProcess(
	ctx context.Context,
	userID, activityID string,
	data *model.ActivityData,
	fileBytes []byte,
	s3Key, format, filename string,
) error {
	// Resolve activity from DB to get full model (GearID etc.)
	activity, err := s.DB.GetActivity(ctx, activityID)
	if err != nil || activity == nil {
		slog.Error("postprocess: failed to fetch activity", "id", activityID, "error", err)
		return err
	}

	// 1. Stage 1 Enrichment (synchronous, ~50ms)
	if data.HasGPS && data.StartLat != 0 {
		enrichment.RunStage1(ctx, s.DB, userID, activityID, data, s.Events)
	}

	// 2a. Insert streams
	if err := s.DB.InsertStreams(ctx, activityID, data.Streams); err != nil {
		slog.Error("insert streams failed", "error", err)
	}

	// 2b. Insert R-R intervals (FIT only; empty for GPX/TCX)
	if len(data.RRIntervals) > 0 {
		if err := s.DB.InsertRRIntervals(ctx, activityID, data.RRIntervals); err != nil {
			slog.Error("insert rr intervals failed", "error", err)
		}
	}

	// 2c. Insert track
	if err := s.DB.InsertTrack(ctx, activityID, data.TrackPoints); err != nil {
		slog.Error("insert track failed", "error", err)
	}

	// 3. Insert heatmap line + invalidate Terra Incognita cache
	if len(data.TrackPoints) >= 2 {
		var subTypePtr *string
		if data.SubType != "" {
			subTypePtr = &data.SubType
		}
		if err := s.DB.InsertHeatmapLine(ctx, userID, activityID, data.SportType, subTypePtr, data.StartedAt); err != nil {
			slog.Error("insert heatmap line failed", "error", err)
		}
		s.DB.InvalidateTerraIncognitaCache(ctx, userID)
		interval, _ := s.DB.GetUserHeatmapInterval(ctx, userID)
		if err := s.DB.InsertHeatmapPoints(ctx, userID, activityID, data.SportType, subTypePtr, data.StartedAt, interval); err != nil {
			slog.Error("insert heatmap points failed", "error", err)
		}
	}

	// 4a. Insert sensors
	if err := s.DB.InsertSensors(ctx, activityID, data.Sensors); err != nil {
		slog.Error("insert sensors failed", "error", err)
	}

	// 4b. Insert activity details: laps, splits, lengths, climb pro
	if err := s.DB.InsertLaps(ctx, activityID, data.Laps); err != nil {
		slog.Error("insert laps failed", "error", err)
	}
	if err := s.DB.InsertSplits(ctx, activityID, data.Splits); err != nil {
		slog.Error("insert splits failed", "error", err)
	}
	if err := s.DB.InsertSplitSummaries(ctx, activityID, data.SplitSummaries); err != nil {
		slog.Error("insert split summaries failed", "error", err)
	}
	if err := s.DB.InsertLengths(ctx, activityID, data.Lengths); err != nil {
		slog.Error("insert lengths failed", "error", err)
	}
	if err := s.DB.InsertClimbProEvents(ctx, activityID, data.ClimbProEvents); err != nil {
		slog.Error("insert climb pro events failed", "error", err)
	}

	// 4c. Save time-in-zone data
	if len(data.TimeInZoneData) > 0 {
		if err := s.DB.SaveTimeInZones(ctx, activityID, data.TimeInZoneData); err != nil {
			slog.Error("save time in zones failed", "error", err)
		}
	}

	// 5a. Update file info
	metaKey := s3Key + ".meta"
	bucket := "skadia-originals"
	if err := s.DB.UpdateActivityFileInfo(ctx, activityID, bucket, s3Key, format, filename, metaKey); err != nil {
		slog.Error("update file info failed", "error", err)
	}

	// 5b. Write .meta sidecar to S3
	meta := sidecar.GenerateMeta(activityID, data)
	metaBytes, marshalErr := meta.Marshal()
	if marshalErr == nil {
		if err := s.Storage.Upload(ctx, metaKey, metaBytes, "application/json"); err != nil {
			slog.Error("meta upload failed", "error", err)
		}
	}

	// 6. Publish NATS events
	if s.Events != nil {
		evt := event.ActivityUploadedEvent{
			ActivityID: activityID,
			UserID:     userID,
			SportType:  data.SportType,
			HasGPS:     data.HasGPS,
		}
		if err := s.Events.PublishActivityUploaded(evt); err != nil {
			slog.Error("event publish failed", "error", err)
		}
		s.Events.PublishAnalyticsInvalidate(userID)
	}

	// 7. Write to Shadow FS (best-effort)
	if s.ShadowWriter != nil {
		if err := s.ShadowWriter(ctx, userID, activityID, data, fileBytes); err != nil {
			slog.Error("shadow fs write failed", "id", activityID, "error", err)
		}
	}

	// 8. Segment matching + lap detection (non-blocking background goroutine)
	if data.HasGPS && s.SegmentMatcher != nil {
		var gearIDPtr *string
		if activity.GearID != nil {
			gearIDPtr = activity.GearID
		}
		sportType := data.SportType
		subType := data.SubType
		startedAt := activity.StartedAt
		matcher := s.SegmentMatcher
		go matcher(activityID, userID, sportType, subType, gearIDPtr, startedAt)
	}

	slog.Info("activity post-processed",
		"id", activityID,
		"sport", data.SportType,
		"format", format,
		"streams", len(data.Streams),
		"sensors", len(data.Sensors),
		"has_gps", data.HasGPS,
	)
	return nil
}
