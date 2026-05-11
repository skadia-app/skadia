package handler

import (
	"os"
	"os/exec"
	"strings"
	"testing"

	gearpkg "github.com/skadia-app/skadia/services/api/internal/gear"
)

func TestNormalizeGearDefaultSportsForStorageAllowsImportedDefaults(t *testing.T) {
	got, err := gearpkg.NormalizeGearDefaultSports([]string{"fitness_equipment"})
	if err != nil {
		t.Fatalf("NormalizeGearDefaultSports() error = %v", err)
	}
	if len(got) != 1 || got[0] != "fitness_equipment" {
		t.Fatalf("normalized sports = %#v, want []string{fitness_equipment}", got)
	}
}

func TestExtractPDFTextDoesNotExitWhenConfigHomeIsNotWritable(t *testing.T) {
	if os.Getenv("SKADIA_TEST_EXTRACT_PDF_TEXT_CHILD") == "1" {
		_ = extractPDFText([]byte("%PDF-1.4\n%%EOF\n"))
		return
	}

	cmd := exec.Command(os.Args[0], "-test.run=TestExtractPDFTextDoesNotExitWhenConfigHomeIsNotWritable")
	cmd.Env = append(withoutEnv(os.Environ(), "HOME", "XDG_CONFIG_HOME"),
		"SKADIA_TEST_EXTRACT_PDF_TEXT_CHILD=1",
		"HOME=/nonexistent",
		"XDG_CONFIG_HOME=/nonexistent",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("extractPDFText child exited unexpectedly: %v\n%s", err, out)
	}
}

func withoutEnv(env []string, keys ...string) []string {
	blocked := make(map[string]struct{}, len(keys))
	for _, key := range keys {
		blocked[key] = struct{}{}
	}

	filtered := env[:0]
	for _, entry := range env {
		key, _, ok := strings.Cut(entry, "=")
		if !ok {
			continue
		}
		if _, drop := blocked[key]; drop {
			continue
		}
		filtered = append(filtered, entry)
	}
	return filtered
}
