package database

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
)

// SetActivityKomootTourID sets the komoot_tour_id on an existing activity.
func (db *DB) SetActivityKomootTourID(ctx context.Context, activityID string, tourID int64) error {
	_, err := db.Pool.Exec(ctx,
		`UPDATE activities SET komoot_tour_id = COALESCE(komoot_tour_id, $2) WHERE id = $1`,
		activityID, tourID,
	)
	return err
}

// ActivityExistsByKomootTourID returns true if the user already has an activity with this Komoot tour ID.
func (db *DB) ActivityExistsByKomootTourID(ctx context.Context, userID string, tourID int64) (bool, error) {
	var exists bool
	err := db.Pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM activities WHERE user_id = $1 AND komoot_tour_id = $2)`,
		userID, tourID,
	).Scan(&exists)
	return exists, err
}

// GetActivityIDByKomootTourID returns the activity ID for a given Komoot tour ID.
func (db *DB) GetActivityIDByKomootTourID(ctx context.Context, userID string, tourID int64) (string, error) {
	var id string
	err := db.Pool.QueryRow(ctx,
		`SELECT id FROM activities WHERE user_id = $1 AND komoot_tour_id = $2`,
		userID, tourID,
	).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	return id, err
}

// KomootCredentials holds the stored Komoot credentials for a user.
type KomootCredentials struct {
	Email             string
	EncryptedPassword string
	KomootUserID      string
	SyncEnabled       bool
	ImportPlanned     bool
	ImportRecorded    bool
	ImportPhotos      bool
}

// SaveKomootCredentials upserts Komoot credentials for a user.
func (db *DB) SaveKomootCredentials(ctx context.Context, userID string, creds KomootCredentials) error {
	_, err := db.Pool.Exec(ctx, `
		INSERT INTO komoot_credentials (
			user_id, email, encrypted_password, komoot_user_id,
			sync_enabled, import_planned, import_recorded, import_photos,
			updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, NOW())
		ON CONFLICT (user_id) DO UPDATE SET
			email = EXCLUDED.email,
			encrypted_password = EXCLUDED.encrypted_password,
			komoot_user_id = EXCLUDED.komoot_user_id,
			sync_enabled = EXCLUDED.sync_enabled,
			import_planned = EXCLUDED.import_planned,
			import_recorded = EXCLUDED.import_recorded,
			import_photos = EXCLUDED.import_photos
	`, userID, creds.Email, creds.EncryptedPassword, creds.KomootUserID,
		creds.SyncEnabled, creds.ImportPlanned, creds.ImportRecorded, creds.ImportPhotos,
	)
	return err
}

// GetKomootCredentials loads Komoot credentials for a user.
// Returns nil if no credentials are stored.
func (db *DB) GetKomootCredentials(ctx context.Context, userID string) (*KomootCredentials, error) {
	var c KomootCredentials
	err := db.Pool.QueryRow(ctx, `
		SELECT email, encrypted_password, COALESCE(komoot_user_id, ''),
		       sync_enabled, import_planned, import_recorded, import_photos
		FROM komoot_credentials WHERE user_id = $1
	`, userID).Scan(
		&c.Email, &c.EncryptedPassword, &c.KomootUserID,
		&c.SyncEnabled, &c.ImportPlanned, &c.ImportRecorded, &c.ImportPhotos,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return &c, err
}

// UpdateKomootLastSync sets the last_sync_at timestamp and last-synced komoot_user_id.
func (db *DB) UpdateKomootLastSync(ctx context.Context, userID, komootUserID string) error {
	_, err := db.Pool.Exec(ctx,
		`UPDATE komoot_credentials SET last_sync_at = NOW(), komoot_user_id = $2 WHERE user_id = $1`,
		userID, komootUserID,
	)
	return err
}

// HasActiveKomootImportJob returns true if the user already has a pending or running komoot_import job.
func (db *DB) HasActiveKomootImportJob(ctx context.Context, userID string) (bool, error) {
	var exists bool
	err := db.Pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM background_jobs WHERE user_id = $1 AND job_type = 'komoot_import' AND status IN ('pending', 'running'))`,
		userID).Scan(&exists)
	return exists, err
}

// GetKomootImportJob fetches a single background job by ID, scoped to the user.
func (db *DB) GetKomootImportJob(ctx context.Context, userID, jobID string) (*BackgroundJob, error) {
	var j BackgroundJob
	err := db.Pool.QueryRow(ctx, `
		SELECT id, user_id, job_type, entity_id, entity_name, status, phase,
		       progress_current, progress_total, started_at, finished_at
		FROM background_jobs
		WHERE id = $1 AND user_id = $2
	`, jobID, userID).Scan(
		&j.ID, &j.UserID, &j.JobType, &j.EntityID, &j.EntityName,
		&j.Status, &j.Phase,
		&j.ProgressCurrent, &j.ProgressTotal, &j.StartedAt, &j.FinishedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, pgx.ErrNoRows
	}
	return &j, err
}

// DeleteKomootCredentials removes the stored Komoot credentials for a user.
func (db *DB) DeleteKomootCredentials(ctx context.Context, userID string) error {
	_, err := db.Pool.Exec(ctx, `DELETE FROM komoot_credentials WHERE user_id = $1`, userID)
	return err
}
