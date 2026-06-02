package database

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// BackgroundJob represents a running or completed background job.
type BackgroundJob struct {
	ID              string         `json:"id"`
	UserID          string         `json:"user_id"`
	JobType         string         `json:"job_type"`
	EntityID        *string        `json:"entity_id,omitempty"`
	EntityName      *string        `json:"entity_name,omitempty"`
	Status          string         `json:"status"`
	Phase           *string        `json:"phase,omitempty"` // "download" | "import" | "index" | "optimize" | "topology" | "enrichment" | "splitting"
	ProgressCurrent int64          `json:"progress_current"`
	ProgressTotal   int64          `json:"progress_total"`
	StartedAt       time.Time      `json:"started_at"`
	FinishedAt      *time.Time     `json:"finished_at,omitempty"`
	Error           *string        `json:"error,omitempty"`
	MessageCode     string         `json:"message_code,omitempty"`
	StatusCode      string         `json:"status_code,omitempty"`
	EntityKey       string         `json:"entity_key,omitempty"`
	Params          map[string]any `json:"params,omitempty"`
	Metadata        []byte         `json:"-"`
}

type backgroundJobI18nMetadata struct {
	MessageCode string         `json:"message_code"`
	StatusCode  string         `json:"status_code"`
	EntityKey   string         `json:"entity_key"`
	Params      map[string]any `json:"params"`
}

func applyBackgroundJobI18nMetadata(job *BackgroundJob) {
	if job == nil || len(job.Metadata) == 0 {
		return
	}
	var meta backgroundJobI18nMetadata
	if err := json.Unmarshal(job.Metadata, &meta); err != nil {
		return
	}
	job.MessageCode = meta.MessageCode
	job.StatusCode = meta.StatusCode
	job.EntityKey = meta.EntityKey
	job.Params = meta.Params
}

// InsertBackgroundJob writes a new job row and returns the generated ID.
func (db *DB) InsertBackgroundJob(ctx context.Context, userID, jobType string, entityID, entityName *string, total int64) (string, error) {
	id := uuid.New().String()
	return id, db.InsertBackgroundJobWithID(ctx, id, userID, jobType, entityID, entityName, total)
}

func (db *DB) InsertBackgroundJobTx(ctx context.Context, tx pgx.Tx, userID, jobType string, entityID, entityName *string, total int64) (string, error) {
	id := uuid.New().String()
	return id, db.insertBackgroundJobOn(ctx, tx, id, userID, jobType, entityID, entityName, total)
}

func (db *DB) InsertBackgroundJobWithID(ctx context.Context, id, userID, jobType string, entityID, entityName *string, total int64) error {
	return db.insertBackgroundJobOn(ctx, db.Pool, id, userID, jobType, entityID, entityName, total)
}

func (db *DB) insertBackgroundJobOn(ctx context.Context, q Querier, id, userID, jobType string, entityID, entityName *string, total int64) error {
	_, err := q.Exec(ctx, `
		INSERT INTO background_jobs (id, user_id, job_type, entity_id, entity_name, status, progress_total)
		VALUES ($1, $2, $3, $4, $5, 'running', $6)
	`, id, userID, jobType, entityID, entityName, total)
	return err
}

// InsertBackgroundJobGuarded inserts a new running job only if no job of the same
// type is already running FOR THE SAME USER. The guard is per-user (not global)
// since migration 000285 scoped idx_bg_jobs_one_running to (user_id, job_type)
// WHERE status='running'. It is race-free under READ COMMITTED because the
// partial unique index causes the second concurrent INSERT from the same user
// to fail with a unique_violation (23505); different users can still run
// same-type jobs in parallel.
// Returns ("", false, nil) when a job is already running for this user.
// Returns (jobID, true, nil) when the job was successfully inserted.
func (db *DB) InsertBackgroundJobGuarded(ctx context.Context, userID, jobType string, entityID, entityName *string, total int64) (string, bool, error) {
	id := uuid.New().String()
	_, err := db.Pool.Exec(ctx, `
		INSERT INTO background_jobs (id, user_id, job_type, entity_id, entity_name, status, progress_total)
		VALUES ($1, $2, $3, $4, $5, 'running', $6)
	`, id, userID, jobType, entityID, entityName, total)
	if err != nil {
		// Partial unique index idx_bg_jobs_one_running fires when a running job
		// of the same type already exists — treat this as already_running, not an error.
		if pgErr, ok := errors.AsType[*pgconn.PgError](err); ok && pgErr.Code == "23505" {
			return "", false, nil
		}
		return "", false, err
	}
	return id, true, nil
}

