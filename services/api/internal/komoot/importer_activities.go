package komoot

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"slices"

	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/fitimport"
	"github.com/skadia-app/skadia/services/api/internal/gpximport"
	"github.com/skadia-app/skadia/services/api/internal/importer"
	"github.com/skadia-app/skadia/services/api/internal/model"
	"github.com/skadia-app/skadia/services/api/internal/sport"
	"github.com/skadia-app/skadia/services/api/internal/storage"
)

// ActivitiesImporter imports recorded tours (tour_recorded) from the Komoot API.
type ActivitiesImporter struct {
	Client *Client
}

func (i *ActivitiesImporter) Name() string        { return "activities" }
func (i *ActivitiesImporter) Description() string { return "Aufgezeichnete Touren" }
func (i *ActivitiesImporter) Priority() int       { return 20 }

// KomootImportParams bundles everything the Komoot importers need.
type KomootImportParams struct {
	DB            *database.DB
	Storage       *storage.S3Client
	ImportService *importer.Service // centralised duplicate detection + insert
	UserID        string
	BatchID       string  // correlates all import_log entries for one import run
	TourIDs       []int64 // nil/empty = all tours
	Options       importer.ImportOptions
	OnProgress    func(current, total int, msg string)
}

// KomootImportResult summarises the outcome of an import run.
type KomootImportResult struct {
	Imported        int
	Skipped         int
	Errors          []string
	TourActivityMap map[int64]string // komoot_tour_id → activity.ID (for photo importer)
}

// Import fetches all tour_recorded tours from Komoot and imports them as activities.
// Duplicate detection is delegated to ImportService.ProcessActivity which runs
// Tier-0 (Komoot Tour-ID) and Tier-1 (file hash + temporal match) internally.
func (i *ActivitiesImporter) Import(ctx context.Context, p KomootImportParams) (*KomootImportResult, error) {
	result := &KomootImportResult{
		TourActivityMap: make(map[int64]string),
	}

	// Build knownIDs set for incremental sync optimization (skip API pages with all-known tours)
	knownIDs, err := buildKnownKomootIDs(ctx, p.DB, p.UserID)
	if err != nil {
		slog.Warn("komoot activities: cannot build known IDs set", "error", err)
		knownIDs = map[int64]bool{}
	}

	// Fetch tour list from Komoot API
	tours, err := i.Client.ListTours(ctx, knownIDs)
	if err != nil {
		return result, fmt.Errorf("komoot activities: list tours: %w", err)
	}

	// Filter to tour_recorded only, and optionally to specific TourIDs
	var recorded []Tour
	for _, t := range tours {
		if t.Type != "tour_recorded" {
			continue
		}
		if len(p.TourIDs) > 0 && !containsID(p.TourIDs, t.ID) {
			continue
		}
		recorded = append(recorded, t)
	}

	total := len(recorded)
	slog.Info("komoot activities: found tours to import", "count", total)

	for idx, tour := range recorded {
		if p.OnProgress != nil {
			p.OnProgress(idx+1, total, fmt.Sprintf("activities: %d/%d %s", idx+1, total, tour.Name))
		}

		// --- Import log tracking ---
		logFilename := fmt.Sprintf("%d", tour.ID)
		var batchIDPtr *string
		if p.BatchID != "" {
			batchIDPtr = &p.BatchID
		}
		tracker := p.DB.NewImportTracker(ctx, p.UserID, "komoot", logFilename, nil, batchIDPtr)
		tracker.Processing(ctx)

		// Date filter
		if p.Options.DateFrom != nil && tour.Date.Before(*p.Options.DateFrom) {
			result.Skipped++
			tracker.Skipped(ctx, "date_filtered", nil)
			continue
		}
		if p.Options.DateTo != nil && tour.Date.After(*p.Options.DateTo) {
			result.Skipped++
			tracker.Skipped(ctx, "date_filtered", nil)
			continue
		}

		// Download FIT (preferred) or GPX
		fileBytes, format, err := i.Client.DownloadFIT(ctx, tour.ID)
		if err != nil {
			slog.Warn("komoot activities: download failed", "tour_id", tour.ID, "error", err)
			result.Errors = append(result.Errors, fmt.Sprintf("download tour %d: %s", tour.ID, err))
			tracker.Error(ctx, err)
			continue
		}

		// Compute file hash for dedup
		sum := sha256.Sum256(fileBytes)
		hashStr := hex.EncodeToString(sum[:])
		tracker.SetFileHash(ctx, hashStr)

		// Parse file to get ActivityData
		var data *model.ActivityData
		switch format {
		case "fit":
			data, err = fitimport.ParseFIT(bytes.NewReader(fileBytes), fmt.Sprintf("%d.fit", tour.ID))
		case "gpx":
			data, err = gpximport.ParseGPX(bytes.NewReader(fileBytes), fmt.Sprintf("%d.gpx", tour.ID))
		}
		if err != nil || data == nil {
			slog.Warn("komoot activities: parse failed", "tour_id", tour.ID, "format", format, "error", err)
			// Fallback: build minimal ActivityData from tour metadata
			data = tourToActivityData(tour)
		}

		// Sport type resolution: compare file sport vs Komoot API sport using SegmentGroup.
		// Silent resolution — no queue, no UI. File parser wins except when generic.
		if mapping, ok := KomootSportMapping[tour.Sport]; ok {
			fileSportType := data.SportType
			fileGroup := sport.SegmentGroup(fileSportType)
			komootGroup := sport.SegmentGroup(mapping.SportType)

			switch {
			case fileGroup == "generic":
				// File parser returned generic → Komoot wins
				data.SportType = mapping.SportType
				data.SubType = mapping.SubType
			case komootGroup == "generic":
				// Komoot returned generic → file wins (already set)
			case fileGroup == komootGroup:
				// Same group → file wins (more specific from device)
				// NOTE: SubType from Komoot is NOT adopted here — SubType and SportType
				// are a pair and mixing sources can create inconsistent combinations.
				// The existing mergeActivityData/isSportRefinement handles this correctly.
			default:
				// Different specific groups → file wins (device is authoritative)
			}

			// E-Bike flag is always additive (Komoot knows if tour was e-bike)
			if mapping.IsEbike {
				data.IsEbike = true
			}
		}

		// Map Komoot tour visibility to activity visibility
		if tour.Status == "public" {
			data.Visibility = "public"
		} else {
			data.Visibility = "private"
		}

		data.Source = "Komoot"
		data.FileHash = hashStr
		data.FileFormat = format
		if data.OriginalName == "" && tour.Name != "" {
			data.OriginalName = tour.Name
		}

		// Duplicate detection + insert via ImportService (handles Tier-0 via KomootTourID + Tier-1 hash/temporal)
		actResult, processErr := p.ImportService.ProcessActivity(ctx, p.UserID, data, fileBytes,
			importer.ImportMetadata{KomootTourID: tour.ID, SourcePlatform: "komoot", ConflictStrategy: p.Options.ConflictStrategy})
		if processErr != nil {
			slog.Warn("komoot activities: process failed", "tour_id", tour.ID, "error", processErr)
			result.Errors = append(result.Errors, fmt.Sprintf("process tour %d: %s", tour.ID, processErr))
			tracker.Error(ctx, processErr)
			continue
		}

		// Always populate TourActivityMap — even for duplicates — so the PhotosImporter can look up the activity ID.
		result.TourActivityMap[tour.ID] = actResult.ActivityID

		if actResult.Status == "enriched" || actResult.Status == "replaced" {
			result.Skipped++ // stats: not newly created
			tracker.Completed(ctx, actResult.ActivityID)
			continue
		}
		if actResult.Status == "skipped" {
			result.Skipped++
			tracker.Skipped(ctx, "duplicate", &actResult.ActivityID)
			continue
		}

		activityID := actResult.ActivityID

		// Upload original file to S3
		var s3Key string
		if len(fileBytes) > 0 && p.Storage != nil {
			s3Key = fmt.Sprintf("%s/%s.%s", p.UserID, hashStr, format)
			contentType := "application/octet-stream"
			if format == "gpx" {
				contentType = "application/xml"
			}
			if err := p.Storage.Upload(ctx, s3Key, fileBytes, contentType); err != nil {
				slog.Warn("komoot activities: S3 upload failed", "tour_id", tour.ID, "error", err)
			}
		}

		// Set Komoot tour title (before PostProcess which may generate auto-title)
		if tour.Name != "" {
			_ = p.DB.UpdateEnrichmentFields(ctx, activityID, map[string]any{"title": tour.Name})
		}

		// PostProcess handles file info, sidecar, streams, track, heatmap, laps, segment matching, Stage 1 etc.
		if err := p.ImportService.PostProcess(ctx, p.UserID, activityID, data, fileBytes, s3Key, format, fmt.Sprintf("%d.%s", tour.ID, format)); err != nil {
			slog.Warn("komoot activities: PostProcess failed", "tour_id", tour.ID, "activity_id", activityID, "error", err)
		}

		tracker.Completed(ctx, activityID)
		result.Imported++
	}

	slog.Info("komoot activities import done",
		"imported", result.Imported,
		"skipped", result.Skipped,
		"errors", len(result.Errors),
	)
	return result, nil
}

