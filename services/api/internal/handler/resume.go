package handler

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"sync/atomic"
	"time"

	"github.com/google/uuid"

	"github.com/skadia-app/skadia/services/api/internal/bestefforts"
	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/shutdown"
)

// resumableJobTypes defines which job types can be automatically resumed after
// a container restart. Only fully idempotent jobs belong here — the resume
// logic deletes prior results and re-runs from scratch.
var resumableJobTypes = map[string]bool{
	"segment_recompute":       true,
	"segment_publish":         true,
	"recalc_best_efforts":     true,
	"recalc_best_efforts_all": true,
}

var segmentRecalculationWorkerRunning atomic.Bool

// maxResumableJobAge is the maximum age of a job that can be resumed.
// Jobs older than this are considered truly stuck and will be marked as failed.
const maxResumableJobAge = 4 * time.Hour

// ResumeInterruptedJobs inspects all background_jobs with status='running'
// (leftovers from the previous server process) and either resumes or cancels them.
// Must be called after the Handler is fully initialised.
func (h *Handler) ResumeInterruptedJobs(ctx context.Context) {
	// Reset aggressive PG import settings that may persist after container crash
	for _, stmt := range []string{
		"ALTER SYSTEM RESET maintenance_work_mem",
		"ALTER SYSTEM RESET max_parallel_maintenance_workers",
		"ALTER SYSTEM RESET work_mem",
		"ALTER SYSTEM RESET max_parallel_workers_per_gather",
		"SELECT pg_reload_conf()",
	} {
		if _, err := h.DB.Pool.Exec(ctx, stmt); err != nil {
			slog.Warn("resume: PG settings reset failed (non-fatal)", "error", err)
		}
	}

	// Kill stale PostgreSQL queries from previous API instance.
	// After a container restart, orphaned connections may still hold row locks
	// from long-running enrichment UPDATEs or import DELETEs. Our new pool
	// connections are <2s old, so any query running >2min belongs to the old instance.
	var terminated int
	rows, err := h.DB.Pool.Query(ctx, `
		SELECT pg_terminate_backend(pid), pid, left(query, 80), query_start
		FROM pg_stat_activity
		WHERE usename = current_user
		  AND pid != pg_backend_pid()
		  AND (
			(state = 'active' AND query_start < now() - interval '2 minutes')
			OR (state = 'idle in transaction' AND state_change < now() - interval '2 minutes')
		  )`)
	if err != nil {
		slog.Warn("resume: stale connection query failed (non-fatal)", "error", err)
	} else {
		defer rows.Close()
		for rows.Next() {
			var ok bool
			var pid int
			var query string
			var queryStart time.Time
			if scanErr := rows.Scan(&ok, &pid, &query, &queryStart); scanErr == nil && ok {
				terminated++
				slog.Warn("resume: terminated stale connection",
					"pid", pid, "running_since", queryStart, "query", query)
			}
		}
	}
	if terminated > 0 {
		slog.Info("resume: terminated stale connections from previous instance", "count", terminated)
	}

	// Stale 'importing' regions zurücksetzen (Import war nicht abgeschlossen).
	// Must run unconditionally — even when no stale background_jobs exist,
	// osm_import_regions can be stuck on 'importing' from a crashed import.
	if res, err := h.DB.Pool.Exec(ctx,
		`UPDATE osm_import_regions SET status = 'failed' WHERE status = 'importing'`); err != nil {
		slog.Warn("resume: failed to reset stale importing regions", "error", err)
	} else if res.RowsAffected() > 0 {
		slog.Info("resume: reset stale importing regions", "count", res.RowsAffected())
	}

	h.resumeSegmentRecalculationRuns(ctx)

	jobs, err := h.DB.ListStaleRunningJobs(ctx)
	if err != nil {
		slog.Warn("resume: failed to list stale jobs", "error", err)
		return
	}
	if len(jobs) == 0 {
		return
	}

	slog.Info("resume: found stale running jobs", "count", len(jobs))

	for _, job := range jobs {
		age := time.Since(job.StartedAt)

		// Too old — definitely stuck, not just interrupted.
		if age > maxResumableJobAge {
			slog.Warn("resume: job too old, marking failed",
				"job_id", job.ID, "job_type", job.JobType, "age", age.Round(time.Second))
			_ = h.DB.FailBackgroundJob(ctx, job.ID)
			continue
		}

		if !resumableJobTypes[job.JobType] {
			slog.Info("resume: non-resumable job type, marking failed",
				"job_id", job.ID, "job_type", job.JobType)
			_ = h.DB.FailBackgroundJob(ctx, job.ID)
			continue
		}
		if !h.resumeJobAllowedByCapabilities(job.JobType) {
			slog.Warn("resume: job blocked by deployment capabilities, marking failed",
				"job_id", job.ID, "job_type", job.JobType, "deployment_profile", h.deploymentProfile())
			_ = h.DB.FailBackgroundJob(ctx, job.ID)
			continue
		}

		switch job.JobType {
		case "segment_recompute":
			h.resumeSegmentRecompute(job)
		case "segment_publish":
			h.resumeSegmentPublish(job)
		case "recalc_best_efforts":
			h.resumeBestEffortsRecalc(job)
		case "recalc_best_efforts_all":
			h.resumeAllUsersBestEffortsRecalc(job)
		default:
			_ = h.DB.FailBackgroundJob(ctx, job.ID)
		}
	}

}

