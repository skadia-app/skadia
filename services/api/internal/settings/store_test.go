package settings

import (
	"context"
	"sync"
	"testing"
	"time"
)

// newTestStore creates a SettingsStore without a DB connection by building the
// cache directly from env vars and defaults.  This is safe for testing
// Get/GetAll and all Validate functions.
func newTestStore(t *testing.T) *SettingsStore {
	t.Helper()
	s := &SettingsStore{
		db:    nil,
		cache: make(map[string]CachedSetting, len(AllKeys)),
	}

	// Build cache from env > default (no DB rows).
	emptyDBRows := map[string]struct {
		raw       string
		encrypted bool
	}{}

	for _, kd := range AllKeys {
		cs, err := s.buildCachedSetting(kd, emptyDBRows)
		if err != nil {
			// Encrypted key with no encryption key configured — use default.
			cs = CachedSetting{
				Value:     kd.Default,
				Source:    "default",
				Encrypted: kd.Encrypted,
			}
		}
		s.cache[kd.Key] = cs
	}
	return s
}

// TestGet_DefaultValue verifies that an unset key returns its Default value
// with Source="default" and Locked=false.
func TestGet_DefaultValue(t *testing.T) {
	s := newTestStore(t)

	got := s.Get("smtp.host")
	if got.Value != "" {
		t.Errorf("expected empty default, got %q", got.Value)
	}
	if got.Source != "default" {
		t.Errorf("expected source=default, got %q", got.Source)
	}
	if got.Locked {
		t.Error("expected Locked=false for default value")
	}
}

// TestGet_EnvOverride verifies that setting the env var causes Get to return
// the env value with Source="env" and Locked=true.
func TestGet_EnvOverride(t *testing.T) {
	t.Setenv("SMTP_HOST", "mail.example.com")
	s := newTestStore(t)

	got := s.Get("smtp.host")
	if got.Value != "mail.example.com" {
		t.Errorf("expected env value, got %q", got.Value)
	}
	if got.Source != "env" {
		t.Errorf("expected source=env, got %q", got.Source)
	}
	if !got.Locked {
		t.Error("expected Locked=true for env override")
	}
}

// TestGet_UnknownKey verifies that Get for an unregistered key returns a zero
// CachedSetting (empty value, empty source) rather than panicking.
func TestGet_UnknownKey(t *testing.T) {
	s := newTestStore(t)

	got := s.Get("nonexistent.key")
	if got.Value != "" {
		t.Errorf("expected empty value for unknown key, got %q", got.Value)
	}
	if got.Locked {
		t.Error("unknown key should not be locked")
	}
}

