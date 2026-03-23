package database

import (
	"context"
	"fmt"
	"time"
)

// listActiveImportJobs queries active import jobs from the given table and returns
// them as BackgroundJob structs. Uses a switch to select the query per table name
// rather than string interpolation, preventing SQL injection.
func (db *DB) listActiveImportJobs(ctx context.Context, userID, tableName, entityName, jobType string) ([]BackgroundJob, error) {
	var query string
	switch tableName {
	case "garmin_import_jobs":
		query = `SELECT id, status, total_modules, completed_modules, started_at, completed_at
		 FROM garmin_import_jobs
		 WHERE user_id = $1
		   AND (
		         (status = 'running' AND started_at > NOW() - INTERVAL '2 hours')
		         OR (status IN ('completed', 'failed')
		             AND completed_at > NOW() - INTERVAL '5 minutes')
		       )
		 ORDER BY started_at DESC`
	case "strava_import_jobs":
		query = `SELECT id, status, total_modules, completed_modules, started_at, completed_at
		 FROM strava_import_jobs
		 WHERE user_id = $1
		   AND (
		         (status = 'running' AND started_at > NOW() - INTERVAL '2 hours')
		         OR (status IN ('completed', 'failed')
		             AND completed_at > NOW() - INTERVAL '5 minutes')
		       )
		 ORDER BY started_at DESC`
	default:
		return nil, fmt.Errorf("invalid import table: %s", tableName)
	}

	rows, err := db.Pool.Query(ctx, query, userID)
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
		en := entityName
		jobs = append(jobs, BackgroundJob{
			ID:              id,
			UserID:          userID,
			JobType:         jobType,
			EntityName:      &en,
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
