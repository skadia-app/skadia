package database

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// --- Refresh Tokens ---

// CreateRefreshToken stores a new refresh token.
func (db *DB) CreateRefreshToken(ctx context.Context, userID, familyID, tokenHash, userAgent string, ipAddress net.IP, expiresAt time.Time) error {
	_, err := db.Pool.Exec(ctx,
		`INSERT INTO refresh_tokens (user_id, family_id, token_hash, user_agent, ip_address, expires_at)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		userID, familyID, tokenHash, userAgent, ipAddress, expiresAt,
	)
	if err != nil {
		return fmt.Errorf("create refresh token: %w", err)
	}
	return nil
}

// RotateRefreshToken atomically revokes an old token and returns the associated user/family.
// Returns (userID, familyID, createdAt, error).
// If pgx.ErrNoRows: token was already revoked (reuse) or doesn't exist.
func (db *DB) RotateRefreshToken(ctx context.Context, tokenHash string) (uuid.UUID, string, time.Time, error) {
	var (
		userID    uuid.UUID
		familyID  string
		createdAt time.Time
	)
	err := db.Pool.QueryRow(ctx,
		`UPDATE refresh_tokens SET revoked = true
		 WHERE token_hash = $1 AND revoked = false AND expires_at > NOW()
		 RETURNING user_id, family_id, created_at`,
		tokenHash,
	).Scan(&userID, &familyID, &createdAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return uuid.Nil, "", time.Time{}, pgx.ErrNoRows
		}
		return uuid.Nil, "", time.Time{}, fmt.Errorf("rotate refresh token: %w", err)
	}
	return userID, familyID, createdAt, nil
}

// RevokeTokenFamily revokes ALL tokens in a family (reuse detection).
func (db *DB) RevokeTokenFamily(ctx context.Context, familyID string) error {
	_, err := db.Pool.Exec(ctx,
		`UPDATE refresh_tokens SET revoked = true WHERE family_id = $1`,
		familyID,
	)
	if err != nil {
		return fmt.Errorf("revoke token family: %w", err)
	}
	return nil
}

// RevokeAllUserTokens revokes all refresh tokens for a user.
func (db *DB) RevokeAllUserTokens(ctx context.Context, userID string) error {
	_, err := db.Pool.Exec(ctx,
		"UPDATE refresh_tokens SET revoked = true WHERE user_id = $1 AND NOT revoked", userID)
	return err
}

// GetTokenFamily returns the family_id for a given token hash.
func (db *DB) GetTokenFamily(ctx context.Context, tokenHash string) (string, error) {
	var familyID string
	err := db.Pool.QueryRow(ctx,
		"SELECT family_id FROM refresh_tokens WHERE token_hash = $1", tokenHash).Scan(&familyID)
	return familyID, err
}

// IsRevokedToken checks if a specific token hash exists and is revoked (for reuse detection).
func (db *DB) IsRevokedToken(ctx context.Context, tokenHash string) (bool, error) {
	var exists bool
	err := db.Pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM refresh_tokens WHERE token_hash = $1 AND revoked = true)`,
		tokenHash,
	).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("check revoked token: %w", err)
	}
	return exists, nil
}

// --- Email Tokens ---

// CreateEmailToken stores an email verification or password reset token.
// Deletes any existing token for the same user+type first (anti-email-bombing).
func (db *DB) CreateEmailToken(ctx context.Context, userID uuid.UUID, tokenHash, tokenType string, expiresAt time.Time) error {
	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("create email token: begin tx: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	_, err = tx.Exec(ctx,
		`DELETE FROM email_tokens WHERE user_id = $1 AND type = $2`,
		userID, tokenType,
	)
	if err != nil {
		return fmt.Errorf("create email token: delete existing: %w", err)
	}

	_, err = tx.Exec(ctx,
		`INSERT INTO email_tokens (user_id, token_hash, type, expires_at) VALUES ($1, $2, $3, $4)`,
		userID, tokenHash, tokenType, expiresAt,
	)
	if err != nil {
		return fmt.Errorf("create email token: insert: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("create email token: commit: %w", err)
	}
	return nil
}

// ConsumeEmailToken validates and consumes an email token. Returns userID if valid.
// Also cleans up expired tokens.
func (db *DB) ConsumeEmailToken(ctx context.Context, tokenHash, tokenType string) (uuid.UUID, error) {
	var userID uuid.UUID
	err := db.Pool.QueryRow(ctx,
		`DELETE FROM email_tokens WHERE token_hash = $1 AND type = $2 AND expires_at > NOW()
		 RETURNING user_id`,
		tokenHash, tokenType,
	).Scan(&userID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return uuid.Nil, pgx.ErrNoRows
		}
		return uuid.Nil, fmt.Errorf("consume email token: %w", err)
	}

	// Best-effort cleanup of expired tokens.
	_, _ = db.Pool.Exec(ctx, `DELETE FROM email_tokens WHERE expires_at < NOW()`)

	return userID, nil
}

// --- Auth Codes ---

// CreateAuthCode stores a short-lived auth code for the auth-code exchange pattern.
func (db *DB) CreateAuthCode(ctx context.Context, userID uuid.UUID, codeHash, codeChallenge, redirectURI string, expiresAt time.Time) error {
	_, err := db.Pool.Exec(ctx,
		`INSERT INTO auth_codes (user_id, code_hash, code_challenge, redirect_uri, expires_at)
		 VALUES ($1, $2, $3, $4, $5)`,
		userID, codeHash, codeChallenge, redirectURI, expiresAt,
	)
	if err != nil {
		return fmt.Errorf("create auth code: %w", err)
	}
	return nil
}

