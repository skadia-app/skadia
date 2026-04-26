package backfill

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/enrichment"
	"github.com/skadia-app/skadia/services/api/internal/routing"
)

const (
	ActivityEnrichmentsJobType = "backfill_activity_enrichments"
	activityEnrichmentsJobType = ActivityEnrichmentsJobType
	backfillBatchSize          = 100
	backfillBatchSleep         = time.Second

	// backfillPerActivityTimeout is the per-activity context deadline so that a
	// single activity enrichment cannot stall the entire backfill indefinitely.
	backfillPerActivityTimeout = 30 * time.Second

	// backfillMaxFailedIDs is the maximum number of failed activity IDs stored
	// in the job metadata to bound metadata size.
	backfillMaxFailedIDs = 50

	// backfillFailureRateAbort is the fraction of activities that may fail
	// before the backfill is considered a global failure (status='failed').
	// Below this threshold the job completes with status='completed_with_errors'.
	backfillFailureRateAbort = 0.50
)

var (
	ErrTileBuildRunning = errors.New("tile_build job is running; activity enrichments backfill refused")
	ErrBackfillRunning  = errors.New("another backfill_activity_enrichments job is running")
)

type BackfillOptions struct {
	Force      bool
	DryRun     bool
	OwnedJobID string
}

type activityEnrichmentBackfillCandidate struct {
	ActivityID string
	UserID     string
}

// RunActivityEnrichmentsBackfill re-runs the activity track enrichment pipeline.
// As of 2026-04-26, candidate selection requires has_gps=true; non-GPS activities
// are excluded from both dry-run and real run regardless of the Force flag.
// To clean up stale enrichment rows on activities that lost their has_gps state,
// use a separate maintenance task — this backfill no longer touches them.
//
// DO NOT parallelize this backfill — see memory oom-concurrent-jobs.md.
// Heavy PostGIS operations under concurrent load cause OOM on this host.
func RunActivityEnrichmentsBackfill(
	ctx context.Context,
	db *database.DB,
	graphReader routing.GraphReader,
	opts BackfillOptions,
) error {
	if err := checkActivityEnrichmentsBackfillMutex(ctx, db, opts.OwnedJobID); err != nil {
		return err
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
	if opts.OwnedJobID != "" {
		_ = db.UpdateBackgroundJobTotal(ctx, opts.OwnedJobID, int64(len(candidates)))
	}
	if opts.DryRun {
		return nil
	}

	return runActivityEnrichmentsBackfill(ctx, db, graphReader, opts, candidates)
}

func DryRunActivityEnrichmentsBackfill(ctx context.Context, db *database.DB, opts BackfillOptions) (int, error) {
	opts.DryRun = true
	if err := checkActivityEnrichmentsBackfillMutex(ctx, db, opts.OwnedJobID); err != nil {
		return 0, err
	}

	candidates, err := listActivityEnrichmentBackfillCandidates(ctx, db, opts.Force)
	if err != nil {
		return 0, err
	}
	return len(candidates), nil
}

func checkActivityEnrichmentsBackfillMutex(ctx context.Context, db *database.DB, ownedJobID string) error {
	if running, err := db.HasRunningJobGlobal(ctx, "tile_build"); err != nil {
		return fmt.Errorf("check running tile_build job: %w", err)
	} else if running {
		return ErrTileBuildRunning
	}

	if ownedJobID == "" {
		if running, err := db.HasRunningJobGlobal(ctx, activityEnrichmentsJobType); err != nil {
			return fmt.Errorf("check running activity enrichments backfill: %w", err)
		} else if running {
			return ErrBackfillRunning
		}
		return nil
	}

	var ownsRunningJob bool
	if err := db.Pool.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1
			FROM background_jobs
			WHERE id = $1
			  AND job_type = $2
			  AND status = 'running'
		)
	`, ownedJobID, activityEnrichmentsJobType).Scan(&ownsRunningJob); err != nil {
		return fmt.Errorf("check owned activity enrichments backfill: %w", err)
	}
	if !ownsRunningJob {
		return ErrBackfillRunning
	}

	var otherRunning bool
	if err := db.Pool.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1
			FROM background_jobs
			WHERE job_type = $1
			  AND status = 'running'
			  AND id <> $2
		)
	`, activityEnrichmentsJobType, ownedJobID).Scan(&otherRunning); err != nil {
		return fmt.Errorf("check other activity enrichments backfill: %w", err)
	}
	if otherRunning {
		return ErrBackfillRunning
	}
	return nil
}

