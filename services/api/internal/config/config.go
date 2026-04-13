package config

import (
	"encoding/hex"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type Config struct {
	DatabaseURL     string
	NatsURL         string
	GarageEndpoint  string
	GarageAccessKey string
	GarageSecretKey string
	GarageBucket    string
	ListenAddr      string
	DevAuth         bool
	FrontendURL     string // e.g. "http://localhost:5173"

	// JWT (HS256) issued by this backend after OIDC login
	JWTSecret      string
	JWTSecretBytes []byte // decoded at startup by auth.ValidateConfig

	// Shadow File System
	ShadowFSBasePath string
	HotfolderPath    string
	HotfolderEnabled bool
	HotfolderUserID  string // User ID that hotfolder imports are attributed to
	MaxUploadSize    int64  // Maximum upload size in bytes (default 50 MB)

	// ShadowFS (unified storage)
	StorageBackend        string // "local" (default) or "s3"
	LibraryBasePath       string // e.g. /data/library
	LibraryKeepVersions   int    // 0=delete, 1=keep last, -1=keep all
	LibrarySyncEnabled    bool
	LibrarySyncAutoImport bool
	MaxUserStorageBytes   int64 // Safety-valve quota (0=unlimited, default 50GB)

	// S3 (generic — works with Garage, AWS, MinIO, etc.)
	S3Endpoint       string
	S3AccessKey      string
	S3SecretKey      string
	S3Bucket         string
	S3Region         string
	S3ForcePathStyle bool
	S3BucketActivity string // Optional: separate bucket for activities
	S3BucketMedia    string // Optional: separate bucket for media

	// LLM / LiteLLM
	LiteLLMConfigPath  string // writable path to litellm config.yaml
	LiteLLMInternalURL string // e.g. http://litellm:4000
	LLMEncryptionKey   string // 32-byte hex for AES-256-GCM API key encryption

	// Komoot
	KomootEncryptionKey string // 32-byte hex for AES-256-GCM password encryption

	// ML Service
	MLServiceURL string // e.g. http://ml:8000

	// Routing
	BRouterURL       string // e.g. http://brouter:17777
	RoutingGraphPath string // path to binary graph file

	// Data directories — all with /data/ defaults, overridable via env vars
	OSMDataDir         string
	DEMDataDir         string
	BRouterSegmentsDir string
	BRouterProfilesDir string
	TilesDir           string

	// Auth (Go-native)
	RegistrationMode   string // "invite" (default), "open", "approval"
	OAuthEncryptionKey string // 32-byte hex for AES-256-GCM (social login tokens)
	StravaClientID     string
	StravaClientSecret string
	GoogleClientID     string
	GoogleClientSecret string
	AppName            string // e.g. "Skadia"
	AppURL             string // e.g. "https://fit.example.com" (used in emails, redirects)
	TrustedProxyCIDRs  string // comma-separated CIDRs
	MobileRedirectURI  string // e.g. "skadia://auth"

	// SMTP
	SMTPHost     string
	SMTPPort     int
	SMTPUsername string
	SMTPPassword string
	SMTPFrom     string
	SMTPTLSMode  string // starttls (default) | ssl | none

	// Settings store encryption
	SettingsEncryptionKey string // 32-byte hex for AES-256-GCM (server_settings table)

	// Bootstrap admin (consumed once on first start when no users exist)
	BootstrapAdminUser     string
	BootstrapAdminPassword string

	// Hotfolder (new 3-mode model, replaces HotfolderEnabled boolean)
	HotfolderMode string // "disabled" (default), "admin-only", "self-service"
}

func Load() (*Config, error) {
	cfg := &Config{
		DatabaseURL:     os.Getenv("DATABASE_URL"),
		NatsURL:         getEnvDefault("NATS_URL", "nats://localhost:4222"),
		GarageEndpoint:  getEnvDefault("GARAGE_ENDPOINT", "http://localhost:3900"),
		GarageAccessKey: os.Getenv("GARAGE_ACCESS_KEY"),
		GarageSecretKey: os.Getenv("GARAGE_SECRET_KEY"),
		GarageBucket:    getEnvDefault("GARAGE_BUCKET", "skadia-originals"),
		ListenAddr:      getEnvDefault("LISTEN_ADDR", ":8080"),
		DevAuth:         os.Getenv("DEV_AUTH") == "true",
		FrontendURL:     getEnvDefault("FRONTEND_URL", "http://localhost:5173"),
		JWTSecret:       getSecret("JWT_SECRET", ""),

		ShadowFSBasePath: os.Getenv("SHADOW_FS_BASE_PATH"),
		HotfolderPath:    os.Getenv("HOTFOLDER_PATH"),
		HotfolderEnabled: os.Getenv("HOTFOLDER_ENABLED") == "true",
		HotfolderUserID:  os.Getenv("HOTFOLDER_USER_ID"),
		MaxUploadSize:    parseMaxUploadSize(os.Getenv("MAX_UPLOAD_SIZE"), 52428800),

		StorageBackend:        getEnvDefault("STORAGE_BACKEND", "local"),
		LibraryBasePath:       getEnvDefault("LIBRARY_BASE_PATH", "/data/library"),
		LibraryKeepVersions:   parseIntDefault(os.Getenv("LIBRARY_KEEP_VERSIONS"), 1),
		LibrarySyncEnabled:    os.Getenv("LIBRARY_SYNC_ENABLED") == "true",
		LibrarySyncAutoImport: os.Getenv("LIBRARY_SYNC_AUTO_IMPORT") == "true",
		MaxUserStorageBytes:   parseMaxStorageBytes(os.Getenv("MAX_USER_STORAGE_BYTES"), int64(50)*1024*1024*1024),

		// S3 with fallback to GARAGE_* for backward compatibility
		S3Endpoint:       getEnvDefault("S3_ENDPOINT", os.Getenv("GARAGE_ENDPOINT")),
		S3AccessKey:      getSecret("S3_ACCESS_KEY", os.Getenv("GARAGE_ACCESS_KEY")),
		S3SecretKey:      getSecret("S3_SECRET_KEY", os.Getenv("GARAGE_SECRET_KEY")),
		S3Bucket:         getEnvDefault("S3_BUCKET", "skadia"),
		S3Region:         getEnvDefault("S3_REGION", "garage"),
		S3ForcePathStyle: os.Getenv("S3_FORCE_PATH_STYLE") == "true",
		S3BucketActivity: os.Getenv("S3_BUCKET_ACTIVITY"),
		S3BucketMedia:    os.Getenv("S3_BUCKET_MEDIA"),

		LiteLLMConfigPath:  getEnvDefault("LITELLM_CONFIG_PATH", "/app/litellm/config.yaml"),
		LiteLLMInternalURL: getEnvDefault("LITELLM_INTERNAL_URL", "http://litellm:4000"),
		LLMEncryptionKey:   getSecret("LLM_ENCRYPTION_KEY", ""),

		KomootEncryptionKey: getSecret("KOMOOT_ENCRYPTION_KEY", getSecret("LLM_ENCRYPTION_KEY", "")),

		MLServiceURL: getEnvDefault("ML_SERVICE_URL", "http://ml:8000"),

		BRouterURL:       os.Getenv("BROUTER_URL"),
		RoutingGraphPath: getEnvDefault("ROUTING_GRAPH_PATH", "/data/routing/graph.bin"),

		OSMDataDir:         getEnvDefault("OSM_DATA_DIR", "/data/osm"),
		DEMDataDir:         getEnvDefault("DEM_DATA_DIR", "/data/dem"),
		BRouterSegmentsDir: getEnvDefault("BROUTER_SEGMENTS_DIR", "/data/brouter/segments"),
		BRouterProfilesDir: getEnvDefault("BROUTER_PROFILES_DIR", "/data/brouter/profiles"),
		TilesDir:           getEnvDefault("TILES_DIR", "/data/tiles"),

		RegistrationMode:   getEnvDefault("REGISTRATION_MODE", "invite"),
		OAuthEncryptionKey: getSecret("OAUTH_ENCRYPTION_KEY", ""),
		StravaClientID:     os.Getenv("STRAVA_CLIENT_ID"),
		StravaClientSecret: getSecret("STRAVA_CLIENT_SECRET", ""),
		GoogleClientID:     os.Getenv("GOOGLE_CLIENT_ID"),
		GoogleClientSecret: getSecret("GOOGLE_CLIENT_SECRET", ""),
		AppName:            getEnvDefault("APP_NAME", "Skadia"),
		AppURL:             getEnvDefault("APP_URL", "http://localhost:5173"),
		TrustedProxyCIDRs:  os.Getenv("TRUSTED_PROXY_CIDRS"),
		MobileRedirectURI:  os.Getenv("MOBILE_REDIRECT_URI"),

		SMTPHost:     os.Getenv("SMTP_HOST"),
		SMTPPort:     parseIntDefault(os.Getenv("SMTP_PORT"), 587),
		SMTPUsername: os.Getenv("SMTP_USERNAME"),
		SMTPPassword: getSecret("SMTP_PASSWORD", ""),
		SMTPFrom:     os.Getenv("SMTP_FROM"),
		SMTPTLSMode:  getEnvDefault("SMTP_TLS", "starttls"),

		SettingsEncryptionKey: getSecret("SETTINGS_ENCRYPTION_KEY", getSecret("OAUTH_ENCRYPTION_KEY", "")),

		BootstrapAdminUser:     os.Getenv("BOOTSTRAP_ADMIN_USER"),
		BootstrapAdminPassword: getSecret("BOOTSTRAP_ADMIN_PASSWORD", ""),

		HotfolderMode: getEnvDefault("HOTFOLDER_MODE", ""),
	}

	// FRONTEND_URL → APP_URL unification: FRONTEND_URL is the legacy alias.
	if os.Getenv("APP_URL") == "" && os.Getenv("FRONTEND_URL") != "" {
		cfg.AppURL = cfg.FrontendURL
	}

	// HOTFOLDER_ENABLED backward compatibility → HOTFOLDER_MODE
	if cfg.HotfolderMode == "" {
		if cfg.HotfolderEnabled {
			cfg.HotfolderMode = "self-service"
		} else {
			cfg.HotfolderMode = "disabled"
		}
	}

	if cfg.DatabaseURL == "" {
		return nil, fmt.Errorf("DATABASE_URL is required")
	}

	// M24: DEV_AUTH production guard — refuse to enable in non-localhost deployments.
	// Parse APP_URL and check hostname exactly to prevent bypass via localhost.evil.com.
	if cfg.DevAuth {
		appParsed, parseErr := url.Parse(cfg.AppURL)
		if parseErr != nil || (appParsed.Hostname() != "localhost" && appParsed.Hostname() != "127.0.0.1") {
			return nil, fmt.Errorf("DEV_AUTH=true is only allowed when APP_URL points to localhost; got APP_URL=%q", cfg.AppURL)
		}
	}

	// M48: Warn when SMTP is configured without TLS.
	if cfg.SMTPTLSMode == "none" && cfg.SMTPHost != "" {
		slog.Warn("SMTP_TLS=none: SMTP traffic is unencrypted — credentials and emails are transmitted in plaintext")
	}

	// M28: Validate encryption keys when set — must be 64 hex chars (32 bytes for AES-256-GCM).
	if cfg.LLMEncryptionKey != "" {
		if decoded, err := hex.DecodeString(cfg.LLMEncryptionKey); err != nil || len(decoded) != 32 {
			return nil, fmt.Errorf("LLM_ENCRYPTION_KEY must be 64 hex chars (32 bytes). Generate: openssl rand -hex 32")
		}
	}
	if cfg.KomootEncryptionKey != "" {
		if decoded, err := hex.DecodeString(cfg.KomootEncryptionKey); err != nil || len(decoded) != 32 {
			return nil, fmt.Errorf("KOMOOT_ENCRYPTION_KEY must be 64 hex chars (32 bytes). Generate: openssl rand -hex 32")
		}
	}

	// M29: Warn when KomootEncryptionKey falls back to LLMEncryptionKey — key-separation best practice.
	if os.Getenv("KOMOOT_ENCRYPTION_KEY") == "" && os.Getenv("KOMOOT_ENCRYPTION_KEY_FILE") == "" && cfg.LLMEncryptionKey != "" {
		slog.Warn("KOMOOT_ENCRYPTION_KEY not set — falling back to LLM_ENCRYPTION_KEY; set KOMOOT_ENCRYPTION_KEY for key separation")
	}

	// Validate SETTINGS_ENCRYPTION_KEY when set.
	if cfg.SettingsEncryptionKey != "" {
		if decoded, err := hex.DecodeString(cfg.SettingsEncryptionKey); err != nil || len(decoded) != 32 {
			return nil, fmt.Errorf("SETTINGS_ENCRYPTION_KEY must be 64 hex chars (32 bytes). Generate: openssl rand -hex 32")
		}
	}
	if os.Getenv("SETTINGS_ENCRYPTION_KEY") == "" && os.Getenv("SETTINGS_ENCRYPTION_KEY_FILE") == "" && cfg.SettingsEncryptionKey != "" {
		slog.Warn("SETTINGS_ENCRYPTION_KEY not set — falling back to OAUTH_ENCRYPTION_KEY; set SETTINGS_ENCRYPTION_KEY for key separation")
	}

	// L29: Validate STORAGE_BACKEND.
	if cfg.StorageBackend != "local" && cfg.StorageBackend != "s3" {
		return nil, fmt.Errorf("STORAGE_BACKEND must be 'local' or 's3', got %q", cfg.StorageBackend)
	}

	if cfg.StorageBackend == "s3" {
		if cfg.S3AccessKey == "" {
			return nil, fmt.Errorf("S3_ACCESS_KEY (or GARAGE_ACCESS_KEY) is required when STORAGE_BACKEND=s3")
		}
		if cfg.S3SecretKey == "" {
			return nil, fmt.Errorf("S3_SECRET_KEY (or GARAGE_SECRET_KEY) is required when STORAGE_BACKEND=s3")
		}
		// Garage requires path-style access. Warn if ForcePathStyle is off with a Garage-like region.
		if !cfg.S3ForcePathStyle && cfg.S3Region == "garage" {
			slog.Warn("S3_FORCE_PATH_STYLE is not set but S3_REGION=garage — Garage requires path-style access; set S3_FORCE_PATH_STYLE=true")
		}
	}

	// M21: Validate LIBRARY_BASE_PATH.
	if err := validateLibraryBasePath(cfg.LibraryBasePath); err != nil {
		return nil, err
	}

	return cfg, nil
}

// validateLibraryBasePath ensures the library path is safe and absolute.
func validateLibraryBasePath(p string) error {
	if p == "" {
		return fmt.Errorf("LIBRARY_BASE_PATH must not be empty")
	}
	// Canonicalize before any checks to defeat traversal like /data/../etc.
	p = filepath.Clean(p)
	if p[0] != '/' {
		return fmt.Errorf("LIBRARY_BASE_PATH must be absolute, got %q", p)
	}
	blocked := []string{
		"/", "/etc", "/var", "/usr", "/home",
		"/tmp", "/proc", "/sys", "/dev", "/root",
		"/bin", "/sbin", "/boot", "/run",
		"/data",
	}
	for _, dangerous := range blocked {
		if p == dangerous {
			return fmt.Errorf("LIBRARY_BASE_PATH must not be %q", dangerous)
		}
	}
	// Require the path to be under /data/ or /srv/ to prevent accidental
	// use of arbitrary system directories.
	if !strings.HasPrefix(p, "/data/") && !strings.HasPrefix(p, "/srv/") && !strings.HasPrefix(p, "/var/lib/") {
		return fmt.Errorf("LIBRARY_BASE_PATH must be under /data/, /srv/ or /var/lib/, got %q", p)
	}
	return nil
}

// getSecret reads a secret from either KEY_FILE (file path) or KEY (direct value).
// _FILE takes precedence. The file content is trimmed of leading/trailing whitespace.
// When _FILE is set, the file MUST be readable — falling back to the direct env var
// silently would be a security risk (operator thinks they use file secrets but actually
// uses a potentially compromised env var).
func getSecret(key, defaultValue string) string {
	if filePath := os.Getenv(key + "_FILE"); filePath != "" {
		filePath = filepath.Clean(filePath)
		if !filepath.IsAbs(filePath) {
			slog.Error("FATAL: secret file path must be absolute",
				"key", key+"_FILE", "path", filePath)
			os.Exit(1)
		}
		data, err := os.ReadFile(filePath)
		if err != nil {
			slog.Error("FATAL: secret file unreadable — refusing to fall back to env var",
				"key", key+"_FILE", "path", filePath, "error", err)
			os.Exit(1)
		}
		v := strings.TrimSpace(string(data))
		if v == "" {
			slog.Error("FATAL: secret file is empty", "key", key+"_FILE", "path", filePath)
			os.Exit(1)
		}
		return v
	}
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultValue
}

func getEnvDefault(key, defaultValue string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultValue
}

func parseIntDefault(s string, defaultVal int) int {
	if s == "" {
		return defaultVal
	}
	v, err := strconv.Atoi(s)
	if err != nil {
		return defaultVal
	}
	return v
}

func parseMaxStorageBytes(s string, defaultVal int64) int64 {
	if s == "" {
		return defaultVal
	}
	v, err := strconv.ParseInt(s, 10, 64)
	if err != nil || v < 0 {
		slog.Warn("MAX_USER_STORAGE_BYTES parse error, using default", "value", s, "default", defaultVal)
		return defaultVal
	}
	return v
}

func parseMaxUploadSize(s string, defaultVal int64) int64 {
	if s == "" {
		return defaultVal
	}
	v, err := strconv.ParseInt(s, 10, 64)
	if err != nil || v <= 0 {
		slog.Warn("MAX_UPLOAD_SIZE parse error, using default", "value", s, "default", defaultVal)
		return defaultVal
	}
	return v
}