// ConsumeAuthCode validates and consumes an auth code. Returns userID, codeChallenge, redirectURI.
func (db *DB) ConsumeAuthCode(ctx context.Context, codeHash string) (uuid.UUID, string, string, error) {
	var (
		userID        uuid.UUID
		codeChallenge string
		redirectURI   string
	)
	err := db.Pool.QueryRow(ctx,
		`DELETE FROM auth_codes WHERE code_hash = $1 AND expires_at > NOW()
		 RETURNING user_id, code_challenge, redirect_uri`,
		codeHash,
	).Scan(&userID, &codeChallenge, &redirectURI)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return uuid.Nil, "", "", pgx.ErrNoRows
		}
		return uuid.Nil, "", "", fmt.Errorf("consume auth code: %w", err)
	}
	return userID, codeChallenge, redirectURI, nil
}

// --- Audit Log ---

// LogAuthEvent records an auth event in the audit log.
func (db *DB) LogAuthEvent(ctx context.Context, userID, eventType, ipAddress, userAgent string, details map[string]string) {
	var uid *string
	if userID != "" {
		uid = &userID
	}
	var ip *string
	if ipAddress != "" {
		ip = &ipAddress
	}
	var detailsJSON []byte
	if len(details) > 0 {
		detailsJSON, _ = json.Marshal(details)
	}
	_, _ = db.Pool.Exec(ctx,
		`INSERT INTO auth_audit_log (user_id, event_type, ip_address, user_agent, details)
		 VALUES ($1, $2, $3::inet, $4, $5::jsonb)`,
		uid, eventType, ip, userAgent, detailsJSON)
}

// --- Cleanup ---

// CompleteBackgroundJobByType marks all non-completed jobs of a given type for a user as completed.
func (db *DB) CompleteBackgroundJobByType(ctx context.Context, jobType, userID string) {
	db.Pool.Exec(ctx,
		`UPDATE background_jobs SET status = 'completed', completed_at = NOW()
         WHERE job_type = $1 AND user_id = $2 AND status != 'completed'`,
		jobType, userID)
}

// CleanupExpiredTokens removes old tokens per retention policy.
// CleanupExpiredTokens runs daily. Auth codes (30s TTL) and email tokens (1-24h TTL)
// are functionally dead after expiry (checked at consumption time) but accumulate
// in the DB until this cleanup runs. For a low-traffic single-user app this is acceptable.
func (db *DB) CleanupExpiredTokens(ctx context.Context) error {
	// Remove revoked refresh tokens older than 30 days.
	_, err := db.Pool.Exec(ctx,
		`DELETE FROM refresh_tokens WHERE revoked = true AND created_at < NOW() - INTERVAL '30 days'`)
	if err != nil {
		return fmt.Errorf("cleanup revoked refresh tokens: %w", err)
	}

	// Remove expired refresh tokens older than 7 days past expiry.
	_, err = db.Pool.Exec(ctx,
		`DELETE FROM refresh_tokens WHERE expires_at < NOW() - INTERVAL '7 days'`)
	if err != nil {
		return fmt.Errorf("cleanup expired refresh tokens: %w", err)
	}

	// Scrub IP addresses from refresh tokens older than 90 days.
	_, err = db.Pool.Exec(ctx,
		`UPDATE refresh_tokens SET ip_address = NULL WHERE created_at < NOW() - INTERVAL '90 days' AND ip_address IS NOT NULL`)
	if err != nil {
		return fmt.Errorf("scrub old ip addresses: %w", err)
	}

	// Remove expired email tokens.
	_, err = db.Pool.Exec(ctx,
		`DELETE FROM email_tokens WHERE expires_at < NOW()`)
	if err != nil {
		return fmt.Errorf("cleanup expired email tokens: %w", err)
	}

	// Remove expired auth codes.
	_, err = db.Pool.Exec(ctx,
		`DELETE FROM auth_codes WHERE expires_at < NOW()`)
	if err != nil {
		return fmt.Errorf("cleanup expired auth codes: %w", err)
	}

	// GDPR: scrub registration IP after 90 days.
	_, err = db.Pool.Exec(ctx,
		`UPDATE users SET created_by_ip = NULL WHERE created_at < NOW() - INTERVAL '90 days' AND created_by_ip IS NOT NULL`)
	if err != nil {
		return fmt.Errorf("scrub user registration IPs: %w", err)
	}

	// GDPR: IP-Adressen nach 90 Tagen scrubben, Events nach 365 Tagen löschen.
	_, err = db.Pool.Exec(ctx,
		`UPDATE auth_audit_log SET ip_address = NULL
		 WHERE created_at < NOW() - INTERVAL '90 days' AND ip_address IS NOT NULL`)
	if err != nil {
		return fmt.Errorf("scrub audit log IPs: %w", err)
	}
	_, err = db.Pool.Exec(ctx,
		"DELETE FROM auth_audit_log WHERE created_at < NOW() - INTERVAL '365 days'")
	if err != nil {
		return fmt.Errorf("purge old audit log: %w", err)
	}

	return nil
}
