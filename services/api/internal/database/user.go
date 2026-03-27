package database

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/skadia-app/skadia/services/api/internal/crypto"
	"github.com/skadia-app/skadia/services/api/internal/model"
)

// ListActiveUserIDs returns the IDs of all active users.
func (db *DB) ListActiveUserIDs(ctx context.Context) ([]string, error) {
	rows, err := db.Pool.Query(ctx, `SELECT id FROM users WHERE is_active = true AND deleted_at IS NULL`)
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

// ErrEmailCollision is returned when a social login email matches an existing user.
var ErrEmailCollision = fmt.Errorf("email already registered")

// allUserColumns is the shared column list for scanning a full model.User.
const allUserColumns = `id, username, display_name, email, is_admin, is_active,
	created_at, updated_at, password_hash, email_verified, registration_source,
	password_changed_at, last_login_at, failed_login_count, login_delay_until,
	deleted_at, must_change_password, pending_email, COALESCE(timezone, '')`

// scanUser scans a row into a model.User using the allUserColumns column order.
func scanUser(row pgx.Row) (*model.User, error) {
	var u model.User
	err := row.Scan(
		&u.ID, &u.Username, &u.DisplayName, &u.Email,
		&u.IsAdmin, &u.IsActive, &u.CreatedAt, &u.UpdatedAt,
		&u.PasswordHash, &u.EmailVerified, &u.RegistrationSrc,
		&u.PasswordChangedAt, &u.LastLoginAt, &u.FailedLoginCount, &u.LoginDelayUntil,
		&u.DeletedAt, &u.MustChangePassword, &u.PendingEmail, &u.Timezone,
	)
	return &u, err
}

// GetUserByID returns a user by their ID.
func (db *DB) GetUserByID(ctx context.Context, userID string) (*model.User, error) {
	u, err := scanUser(db.Pool.QueryRow(ctx,
		`SELECT `+allUserColumns+` FROM users WHERE id = $1 AND deleted_at IS NULL`, userID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, pgx.ErrNoRows
		}
		return nil, fmt.Errorf("get user by id: %w", err)
	}
	return u, nil
}

// GetUserByEmail returns a user by their email address.
func (db *DB) GetUserByEmail(ctx context.Context, email string) (*model.User, error) {
	u, err := scanUser(db.Pool.QueryRow(ctx,
		`SELECT `+allUserColumns+` FROM users WHERE email = $1 AND deleted_at IS NULL`, email))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, pgx.ErrNoRows
		}
		return nil, fmt.Errorf("get user by email: %w", err)
	}
	return u, nil
}

// CreateUserWithPassword creates a new user with email/password credentials.
func (db *DB) CreateUserWithPassword(ctx context.Context, email, username, passwordHash, registrationSrc string, ipAddress net.IP, isActive bool) (*model.User, error) {
	u, err := scanUser(db.Pool.QueryRow(ctx,
		`INSERT INTO users (username, email, password_hash, registration_source, email_verified, is_active, created_by_ip)
		 VALUES ($1, $2, $3, $4, false, $5, $6)
		 RETURNING `+allUserColumns,
		username, email, passwordHash, registrationSrc, isActive, ipAddress))
	if err != nil {
		return nil, fmt.Errorf("create user with password: %w", err)
	}
	return u, nil
}

