package dataimport

import (
	"context"
	"testing"

	"github.com/google/uuid"
)

func TestImportUserEnrichmentSettingsPreservesAutoTitleDisplayPolicy(t *testing.T) {
	pool := testDB(t)
	userID := createTestUser(t, pool)
	sourceUserID := uuid.New().String()
	settingsID := uuid.New().String()

	zipPath := buildExportZip(t, zipBuildOpts{
		sourceUserID: sourceUserID,
		columns: map[string][]string{
			"user_enrichment_settings": {"id", "user_id", "enable_auto_naming", "auto_title_display_policy"},
		},
		extraFiles: map[string][]byte{
			"data/user_enrichment_settings.json": mustMarshalJSON(t, []map[string]any{{
				"id":                        settingsID,
				"user_id":                   sourceUserID,
				"enable_auto_naming":        true,
				"auto_title_display_policy": "auto_first",
			}}),
		},
	})

	eng := newEngine(pool)
	if _, err := eng.RunImport(context.Background(), ImportInput{
		Source:       openZipSource(t, zipPath),
		TargetUserID: userID,
	}); err != nil {
		t.Fatalf("RunImport: %v", err)
	}

	var policy string
	if err := pool.QueryRow(context.Background(), `
		SELECT auto_title_display_policy
		FROM user_enrichment_settings
		WHERE user_id = $1
	`, userID).Scan(&policy); err != nil {
		t.Fatalf("read imported enrichment settings: %v", err)
	}
	if policy != "auto_first" {
		t.Fatalf("auto_title_display_policy = %q, want auto_first", policy)
	}
}

func TestImportUserEnrichmentSettingsDefaultsMissingAutoTitleDisplayPolicy(t *testing.T) {
	pool := testDB(t)
	userID := createTestUser(t, pool)
	sourceUserID := uuid.New().String()
	settingsID := uuid.New().String()

	zipPath := buildExportZip(t, zipBuildOpts{
		sourceUserID: sourceUserID,
		columns: map[string][]string{
			"user_enrichment_settings": {"id", "user_id", "enable_auto_naming"},
		},
		extraFiles: map[string][]byte{
			"data/user_enrichment_settings.json": mustMarshalJSON(t, []map[string]any{{
				"id":                 settingsID,
				"user_id":            sourceUserID,
				"enable_auto_naming": true,
			}}),
		},
	})

	eng := newEngine(pool)
	if _, err := eng.RunImport(context.Background(), ImportInput{
		Source:       openZipSource(t, zipPath),
		TargetUserID: userID,
	}); err != nil {
		t.Fatalf("RunImport: %v", err)
	}

	var policy string
	if err := pool.QueryRow(context.Background(), `
		SELECT auto_title_display_policy
		FROM user_enrichment_settings
		WHERE user_id = $1
	`, userID).Scan(&policy); err != nil {
		t.Fatalf("read imported enrichment settings: %v", err)
	}
	if policy != "source_unless_generic" {
		t.Fatalf("auto_title_display_policy = %q, want source_unless_generic", policy)
	}
}