// UpdateBackgroundJobPhase sets the current phase of a job.
func (db *DB) UpdateBackgroundJobPhase(ctx context.Context, jobID, phase string) error {
	_, err := db.Pool.Exec(ctx,
		`UPDATE background_jobs SET phase = $1 WHERE id = $2`,
		phase, jobID,
	)
	return err
}

// UpdateBackgroundJobProgress updates the progress counter for a job.
func (db *DB) UpdateBackgroundJobProgress(ctx context.Context, jobID string, current int64) error {
	_, err := db.Pool.Exec(ctx,
		`UPDATE background_jobs SET progress_current = $1 WHERE id = $2`,
		current, jobID,
	)
	return err
}

// UpdateBackgroundJobTotal updates the progress_total for a running job.
func (db *DB) UpdateBackgroundJobTotal(ctx context.Context, jobID string, total int64) error {
	_, err := db.Pool.Exec(ctx,
		`UPDATE background_jobs SET progress_total = $1 WHERE id = $2`,
		total, jobID,
	)
	return err
}

func (db *DB) UpdateBackgroundJobMetadata(ctx context.Context, jobID string, metadata []byte) error {
	_, err := db.Pool.Exec(ctx,
		`UPDATE background_jobs SET metadata = $1, updated_at = NOW() WHERE id = $2`,
		metadata, jobID,
	)
	return err
}

// CompleteBackgroundJob sets status to 'completed' or 'failed' and records finished_at.
// It also creates a best-effort notification for the user.
func (db *DB) CompleteBackgroundJob(ctx context.Context, jobID string, failed bool, errorText string) error {
	status := "completed"
	if failed {
		status = "failed"
	}
	_, err := db.Pool.Exec(ctx,
		`UPDATE background_jobs SET status = $1, finished_at = NOW(), error = NULLIF($3, '') WHERE id = $2`,
		status, jobID, errorText,
	)
	if err != nil {
		return err
	}

	// Create completion notification
	var userID, jobType string
	var entityName *string
	if err := db.Pool.QueryRow(ctx,
		`SELECT user_id, job_type, entity_name FROM background_jobs WHERE id = $1`, jobID,
	).Scan(&userID, &jobType, &entityName); err != nil {
		return nil // Job completion itself succeeded, notification is best-effort
	}

	title := jobCompletionTitle(jobType, entityName, failed)
	payload, err := jobCompletionNotificationData(jobType, entityName, failed)
	if err != nil {
		return nil
	}
	category := "job"
	if failed {
		category = "job_error"
	}
	if _, err := db.Pool.Exec(ctx,
		"INSERT INTO notifications (user_id, category, title, body, data) VALUES ($1, $2, $3, '', $4)",
		userID, category, title, payload,
	); err != nil {
		// Best-effort: log but don't fail
		return nil
	}
	return nil
}

// jobCompletionTitle returns the legacy stored title for clients that do not
// render structured notification data. Job-specific labels are resolved from
// the job_type carried in jobCompletionNotificationData.
func jobCompletionTitle(_ string, entityName *string, failed bool) string {
	label := "Hintergrundjob"
	if entityName != nil && *entityName != "" {
		label += " - " + *entityName
	}
	if failed {
		return label + " fehlgeschlagen"
	}
	return label + " abgeschlossen"
}

func jobCompletionNotificationData(jobType string, entityName *string, failed bool) ([]byte, error) {
	key := "notifications.jobs.completed"
	if failed {
		key = "notifications.jobs.failed"
	}
	params := map[string]any{"jobType": jobType}
	if entityName != nil && *entityName != "" {
		params["entityName"] = *entityName
	}
	return json.Marshal(map[string]any{
		"key":    key,
		"params": params,
	})
}

