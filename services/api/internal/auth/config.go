package auth

import (
	"encoding/hex"
	"log"
	"log/slog"
	"time"

	"github.com/alexedwards/argon2id"
	"github.com/skadia-app/skadia/services/api/internal/config"
)

const (
	// RefreshCookieName is the name of the HttpOnly refresh-token cookie.
	RefreshCookieName = "skadia_refresh"

	// RefreshTokenExpiry is the lifetime of a refresh token.
	RefreshTokenExpiry = 7 * 24 * time.Hour

	// RefreshCookieMaxAge is the cookie max-age in seconds.
	RefreshCookieMaxAge = 7 * 24 * 3600 // seconds
)

// Argon2Params are the project-wide argon2id parameters.
// Fixed parallelism ensures hash portability across machines.
// Memory 64 MiB, 1 iteration, 1 parallelism (RFC 9106 recommended for max-memory approach).
var Argon2Params = argon2id.Params{
	Memory:      64 * 1024, // 64 MiB
	Iterations:  1,
	Parallelism: 1,
	SaltLength:  16,
	KeyLength:   32,
}

// dummyHash is used for timing equalization on login (non-existent users).
var dummyHash string

// GetDummyHash returns the pre-computed argon2id hash for timing equalization.
func GetDummyHash() string {
	return dummyHash
}

// ValidateConfig validates auth-related configuration at startup.
// Calls log.Fatal on critical misconfigurations.
func ValidateConfig(cfg *config.Config) {
	// JWT Secret — hex-only, 64 hex chars = 32 Bytes = 256 Bit
	if !cfg.DevAuth {
		decoded, err := hex.DecodeString(cfg.JWTSecret)
		if err != nil || len(decoded) < 32 {
			log.Fatal("JWT_SECRET must be 64 hex chars (32 bytes). Generate: openssl rand -hex 32")
		}
	}

	// OAuth encryption key — required when social login is configured
	if (cfg.StravaClientID != "" || cfg.GoogleClientID != "") && cfg.OAuthEncryptionKey == "" {
		log.Fatal("OAUTH_ENCRYPTION_KEY required when social login is enabled. Generate: openssl rand -hex 32")
	}

	// Validate registration mode
	switch cfg.RegistrationMode {
	case "invite", "open", "approval":
		// valid
	default:
		log.Fatalf("REGISTRATION_MODE must be invite|open|approval, got: %s", cfg.RegistrationMode)
	}

	// SMTP warning (not fatal — works without for invite mode)
	if cfg.SMTPHost == "" {
		slog.Warn("SMTP not configured — email features disabled")
		if cfg.RegistrationMode == "open" {
			log.Fatal("REGISTRATION_MODE=open requires SMTP to be configured (email verification)")
		}
	}

	// Dummy hash for login timing equalization — MUST succeed
	var err error
	dummyHash, err = argon2id.CreateHash("skadia-timing-dummy", &Argon2Params)
	if err != nil {
		log.Fatal("Failed to create dummy argon2id hash for timing equalization: ", err)
	}
}
