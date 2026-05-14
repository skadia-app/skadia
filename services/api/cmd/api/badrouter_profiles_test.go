package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBadRouterStartupCoreMissingProfilesDoesNotFatal(t *testing.T) {
	status := loadBadRouterProfilesForStartup("core", "env", filepath.Join(t.TempDir(), "missing"))
	if status.Fatal {
		t.Fatalf("core missing profiles marked fatal: %#v", status)
	}
	if status.Available {
		t.Fatalf("core missing profiles marked available: %#v", status)
	}
	if status.Reason != "core_profile" {
		t.Fatalf("reason = %q, want core_profile", status.Reason)
	}
}

func TestBadRouterStartupImplicitLegacyMissingProfilesDoesNotFatal(t *testing.T) {
	status := loadBadRouterProfilesForStartup("full", "implicit_legacy", filepath.Join(t.TempDir(), "missing"))
	if status.Fatal {
		t.Fatalf("implicit legacy missing profiles marked fatal: %#v", status)
	}
	if status.Configured || status.Available {
		t.Fatalf("implicit legacy missing profiles should be unconfigured/unavailable: %#v", status)
	}
	if status.Reason != "missing_profiles" {
		t.Fatalf("reason = %q, want missing_profiles", status.Reason)
	}
}

func TestBadRouterStartupExplicitFullMissingProfilesIsUnavailable(t *testing.T) {
	status := loadBadRouterProfilesForStartup("full", "env", filepath.Join(t.TempDir(), "missing"))
	if status.Fatal {
		t.Fatalf("explicit full missing profiles marked fatal: %#v", status)
	}
	if !status.Configured || status.Available {
		t.Fatalf("explicit full missing profiles should be configured/unavailable: %#v", status)
	}
	if status.Reason != "missing_profiles" {
		t.Fatalf("reason = %q, want missing_profiles", status.Reason)
	}
}

func TestBadRouterStartupMalformedProfilesReportedDistinctly(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "hiking.json"), []byte("{"), 0o644); err != nil {
		t.Fatal(err)
	}
	status := loadBadRouterProfilesForStartup("full", "env", dir)
	if status.Fatal {
		t.Fatalf("malformed profile marked fatal: %#v", status)
	}
	if status.Reason != "malformed_profiles" {
		t.Fatalf("reason = %q, want malformed_profiles", status.Reason)
	}
	if !strings.Contains(status.Error, "parse failed") {
		t.Fatalf("error = %q, want parse failed detail", status.Error)
	}
}
