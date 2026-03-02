package config

import (
	"fmt"
	"os"
)

type Config struct {
	DatabaseURL      string
	NatsURL          string
	GarageEndpoint   string
	GarageAccessKey  string
	GarageSecretKey  string
	GarageBucket     string
	ListenAddr       string
	DevAuth          bool
	OIDCJWKSURL string

	// OIDC OIDC
	OIDCBaseURL      string
	OIDCClientID     string
	OIDCClientSecret string
	OIDCAppSlug      string // e.g. "skadia"
	AppRedirectURI        string // e.g. "http://localhost:8080/api/v1/auth/callback"
	FrontendURL           string // e.g. "http://localhost:5173"

	// JWT (HS256) issued by this backend after OIDC login
	JWTSecret string

	// Shadow File System
	ShadowFSBasePath string
	HotfolderPath    string
	HotfolderEnabled bool
}

func Load() (*Config, error) {
	cfg := &Config{
		DatabaseURL:      os.Getenv("DATABASE_URL"),
		NatsURL:          getEnvDefault("NATS_URL", "nats://localhost:4222"),
		GarageEndpoint:   getEnvDefault("GARAGE_ENDPOINT", "http://localhost:3900"),
		GarageAccessKey:  os.Getenv("GARAGE_ACCESS_KEY"),
		GarageSecretKey:  os.Getenv("GARAGE_SECRET_KEY"),
		GarageBucket:     getEnvDefault("GARAGE_BUCKET", "skadia-originals"),
		ListenAddr:       getEnvDefault("LISTEN_ADDR", ":8080"),
		DevAuth:          os.Getenv("DEV_AUTH") == "true",
		OIDCJWKSURL: getEnvDefault("OIDC_JWKS_URL", "http://oidc:9000/application/o/skadia/jwks/"),

		OIDCBaseURL:      getEnvDefault("OIDC_BASE_URL", "http://oidc:9000"),
		OIDCClientID:     os.Getenv("OIDC_CLIENT_ID"),
		OIDCClientSecret: os.Getenv("OIDC_CLIENT_SECRET"),
		OIDCAppSlug:      getEnvDefault("OIDC_APP_SLUG", "skadia"),
		AppRedirectURI:        getEnvDefault("APP_REDIRECT_URI", "http://localhost:8080/api/v1/auth/callback"),
		FrontendURL:           getEnvDefault("FRONTEND_URL", "http://localhost:5173"),
		JWTSecret:             os.Getenv("JWT_SECRET"),

		ShadowFSBasePath: os.Getenv("SHADOW_FS_BASE_PATH"),
		HotfolderPath:    os.Getenv("HOTFOLDER_PATH"),
		HotfolderEnabled: os.Getenv("HOTFOLDER_ENABLED") == "true",
	}

	if cfg.DatabaseURL == "" {
		return nil, fmt.Errorf("DATABASE_URL is required")
	}
	if cfg.GarageAccessKey == "" {
		return nil, fmt.Errorf("GARAGE_ACCESS_KEY is required")
	}
	if cfg.GarageSecretKey == "" {
		return nil, fmt.Errorf("GARAGE_SECRET_KEY is required")
	}

	return cfg, nil
}

func getEnvDefault(key, defaultValue string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultValue
}