func runActivityEnrichmentsBackfill(
	ctx context.Context,
	db *database.DB,
	graphReader routing.GraphReader,
	opts BackfillOptions,
	candidates []activityEnrichmentBackfillCandidate,
) error {
	total := len(candidates)
	var processed, failed int
	var failedIDs []string // capped at backfillMaxFailedIDs

	for i, candidate := range candidates {
		iterErr := enrichOneCandidate(ctx, db, graphReader, candidate)
		if iterErr != nil {
			failed++
			slog.Warn("activity enrichments backfill: activity failed",
				"activity_id", candidate.ActivityID,
				"user_id", candidate.UserID,
				"error", iterErr,
			)
			if len(failedIDs) < backfillMaxFailedIDs {
				failedIDs = append(failedIDs, candidate.ActivityID)
			}
		} else {
			processed++
		}

		// Progress always advances — every iteration counts, success or failure.
		if opts.OwnedJobID != "" {
			_ = db.UpdateBackgroundJobProgress(ctx, opts.OwnedJobID, int64(i+1))
		}

		// Early abort if failure rate exceeds threshold (avoids burning through
		// the whole candidate set when enrichment is systematically broken).
		if total > 10 && i >= 10 {
			if float64(failed)/float64(i+1) > backfillFailureRateAbort {
				slog.Error("activity enrichments backfill: failure rate too high, aborting",
					"processed", processed,
					"failed", failed,
					"total", total,
					"rate", float64(failed)/float64(i+1),
				)
				return writeBackfillResult(ctx, db, opts.OwnedJobID, processed, failed, total, failedIDs, true)
			}
		}

		if (i+1)%backfillBatchSize == 0 {
			slog.Info("activity enrichments backfill progress",
				"done", i+1,
				"total", total,
				"processed", processed,
				"failed", failed,
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

	slog.Info("activity enrichments backfill complete",
		"total", total,
		"processed", processed,
		"failed", failed,
	)
	return writeBackfillResult(ctx, db, opts.OwnedJobID, processed, failed, total, failedIDs, false)
}

// enrichOneCandidate runs the full enrichment pipeline for a single candidate.
// It fetches the user's settings and loads the stream, then calls EnrichTrackAnalysis.
// A per-activity deadline prevents one slow activity from blocking the backfill.
func enrichOneCandidate(
	ctx context.Context,
	db *database.DB,
	graphReader routing.GraphReader,
	candidate activityEnrichmentBackfillCandidate,
) error {
	actCtx, cancel := context.WithTimeout(ctx, backfillPerActivityTimeout)
	defer cancel()

	settings, err := db.GetEnrichmentSettings(actCtx, candidate.UserID)
	if err != nil {
		return fmt.Errorf("fetch settings: %w", err)
	}
	if settings == nil || (!settings.EnableSurfaceDetection && !settings.EnableDifficultyScales) {
		// User has disabled all relevant enrichment — skip silently, not an error.
		return nil
	}

	stream, err := enrichment.GetEnrichmentStream(actCtx, db, candidate.ActivityID)
	if err != nil {
		return fmt.Errorf("load stream: %w", err)
	}

	if err := enrichment.EnrichTrackAnalysis(actCtx, db, candidate.ActivityID, settings, graphReader, stream); err != nil {
		return fmt.Errorf("enrich track analysis: %w", err)
	}
	return nil
}

// writeBackfillResult stores failure metadata into the job record and returns
// a structured error when the backfill failed catastrophically (aborted).
// When aborted=false the caller always completes the job successfully (the
// handler's outer CompleteBackgroundJob call handles final status).
func writeBackfillResult(
	ctx context.Context,
	db *database.DB,
	jobID string,
	processed, failed, total int,
	failedIDs []string,
	aborted bool,
) error {
	if jobID == "" {
		if aborted {
			return fmt.Errorf("backfill aborted: %d/%d activities failed", failed, total)
		}
		return nil
	}

	// Store partial-failure metadata in the job record so operators can inspect
	// which activities failed without grepping logs.
	type backfillMeta struct {
		Processed int      `json:"processed"`
		Failed    int      `json:"failed"`
		Total     int      `json:"total"`
		FailedIDs []string `json:"failed_ids,omitempty"`
		Aborted   bool     `json:"aborted,omitempty"`
	}
	meta := backfillMeta{
		Processed: processed,
		Failed:    failed,
		Total:     total,
		Aborted:   aborted,
	}
	if len(failedIDs) > 0 {
		meta.FailedIDs = failedIDs
	}
	metaJSON, _ := json.Marshal(meta)
	_, _ = db.Pool.Exec(ctx,
		`UPDATE background_jobs SET metadata = $1 WHERE id = $2`,
		string(metaJSON), jobID,
	)

	if aborted {
		return fmt.Errorf("backfill aborted: %d/%d activities failed (rate >%.0f%%)",
			failed, total, backfillFailureRateAbort*100)
	}
	return nil
}

func listActivityEnrichmentBackfillCandidates(
	ctx context.Context,
	db *database.DB,
	force bool,
) ([]activityEnrichmentBackfillCandidate, error) {
	// Candidate predicate respects each user's enable_difficulty_scales setting:
	// an activity is only considered "missing enrichment" for difficulty-scale
	// categories when the user has that feature enabled. This prevents incorrectly
	// treating activities as needing backfill just because sac_scale/mtb_scale/vf_scale
	// rows are absent for users who never enabled difficulty scales.
	rows, err := db.Pool.Query(ctx, `
		SELECT a.id, a.user_id
		FROM activities a
		LEFT JOIN user_enrichment_settings ues ON ues.user_id = a.user_id
		WHERE a.has_gps = true
		  AND (
			$1
			OR NOT EXISTS (
				SELECT 1
				FROM activity_enrichments e
				WHERE e.activity_id = a.id
				  AND e.category IN (
					'forest_pct', 'protected_pct',
					'surface', 'highway'
				  )
			)
			OR (
				COALESCE(ues.enable_difficulty_scales, true)
				AND NOT EXISTS (
					SELECT 1
					FROM activity_enrichments e
					WHERE e.activity_id = a.id
					  AND e.category IN ('sac_scale', 'mtb_scale', 'vf_scale', 'vf_unknown')
				)
			)
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
