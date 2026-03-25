package database

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// BackgroundJob repräsentiert eine laufende oder abgeschlossene Hintergrundberechnung.
type BackgroundJob struct {
	ID              string     `json:"id"`
	UserID          string     `json:"user_id"`
	JobType         string     `json:"job_type"`
	EntityID        *string    `json:"entity_id,omitempty"`
	EntityName      *string    `json:"entity_name,omitempty"`
	Status          string     `json:"status"`
	Phase           *string    `json:"phase,omitempty"` // "download" | "import" | "index" | "optimize" | "topology" | "enrichment" | "splitting"
	ProgressCurrent int64      `json:"progress_current"`
	ProgressTotal   int64      `json:"progress_total"`
	StartedAt       time.Time  `json:"started_at"`
	FinishedAt      *time.Time `json:"finished_at,omitempty"`
	Error           *string    `json:"error,omitempty"`
}

// InsertBackgroundJob schreibt einen neuen Job-Eintrag und gibt die generierte ID zurück.
func (db *DB) InsertBackgroundJob(ctx context.Context, userID, jobType string, entityID, entityName *string, total int64) (string, error) {
	id := uuid.New().String()
	_, err := db.Pool.Exec(ctx, `
		INSERT INTO background_jobs (id, user_id, job_type, entity_id, entity_name, status, progress_total)
		VALUES ($1, $2, $3, $4, $5, 'running', $6)
	`, id, userID, jobType, entityID, entityName, total)
	return id, err
}

// UpdateBackgroundJobPhase setzt die aktuelle Phase eines Jobs ("download", "import", "index").
func (db *DB) UpdateBackgroundJobPhase(ctx context.Context, jobID, phase string) error {
	_, err := db.Pool.Exec(ctx,
		`UPDATE background_jobs SET phase = $1 WHERE id = $2`,
		phase, jobID,
	)
	return err
}

// UpdateBackgroundJobProgress aktualisiert den Fortschritts-Zähler eines Jobs.
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

// CompleteBackgroundJob setzt status auf 'completed' oder 'failed' und trägt finished_at ein.
// Erzeugt zusätzlich eine Notification für den User (best-effort).
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
	category := "job"
	if failed {
		category = "job_error"
	}
	if _, err := db.Pool.Exec(ctx,
		"INSERT INTO notifications (user_id, category, title, body) VALUES ($1, $2, $3, '')",
		userID, category, title,
	); err != nil {
		// Best-effort: log but don't fail
		return nil
	}
	return nil
}

func jobCompletionTitle(jobType string, entityName *string, failed bool) string {
	labels := map[string]string{
		"osm_import":              "OSM-Import",
		"osm_enrichment":          "OSM-Anreicherung",
		"garmin_import":           "Garmin-Synchronisierung",
		"strava_import":           "Strava-Synchronisierung",
		"komoot_import":           "Komoot-Import",
		"segment_recompute":       "Segmente neu berechnen",
		"activity_recompute_all":  "Alle Aktivitäten neu berechnen",
		"activity_recompute":      "Aktivität neu berechnen",
		"recompute_multisport":    "Multisport neu berechnen",
		"photo_preview_recompute": "Foto-Vorschaubilder neu erstellen",
		"photo_thumb_recompute":   "Foto-Thumbnails neu erstellen",
		"photo_clip_embed":        "Foto-Indexierung",
		"photo_embed_backfill":    "Foto-Einbettungen berechnen",
		"phash_backfill":          "Foto-Duplikaterkennung berechnen",
		"elevation_reset":         "Höhenkorrektur zurücksetzen",
		"elevation_correction":    "Höhenkorrektur anwenden",
		"distance_correction":     "Distanzkorrektur anwenden",
		"distance_reset":          "Distanzkorrektur zurücksetzen",
		"dem_import":              "Höhenmodell importieren",
		"apply_rules":             "Regeln anwenden",
		"routing_graph_build":     "Routing-Graph erstellen",
		"tile_build":              "Routing-Tiles erstellen",
		"weather_backfill":        "Wetter-Backfill",
		"brouter_segment_build":   "BRouter-Segmente erstellen",
		"brouter_segment_download": "BRouter-Segmente herunterladen",
		"device_orphan_cleanup":    "Verwaiste Geräte bereinigen",
		"activity_re_enrich":       "Enrichment-Update",
		"shadowfs_reconcile":       "Dateispeicher-Konsistenzprüfung",
		"shadowfs_migration":       "Dateispeicher-Registry-Migration",
		"shadowfs_file_move":       "Dateispeicher-Dateimigration",
		"shadowfs_rollback":        "Dateispeicher-Rollback",
		"gdpr_purge":               "DSGVO-Datenlöschung",
	}
	label := labels[jobType]
	if label == "" {
		label = jobType
	}
	if entityName != nil && *entityName != "" {
		label += " — " + *entityName
	}
	if failed {
		return label + " fehlgeschlagen"
	}
	return label + " abgeschlossen"
}

// ListActiveBackgroundJobs gibt alle running Jobs (max. 2h alt) + kürzlich abgeschlossene Jobs
// (finished_at in den letzten 5 Minuten) für einen User zurück.
func (db *DB) ListActiveBackgroundJobs(ctx context.Context, userID string) ([]BackgroundJob, error) {
	rows, err := db.Pool.Query(ctx, `
		SELECT id, user_id, job_type, entity_id, entity_name, status, phase,
		       progress_current, progress_total, started_at, finished_at, error
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
		); err != nil {
			return nil, err
		}
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
		       progress_current, progress_total, started_at, finished_at
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
			&j.ProgressCurrent, &j.ProgressTotal, &j.StartedAt, &j.FinishedAt,
		); err != nil {
			return nil, err
		}
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
