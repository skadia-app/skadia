// Package settings — this file is test-only but must be exported for
// cross-package tests (auth, handler) that import settings.NewStoreForTest.
// Do NOT call from production code. It is intentionally NOT named _test.go
// because Go restricts cross-package imports of _test.go files.
//
// R2 LOW #14: cannot rename to store_testhelpers_test.go because Go's test
// compilation model only makes _test.go symbols available within their own
// package test binary — cross-package _test.go imports are forbidden.
// The file remains as testonly_helpers.go with explicit doc marking.
// Callers: auth/handler_test.go, auth/gdpr_fusion_roundtrip_test.go,
//          handler/admin_rename_test.go, handler/testhelper_routing_test.go,
//          handler/admin_routing_coverage_test.go, handler/admin_library_settings_test.go.
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
//
// Deprecated: Test only. Do not use in production code.
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
