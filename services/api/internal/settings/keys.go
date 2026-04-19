// Package settings provides a registry of all server-side configuration keys
// and a persistent store backed by the server_settings database table.
// Values may optionally be AES-256-GCM encrypted (Encrypted=true keys) and
// may be overridden by environment variables (EnvVar field).
package settings

import (
	"fmt"
	"strconv"
	"strings"
)

// KeyDef describes a single configurable setting.
type KeyDef struct {
	Key       string                // e.g. "smtp.host"
	EnvVar    string                // e.g. "SMTP_HOST"; empty = no env override
	Default   string                // value used when neither env nor DB row exists
	Encrypted bool                  // true = AES-256-GCM at rest
	Group     string                // e.g. "smtp" — used for API grouping
	Validate  func(string) error    // nil = no validation
}

// AllKeys is the ordered list of every registered setting.
var AllKeys = []KeyDef{
	{
		Key: "smtp.host", EnvVar: "SMTP_HOST", Default: "", Encrypted: false, Group: "smtp",
	},
	{
		Key: "smtp.port", EnvVar: "SMTP_PORT", Default: "587", Encrypted: false, Group: "smtp",
		Validate: func(v string) error {
			_, err := strconv.Atoi(v)
			if err != nil {
				return fmt.Errorf("smtp.port must be a valid integer, got %q", v)
			}
			return nil
		},
	},
	{
		Key: "smtp.username", EnvVar: "SMTP_USERNAME", Default: "", Encrypted: false, Group: "smtp",
	},
	{
		Key: "smtp.password", EnvVar: "SMTP_PASSWORD", Default: "", Encrypted: true, Group: "smtp",
	},
	{
		Key: "smtp.from", EnvVar: "SMTP_FROM", Default: "", Encrypted: false, Group: "smtp",
	},
	{
		Key: "smtp.tls", EnvVar: "SMTP_TLS", Default: "starttls", Encrypted: false, Group: "smtp",
		Validate: func(v string) error {
			switch v {
			case "starttls", "ssl", "none":
				return nil
			default:
				return fmt.Errorf("smtp.tls must be one of starttls, ssl, none; got %q", v)
			}
		},
	},
	{
		Key: "registration.mode", EnvVar: "REGISTRATION_MODE", Default: "invite", Encrypted: false, Group: "registration",
		Validate: func(v string) error {
			switch v {
			case "invite", "open", "approval":
				return nil
			default:
				return fmt.Errorf("registration.mode must be one of invite, open, approval; got %q", v)
			}
		},
	},
	{
		Key: "app.name", EnvVar: "APP_NAME", Default: "Skadia", Encrypted: false, Group: "app",
		Validate: func(v string) error {
			if v == "" {
				return fmt.Errorf("app.name must not be empty")
			}
			if len(v) > 100 {
				return fmt.Errorf("app.name must not exceed 100 characters")
			}
			return nil
		},
	},
	{
		Key: "app.url", EnvVar: "APP_URL", Default: "http://localhost:5173", Encrypted: false, Group: "app",
		Validate: func(v string) error {
			if v == "" {
				return fmt.Errorf("app.url must not be empty")
			}
			if len(v) > 500 {
				return fmt.Errorf("app.url must not exceed 500 characters")
			}
			if !strings.HasPrefix(v, "http://") && !strings.HasPrefix(v, "https://") {
				return fmt.Errorf("app.url must start with http:// or https://")
			}
			return nil
		},
	},
	{
		Key: "social.strava_client_id", EnvVar: "STRAVA_CLIENT_ID", Default: "", Encrypted: false, Group: "social",
	},
	{
		Key: "social.strava_client_secret", EnvVar: "STRAVA_CLIENT_SECRET", Default: "", Encrypted: true, Group: "social",
	},
	{
		Key: "social.google_client_id", EnvVar: "GOOGLE_CLIENT_ID", Default: "", Encrypted: false, Group: "social",
	},
	{
		Key: "social.google_client_secret", EnvVar: "GOOGLE_CLIENT_SECRET", Default: "", Encrypted: true, Group: "social",
	},
	{
		Key: "limits.max_upload_bytes", EnvVar: "MAX_UPLOAD_SIZE", Default: "52428800", Encrypted: false, Group: "limits",
		Validate: func(v string) error {
			n, err := strconv.ParseInt(v, 10, 64)
			if err != nil || n <= 0 {
				return fmt.Errorf("limits.max_upload_bytes must be a positive integer, got %q", v)
			}
			return nil
		},
	},
	{
		Key: "limits.max_user_storage_bytes", EnvVar: "MAX_USER_STORAGE_BYTES", Default: "53687091200", Encrypted: false, Group: "limits",
		Validate: func(v string) error {
			n, err := strconv.ParseInt(v, 10, 64)
			if err != nil || n <= 0 {
				return fmt.Errorf("limits.max_user_storage_bytes must be a positive integer, got %q", v)
			}
			return nil
		},
	},
	{
		Key: "library.keep_versions", EnvVar: "LIBRARY_KEEP_VERSIONS", Default: "1", Encrypted: false, Group: "library",
		Validate: func(v string) error {
			n, err := strconv.Atoi(v)
			if err != nil || (n != -1 && n != 0 && n != 1) {
				return fmt.Errorf("library.keep_versions must be -1, 0, or 1; got %q", v)
			}
			return nil
		},
	},
	{
		Key: "library.sync_enabled", EnvVar: "LIBRARY_SYNC_ENABLED", Default: "false", Encrypted: false, Group: "library",
	},
	{
		Key: "library.sync_auto_import", EnvVar: "LIBRARY_SYNC_AUTO_IMPORT", Default: "false", Encrypted: false, Group: "library",
	},
	{
		Key: "hotfolder.mode", EnvVar: "HOTFOLDER_MODE", Default: "disabled", Encrypted: false, Group: "hotfolder",
		Validate: func(v string) error {
			switch v {
			case "disabled", "admin-only", "self-service":
				return nil
			default:
				return fmt.Errorf("hotfolder.mode must be one of disabled, admin-only, self-service; got %q", v)
			}
		},
	},
	{
		Key: "timescale.compression", EnvVar: "", Default: "false", Encrypted: false, Group: "timescale",
	},
}

// KeysByKey maps key strings to their KeyDef for O(1) lookup.
var KeysByKey map[string]KeyDef

// KeysByGroup maps group name to the ordered slice of KeyDefs in that group.
var KeysByGroup map[string][]KeyDef

func init() {
	KeysByKey = make(map[string]KeyDef, len(AllKeys))
	KeysByGroup = make(map[string][]KeyDef)

	for _, kd := range AllKeys {
		// F9 defense-in-depth: Key must start with "<Group>." so that GetAll()
		// can strip the group prefix for frontend consumption. A key that
		// doesn't match would silently pass through TrimPrefix and break
		// the frontend lookup.
		if !strings.HasPrefix(kd.Key, kd.Group+".") {
			panic(fmt.Sprintf("settings: key %q must start with group prefix %q.", kd.Key, kd.Group+"."))
		}
		KeysByKey[kd.Key] = kd
		KeysByGroup[kd.Group] = append(KeysByGroup[kd.Group], kd)
	}
}