// TestGetAll_GroupedBySection verifies that GetAll returns all expected top-level
// section keys.
func TestGetAll_GroupedBySection(t *testing.T) {
	s := newTestStore(t)

	result := s.GetAll()

	expectedSections := []string{
		"smtp",
		"registration",
		"app",
		"social",
		"limits",
		"library",
		"hotfolder",
		"timescale",
	}

	for _, section := range expectedSections {
		if _, ok := result[section]; !ok {
			t.Errorf("expected section %q to be present in GetAll result", section)
		}
	}

	// Verify no unexpected extra sections beyond what AllKeys defines.
	for section := range result {
		found := false
		for _, expected := range expectedSections {
			if section == expected {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("unexpected section %q in GetAll result", section)
		}
	}
}

// TestValidation_InvalidPort verifies that smtp.port rejects non-integer values.
func TestValidation_InvalidPort(t *testing.T) {
	kd := KeysByKey["smtp.port"]
	if kd.Validate == nil {
		t.Fatal("smtp.port has no Validate function")
	}
	if err := kd.Validate("abc"); err == nil {
		t.Error("expected error for non-integer port, got nil")
	}
}

// TestValidation_ValidPort verifies that smtp.port accepts a valid port number.
func TestValidation_ValidPort(t *testing.T) {
	kd := KeysByKey["smtp.port"]
	if kd.Validate == nil {
		t.Fatal("smtp.port has no Validate function")
	}
	if err := kd.Validate("587"); err != nil {
		t.Errorf("expected nil for valid port, got: %v", err)
	}
}

// TestValidation_InvalidTLSMode verifies that smtp.tls rejects unknown modes.
func TestValidation_InvalidTLSMode(t *testing.T) {
	kd := KeysByKey["smtp.tls"]
	if kd.Validate == nil {
		t.Fatal("smtp.tls has no Validate function")
	}
	if err := kd.Validate("xxx"); err == nil {
		t.Error("expected error for invalid TLS mode, got nil")
	}
}

// TestValidation_InvalidRegMode verifies that registration.mode rejects unknown modes.
func TestValidation_InvalidRegMode(t *testing.T) {
	kd := KeysByKey["registration.mode"]
	if kd.Validate == nil {
		t.Fatal("registration.mode has no Validate function")
	}
	if err := kd.Validate("xxx"); err == nil {
		t.Error("expected error for invalid registration mode, got nil")
	}
}

// TestValidation_InvalidHotfolderMode verifies that hotfolder.mode rejects unknown modes.
func TestValidation_InvalidHotfolderMode(t *testing.T) {
	kd := KeysByKey["hotfolder.mode"]
	if kd.Validate == nil {
		t.Fatal("hotfolder.mode has no Validate function")
	}
	if err := kd.Validate("xxx"); err == nil {
		t.Error("expected error for invalid hotfolder mode, got nil")
	}
}

// TestValidation_ValidKeepVersions verifies library.keep_versions accepts -1/0/1
// and rejects any other value such as "2".
func TestValidation_ValidKeepVersions(t *testing.T) {
	kd := KeysByKey["library.keep_versions"]
	if kd.Validate == nil {
		t.Fatal("library.keep_versions has no Validate function")
	}

	validValues := []string{"-1", "0", "1"}
	for _, v := range validValues {
		if err := kd.Validate(v); err != nil {
			t.Errorf("expected nil for keep_versions=%q, got: %v", v, err)
		}
	}

	invalidValues := []string{"2", "-2", "10", "abc"}
	for _, v := range invalidValues {
		if err := kd.Validate(v); err == nil {
			t.Errorf("expected error for keep_versions=%q, got nil", v)
		}
	}
}

func TestSettingsStoreTTLRefresh(t *testing.T) {
	pool := settingsTestPool(t)
	ctx := context.Background()

	store := newTestSettingsStore(t, pool)

	// Ensure key exists with initial value
	_, _ = pool.Exec(ctx,
		`INSERT INTO server_settings (key, value, encrypted, updated_at) VALUES ($1, $2, false, NOW())
		 ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value`,
		"library.user_can_edit_schemas", "true")

	// Force cache rebuild
	_ = store.Reload(ctx)
	if got := store.Get("library.user_can_edit_schemas").Value; got != "true" {
		t.Fatalf("initial: got %q", got)
	}

	// Bypass store.Put: modify DB directly (simulates another replica)
	_, _ = pool.Exec(ctx,
		`UPDATE server_settings SET value = 'false' WHERE key = 'library.user_can_edit_schemas'`)

	// Within TTL: old cache value
	if got := store.Get("library.user_can_edit_schemas").Value; got != "true" {
		t.Errorf("within TTL: got %q, want cached 'true'", got)
	}

	// Force expiry by setting lastLoad backward
	store.setLastLoadForTest(time.Now().Add(-35 * time.Second))

	// Next Get triggers rebuild (fire-and-forget). Wait for it to complete.
	_ = store.Get("library.user_can_edit_schemas")
	if err := store.waitForRebuildCompleteForTest(2 * time.Second); err != nil {
		t.Fatalf("wait for rebuild: %v", err)
	}
	if got := store.Get("library.user_can_edit_schemas").Value; got != "false" {
		t.Errorf("after TTL expiry: got %q, want refreshed 'false'", got)
	}
}

// TestSettingsStoreStartupRecoversFromLoadCacheFailure verifies that a fresh
// store where initial loadCache never succeeded (lastLoad is zero) still
// triggers a TTL rebuild on the next Get. Previously an IsZero-guard blocked
// ALL future rebuilds in that scenario (Opus-Kritiker CRIT #3).
func TestSettingsStoreStartupRecoversFromLoadCacheFailure(t *testing.T) {
	pool := settingsTestPool(t)
	ctx := context.Background()

	// Create store — simulate "DB works now".
	store := newTestSettingsStore(t, pool)

	// Nuke lastLoad to simulate "initial loadCache failed" scenario.
	store.setLastLoadForTest(time.Time{}) // zero

	// Baseline: constructor-driven loadCache counts as 0 rebuilds (rebuildCount
	// only increments inside triggerRebuild). The relevant invariant for this
	// test is the delta after the Get below.
	baseline := store.rebuildCountForTest()
	_ = baseline
	_ = ctx

	// Get should see stale=true (IsZero would've been guard) and trigger rebuild.
	_ = store.Get("smtp.host")

	if err := store.waitForRebuildCompleteForTest(2 * time.Second); err != nil {
		t.Fatalf("rebuild never triggered despite zero lastLoad: %v", err)
	}
}

func TestSettingsStoreSingleFlightConcurrentGet(t *testing.T) {
	pool := settingsTestPool(t)
	defer pool.Close()
	store := newTestSettingsStore(t, pool)

	// Trigger TTL expiry
	store.setLastLoadForTest(time.Now().Add(-1 * time.Minute))

	// Fire 100 parallel Gets
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = store.Get("smtp.host")
		}()
	}
	wg.Wait()

	// Wait for the triggered rebuild to complete (singleflight is fire-and-forget).
	// waitForRebuildCompleteForTest blocks until either rebuildCount incremented
	// at least once or timeout — deterministic and race-safe.
	if err := store.waitForRebuildCompleteForTest(2 * time.Second); err != nil {
		t.Fatalf("wait for rebuild: %v", err)
	}

	got := store.rebuildCountForTest()
	// Allow 1-2: single-flight coalesces concurrent calls. If the Goroutine
	// scheduler had one straggler that arrived after rebuild 1 finished,
	// a second rebuild can legitimately fire. Upper bound is what matters —
	// not 100.
	if got < 1 || got > 2 {
		t.Errorf("expected 1-2 rebuilds (single-flight coalescing), got %d", got)
	}
}
