package database

import (
	"context"
	"testing"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/database/testdb"
	"github.com/google/uuid"
)

func TestFusionMembershipActivityTitlesUseDisplayTitlePrecedence(t *testing.T) {
	ctx := context.Background()
	pool := testdb.Open(t)
	db := &DB{Pool: pool}

	var userID string
	username := "fusion-title-" + uuid.NewString()
	if err := pool.QueryRow(ctx, `
		INSERT INTO users (username, email, registration_source)
		VALUES ($1, $2, 'test')
		RETURNING id`, username, username+"@example.test").Scan(&userID); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, userID)
	})

	var generatedID, manualID, fallbackID, trimmedID, whitespaceManualID string
	if err := pool.QueryRow(ctx, `
		INSERT INTO activities (user_id, sport_type, title, title_auto, title_is_manual, started_at)
		VALUES ($1, 'running', '   ', 'Generated title', false, $2)
		RETURNING id`, userID, time.Now().UTC()).Scan(&generatedID); err != nil {
		t.Fatalf("insert generated activity: %v", err)
	}
	if err := pool.QueryRow(ctx, `
		INSERT INTO activities (user_id, sport_type, title, title_auto, title_is_manual, started_at)
		VALUES ($1, 'cycling', 'Manual title', 'Generated manual title', true, $2)
		RETURNING id`, userID, time.Now().UTC().Add(time.Minute)).Scan(&manualID); err != nil {
		t.Fatalf("insert manual activity: %v", err)
	}
	if err := pool.QueryRow(ctx, `
		INSERT INTO activities (user_id, sport_type, title, title_auto, title_is_manual, started_at)
		VALUES ($1, 'hiking', 'Source fallback title', NULL, false, $2)
		RETURNING id`, userID, time.Now().UTC().Add(2*time.Minute)).Scan(&fallbackID); err != nil {
		t.Fatalf("insert fallback activity: %v", err)
	}
	if err := pool.QueryRow(ctx, `
		INSERT INTO activities (user_id, sport_type, title, title_auto, title_is_manual, started_at)
		VALUES ($1, 'hiking', '   ', '  Generated trimmed  ', false, $2)
		RETURNING id`, userID, time.Now().UTC().Add(3*time.Minute)).Scan(&trimmedID); err != nil {
		t.Fatalf("insert trimmed activity: %v", err)
	}
	if err := pool.QueryRow(ctx, `
		INSERT INTO activities (user_id, sport_type, title, title_auto, title_is_manual, started_at)
		VALUES ($1, 'hiking', '   ', 'Generated fallback from whitespace manual', true, $2)
		RETURNING id`, userID, time.Now().UTC().Add(4*time.Minute)).Scan(&whitespaceManualID); err != nil {
		t.Fatalf("insert whitespace manual activity: %v", err)
	}

	titles, err := db.GetActivityTitles(ctx, []string{generatedID, manualID, fallbackID, trimmedID, whitespaceManualID})
	if err != nil {
		t.Fatalf("GetActivityTitles: %v", err)
	}

	if titles[generatedID] != "Generated title" {
		t.Fatalf("generated title = %q, want title_auto", titles[generatedID])
	}
	if titles[manualID] != "Manual title" {
		t.Fatalf("manual title = %q, want manual title", titles[manualID])
	}
	if titles[fallbackID] != "Source fallback title" {
		t.Fatalf("fallback title = %q, want source title", titles[fallbackID])
	}
	if titles[trimmedID] != "Generated trimmed" {
		t.Fatalf("trimmed title = %q, want trimmed generated title", titles[trimmedID])
	}
	if titles[whitespaceManualID] != "Generated fallback from whitespace manual" {
		t.Fatalf("whitespace manual title = %q, want generated fallback", titles[whitespaceManualID])
	}

	gotSingle, err := db.GetActivityTitle(ctx, generatedID)
	if err != nil {
		t.Fatalf("GetActivityTitle: %v", err)
	}
	if gotSingle != "Generated title" {
		t.Fatalf("single title = %q, want title_auto", gotSingle)
	}
}