func (h *Handler) resumeSegmentRecalculationRuns(ctx context.Context) {
	h.startSegmentRecalculationRuns("segment-recalculation-resume")
}

func (h *Handler) startSegmentRecalculationRuns(reason string) {
	if !segmentRecalculationWorkerRunning.CompareAndSwap(false, true) {
		slog.Debug("segment recalculation worker already running", "reason", reason)
		return
	}
	jobID := "segment-recalculation-runs-" + uuid.NewString()
	shutdown.Go(jobID, func(ctx context.Context) {
		defer segmentRecalculationWorkerRunning.Store(false)
		workerID := "api-segment-recalculation"
		for {
			claim, err := h.DB.ClaimNextSegmentRecalculationActivity(ctx, workerID, 5*time.Minute)
			if err != nil {
				slog.Warn("resume: claim segment recalculation activity failed", "error", err)
				return
			}
			if claim == nil {
				return
			}
			if err := h.recomputeSegmentEffortsForActivityInRun(ctx, *claim); err != nil {
				if errors.Is(err, database.ErrSegmentRecalculationObsolete) {
					if skipErr := h.DB.SkipObsoleteSegmentRecalculationActivity(ctx, *claim); skipErr != nil {
						slog.Warn("resume: skip obsolete segment recalculation activity failed", "run_id", claim.RunID, "error", skipErr)
					}
					continue
				}
				slog.Warn("resume: segment recalculation activity failed",
					"run_id", claim.RunID,
					"segment_id", claim.SegmentID,
					"activity_id", claim.ActivityID,
					"error_code", "activity_failed")
				if failErr := h.DB.FailSegmentRecalculationActivity(ctx, *claim, err.Error()); failErr != nil {
					slog.Warn("resume: mark segment recalculation activity failed", "run_id", claim.RunID, "error", failErr)
				}
			}
		}
	})
	slog.Debug("segment recalculation worker started", "job_id", jobID, "reason", reason)
}

func (h *Handler) deploymentProfile() string {
	if h != nil && h.Config != nil && h.Config.DeploymentProfile != "" {
		return h.Config.DeploymentProfile
	}
	return "full"
}

func (h *Handler) resumeJobAllowedByCapabilities(jobType string) bool {
	if h.deploymentProfile() != "core" {
		return true
	}
	switch jobType {
	case "segment_recompute", "segment_publish":
		return false
	default:
		return true
	}
}

func bestEffortsCheckpointFromJob(job database.BackgroundJob) allUsersBestEffortsCheckpoint {
	var checkpoint allUsersBestEffortsCheckpoint
	if len(job.Metadata) == 0 {
		return checkpoint
	}
	if err := json.Unmarshal(job.Metadata, &checkpoint); err != nil {
		slog.Warn("resume: best-efforts checkpoint metadata invalid", "job_id", job.ID, "error", err)
	}
	return checkpoint
}

func (h *Handler) resumeBestEffortsRecalc(job database.BackgroundJob) {
	if h.BestEfforts == nil {
		_ = h.DB.FailBackgroundJob(context.Background(), job.ID)
		return
	}
	checkpoint := bestEffortsCheckpointFromJob(job)
	shutdown.Go(job.ID, func(ctx context.Context) {
		err := h.BestEfforts.RecalcAllFromCheckpoint(ctx, job.UserID, checkpoint.LastActivityID, func(progress bestefforts.RecalcProgress) {
			_ = h.DB.UpdateBackgroundJobProgress(ctx, job.ID, int64(progress.Done))
			if progress.Total > 0 {
				_ = h.DB.UpdateBackgroundJobTotal(ctx, job.ID, int64(progress.Total))
			}
			updateAllUsersBestEffortsCheckpoint(ctx, h.DB, job.ID, allUsersBestEffortsCheckpoint{
				LastUserID:     job.UserID,
				LastActivityID: progress.LastProcessedActivityID,
				UserDone:       progress.Done,
				UserTotal:      progress.Total,
				TotalUsers:     1,
			})
		})
		if err != nil {
			slog.Error("resume: best-efforts recalc failed", "job_id", job.ID, "user_id", job.UserID, "error", err)
			_ = h.DB.CompleteBackgroundJob(context.Background(), job.ID, true, err.Error())
			return
		}
		_ = h.DB.CompleteBackgroundJob(context.Background(), job.ID, false, "")
	})
}

func (h *Handler) resumeAllUsersBestEffortsRecalc(job database.BackgroundJob) {
	if h.BestEfforts == nil {
		_ = h.DB.FailBackgroundJob(context.Background(), job.ID)
		return
	}
	checkpoint := bestEffortsCheckpointFromJob(job)
	shutdown.Go(job.ID, func(ctx context.Context) {
		runAllUsersBestEffortsRecalcFromCheckpoint(ctx, h.DB, h.BestEfforts, job.ID, checkpoint)
	})
}