// GetOrCreateUserByOAuth looks up or creates a user via OAuth provider.
// Returns (user, isNewUser, error). Returns ErrEmailCollision if the provider
// email already belongs to an existing user.
func (db *DB) GetOrCreateUserByOAuth(ctx context.Context, provider, providerID, providerEmail, username string, accessToken, refreshToken []byte, tokenExpiry *time.Time, oauthEncKey string) (*model.User, bool, error) {
	// 1. Check oauth_connections for existing link.
	var userID string
	err := db.Pool.QueryRow(ctx,
		`SELECT user_id FROM oauth_connections WHERE provider = $1 AND provider_id = $2`,
		provider, providerID,
	).Scan(&userID)
	if err == nil {
		// Found existing connection — load user, but only if not soft-deleted.
		u, err := scanUser(db.Pool.QueryRow(ctx,
			`SELECT `+allUserColumns+` FROM users WHERE id = $1 AND deleted_at IS NULL`, userID))
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				// User was soft-deleted — remove stale oauth_connection so we
				// can create a fresh user below.
				_, _ = db.Pool.Exec(ctx,
					`DELETE FROM oauth_connections WHERE provider = $1 AND provider_id = $2`,
					provider, providerID)
			} else {
				return nil, false, fmt.Errorf("get user by oauth: %w", err)
			}
		} else {
			return u, false, nil
		}
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return nil, false, fmt.Errorf("lookup oauth connection: %w", err)
	}

	// 2. Create new user + oauth_connection in a transaction.
	// Email collision check is INSIDE the tx to prevent TOCTOU races.
	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		return nil, false, fmt.Errorf("get or create user by oauth: begin tx: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	// Email may be NULL for providers like Strava that don't always provide it.
	var emailPtr *string
	if providerEmail != "" {
		emailPtr = &providerEmail

		// Check email collision inside tx to prevent TOCTOU race.
		// Ignore soft-deleted users so the email can be re-used.
		var exists bool
		err = tx.QueryRow(ctx,
			`SELECT EXISTS(SELECT 1 FROM users WHERE email = $1 AND deleted_at IS NULL)`,
			providerEmail,
		).Scan(&exists)
		if err != nil {
			return nil, false, fmt.Errorf("check email collision: %w", err)
		}
		if exists {
			return nil, false, ErrEmailCollision
		}
	}

	// 3. Create the user.
	newID := uuid.New().String()
	u, err := scanUser(tx.QueryRow(ctx,
		`INSERT INTO users (id, username, email, email_verified, registration_source)
		 VALUES ($1, $2, $3, true, $4)
		 RETURNING `+allUserColumns,
		newID, username, emailPtr, provider))
	if err != nil {
		return nil, false, fmt.Errorf("create oauth user: %w", err)
	}

	if err := db.CreateOAuthConnection(ctx, tx, u.ID, provider, providerID, providerEmail, "", accessToken, refreshToken, tokenExpiry, oauthEncKey); err != nil {
		return nil, false, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, false, fmt.Errorf("get or create user by oauth: commit: %w", err)
	}
	return u, true, nil
}

// CreateOAuthConnection stores an OAuth provider connection for a user.
// Tokens are encrypted with AES-256-GCM before storage.
func (db *DB) CreateOAuthConnection(ctx context.Context, tx pgx.Tx, userID, provider, providerID, providerEmail, scopes string, accessToken, refreshToken []byte, tokenExpiry *time.Time, oauthEncKey string) error {
	var encAccess, encRefresh []byte

	if len(accessToken) > 0 {
		enc, err := crypto.Encrypt(string(accessToken), oauthEncKey)
		if err != nil {
			return fmt.Errorf("encrypt access token: %w", err)
		}
		encAccess = []byte(enc)
	}
	if len(refreshToken) > 0 {
		enc, err := crypto.Encrypt(string(refreshToken), oauthEncKey)
		if err != nil {
			return fmt.Errorf("encrypt refresh token: %w", err)
		}
		encRefresh = []byte(enc)
	}

	var emailPtr *string
	if providerEmail != "" {
		emailPtr = &providerEmail
	}

	_, err := tx.Exec(ctx,
		`INSERT INTO oauth_connections (user_id, provider, provider_id, provider_email, scopes, access_token, refresh_token, token_expiry)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		userID, provider, providerID, emailPtr, scopes, encAccess, encRefresh, tokenExpiry,
	)
	if err != nil {
		return fmt.Errorf("create oauth connection: %w", err)
	}
	return nil
}

// UpdateLoginSuccess records a successful login for the user.
func (db *DB) UpdateLoginSuccess(ctx context.Context, userID string) error {
	_, err := db.Pool.Exec(ctx,
		`UPDATE users SET last_login_at = NOW(), failed_login_count = 0, login_delay_until = NULL, updated_at = NOW()
		 WHERE id = $1`,
		userID,
	)
	if err != nil {
		return fmt.Errorf("update login success: %w", err)
	}
	return nil
}

// UpdateLoginFailure increments the failed login counter and sets a progressive delay.
// Progressive delay: 2^(n-3) seconds after 3 failures, capped at 300s (5 min).
// With per-email rate limit (5/15min), an attacker can attempt ~5 passwords per 20 minutes.
// For passwords >= 10 chars this makes brute force impractical.
func (db *DB) UpdateLoginFailure(ctx context.Context, userID string) error {
	_, err := db.Pool.Exec(ctx,
		`UPDATE users SET
			failed_login_count = failed_login_count + 1,
			login_delay_until = CASE
				WHEN failed_login_count + 1 > 3
				THEN NOW() + (LEAST(POWER(2, failed_login_count + 1 - 3), 300) || ' seconds')::interval
				ELSE NULL
			END,
			updated_at = NOW()
		 WHERE id = $1`,
		userID,
	)
	if err != nil {
		return fmt.Errorf("update login failure: %w", err)
	}
	return nil
}

// SetEmailVerified marks a user's email as verified.
func (db *DB) SetEmailVerified(ctx context.Context, userID string) error {
	_, err := db.Pool.Exec(ctx,
		`UPDATE users SET email_verified = true, updated_at = NOW() WHERE id = $1`,
		userID,
	)
	if err != nil {
		return fmt.Errorf("set email verified: %w", err)
	}
	return nil
}

// UpdatePassword sets a new password hash for the user.
func (db *DB) UpdatePassword(ctx context.Context, userID, passwordHash string) error {
	_, err := db.Pool.Exec(ctx,
		`UPDATE users SET password_hash = $2, password_changed_at = NOW(), updated_at = NOW() WHERE id = $1`,
		userID, passwordHash,
	)
	if err != nil {
		return fmt.Errorf("update password: %w", err)
	}
	return nil
}

// CheckEmailCollision returns true if a user with the given email already exists.
func (db *DB) CheckEmailCollision(ctx context.Context, email string) (bool, error) {
	var exists bool
	err := db.Pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM users WHERE email = $1 AND deleted_at IS NULL)`,
		email,
	).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("check email collision: %w", err)
	}
	return exists, nil
}

