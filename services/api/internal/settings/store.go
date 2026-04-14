package settings

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/skadia-app/skadia/services/api/internal/crypto"
	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/jackc/pgx/v5"
)

// sentinelMasked is the API-visible placeholder that the frontend sends back
// when it received "***" for an encrypted field and does not want to change it.
const sentinelMasked = "***"

// CachedSetting holds the resolved value and metadata for a single key.
type CachedSetting struct {
	Value     string `json:"value"`
	Source    string `json:"source"`           // "env", "db", "default"
	Locked    bool   `json:"locked,omitempty"` // true when overridden by env var
	Encrypted bool   `json:"-"`
}

// SettingsStore is a write-through cache over the server_settings table.
// All public methods are safe for concurrent use.
type SettingsStore struct {
	db             *database.DB
	encryptionKey  string // hex-encoded 32-byte key; may be empty
	storageBackend string // "local" or "s3"; set from config at startup
	mu             sync.RWMutex
	cache          map[string]CachedSetting
}

// NewSettingsStore creates a store and eagerly loads the cache from the DB.
// ctx is used only for the initial DB query; the store can be used after ctx
// is cancelled.
// storageBackend must be "local" or "s3" (mirrors config.StorageBackend).
func NewSettingsStore(db *database.DB, encryptionKey, storageBackend string) *SettingsStore {
	s := &SettingsStore{
		db:             db,
		encryptionKey:  encryptionKey,
		storageBackend: storageBackend,
		cache:          make(map[string]CachedSetting, len(AllKeys)),
	}
	// loadCache is best-effort at startup; log but don't panic on DB error.
	if err := s.loadCache(context.Background()); err != nil {
		slog.Error("settings: initial cache load failed — using defaults/env", "error", err)
	}
	return s
}

// readEnvSecret reads a secret from KEY_FILE first, falling back to KEY.
// Unlike config.getSecret it does NOT call os.Exit on file errors; it warns
// and falls back to the direct env var instead.
func readEnvSecret(key string) string {
	if filePath := os.Getenv(key + "_FILE"); filePath != "" {
		data, err := os.ReadFile(filepath.Clean(filePath))
		if err != nil {
			slog.Warn("settings: secret file unreadable, falling back to env var",
				"key", key+"_FILE", "error", err)
		} else if v := strings.TrimSpace(string(data)); v != "" {
			return v
		}
	}
	return os.Getenv(key)
}

// loadCache rebuilds the in-memory cache from DB + env vars.
// The caller must NOT hold mu when calling loadCache.
func (s *SettingsStore) loadCache(ctx context.Context) error {
	// Fetch all rows from server_settings in one round-trip.
	rows, err := s.db.Pool.Query(ctx,
		`SELECT key, value, encrypted FROM server_settings`)
	if err != nil {
		return fmt.Errorf("settings: query server_settings: %w", err)
	}
	defer rows.Close()

	dbRows := make(map[string]struct {
		raw       string
		encrypted bool
	})
	for rows.Next() {
		var key, raw string
		var enc bool
		if err := rows.Scan(&key, &raw, &enc); err != nil {
			return fmt.Errorf("settings: scan row: %w", err)
		}
		dbRows[key] = struct {
			raw       string
			encrypted bool
		}{raw, enc}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("settings: iterate rows: %w", err)
	}

	fresh := make(map[string]CachedSetting, len(AllKeys))

	for _, kd := range AllKeys {
		cs, buildErr := s.buildCachedSetting(kd, dbRows)
		if buildErr != nil {
			// Log and fall back to default rather than poisoning the whole cache.
			slog.Warn("settings: could not resolve setting, using default",
				"key", kd.Key, "error", buildErr)
			cs = CachedSetting{
				Value:     kd.Default,
				Source:    "default",
				Encrypted: kd.Encrypted,
			}
		}
		fresh[kd.Key] = cs
	}

	s.mu.Lock()
	s.cache = fresh
	s.mu.Unlock()
	return nil
}