// resumeSegmentPublish restarts a segment_publish job from scratch after an interrupted API restart.
// The segment is already public at this point; we re-delete all efforts and rematch all users' activities.
func (h *Handler) resumeSegmentPublish(job database.BackgroundJob) {
	segmentID := ""
	if job.EntityID != nil {
		segmentID = *job.EntityID
	}
	if segmentID == "" {
		slog.Warn("resume: segment_publish job has no entity_id", "job_id", job.ID)
		_ = h.DB.FailBackgroundJob(context.Background(), job.ID)
		return
	}

	slog.Info("resume: restarting segment publish recompute",
		"job_id", job.ID, "segment_id", segmentID)

	shutdown.Go("resume-segment-publish-"+segmentID, func(ctx context.Context) {
		failed := false

		ids, err := h.DB.ListAllActivityIDsForSegmentRecompute(ctx, segmentID)
		if err != nil {
			slog.Error("resume segment_publish: list activities failed", "job_id", job.ID, "error", err)
			_ = h.DB.FailBackgroundJob(ctx, job.ID)
			return
		}

		if err := h.DB.ResetBackgroundJobProgress(ctx, job.ID, int64(len(ids))); err != nil {
			slog.Warn("resume segment_publish: reset progress failed", "job_id", job.ID, "error", err)
		}

		for i, actID := range ids {
			act, err := h.DB.GetActivity(ctx, actID)
			if err != nil || act == nil {
				continue
			}
			if err := h.recomputeSegmentEffortsForActivity(ctx, segmentID, actID, act.UserID); err != nil {
				slog.Warn("resume segment_publish: activity match failed",
					"segment_id", segmentID,
					"activity_id", actID,
					"error", err)
			}
			if i%10 == 0 || i == len(ids)-1 {
				h.DB.UpdateBackgroundJobProgress(ctx, job.ID, int64(i+1))
			}
		}
		slog.Info("resume segment_publish: recompute done", "job_id", job.ID, "segment_id", segmentID)

		if err := h.DB.UpdateSegmentEffortRanks(ctx, segmentID); err != nil {
			slog.Warn("resume segment_publish: final rank update failed", "segment_id", segmentID, "error", err)
		}

		if err := h.DB.CompleteBackgroundJob(ctx, job.ID, failed, ""); err != nil {
			slog.Warn("resume segment_publish: complete job failed", "job_id", job.ID, "error", err)
		}
	})
}

// resumeSegmentRecompute restarts a segment recompute job from scratch.
// The matching is idempotent for the activity set selected for recompute.
func (h *Handler) resumeSegmentRecompute(job database.BackgroundJob) {
	segmentID := ""
	if job.EntityID != nil {
		segmentID = *job.EntityID
	}
	if segmentID == "" {
		slog.Warn("resume: segment_recompute job has no entity_id", "job_id", job.ID)
		_ = h.DB.FailBackgroundJob(context.Background(), job.ID)
		return
	}

	slog.Info("resume: restarting segment recompute",
		"job_id", job.ID, "segment_id", segmentID, "user_id", job.UserID)

	shutdown.Go("resume-segment-recompute-"+segmentID, func(ctx context.Context) {
		failed := false

		// List activity IDs and reset progress.
		ids, err := h.DB.ListActivityIDsForSegmentRecompute(ctx, job.UserID, segmentID)
		if err != nil {
			slog.Error("resume: list activities failed", "job_id", job.ID, "error", err)
			_ = h.DB.FailBackgroundJob(ctx, job.ID)
			return
		}

		if err := h.DB.ResetBackgroundJobProgress(ctx, job.ID, int64(len(ids))); err != nil {
			slog.Warn("resume: reset progress failed", "job_id", job.ID, "error", err)
		}

		for i, actID := range ids {
			if err := h.recomputeSegmentEffortsForActivity(ctx, segmentID, actID, job.UserID); err != nil {
				slog.Warn("resume: activity match failed",
					"segment_id", segmentID,
					"activity_id", actID,
					"error", err)
			}

			if i%10 == 0 || i == len(ids)-1 {
				if err := h.DB.UpdateBackgroundJobProgress(ctx, job.ID, int64(i+1)); err != nil {
					slog.Warn("resume: update progress failed", "job_id", job.ID, "error", err)
				}
			}
		}
		slog.Info("resume: segment recompute done", "job_id", job.ID, "segment_id", segmentID)

		if err := h.DB.UpdateSegmentEffortRanks(ctx, segmentID); err != nil {
			slog.Warn("resume: final rank update failed", "segment_id", segmentID, "error", err)
		}

		if err := h.DB.CompleteBackgroundJob(ctx, job.ID, failed, ""); err != nil {
			slog.Warn("resume: complete job failed", "job_id", job.ID, "error", err)
		}
	})
}
