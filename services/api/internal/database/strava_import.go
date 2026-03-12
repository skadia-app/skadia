package database

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// --- Strava Import Jobs ---

// CreateStravaImportJob creates a new import job row and returns the job UUID.
func (db *DB) CreateStravaImportJob(ctx context.Context, userID, sourcePath string) (string, error) {
	var id string
	err := db.Pool.QueryRow(ctx,
		`INSERT INTO strava_import_jobs (user_id, source_path, status, started_at)
		 VALUES ($1, $2, 'running', now())
		 RETURNING id`,
		userID, sourcePath,
	).Scan(&id)
	if err != nil {
		return "", fmt.Errorf("create strava import job: %w", err)
	}
	return id, nil
}

// GetStravaImportJob returns a job row as a generic map (for JSON serialisation).
func (db *DB) GetStravaImportJob(ctx context.Context, userID, jobID string) (map[string]interface{}, error) {
	row := db.Pool.QueryRow(ctx,
		`SELECT id, user_id, status, source_path, total_modules, completed_modules,
		        current_module, module_results, errors, started_at, completed_at, created_at
		 FROM strava_import_jobs
		 WHERE id = $1 AND user_id = $2`,
		jobID, userID,
	)

	var (
		id               string
		uid              string
		status           string
		sourcePath       string
		totalModules     int
		completedModules int
		currentModule    *string
		moduleResults    []byte
		errors           []byte
		startedAt        *time.Time
		completedAt      *time.Time
		createdAt        time.Time
	)

	err := row.Scan(
		&id, &uid, &status, &sourcePath, &totalModules, &completedModules,
		&currentModule, &moduleResults, &errors, &startedAt, &completedAt, &createdAt,
	)
	if err == pgx.ErrNoRows {
		return nil, fmt.Errorf("job not found")
	}
	if err != nil {
		return nil, fmt.Errorf("get strava import job: %w", err)
	}

	var parsedErrors []string
	if len(moduleResults) > 0 {
		// moduleResults not yet used for display
		_ = moduleResults
	}
	if len(errors) > 0 {
		_ = json.Unmarshal(errors, &parsedErrors)
	}

	result := map[string]interface{}{
		"id":                id,
		"user_id":           uid,
		"status":            status,
		"source_path":       sourcePath,
		"total_modules":     totalModules,
		"completed_modules": completedModules,
		"current_module":    currentModule,
		"errors":            parsedErrors,
		"started_at":        startedAt,
		"completed_at":      completedAt,
		"created_at":        createdAt,
	}
	return result, nil
}

// UpdateStravaImportJob updates progress on a running job.
func (db *DB) UpdateStravaImportJob(ctx context.Context, jobID string, completed, total int, currentModule string) error {
	_, err := db.Pool.Exec(ctx,
		`UPDATE strava_import_jobs
		 SET completed_modules = $2, total_modules = $3, current_module = $4
		 WHERE id = $1`,
		jobID, completed, total, currentModule,
	)
	if err != nil {
		return fmt.Errorf("update strava import job: %w", err)
	}
	return nil
}

// UpdateStravaImportCurrentModule updates only the current_module log string (per-item progress).
func (db *DB) UpdateStravaImportCurrentModule(ctx context.Context, jobID, currentModule string) error {
	_, err := db.Pool.Exec(ctx,
		`UPDATE strava_import_jobs SET current_module = $2 WHERE id = $1`,
		jobID, currentModule,
	)
	return err
}

// UpdateStravaImportModuleProgress updates module-level completed/total counts.
func (db *DB) UpdateStravaImportModuleProgress(ctx context.Context, jobID string, completed, total int) error {
	_, err := db.Pool.Exec(ctx,
		`UPDATE strava_import_jobs SET completed_modules = $2, total_modules = $3 WHERE id = $1`,
		jobID, completed, total,
	)
	return err
}

// CompleteStravaImportJob marks a job as completed, optionally recording non-fatal errors.
func (db *DB) CompleteStravaImportJob(ctx context.Context, jobID string, errs []string) error {
	var errJSON []byte
	if len(errs) > 0 {
		errJSON, _ = json.Marshal(errs)
	}
	_, err := db.Pool.Exec(ctx,
		`UPDATE strava_import_jobs
		 SET status = 'completed', completed_at = now(), errors = $2
		 WHERE id = $1`,
		jobID, errJSON,
	)
	if err != nil {
		return fmt.Errorf("complete strava import job: %w", err)
	}
	return nil
}

