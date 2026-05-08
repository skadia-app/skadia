package migrations

import (
	"os"
	"strings"
	"testing"
)

func TestMigration000027ActivityMetricsUsesCanonicalTrainingEffectNames(t *testing.T) {
	upBytes, err := os.ReadFile("000027_activity_metrics.up.sql")
	if err != nil {
		t.Fatalf("read up migration: %v", err)
	}
	downBytes, err := os.ReadFile("000027_activity_metrics.down.sql")
	if err != nil {
		t.Fatalf("read down migration: %v", err)
	}

	upSQL := string(upBytes)
	downSQL := string(downBytes)
	for _, want := range []string{
		"ADD COLUMN IF NOT EXISTS aerobic_training_effect",
		"ADD COLUMN IF NOT EXISTS anaerobic_training_effect",
	} {
		if !strings.Contains(upSQL, want) {
			t.Fatalf("000027 up migration missing %q", want)
		}
	}
	for _, want := range []string{
		"DROP COLUMN IF EXISTS aerobic_training_effect",
		"DROP COLUMN IF EXISTS anaerobic_training_effect",
	} {
		if !strings.Contains(downSQL, want) {
			t.Fatalf("000027 down migration missing %q", want)
		}
	}
	for _, stale := range []string{
		"training_effect_" + "aerobic",
		"training_effect_" + "anaerobic",
	} {
		if strings.Contains(upSQL, stale) || strings.Contains(downSQL, stale) {
			t.Fatalf("000027 migration still references stale column name %q", stale)
		}
	}
}
