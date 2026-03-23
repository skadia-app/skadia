package database

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/skadia-app/skadia/services/api/internal/model"
)

// ListActiveUserIDs returns the IDs of all active users.
func (db *DB) ListActiveUserIDs(ctx context.Context) ([]string, error) {
	rows, err := db.Pool.Query(ctx, `SELECT id FROM users WHERE is_active = true`)
	if err != nil {
		return nil, fmt.Errorf("list active user ids: %w", err)
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan user id: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list active user ids: %w", err)
	}
	return ids, nil
}

// GetOrCreateUserByOIDCSubject finds a user by their OIDC subject ID,
// creating a new user row if none exists yet.
func (db *DB) GetOrCreateUserByOIDCSubject(ctx context.Context, legacyOIDCSubject, username, email string) (*model.User, error) {
	var u model.User

	// Attempt to find existing user first.
	err := db.Pool.QueryRow(ctx,
		`SELECT id, username, display_name, email, legacy_oidc_subject, is_admin, is_active, created_at, updated_at
		 FROM users WHERE legacy_oidc_subject = $1`,
		legacyOIDCSubject,
	).Scan(&u.ID, &u.Username, &u.DisplayName, &u.Email, &u.LegacyOIDCSubject,
		&u.IsAdmin, &u.IsActive, &u.CreatedAt, &u.UpdatedAt)

	if err == nil {
		return &u, nil
	}
	if err != pgx.ErrNoRows {
		return nil, fmt.Errorf("get user by legacy_oidc_subject: %w", err)
	}

	// User does not exist yet – insert.
	// ON CONFLICT (username): link the existing user to this OIDC ID
	// (covers dev users pre-created without legacy_oidc_subject).
	err = db.Pool.QueryRow(ctx,
		`INSERT INTO users (username, email, legacy_oidc_subject)
		 VALUES ($1, $2, $3)
		 ON CONFLICT (username) DO UPDATE
		   SET legacy_oidc_subject = EXCLUDED.legacy_oidc_subject,
		       email        = COALESCE(EXCLUDED.email, users.email)
		 RETURNING id, username, display_name, email, legacy_oidc_subject, is_admin, is_active, created_at, updated_at`,
		username, nilIfEmpty(email), legacyOIDCSubject,
	).Scan(&u.ID, &u.Username, &u.DisplayName, &u.Email, &u.LegacyOIDCSubject,
		&u.IsAdmin, &u.IsActive, &u.CreatedAt, &u.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("create user: %w", err)
	}

	return &u, nil
}

// GetUserProfile returns the physiological profile of a user.
func (db *DB) GetUserProfile(ctx context.Context, userID string) (*model.UserProfile, error) {
	var p model.UserProfile
	var birthDate *time.Time
	err := db.Pool.QueryRow(ctx,
		`SELECT birth_date, COALESCE(gender,''), height_cm, weight_kg, ftp_w, lthr_bpm,
		        max_hr_bpm, max_hr_detected_bpm, resting_hr_bpm
		 FROM users WHERE id = $1`,
		userID,
	).Scan(&birthDate, &p.Gender, &p.HeightCm, &p.WeightKg, &p.FtpW, &p.LthrBpm,
		&p.MaxHrBpm, &p.MaxHrDetectedBpm, &p.RestingHrBpm)
	if err != nil {
		return nil, fmt.Errorf("get user profile: %w", err)
	}
	if birthDate != nil {
		d := model.DateOnly{Time: *birthDate}
		p.BirthDate = &d
		// Tanaka formula: 208 - 0.7 × age (computed in Go, not stored in DB)
		age := time.Now().Year() - birthDate.Year()
		formula := int(208.0 - 0.7*float64(age))
		p.MaxHrFormulaBpm = &formula
	}
	p.EffectiveMaxHr = p.EffectiveMaxHR()
	return &p, nil
}

// UpdateUserProfile updates writable physiological profile fields.
func (db *DB) UpdateUserProfile(ctx context.Context, userID string, u *model.UserProfileUpdate) (*model.UserProfile, error) {
	_, err := db.Pool.Exec(ctx,
		`UPDATE users SET
		    birth_date     = COALESCE($2, birth_date),
		    gender         = COALESCE($3, gender),
		    height_cm      = COALESCE($4, height_cm),
		    weight_kg      = COALESCE($5, weight_kg),
		    ftp_w          = COALESCE($6, ftp_w),
		    lthr_bpm       = COALESCE($7, lthr_bpm),
		    max_hr_bpm     = COALESCE($8, max_hr_bpm),
		    resting_hr_bpm = COALESCE($9, resting_hr_bpm)
		 WHERE id = $1`,
		userID, birthDateForDB(u.BirthDate), u.Gender, u.HeightCm, u.WeightKg,
		u.FtpW, u.LthrBpm, u.MaxHrBpm, u.RestingHrBpm,
	)
	if err != nil {
		return nil, fmt.Errorf("update user profile: %w", err)
	}
	return db.GetUserProfile(ctx, userID)
}

