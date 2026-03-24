package auth

import (
	"encoding/hex"
	"log"
	"log/slog"

	"github.com/alexedwards/argon2id"
	"github.com/skadia-app/skadia/services/api/internal/config"
)

// DummyHash is used for timing equalization on login (non-existent users)
var DummyHash string

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
	DummyHash, err = argon2id.CreateHash("skadia-timing-dummy", argon2id.DefaultParams)
	if err != nil {
		log.Fatal("Failed to create dummy argon2id hash for timing equalization: ", err)
	}
}