// ListActiveStravaImportJobs gibt laufende (und kürzlich abgeschlossene) Strava-Import-Jobs
// im BackgroundJob-Format zurück, damit sie in der Statusleiste erscheinen.
func (db *DB) ListActiveStravaImportJobs(ctx context.Context, userID string) ([]BackgroundJob, error) {
	rows, err := db.Pool.Query(ctx,
		`SELECT id, status, total_modules, completed_modules, started_at, completed_at
		 FROM strava_import_jobs
		 WHERE user_id = $1
		   AND (
		         (status = 'running' AND started_at > NOW() - INTERVAL '2 hours')
		         OR (status IN ('completed', 'failed')
		             AND completed_at > NOW() - INTERVAL '5 minutes')
		       )
		 ORDER BY started_at DESC`,
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var jobs []BackgroundJob
	for rows.Next() {
		var (
			id           string
			status       string
			totalMod     int
			completedMod int
			startedAt    time.Time
			finishedAt   *time.Time
		)
		if err := rows.Scan(&id, &status, &totalMod, &completedMod, &startedAt, &finishedAt); err != nil {
			return nil, err
		}
		entityName := "Strava-Import"
		jobs = append(jobs, BackgroundJob{
			ID:              id,
			UserID:          userID,
			JobType:         "strava_import",
			EntityName:      &entityName,
			Status:          status,
			ProgressCurrent: int64(completedMod),
			ProgressTotal:   int64(totalMod),
			StartedAt:       startedAt,
			FinishedAt:      finishedAt,
		})
	}
	if jobs == nil {
		jobs = []BackgroundJob{}
	}
	return jobs, rows.Err()
}

// FindActivityIDAtTime returns the ID of the activity that was running at the given time.
// Returns an empty string if no matching activity is found.
func (db *DB) FindActivityIDAtTime(ctx context.Context, userID string, ts time.Time) (string, error) {
	var id string
	err := db.Pool.QueryRow(ctx,
		`SELECT id FROM activities
		 WHERE user_id = $1
		   AND deleted_at IS NULL
		   AND started_at <= $2 + INTERVAL '30 minutes'
		   AND (ended_at IS NULL OR ended_at >= $2 - INTERVAL '30 minutes')
		 ORDER BY ABS(EXTRACT(EPOCH FROM (started_at - $2)))
		 LIMIT 1`,
		userID, ts,
	).Scan(&id)
	if err == pgx.ErrNoRows {
		return "", nil
	}
	return id, err
}

// FailStravaImportJob marks a job as failed.
func (db *DB) FailStravaImportJob(ctx context.Context, jobID, errMsg string) error {
	_, err := db.Pool.Exec(ctx,
		`UPDATE strava_import_jobs
		 SET status = 'failed', completed_at = now(),
		     errors = jsonb_build_array($2::text)
		 WHERE id = $1`,
		jobID, errMsg,
	)
	if err != nil {
		return fmt.Errorf("fail strava import job: %w", err)
	}
	return nil
}

// --- Strava helper methods ---

// GearExistsByNameForUser checks whether a gear item with the given name already exists for this user.
// Returns (exists, gearID, error).
func (db *DB) GearExistsByNameForUser(ctx context.Context, userID, name string) (bool, string, error) {
	var id string
	err := db.Pool.QueryRow(ctx,
		`SELECT id FROM gear WHERE user_id = $1 AND name = $2 LIMIT 1`,
		userID, name,
	).Scan(&id)
	if err == pgx.ErrNoRows {
		return false, "", nil
	}
	if err != nil {
		return false, "", err
	}
	return true, id, nil
}

// PrivacyZoneExistsByName returns true if a privacy zone with the given name already exists for this user.
func (db *DB) PrivacyZoneExistsByName(ctx context.Context, userID, name string) (bool, error) {
	var exists bool
	err := db.Pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM user_privacy_zones WHERE user_id = $1 AND name = $2)`,
		userID, name,
	).Scan(&exists)
	if err != nil {
		return false, err
	}
	return exists, nil
}

// SetActivityStravaID sets the strava_activity_id on an existing activity.
func (db *DB) SetActivityStravaID(ctx context.Context, activityID string, stravaID int64) error {
	_, err := db.Pool.Exec(ctx,
		`UPDATE activities SET strava_activity_id = COALESCE(strava_activity_id, $2) WHERE id = $1`,
		activityID, stravaID,
	)
	return err
}

// ActivityExistsByStravaID returns true if the user already has an activity with this Strava ID.
func (db *DB) ActivityExistsByStravaID(ctx context.Context, userID string, stravaID int64) (bool, error) {
	var exists bool
	err := db.Pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM activities WHERE user_id = $1 AND strava_activity_id = $2)`,
		userID, stravaID,
	).Scan(&exists)
	if err != nil {
		return false, err
	}
	return exists, nil
}

// CountExistingStravaActivityIDs returns how many of the given Strava activity IDs
// are already stored for the user. It uses a single bulk query (ANY) for efficiency.
func (db *DB) CountExistingStravaActivityIDs(ctx context.Context, userID string, stravaIDs []int64) (int, error) {
	if len(stravaIDs) == 0 {
		return 0, nil
	}
	var count int
	err := db.Pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM activities
		 WHERE user_id = $1 AND strava_activity_id = ANY($2)`,
		userID, stravaIDs,
	).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count existing strava activity ids: %w", err)
	}
	return count, nil
}

// GetActivityIDByStravaID gibt die interne ID einer Aktivität anhand der Strava-ID zurück.
func (db *DB) GetActivityIDByStravaID(ctx context.Context, userID string, stravaID int64) (string, error) {
	var id string
	err := db.Pool.QueryRow(ctx,
		`SELECT id FROM activities WHERE user_id = $1 AND strava_activity_id = $2 LIMIT 1`,
		userID, stravaID,
	).Scan(&id)
	return id, err
}
