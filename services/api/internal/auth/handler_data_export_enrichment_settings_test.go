package auth

import (
	"context"
	"testing"
)

func TestDataExportIncludesAutoTitleDisplayPolicyScopedToUser(t *testing.T) {
	pool := openGearExportDB(t)
	ctx := context.Background()
	userA := createUserForGearTest(t, pool)
	userB := createUserForGearTest(t, pool)

	if _, err := pool.Exec(ctx, `
		INSERT INTO user_enrichment_settings (user_id, auto_title_display_policy)
		VALUES ($1, 'auto_first'), ($2, 'source_first')
	`, userA, userB); err != nil {
		t.Fatalf("insert enrichment settings: %v", err)
	}

	zipPath := runExportForUser(t, pool, userA)
	rows := readZIPEntry(t, zipPath, "data/user_enrichment_settings.json")
	if len(rows) != 1 {
		t.Fatalf("user_enrichment_settings rows = %d, want 1: %#v", len(rows), rows)
	}
	if got := rows[0]["user_id"]; got != userA {
		t.Fatalf("exported user_id = %v, want %s", got, userA)
	}
	if got := rows[0]["auto_title_display_policy"]; got != "auto_first" {
		t.Fatalf("auto_title_display_policy = %v, want auto_first", got)
	}
}
