package config

import (
	"fmt"
	"os"
	"strconv"
)

type Config struct {
	DatabaseURL      string
	NatsURL          string
	GarageEndpoint   string
	GarageAccessKey  string
	GarageSecretKey  string
	GarageBucket     string
	ListenAddr       string
	DevAuth     bool
	FrontendURL string // e.g. "http://localhost:5173"

	// JWT (HS256) issued by this backend after OIDC login
	JWTSecret string

	// Shadow File System
	ShadowFSBasePath string
	HotfolderPath    string
	HotfolderEnabled bool
	HotfolderUserID  string // User ID that hotfolder imports are attributed to
	MaxUploadSize    int64  // Maximum upload size in bytes (default 50 MB)

	// ShadowFS (unified storage)
	StorageBackend      string // "local" (default) or "s3"
	LibraryBasePath     string // e.g. /data/library
	LibraryKeepVersions int    // 0=delete, 1=keep last, -1=keep all
	LibrarySyncEnabled    bool
	LibrarySyncAutoImport bool
	MaxUserStorageBytes   int64  // Safety-valve quota (0=unlimited, default 50GB)

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
		DevAuth:     os.Getenv("DEV_AUTH") == "true",
		FrontendURL: getEnvDefault("FRONTEND_URL", "http://localhost:5173"),
		JWTSecret:             os.Getenv("JWT_SECRET"),

		ShadowFSBasePath: os.Getenv("SHADOW_FS_BASE_PATH"),
		HotfolderPath:    os.Getenv("HOTFOLDER_PATH"),
		HotfolderEnabled: os.Getenv("HOTFOLDER_ENABLED") == "true",
		HotfolderUserID:  os.Getenv("HOTFOLDER_USER_ID"),
		MaxUploadSize:    parseMaxUploadSize(os.Getenv("MAX_UPLOAD_SIZE"), 52428800),

		StorageBackend:      getEnvDefault("STORAGE_BACKEND", "local"),
		LibraryBasePath:     getEnvDefault("LIBRARY_BASE_PATH", "/data/library"),
		LibraryKeepVersions: parseIntDefault(os.Getenv("LIBRARY_KEEP_VERSIONS"), 1),
		LibrarySyncEnabled:  os.Getenv("LIBRARY_SYNC_ENABLED") == "true",
		LibrarySyncAutoImport: os.Getenv("LIBRARY_SYNC_AUTO_IMPORT") == "true",
		MaxUserStorageBytes:   parseMaxStorageBytes(os.Getenv("MAX_USER_STORAGE_BYTES"), 50*1024*1024*1024),

		// S3 with fallback to GARAGE_* for backward compatibility
		S3Endpoint:       getEnvDefault("S3_ENDPOINT", os.Getenv("GARAGE_ENDPOINT")),
		S3AccessKey:      getEnvDefault("S3_ACCESS_KEY", os.Getenv("GARAGE_ACCESS_KEY")),
		S3SecretKey:      getEnvDefault("S3_SECRET_KEY", os.Getenv("GARAGE_SECRET_KEY")),
		S3Bucket:         getEnvDefault("S3_BUCKET", "skadia"),
		S3Region:         getEnvDefault("S3_REGION", "garage"),
		S3ForcePathStyle: os.Getenv("S3_FORCE_PATH_STYLE") != "false",
		S3BucketActivity: os.Getenv("S3_BUCKET_ACTIVITY"),
		S3BucketMedia:    os.Getenv("S3_BUCKET_MEDIA"),

		LiteLLMConfigPath:  getEnvDefault("LITELLM_CONFIG_PATH", "/app/litellm/config.yaml"),
		LiteLLMInternalURL: getEnvDefault("LITELLM_INTERNAL_URL", "http://litellm:4000"),
		LLMEncryptionKey:   os.Getenv("LLM_ENCRYPTION_KEY"),

		KomootEncryptionKey: getEnvDefault("KOMOOT_ENCRYPTION_KEY", os.Getenv("LLM_ENCRYPTION_KEY")),

		MLServiceURL: getEnvDefault("ML_SERVICE_URL", "http://ml:8000"),

		BRouterURL:       os.Getenv("BROUTER_URL"),
		RoutingGraphPath: getEnvDefault("ROUTING_GRAPH_PATH", "/data/routing/graph.bin"),

		RegistrationMode:   getEnvDefault("REGISTRATION_MODE", "invite"),
		OAuthEncryptionKey: os.Getenv("OAUTH_ENCRYPTION_KEY"),
		StravaClientID:     os.Getenv("STRAVA_CLIENT_ID"),
		StravaClientSecret: os.Getenv("STRAVA_CLIENT_SECRET"),
		GoogleClientID:     os.Getenv("GOOGLE_CLIENT_ID"),
		GoogleClientSecret: os.Getenv("GOOGLE_CLIENT_SECRET"),
		AppName:            getEnvDefault("APP_NAME", "Skadia"),
		AppURL:             getEnvDefault("APP_URL", "http://localhost:5173"),
		TrustedProxyCIDRs:  os.Getenv("TRUSTED_PROXY_CIDRS"),
		MobileRedirectURI:  os.Getenv("MOBILE_REDIRECT_URI"),

		SMTPHost:     os.Getenv("SMTP_HOST"),
		SMTPPort:     parseIntDefault(os.Getenv("SMTP_PORT"), 587),
		SMTPUsername: os.Getenv("SMTP_USERNAME"),
		SMTPPassword: os.Getenv("SMTP_PASSWORD"),
		SMTPFrom:     os.Getenv("SMTP_FROM"),
		SMTPTLSMode:  getEnvDefault("SMTP_TLS", "starttls"),
	}

	if cfg.DatabaseURL == "" {
		return nil, fmt.Errorf("DATABASE_URL is required")
	}
	if cfg.StorageBackend == "s3" {
		if cfg.S3AccessKey == "" {
			return nil, fmt.Errorf("S3_ACCESS_KEY (or GARAGE_ACCESS_KEY) is required when STORAGE_BACKEND=s3")
		}
		if cfg.S3SecretKey == "" {
			return nil, fmt.Errorf("S3_SECRET_KEY (or GARAGE_SECRET_KEY) is required when STORAGE_BACKEND=s3")
		}
	}

	return cfg, nil
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
		return defaultVal
	}
	return v
}
