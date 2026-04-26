package backfill

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/enrichment"
	"github.com/skadia-app/skadia/services/api/internal/routing"
)

const (
	activityEnrichmentsJobType = "backfill_activity_enrichments"
	backfillBatchSize          = 100
	backfillBatchSleep         = time.Second
)

var (
	ErrTileBuildRunning = errors.New("tile_build job is running; activity enrichments backfill refused")
	ErrBackfillRunning  = errors.New("another backfill_activity_enrichments job is running")
)

type BackfillOptions struct {
	Force  bool
	DryRun bool
}

type activityEnrichmentBackfillCandidate struct {
	ActivityID string
	UserID     string
}

func RunActivityEnrichmentsBackfill(
	ctx context.Context,
	db *database.DB,
	graphReader routing.GraphReader,
	opts BackfillOptions,
) error {
	if running, err := db.HasRunningJobGlobal(ctx, "tile_build"); err != nil {
		return fmt.Errorf("check running tile_build job: %w", err)
	} else if running {
		return ErrTileBuildRunning
	}

	if running, err := db.HasRunningJobGlobal(ctx, activityEnrichmentsJobType); err != nil {
		return fmt.Errorf("check running activity enrichments backfill: %w", err)
	} else if running {
		return ErrBackfillRunning
	}

	candidates, err := listActivityEnrichmentBackfillCandidates(ctx, db, opts.Force)
	if err != nil {
		return err
	}

	slog.Info("activity enrichments backfill planned",
		"total", len(candidates),
		"dry_run", opts.DryRun,
		"force", opts.Force,
	)
	if opts.DryRun {
		return nil
	}

	for i, candidate := range candidates {
		settings, err := db.GetEnrichmentSettings(ctx, candidate.UserID)
		if err != nil {
			slog.Warn("activity enrichments backfill: fetch settings failed",
				"activity_id", candidate.ActivityID,
				"user_id", candidate.UserID,
				"error", err,
			)
			continue
		}
		if settings == nil || (!settings.EnableSurfaceDetection && !settings.EnableDifficultyScales) {
			continue
		}

		stream, err := enrichment.GetEnrichmentStream(ctx, db, candidate.ActivityID)
		if err != nil {
			slog.Warn("activity enrichments backfill: load stream failed",
				"activity_id", candidate.ActivityID,
				"user_id", candidate.UserID,
				"error", err,
			)
			continue
		}

		if err := enrichment.EnrichTrackAnalysis(ctx, db, candidate.ActivityID, settings, graphReader, stream); err != nil {
			slog.Warn("activity enrichments backfill: enrich activity failed",
				"activity_id", candidate.ActivityID,
				"user_id", candidate.UserID,
				"error", err,
			)
			continue
		}

		if (i+1)%backfillBatchSize == 0 {
			slog.Info("activity enrichments backfill progress",
				"done", i+1,
				"total", len(candidates),
			)
			timer := time.NewTimer(backfillBatchSleep)
			select {
			case <-ctx.Done():
				timer.Stop()
				return ctx.Err()
			case <-timer.C:
			}
		}
	}

	slog.Info("activity enrichments backfill complete", "processed", len(candidates))
	return nil
}

func listActivityEnrichmentBackfillCandidates(
	ctx context.Context,
	db *database.DB,
	force bool,
) ([]activityEnrichmentBackfillCandidate, error) {
	rows, err := db.Pool.Query(ctx, `
		SELECT a.id, a.user_id
		FROM activities a
		WHERE $1
		   OR NOT EXISTS (
			SELECT 1
			FROM activity_enrichments e
			WHERE e.activity_id = a.id
			  AND e.category IN ('forest_pct', 'protected_pct', 'sac_scale', 'mtb_scale', 'vf_scale', 'vf_unknown')
		   )
		ORDER BY a.created_at DESC
	`, force)
	if err != nil {
		return nil, fmt.Errorf("list activity enrichments backfill candidates: query: %w", err)
	}
	defer rows.Close()

	var candidates []activityEnrichmentBackfillCandidate
	for rows.Next() {
		var candidate activityEnrichmentBackfillCandidate
		if err := rows.Scan(&candidate.ActivityID, &candidate.UserID); err != nil {
			return nil, fmt.Errorf("list activity enrichments backfill candidates: scan: %w", err)
		}
		candidates = append(candidates, candidate)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list activity enrichments backfill candidates: rows: %w", err)
	}
	return candidates, nil
}