// ClearLoginDelays resets failed login count and delay for a user.
func (db *DB) ClearLoginDelays(ctx context.Context, userID string) error {
	_, err := db.Pool.Exec(ctx,
		"UPDATE users SET failed_login_count = 0, login_delay_until = NULL, updated_at = NOW() WHERE id = $1", userID)
	return err
}

// SetUserActive sets the is_active flag for a user.
func (db *DB) SetUserActive(ctx context.Context, userID string, active bool) error {
	_, err := db.Pool.Exec(ctx,
		"UPDATE users SET is_active = $2, updated_at = NOW() WHERE id = $1", userID, active)
	return err
}

// ClearMustChangePassword resets the must_change_password flag after a successful password change.
func (db *DB) ClearMustChangePassword(ctx context.Context, userID string) error {
	_, err := db.Pool.Exec(ctx,
		`UPDATE users SET must_change_password = false WHERE id = $1`, userID)
	return err
}

// ClearPendingEmail removes a pending email change request.
func (db *DB) ClearPendingEmail(ctx context.Context, userID string) error {
	_, err := db.Pool.Exec(ctx,
		`UPDATE users SET pending_email = NULL WHERE id = $1`, userID)
	return err
}

// CreateUserWithoutPassword creates a new user without a password (for admin invite flow).
// The user must set a password via the reset-password token flow.
func (db *DB) CreateUserWithoutPassword(ctx context.Context, email, username, registrationSrc string, ipAddress net.IP, isActive bool) (*model.User, error) {
	u, err := scanUser(db.Pool.QueryRow(ctx,
		`INSERT INTO users (username, email, registration_source, created_by_ip,
			is_active, email_verified, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, false, NOW(), NOW())
		 RETURNING `+allUserColumns,
		username, email, registrationSrc, ipAddress, isActive))
	if err != nil {
		return nil, fmt.Errorf("create user without password: %w", err)
	}
	return u, nil
}

// CheckUsernameCollision returns true if a user with the given username already
// exists, excluding the user identified by excludeUserID.
func (db *DB) CheckUsernameCollision(ctx context.Context, username, excludeUserID string) (bool, error) {
	var exists bool
	err := db.Pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM users WHERE username = $1 AND id != $2 AND deleted_at IS NULL)`,
		username, excludeUserID).Scan(&exists)
	return exists, err
}

// UpdateAccountInfo updates the username and/or display_name for a user.
// Nil pointers leave the corresponding field unchanged (COALESCE semantics).
func (db *DB) UpdateAccountInfo(ctx context.Context, userID string, username, displayName *string) error {
	_, err := db.Pool.Exec(ctx,
		`UPDATE users SET
			username     = COALESCE($1, username),
			display_name = COALESCE($2, display_name),
			updated_at   = NOW()
		 WHERE id = $3 AND deleted_at IS NULL`,
		username, displayName, userID)
	return err
}