// ListActiveBackgroundJobs returns running jobs plus recently completed jobs for one user.
func (db *DB) ListActiveBackgroundJobs(ctx context.Context, userID string) ([]BackgroundJob, error) {
	rows, err := db.Pool.Query(ctx, `
		SELECT id, user_id, job_type, entity_id, entity_name, status, phase,
		       progress_current, progress_total, started_at, finished_at, error,
		       COALESCE(metadata, '{}'::jsonb)
		FROM background_jobs
		WHERE user_id = $1
		  AND (
		        status = 'running'
		        OR (status IN ('completed', 'failed')
		            AND finished_at > NOW() - INTERVAL '5 minutes')
		      )
		ORDER BY started_at DESC
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var jobs []BackgroundJob
	for rows.Next() {
		var j BackgroundJob
		if err := rows.Scan(
			&j.ID, &j.UserID, &j.JobType, &j.EntityID, &j.EntityName,
			&j.Status, &j.Phase,
			&j.ProgressCurrent, &j.ProgressTotal, &j.StartedAt, &j.FinishedAt, &j.Error,
			&j.Metadata,
		); err != nil {
			return nil, err
		}
		applyBackgroundJobI18nMetadata(&j)
		jobs = append(jobs, j)
	}
	if jobs == nil {
		jobs = []BackgroundJob{}
	}
	return jobs, rows.Err()
}

// ListStaleRunningJobs returns all jobs with status='running' — these are leftovers
// from a previous server process and need to be either resumed or cancelled.
func (db *DB) ListStaleRunningJobs(ctx context.Context) ([]BackgroundJob, error) {
	rows, err := db.Pool.Query(ctx, `
			SELECT id, user_id, job_type, entity_id, entity_name, status, phase,
			       progress_current, progress_total, started_at, finished_at, COALESCE(metadata, '{}'::jsonb)
		FROM background_jobs
		WHERE status = 'running'
		ORDER BY started_at ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var jobs []BackgroundJob
	for rows.Next() {
		var j BackgroundJob
		if err := rows.Scan(
			&j.ID, &j.UserID, &j.JobType, &j.EntityID, &j.EntityName,
			&j.Status, &j.Phase,
			&j.ProgressCurrent, &j.ProgressTotal, &j.StartedAt, &j.FinishedAt, &j.Metadata,
		); err != nil {
			return nil, err
		}
		applyBackgroundJobI18nMetadata(&j)
		jobs = append(jobs, j)
	}
	return jobs, rows.Err()
}

// FailBackgroundJob marks a single job as failed.
func (db *DB) FailBackgroundJob(ctx context.Context, jobID string) error {
	_, err := db.Pool.Exec(ctx,
		`UPDATE background_jobs SET status = 'failed', finished_at = NOW() WHERE id = $1`,
		jobID,
	)
	return err
}

// ResetBackgroundJobProgress resets progress_current to 0 for a job being resumed.
func (db *DB) ResetBackgroundJobProgress(ctx context.Context, jobID string, newTotal int64) error {
	_, err := db.Pool.Exec(ctx,
		`UPDATE background_jobs SET progress_current = 0, progress_total = $2 WHERE id = $1`,
		jobID, newTotal,
	)
	return err
}

// CancelRunningJobsForEntity setzt alle laufenden Jobs für eine entity auf 'failed'.
// Wird aufgerufen bevor ein neuer Job für dieselbe Entity gestartet wird.
func (db *DB) CancelRunningJobsForEntity(ctx context.Context, userID, entityID string) error {
	_, err := db.Pool.Exec(ctx,
		`UPDATE background_jobs SET status = 'failed', finished_at = NOW()
		 WHERE user_id = $1 AND entity_id = $2 AND status = 'running'`,
		userID, entityID,
	)
	return err
}

// HasRunningJob checks whether a job of the given type is currently running for the user.
func (db *DB) HasRunningJob(ctx context.Context, userID, jobType string) (bool, error) {
	var exists bool
	err := db.Pool.QueryRow(ctx,
		"SELECT EXISTS(SELECT 1 FROM background_jobs WHERE user_id = $1 AND job_type = $2 AND status = 'running')",
		userID, jobType).Scan(&exists)
	return exists, err
}

// HasRunningJobGlobal checks if any job of the given type is running (not user-scoped).
func (db *DB) HasRunningJobGlobal(ctx context.Context, jobType string) (bool, error) {
	var exists bool
	err := db.Pool.QueryRow(ctx,
		"SELECT EXISTS(SELECT 1 FROM background_jobs WHERE job_type = $1 AND status = 'running')",
		jobType).Scan(&exists)
	return exists, err
}

// CountPendingEnrichments zählt Aktivitäten mit enrichment_status='pending' für einen User.
func (db *DB) CountPendingEnrichments(ctx context.Context, userID string) (int, error) {
	var count int
	err := db.Pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM activities WHERE user_id = $1 AND enrichment_status = 'pending'`,
		userID,
	).Scan(&count)
	return count, err
}
