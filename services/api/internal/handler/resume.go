package handler

import (
	"context"
	"log/slog"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/database"
)

// resumableJobTypes defines which job types can be automatically resumed after
// a container restart. Only fully idempotent jobs belong here — the resume
// logic deletes prior results and re-runs from scratch.
var resumableJobTypes = map[string]bool{
	"segment_recompute": true,
}

// maxResumableJobAge is the maximum age of a job that can be resumed.
// Jobs older than this are considered truly stuck and will be marked as failed.
const maxResumableJobAge = 4 * time.Hour

// ResumeInterruptedJobs inspects all background_jobs with status='running'
// (leftovers from the previous server process) and either resumes or cancels them.
// Must be called after the Handler is fully initialised.
func (h *Handler) ResumeInterruptedJobs(ctx context.Context) {
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

		switch job.JobType {
		case "segment_recompute":
			h.resumeSegmentRecompute(job)
		default:
			_ = h.DB.FailBackgroundJob(ctx, job.ID)
		}
	}
}

// resumeSegmentRecompute restarts a segment recompute job from scratch.
// The matching is fully idempotent: existing efforts are deleted first.
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

	go func() {
		ctx := context.Background()
		failed := false

		// List activity IDs and reset progress.
		ids, err := h.DB.ListActivityIDsForSegmentBackfill(ctx, job.UserID, segmentID)
		if err != nil {
			slog.Error("resume: list activities failed", "job_id", job.ID, "error", err)
			_ = h.DB.FailBackgroundJob(ctx, job.ID)
			return
		}

		if err := h.DB.ResetBackgroundJobProgress(ctx, job.ID, len(ids)); err != nil {
			slog.Warn("resume: reset progress failed", "job_id", job.ID, "error", err)
		}

		// Delete existing efforts (idempotent restart).
		if err := h.DB.DeleteSegmentEffortsBySegment(ctx, segmentID, job.UserID); err != nil {
			slog.Error("resume: delete efforts failed", "job_id", job.ID, "error", err)
			failed = true
		}

		if !failed {
			for i, actID := range ids {
				act, err := h.DB.GetActivity(ctx, actID)
				if err != nil || act == nil {
					continue
				}
				subType := ""
				if act.SubType != nil {
					subType = *act.SubType
				}
				MatchActivitySegments(ctx, h.DB, actID, job.UserID, act.SportType, subType, act.GearID)

				if i%10 == 0 || i == len(ids)-1 {
					if err := h.DB.UpdateBackgroundJobProgress(ctx, job.ID, i+1); err != nil {
						slog.Warn("resume: update progress failed", "job_id", job.ID, "error", err)
					}
				}
			}
			slog.Info("resume: segment recompute done", "job_id", job.ID, "segment_id", segmentID)
		}

		if err := h.DB.CompleteBackgroundJob(ctx, job.ID, failed); err != nil {
			slog.Warn("resume: complete job failed", "job_id", job.ID, "error", err)
		}
	}()
}