// buildCachedSetting resolves a single key's value from env > db > default.
func (s *SettingsStore) buildCachedSetting(kd KeyDef, dbRows map[string]struct {
	raw       string
	encrypted bool
}) (CachedSetting, error) {
	// 1. Env var takes precedence and locks the field.
	if kd.EnvVar != "" {
		if v := readEnvSecret(kd.EnvVar); v != "" {
			return CachedSetting{
				Value:     v,
				Source:    "env",
				Locked:    true,
				Encrypted: kd.Encrypted,
			}, nil
		}
	}

	// 2. DB row.
	if row, ok := dbRows[kd.Key]; ok {
		value := row.raw
		if row.encrypted || kd.Encrypted {
			if s.encryptionKey == "" {
				return CachedSetting{}, fmt.Errorf("key %q is encrypted but SETTINGS_ENCRYPTION_KEY is not configured", kd.Key)
			}
			plain, err := crypto.Decrypt(value, s.encryptionKey)
			if err != nil {
				return CachedSetting{}, fmt.Errorf("decrypt %q: %w", kd.Key, err)
			}
			value = plain
		}
		return CachedSetting{
			Value:     value,
			Source:    "db",
			Encrypted: kd.Encrypted,
		}, nil
	}

	// 3. Default.
	return CachedSetting{
		Value:     kd.Default,
		Source:    "default",
		Encrypted: kd.Encrypted,
	}, nil
}

// Get returns the cached setting for key. If key is unknown it returns a
// zero CachedSetting (empty value, source "default").
func (s *SettingsStore) Get(key string) CachedSetting {
	s.mu.RLock()
	cs := s.cache[key]
	s.mu.RUnlock()
	return cs
}

// GetAll returns all settings organised by group.
// Encrypted values are returned in plaintext (the caller is trusted; masking
// for API responses is the handler's responsibility).
func (s *SettingsStore) GetAll() map[string]map[string]CachedSetting {
	s.mu.RLock()
	snapshot := make(map[string]CachedSetting, len(s.cache))
	for k, v := range s.cache {
		snapshot[k] = v
	}
	s.mu.RUnlock()

	result := make(map[string]map[string]CachedSetting)
	for _, kd := range AllKeys {
		if _, ok := result[kd.Group]; !ok {
			result[kd.Group] = make(map[string]CachedSetting)
		}
		// Use the short key (strip "<group>." prefix) so the frontend can access
		// nested fields with getValue(settings, group, shortKey) without repeating
		// the group prefix. E.g. "smtp.host" is stored as settings["smtp"]["host"].
		prefix := kd.Group + "."
		shortKey := strings.TrimPrefix(kd.Key, prefix)
		result[kd.Group][shortKey] = snapshot[kd.Key]
	}
	return result
}

