package komoot

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"

	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/enrichment"
	"github.com/skadia-app/skadia/services/api/internal/fitimport"
	"github.com/skadia-app/skadia/services/api/internal/gpximport"
	"github.com/skadia-app/skadia/services/api/internal/importer"
	"github.com/skadia-app/skadia/services/api/internal/model"
	"github.com/skadia-app/skadia/services/api/internal/sidecar"
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
	DB         *database.DB
	Storage    *storage.S3Client
	UserID     string
	TourIDs    []int64 // nil/empty = all tours
	Options    importer.ImportOptions
	OnProgress func(current, total int, msg string)
}

// KomootImportResult summarises the outcome of an import run.
type KomootImportResult struct {
	Imported        int
	Skipped         int
	Errors          []string
	TourActivityMap map[int64]string // komoot_tour_id → activity.ID (for photo importer)
}

// Import fetches all tour_recorded tours from Komoot and imports them as activities.
// Duplicate detection:
//  1. Komoot Tour-ID (most reliable — exact match)
//  2. File hash + temporal match via importer.CheckTier1
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

		// Date filter
		if p.Options.DateFrom != nil && tour.Date.Before(*p.Options.DateFrom) {
			result.Skipped++
			continue
		}
		if p.Options.DateTo != nil && tour.Date.After(*p.Options.DateTo) {
			result.Skipped++
			continue
		}

		// Tier-0: Komoot Tour-ID (most reliable dedup)
		exists, err := p.DB.ActivityExistsByKomootTourID(ctx, p.UserID, tour.ID)
		if err != nil {
			slog.Warn("komoot activities: tour ID check failed", "tour_id", tour.ID, "error", err)
		}
		if exists {
			existingID, _ := p.DB.GetActivityIDByKomootTourID(ctx, p.UserID, tour.ID)
			result.TourActivityMap[tour.ID] = existingID
			result.Skipped++
			continue
		}

		// Download FIT (preferred) or GPX
		fileBytes, format, err := i.Client.DownloadFIT(ctx, tour.ID)
		if err != nil {
			slog.Warn("komoot activities: download failed", "tour_id", tour.ID, "error", err)
			result.Errors = append(result.Errors, fmt.Sprintf("download tour %d: %s", tour.ID, err))
			continue
		}

		// Compute file hash for dedup
		sum := sha256.Sum256(fileBytes)
		hashStr := hex.EncodeToString(sum[:])

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

		// Apply Komoot sport mapping (Komoot sport takes priority when parser returns generic/unknown)
		if mapping, ok := KomootSportMapping[tour.Sport]; ok {
			if data.SportType == "" || data.SportType == "generic" {
				data.SportType = mapping.SportType
				data.SubType = mapping.SubType
			}
			if mapping.IsEbike {
				data.IsEbike = true
			}
		}

		data.Source = "Komoot"
		data.FileHash = hashStr
		data.FileFormat = format
		if data.OriginalName == "" && tour.Name != "" {
			data.OriginalName = tour.Name
		}

		// Tier-1: File hash + temporal dedup
		match, dupErr := importer.CheckTier1(ctx, p.DB, p.UserID, fileBytes, data, 0, 0, tour.ID)
		if dupErr != nil {
			slog.Warn("komoot activities: tier-1 check failed", "tour_id", tour.ID, "error", dupErr)
			result.Errors = append(result.Errors, fmt.Sprintf("dedup tour %d: %s", tour.ID, dupErr))
			continue
		}
		if match != nil {
			switch p.Options.ConflictStrategy {
			case importer.StrategyReplaceAll:
				_ = importer.ReplaceActivity(ctx, p.DB, match.ExistingActivityID, data)
			case importer.StrategySkip:
				// explicit skip — no enrichment
			default: // enrich_missing
				existing, getErr := p.DB.GetActivity(ctx, match.ExistingActivityID)
				if getErr != nil {
					slog.Warn("komoot activities: cannot fetch existing activity for enrich",
						"id", match.ExistingActivityID, "error", getErr)
				}
				_, _ = importer.EnrichActivity(ctx, p.DB, match.ExistingActivityID, existing, data)
			}
			_ = p.DB.SetActivityKomootTourID(ctx, match.ExistingActivityID, tour.ID)
			result.TourActivityMap[tour.ID] = match.ExistingActivityID
			result.Skipped++
			continue
		}

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

		// Insert activity
		activity, err := p.DB.InsertActivity(ctx, p.UserID, data)
		if err != nil {
			slog.Warn("komoot activities: insert failed", "tour_id", tour.ID, "error", err)
			result.Errors = append(result.Errors, fmt.Sprintf("insert tour %d: %s", tour.ID, err))
			continue
		}

		// Set komoot_tour_id for future dedup
		if err := p.DB.SetActivityKomootTourID(ctx, activity.ID, tour.ID); err != nil {
			slog.Warn("komoot activities: set komoot_tour_id failed", "error", err)
		}

		// Set title
		if tour.Name != "" {
			_ = p.DB.UpdateEnrichmentFields(ctx, activity.ID, map[string]any{"title": tour.Name})
		}

		// Update file info in DB
		if s3Key != "" {
			metaKey := s3Key + ".meta"
			bucket := "skadia-originals"
			if err := p.DB.UpdateActivityFileInfo(ctx, activity.ID, bucket, s3Key, format, fmt.Sprintf("%d.%s", tour.ID, format), metaKey); err != nil {
				slog.Warn("komoot activities: update file info failed", "error", err)
			}
			// Write .meta sidecar
			meta := sidecar.GenerateMeta(activity.ID, data)
			if metaBytes, err := meta.Marshal(); err == nil {
				if err := p.Storage.Upload(ctx, metaKey, metaBytes, "application/json"); err != nil {
					slog.Warn("komoot activities: meta upload failed", "error", err)
				}
			}
		}

		// Insert streams, track, heatmap
		if len(data.Streams) > 0 {
			if err := p.DB.InsertStreams(ctx, activity.ID, data.Streams); err != nil {
				slog.Warn("komoot activities: insert streams failed", "error", err)
			}
		}
		if len(data.TrackPoints) > 0 {
			if err := p.DB.InsertTrack(ctx, activity.ID, data.TrackPoints); err != nil {
				slog.Warn("komoot activities: insert track failed", "error", err)
			}
			if len(data.TrackPoints) >= 2 {
				var subTypePtr *string
				if data.SubType != "" {
					subTypePtr = &data.SubType
				}
				if err := p.DB.InsertHeatmapLine(ctx, p.UserID, activity.ID, data.SportType, subTypePtr, data.StartedAt); err != nil {
					slog.Warn("komoot activities: insert heatmap line failed", "error", err)
				}
				interval, _ := p.DB.GetUserHeatmapInterval(ctx, p.UserID)
				if err := p.DB.InsertHeatmapPoints(ctx, p.UserID, activity.ID, data.SportType, subTypePtr, data.StartedAt, interval); err != nil {
					slog.Warn("komoot activities: insert heatmap points failed", "error", err)
				}
			}
		}
		if len(data.Laps) > 0 {
			if err := p.DB.InsertLaps(ctx, activity.ID, data.Laps); err != nil {
				slog.Warn("komoot activities: insert laps failed", "error", err)
			}
		}

		// Stage 1 enrichment
		if data.HasGPS && data.StartLat != 0 {
			enrichment.RunStage1(ctx, p.DB, p.UserID, activity.ID, data)
		}

		result.TourActivityMap[tour.ID] = activity.ID
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
	for _, v := range ids {
		if v == id {
			return true
		}
	}
	return false
}
