package database

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// --- Garmin Import Jobs ---

// CreateGarminImportJob creates a new import job row and returns the job UUID.
func (db *DB) CreateGarminImportJob(ctx context.Context, userID, sourcePath string) (string, error) {
	var id string
	err := db.Pool.QueryRow(ctx,
		`INSERT INTO garmin_import_jobs (user_id, source_path, status, started_at)
		 VALUES ($1, $2, 'running', now())
		 RETURNING id`,
		userID, sourcePath,
	).Scan(&id)
	if err != nil {
		return "", fmt.Errorf("create garmin import job: %w", err)
	}
	return id, nil
}

// GetGarminImportJob returns a job row as a generic map (for JSON serialisation).
func (db *DB) GetGarminImportJob(ctx context.Context, userID, jobID string) (map[string]interface{}, error) {
	row := db.Pool.QueryRow(ctx,
		`SELECT id, user_id, status, source_path, total_modules, completed_modules,
		        current_module, module_results, errors, started_at, completed_at, created_at
		 FROM garmin_import_jobs
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
		return nil, fmt.Errorf("get garmin import job: %w", err)
	}

	result := map[string]interface{}{
		"id":                id,
		"user_id":           uid,
		"status":            status,
		"source_path":       sourcePath,
		"total_modules":     totalModules,
		"completed_modules": completedModules,
		"current_module":    currentModule,
		"started_at":        startedAt,
		"completed_at":      completedAt,
		"created_at":        createdAt,
	}
	return result, nil
}

// UpdateGarminImportJob updates progress on a running job.
func (db *DB) UpdateGarminImportJob(ctx context.Context, jobID string, completed, total int, currentModule string) error {
	_, err := db.Pool.Exec(ctx,
		`UPDATE garmin_import_jobs
		 SET completed_modules = $2, total_modules = $3, current_module = $4
		 WHERE id = $1`,
		jobID, completed, total, currentModule,
	)
	if err != nil {
		return fmt.Errorf("update garmin import job: %w", err)
	}
	return nil
}

// CompleteGarminImportJob marks a job as completed.
func (db *DB) CompleteGarminImportJob(ctx context.Context, jobID string) error {
	_, err := db.Pool.Exec(ctx,
		`UPDATE garmin_import_jobs
		 SET status = 'completed', completed_at = now()
		 WHERE id = $1`,
		jobID,
	)
	if err != nil {
		return fmt.Errorf("complete garmin import job: %w", err)
	}
	return nil
}

// FailGarminImportJob marks a job as failed.
func (db *DB) FailGarminImportJob(ctx context.Context, jobID, errMsg string) error {
	_, err := db.Pool.Exec(ctx,
		`UPDATE garmin_import_jobs
		 SET status = 'failed', completed_at = now(),
		     errors = jsonb_build_array($2::text)
		 WHERE id = $1`,
		jobID, errMsg,
	)
	if err != nil {
		return fmt.Errorf("fail garmin import job: %w", err)
	}
	return nil
}

// --- Garmin IDs on activities / gear ---

// SetActivityGarminID sets the garmin_activity_id on an existing activity.
func (db *DB) SetActivityGarminID(ctx context.Context, activityID string, garminID int64) error {
	_, err := db.Pool.Exec(ctx,
		`UPDATE activities SET garmin_activity_id = $2 WHERE id = $1`,
		activityID, garminID,
	)
	return err
}

// ActivityExistsByGarminID returns true if the user already has an activity with this Garmin ID.
func (db *DB) ActivityExistsByGarminID(ctx context.Context, userID string, garminID int64) (bool, error) {
	var exists bool
	err := db.Pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM activities WHERE user_id = $1 AND garmin_activity_id = $2)`,
		userID, garminID,
	).Scan(&exists)
	if err != nil {
		return false, err
	}
	return exists, nil
}

// GetActivityIDByGarminID gibt die interne ID einer Aktivität anhand der Garmin-ID zurück.
func (db *DB) GetActivityIDByGarminID(ctx context.Context, userID string, garminID int64) (string, error) {
	var id string
	err := db.Pool.QueryRow(ctx,
		`SELECT id FROM activities WHERE user_id = $1 AND garmin_activity_id = $2 LIMIT 1`,
		userID, garminID,
	).Scan(&id)
	return id, err
}

// SetGearGarminID sets the garmin_gear_id on an existing gear item.
func (db *DB) SetGearGarminID(ctx context.Context, gearID, garminGearID string) error {
	_, err := db.Pool.Exec(ctx,
		`UPDATE gear SET garmin_gear_id = $2 WHERE id = $1`,
		gearID, garminGearID,
	)
	return err
}

// GearExistsByGarminID returns true if the user already has a gear item with this Garmin ID.
func (db *DB) GearExistsByGarminID(ctx context.Context, userID, garminGearID string) (bool, error) {
	var exists bool
	err := db.Pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM gear WHERE user_id = $1 AND garmin_gear_id = $2)`,
		userID, garminGearID,
	).Scan(&exists)
	if err != nil {
		return false, err
	}
	return exists, nil
}

// SetUserBirthDateIfNull sets birth_date on users only when it is not already set.
func (db *DB) SetUserBirthDateIfNull(ctx context.Context, userID, isoDate string) error {
	_, err := db.Pool.Exec(ctx,
		`UPDATE users SET birth_date = $2::date WHERE id = $1 AND birth_date IS NULL`,
		userID, isoDate,
	)
	return err
}