// Put validates and persists a batch of setting updates.
//
// updates maps key → *string:
//   - nil value  → delete the DB row (revert to env/default)
//   - "***"      → encrypted field sentinel: no change
//   - any string → new value
//
// adminID is recorded in server_settings.updated_by (may be empty string).
// Returns a map of actually changed keys → new plaintext values.
func (s *SettingsStore) Put(ctx context.Context, updates map[string]*string, adminID string) (changed map[string]string, err error) {
	if len(updates) == 0 {
		return nil, nil
	}

	// --- Phase 1: validate all inputs before touching the DB ---

	// Resolve the effective value of every key after applying updates (for
	// cross-field validation we need the full picture).
	effective := make(map[string]string, len(AllKeys))
	s.mu.RLock()
	for _, kd := range AllKeys {
		effective[kd.Key] = s.cache[kd.Key].Value
	}
	s.mu.RUnlock()

	for key, valPtr := range updates {
		kd, ok := KeysByKey[key]
		if !ok {
			return nil, fmt.Errorf("unknown settings key %q", key)
		}

		// Locked (env-overridden) fields cannot be changed via the API.
		s.mu.RLock()
		locked := s.cache[key].Locked
		s.mu.RUnlock()
		if locked {
			return nil, fmt.Errorf("setting %q is locked by environment variable %q and cannot be changed via the API", key, kd.EnvVar)
		}

		if valPtr == nil {
			// Delete → revert to default.
			effective[key] = kd.Default
			continue
		}

		v := *valPtr

		// Masked sentinel for encrypted fields = keep current value.
		if v == sentinelMasked && kd.Encrypted {
			// effective[key] already contains current value — nothing to do.
			continue
		}

		// Per-key validation.
		if kd.Validate != nil {
			if err := kd.Validate(v); err != nil {
				return nil, err
			}
		}

		// Encryption requirement check.
		if kd.Encrypted && s.encryptionKey == "" {
			return nil, fmt.Errorf("cannot store encrypted setting %q: SETTINGS_ENCRYPTION_KEY is not configured", key)
		}

		effective[key] = v
	}

	// --- Phase 2: cross-field validation ---

	if effective["registration.mode"] == "open" && effective["smtp.host"] == "" {
		return nil, errors.New("registration.mode=open requires smtp.host to be configured")
	}
	if effective["library.sync_auto_import"] == "true" && effective["library.sync_enabled"] != "true" {
		return nil, errors.New("library.sync_auto_import=true requires library.sync_enabled=true")
	}
	if effective["hotfolder.mode"] != "disabled" && os.Getenv("HOTFOLDER_PATH") == "" {
		return nil, errors.New("hotfolder.mode requires the HOTFOLDER_PATH environment variable to be set")
	}
	if s.storageBackend == "s3" {
		if v, ok := updates["library.sync_enabled"]; ok && v != nil && *v == "true" {
			return nil, errors.New("library.sync_enabled requires local storage backend (current: s3)")
		}
		if v, ok := updates["library.sync_auto_import"]; ok && v != nil && *v == "true" {
			return nil, errors.New("library.sync_auto_import requires local storage backend (current: s3)")
		}
	}

	// --- Phase 3: write to DB (single transaction for atomicity) ---

	changed = make(map[string]string)

	// Determine the admin UUID parameter (NULL when empty).
	var adminIDParam any
	if adminID != "" {
		adminIDParam = adminID
	}

	tx, err := s.db.Pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("settings: begin transaction: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	for key, valPtr := range updates {
		kd := KeysByKey[key]

		if valPtr == nil {
			// Delete row — revert to env/default.
			if _, execErr := tx.Exec(ctx,
				`DELETE FROM server_settings WHERE key = $1`, key); execErr != nil {
				return nil, fmt.Errorf("settings: delete %q: %w", key, execErr)
			}
			changed[key] = kd.Default
			continue
		}

		v := *valPtr

		// Skip masked-sentinel for encrypted fields.
		if v == sentinelMasked && kd.Encrypted {
			continue
		}

		// Encrypt if needed.
		storeValue := v
		if kd.Encrypted {
			enc, encErr := crypto.Encrypt(v, s.encryptionKey)
			if encErr != nil {
				return nil, fmt.Errorf("settings: encrypt %q: %w", key, encErr)
			}
			storeValue = enc
		}

		_, execErr := tx.Exec(ctx,
			`INSERT INTO server_settings (key, value, encrypted, updated_at, updated_by)
			 VALUES ($1, $2, $3, now(), $4)
			 ON CONFLICT (key) DO UPDATE
			   SET value      = EXCLUDED.value,
			       encrypted  = EXCLUDED.encrypted,
			       updated_at = EXCLUDED.updated_at,
			       updated_by = EXCLUDED.updated_by`,
			key, storeValue, kd.Encrypted, adminIDParam,
		)
		if execErr != nil {
			return nil, fmt.Errorf("settings: upsert %q: %w", key, execErr)
		}
		changed[key] = v
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("settings: commit: %w", err)
	}

	// --- Phase 4: invalidate cache ---
	if reloadErr := s.loadCache(ctx); reloadErr != nil {
		slog.Error("settings: cache reload after Put failed", "error", reloadErr)
		// Don't return the reload error — the DB write succeeded.
	}

	return changed, nil
}

// Reload re-reads all rows from the DB and rebuilds the in-memory cache.
// Call this if the DB was modified by an external process.
func (s *SettingsStore) Reload(ctx context.Context) error {
	return s.loadCache(ctx)
}

// ensure pgx is used (rows.Next / rows.Scan / rows.Err come from pgx).
var _ = pgx.ErrNoRows
