package settings

import (
	"testing"
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
		"routing",
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