// tourToActivityData converts Komoot tour metadata to a minimal ActivityData.
// Used as fallback when FIT/GPX parsing fails.
func tourToActivityData(tour Tour) *model.ActivityData {
	data := &model.ActivityData{
		Source:    "Komoot",
		StartedAt: tour.Date,
	}
	if mapping, ok := KomootSportMapping[tour.Sport]; ok {
		data.SportType = mapping.SportType
		data.SubType = mapping.SubType
		data.IsEbike = mapping.IsEbike
	} else {
		data.SportType = "generic"
	}
	if tour.Distance > 0 {
		data.DistanceM = tour.Distance
	}
	if tour.ElevationUp > 0 {
		data.ElevationGainM = tour.ElevationUp
	}
	if tour.Duration > 0 {
		data.ElapsedTimeS = int(tour.Duration)
		data.MovingTimeS = int(tour.Duration)
	}
	if tour.TimeInMotion > 0 {
		data.MovingTimeS = int(tour.TimeInMotion)
	}
	return data
}

// buildKnownKomootIDs fetches all known komoot_tour_ids for a user (for incremental sync).
func buildKnownKomootIDs(ctx context.Context, db *database.DB, userID string) (map[int64]bool, error) {
	rows, err := db.Pool.Query(ctx,
		`SELECT komoot_tour_id FROM activities WHERE user_id = $1 AND komoot_tour_id IS NOT NULL`,
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := map[int64]bool{}
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err == nil {
			result[id] = true
		}
	}
	return result, rows.Err()
}

// containsID checks if id is in ids.
func containsID(ids []int64, id int64) bool {
	return slices.Contains(ids, id)
}