// SetUserProfileFieldsIfNull sets profile fields from FIT import only if currently NULL.
func (db *DB) SetUserProfileFieldsIfNull(ctx context.Context, userID string, imp *model.UserProfileImport) error {
	if imp == nil {
		return nil
	}
	if imp.Gender != "" {
		if _, err := db.Pool.Exec(ctx, `UPDATE users SET gender = $2 WHERE id = $1 AND (gender IS NULL OR gender = '')`, userID, imp.Gender); err != nil {
			slog.Warn("SetUserProfileFieldsIfNull: failed to set gender", "err", err)
		}
	}
	if imp.AgeYears > 0 {
		year := time.Now().Year() - imp.AgeYears
		birthDate := time.Date(year, 1, 1, 0, 0, 0, 0, time.UTC)
		if _, err := db.Pool.Exec(ctx, `UPDATE users SET birth_date = $2 WHERE id = $1 AND birth_date IS NULL`, userID, birthDate); err != nil {
			slog.Warn("SetUserProfileFieldsIfNull: failed to set birth_date", "err", err)
		}
	}
	if imp.HeightCm > 0 {
		if _, err := db.Pool.Exec(ctx, `UPDATE users SET height_cm = $2 WHERE id = $1 AND height_cm IS NULL`, userID, imp.HeightCm); err != nil {
			slog.Warn("SetUserProfileFieldsIfNull: failed to set height_cm", "err", err)
		}
	}
	if imp.WeightKg > 0 {
		if _, err := db.Pool.Exec(ctx, `UPDATE users SET weight_kg = $2 WHERE id = $1 AND weight_kg IS NULL`, userID, imp.WeightKg); err != nil {
			slog.Warn("SetUserProfileFieldsIfNull: failed to set weight_kg", "err", err)
		}
	}
	if imp.RestingHR > 0 {
		if _, err := db.Pool.Exec(ctx, `UPDATE users SET resting_hr_bpm = $2 WHERE id = $1 AND resting_hr_bpm IS NULL`, userID, imp.RestingHR); err != nil {
			slog.Warn("SetUserProfileFieldsIfNull: failed to set resting_hr_bpm", "err", err)
		}
	}
	if imp.MaxHRDetected > 0 {
		if _, err := db.Pool.Exec(ctx, `UPDATE users SET max_hr_detected_bpm = $2 WHERE id = $1 AND (max_hr_detected_bpm IS NULL OR max_hr_detected_bpm < $2)`, userID, imp.MaxHRDetected); err != nil {
			slog.Warn("SetUserProfileFieldsIfNull: failed to set max_hr_detected_bpm", "err", err)
		}
	}
	if imp.LTHR > 0 {
		if _, err := db.Pool.Exec(ctx, `UPDATE users SET lthr_bpm = $2 WHERE id = $1 AND lthr_bpm IS NULL`, userID, imp.LTHR); err != nil {
			slog.Warn("SetUserProfileFieldsIfNull: failed to set lthr_bpm", "err", err)
		}
	}
	if imp.FTP > 0 {
		if _, err := db.Pool.Exec(ctx, `UPDATE users SET ftp_w = $2 WHERE id = $1 AND ftp_w IS NULL`, userID, imp.FTP); err != nil {
			slog.Warn("SetUserProfileFieldsIfNull: failed to set ftp_w", "err", err)
		}
	}
	if imp.MaxHR > 0 {
		if _, err := db.Pool.Exec(ctx, `UPDATE users SET max_hr_detected_bpm = $2 WHERE id = $1 AND (max_hr_detected_bpm IS NULL OR max_hr_detected_bpm < $2)`, userID, imp.MaxHR); err != nil {
			slog.Warn("SetUserProfileFieldsIfNull: failed to set max_hr_detected_bpm (MaxHR)", "err", err)
		}
	}
	return nil
}

// GetMaxHRPercentile returns the 90th percentile max heart rate from historical activity data.
func (db *DB) GetMaxHRPercentile(ctx context.Context, userID string) (int, error) {
	var maxHR int
	err := db.Pool.QueryRow(ctx,
		`SELECT COALESCE(PERCENTILE_CONT(0.9) WITHIN GROUP (ORDER BY max_heart_rate), 0)::int
		 FROM activities WHERE user_id = $1 AND max_heart_rate IS NOT NULL AND max_heart_rate > 0`,
		userID,
	).Scan(&maxHR)
	if err != nil {
		return 0, fmt.Errorf("get max hr percentile: %w", err)
	}
	return maxHR, nil
}

// birthDateForDB converts *model.DateOnly to *time.Time for pgx.
func birthDateForDB(d *model.DateOnly) *time.Time {
	if d == nil || d.IsZero() {
		return nil
	}
	t := d.Time
	return &t
}
