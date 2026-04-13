package settings

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
)

// projectRoot resolves the repository root relative to this test file's location.
func projectRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	// services/api/internal/settings/ → ../../../../
	root := filepath.Join(filepath.Dir(file), "..", "..", "..", "..")
	abs, err := filepath.Abs(root)
	if err != nil {
		t.Fatalf("filepath.Abs: %v", err)
	}
	return abs
}

// TestEnvVarSync_ComposeVarsInEnvExample verifies that every ${VAR:-...} variable
// referenced in docker-compose.yml is also documented in .env.example,
// unless it is on the known-compose-only exception list.
func TestEnvVarSync_ComposeVarsInEnvExample(t *testing.T) {
	// Variables that live only in docker-compose.yml by design — they are either
	// internal Docker/Traefik plumbing, host-path overrides, _FILE secret-mount
	// variants (documented generically in .env.example), or internal feature flags.
	composeOnlyVars := map[string]bool{
		// Docker/Traefik infrastructure — not user-facing .env knobs
		"DOCKER_SOCKET":      true,
		"READONLY_PASSWORD":  true,
		"COMPOSE_PROFILES":   true,
		"TRAEFIK_ENTRYPOINT": true,
		"ACME_EMAIL":         true,
		// Host bind-mount path overrides (documented as comments in .env.example)
		"SHADOW_FS_HOST_PATH":   true,
		"HOTFOLDER_HOST_PATH":   true,
		"IMPORT_DATA_HOST_PATH": true,
		"LIBRARY_HOST_PATH":     true,
		"HOTFOLDER_ENABLED":     true,
		// Docker secrets _FILE variants — documented generically, not individually
		"S3_ACCESS_KEY_FILE":           true,
		"S3_SECRET_KEY_FILE":           true,
		"JWT_SECRET_FILE":              true,
		"OAUTH_ENCRYPTION_KEY_FILE":    true,
		"STRAVA_CLIENT_SECRET_FILE":    true,
		"GOOGLE_CLIENT_SECRET_FILE":    true,
		"LLM_ENCRYPTION_KEY_FILE":      true,
		"KOMOOT_ENCRYPTION_KEY_FILE":   true,
		"SETTINGS_ENCRYPTION_KEY_FILE": true,
		"BOOTSTRAP_ADMIN_PASSWORD_FILE": true,
		// Internal/experimental routing feature flags (not end-user config)
		"ROUTING_BIDIRECTIONAL": true,
		"ROUTING_VALLE_SEG":     true,
		"LANDMARK_STRATEGY":     true,
		"BADROUTER_PROFILE_DIR": true,
	}

	root := projectRoot(t)

	composeData, err := os.ReadFile(filepath.Join(root, "docker-compose.yml"))
	if err != nil {
		t.Fatalf("reading docker-compose.yml: %v", err)
	}

	envData, err := os.ReadFile(filepath.Join(root, ".env.example"))
	if err != nil {
		t.Fatalf("reading .env.example: %v", err)
	}
	envContent := string(envData)

	// Match ${VAR_NAME} and ${VAR_NAME:-...} — capture just the name.
	re := regexp.MustCompile(`\$\{([A-Z_][A-Z0-9_]*)(?::-[^}]*)?\}`)
	matches := re.FindAllSubmatch(composeData, -1)

	// Deduplicate.
	seen := make(map[string]bool)
	var missing []string

	for _, m := range matches {
		name := string(m[1])
		if seen[name] || composeOnlyVars[name] {
			seen[name] = true
			continue
		}
		seen[name] = true

		// Accept: "VAR_NAME=", "# VAR_NAME=", or the name appearing anywhere
		// in a comment line (e.g. documentation references).
		if !strings.Contains(envContent, name) {
			missing = append(missing, name)
		}
	}

	if len(missing) > 0 {
		t.Errorf("docker-compose.yml references variables not found in .env.example.\n"+
			"Add them to .env.example (or to composeOnlyVars if intentionally compose-only):\n  %s",
			strings.Join(missing, "\n  "))
	}
}

// TestEnvVarSync_SecretsInSetupScript verifies that every *_SECRET, *_PASSWORD,
// or *_KEY variable in .env.example that must be auto-generated is also handled
// by scripts/setup.sh.
func TestEnvVarSync_SecretsInSetupScript(t *testing.T) {
	// These are user-provided secrets — setup.sh intentionally does NOT generate
	// them because they require external accounts/infrastructure, or they have a
	// fixed dev default that the user is expected to change for production.
	userProvidedSecrets := map[string]bool{
		"STRAVA_CLIENT_SECRET": true,
		"GOOGLE_CLIENT_SECRET": true,
		"S3_SECRET_KEY":        true,
		"S3_ACCESS_KEY":        true,
		"GARAGE_ACCESS_KEY":    true,
		"GARAGE_SECRET_KEY":    true,
		"SMTP_PASSWORD":        true,
		// Falls back to OAUTH_ENCRYPTION_KEY at runtime — not auto-generated.
		"SETTINGS_ENCRYPTION_KEY": true,
		// Has a dev default in .env.example — user must change for production.
		"POSTGRES_PASSWORD": true,
	}

	root := projectRoot(t)

	envData, err := os.ReadFile(filepath.Join(root, ".env.example"))
	if err != nil {
		t.Fatalf("reading .env.example: %v", err)
	}

	setupData, err := os.ReadFile(filepath.Join(root, "scripts", "setup.sh"))
	if err != nil {
		t.Fatalf("reading scripts/setup.sh: %v", err)
	}
	setupContent := string(setupData)

	// Find non-commented lines that define a secret variable.
	// Matches lines like:  VAR_NAME=  or  VAR_NAME=value
	secretLineRe := regexp.MustCompile(`(?m)^([A-Z_][A-Z0-9_]*(?:_SECRET|_PASSWORD|_KEY))=`)
	matches := secretLineRe.FindAllStringSubmatch(string(envData), -1)

	seen := make(map[string]bool)
	var missing []string

	for _, m := range matches {
		name := m[1]
		if seen[name] || userProvidedSecrets[name] {
			seen[name] = true
			continue
		}
		seen[name] = true

		if !strings.Contains(setupContent, name) {
			missing = append(missing, name)
		}
	}

	if len(missing) > 0 {
		t.Errorf("secrets in .env.example not handled by scripts/setup.sh.\n"+
			"Add generation logic to setup.sh, or to userProvidedSecrets if user-supplied:\n  %s",
			strings.Join(missing, "\n  "))
	}
}
