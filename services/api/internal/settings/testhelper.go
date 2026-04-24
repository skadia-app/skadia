package settings

import (
	"time"
)

// NewStoreForTest builds a SettingsStore with a pre-populated cache and no DB
// connection. Values are stored as source="test" so callers that inspect the
// Source field can differentiate from env/db-loaded values.
//
// Intended for tests that need to exercise code paths which read settings
// (e.g. auth handlers calling h.Settings.Get("registration.mode")) without
// booting a real SettingsStore against a live DB. Writing via Set() is not
// supported in test stores — the db pointer is nil.
//
// ttl and lastLoad are initialised so that isStale() returns false and the
// store does not attempt a background DB refresh during tests (MED #21).
func NewStoreForTest(values map[string]string) *SettingsStore {
	s := &SettingsStore{
		cache:    make(map[string]CachedSetting, len(values)),
		ttl:      30 * time.Second,
		lastLoad: time.Now(),
		// db stays nil — callers that need Set() must wire a real DB.
	}
	for k, v := range values {
		s.cache[k] = CachedSetting{Value: v, Source: "test"}
	}
	return s
}
