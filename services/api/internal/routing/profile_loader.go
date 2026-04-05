package routing

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
)

// Profile source constants used across the application.
const (
	SourceSystem     = "system"
	SourceFilesystem = "filesystem"
	SourceUser       = "user"
)

// loadedBuiltins holds built-in profiles loaded from config/badrouter/*.json.
// Thread-safe: written via Store at startup, read via Load on every routing request.
// nil (zero value) means LoadBuiltinProfiles was not called yet.
var loadedBuiltins atomic.Pointer[map[string]ProfileConfig]

// LoadProfilesFromDir reads *.json profile files from dir, parses and validates them.
// Returns a map of name→ProfileConfig and aggregated errors for failed files.
// Missing or empty dir returns an empty map and nil error.
func LoadProfilesFromDir(dir string) (map[string]ProfileConfig, error) {
	matches, err := filepath.Glob(filepath.Join(dir, "*.json"))
	if err != nil {
		return nil, fmt.Errorf("LoadProfilesFromDir: glob %q: %w", dir, err)
	}

	loaded := make(map[string]ProfileConfig, len(matches))
	var fileErrors []string

	for _, path := range matches {
		name := strings.TrimSuffix(filepath.Base(path), ".json")

		data, err := os.ReadFile(path)
		if err != nil {
			fileErrors = append(fileErrors, fmt.Sprintf("%s: read failed: %v", name, err))
			continue
		}

		var cfg ProfileConfig
		if err := json.Unmarshal(data, &cfg); err != nil {
			fileErrors = append(fileErrors, fmt.Sprintf("%s: parse failed: %v", name, err))
			continue
		}

		if err := ValidateProfileConfig(cfg); err != nil {
			fileErrors = append(fileErrors, fmt.Sprintf("%s: validation failed: %v", name, err))
			continue
		}

		if cfg.Name == "" {
			cfg.Name = name
		}

		if len(cfg.SportTypes) == 0 {
			cfg.SportTypes = DefaultSportTypes(cfg.Profile)
		}

		loaded[name] = cfg
	}

	if len(fileErrors) > 0 {
		return loaded, fmt.Errorf("%d file(s) failed: %s",
			len(fileErrors), strings.Join(fileErrors, "; "))
	}
	return loaded, nil
}

// LoadBuiltinProfiles reads *.json files from dir and registers them as built-in
// profiles. Replaces any previous set atomically. Always sets the internal map
// (even if empty), so BuiltinProfile can distinguish "not called" from "no profiles".
// Per-file errors are collected and returned as an aggregated error alongside the
// count of successful loads.
//
// In addition to the base validation in LoadProfilesFromDir, this enforces that
// each file's profile field matches its filename and that all builtin-required
// fields are present (ValidateBuiltinCompleteness).
func LoadBuiltinProfiles(dir string) (int, error) {
	profiles, err := LoadProfilesFromDir(dir)

	// Even on partial error we have the successfully parsed profiles.
	// Apply builtin-specific checks and filter out invalid ones.
	filtered := make(map[string]ProfileConfig, len(profiles))
	var builtinErrors []string

	for name, cfg := range profiles {
		if cfg.Profile != name {
			builtinErrors = append(builtinErrors, fmt.Sprintf("%s: profile field mismatch (expected %q, got %q)", name, name, cfg.Profile))
			continue
		}
		if err := ValidateBuiltinCompleteness(cfg); err != nil {
			builtinErrors = append(builtinErrors, fmt.Sprintf("%s: incomplete: %v", name, err))
			continue
		}
		filtered[name] = cfg
	}

	loadedBuiltins.Store(&filtered)

	// Aggregate errors from both LoadProfilesFromDir and builtin checks.
	var allErrors []string
	if err != nil {
		allErrors = append(allErrors, err.Error())
	}
	allErrors = append(allErrors, builtinErrors...)

	if len(allErrors) > 0 {
		return len(filtered), fmt.Errorf("%s", strings.Join(allErrors, "; "))
	}
	return len(filtered), nil
}

// ValidateBuiltinCompleteness checks that all fields required by the routing
// engine are non-nil. These 10 fields are set in every built-in profile.
func ValidateBuiltinCompleteness(cfg ProfileConfig) error {
	if cfg.UphillCost == nil {
		return fmt.Errorf("uphill_cost is required")
	}
	if cfg.DownhillCost == nil {
		return fmt.Errorf("downhill_cost is required")
	}
	if cfg.UphillCutoffPct == nil {
		return fmt.Errorf("uphill_cutoff_pct is required")
	}
	if cfg.DownhillCutoffPct == nil {
		return fmt.Errorf("downhill_cutoff_pct is required")
	}
	if cfg.ElevPenaltyBuffer == nil {
		return fmt.Errorf("elev_penalty_buffer_m is required")
	}
	if cfg.ElevMaxBuffer == nil {
		return fmt.Errorf("elev_max_buffer_m is required")
	}
	if cfg.ElevBufferReduce == nil {
		return fmt.Errorf("elev_buffer_reduce is required")
	}
	if cfg.AllowSteps == nil {
		return fmt.Errorf("allow_steps is required")
	}
	if cfg.TurnCost == nil {
		return fmt.Errorf("turn_cost is required")
	}
	if cfg.Pass0Coefficient == nil {
		return fmt.Errorf("pass0_coefficient is required")
	}
	return nil
}

// DefaultSportTypes returns the default sport type associations for a profile type.
func DefaultSportTypes(profile string) []string {
	switch profile {
	case "hiking":
		return []string{"hiking", "trail_running", "running", "walking", "mountaineering"}
	case "mtb":
		return []string{"mountain_biking"}
	case "gravel":
		return []string{"gravel_cycling"}
	case "trekking":
		return []string{"cycling"}
	case "road":
		return []string{"cycling", "gravel_cycling"}
	default:
		return []string{}
	}
}
