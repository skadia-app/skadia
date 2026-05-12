package database

import (
	"context"
	"testing"

	"github.com/skadia-app/skadia/services/api/internal/database/testdb"
	"github.com/google/uuid"
)

func TestActivityDisplayTitleSQLRespectsAutoTitleDisplayPolicy(t *testing.T) {
	ctx := context.Background()
	pool := testdb.Open(t)

	userID := uuid.NewString()
	if _, err := pool.Exec(ctx,
		`INSERT INTO users (id, username, email, email_verified, registration_source)
		 VALUES ($1, $2, $3, true, 'test')`,
		userID, "title-policy-"+userID[:8], "title-policy-"+userID[:8]+"@example.com"); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, userID)
	})
	if _, err := pool.Exec(ctx, `INSERT INTO user_enrichment_settings (user_id) VALUES ($1)`, userID); err != nil {
		t.Fatalf("insert settings: %v", err)
	}

	cases := []struct {
		name             string
		sourceTitle      string
		autoTitle        string
		manual           bool
		enableAutoNaming bool
		policy           string
		want             string
	}{
		{
			name:             "manual title wins",
			sourceTitle:      "Manual source",
			autoTitle:        "Generated route title",
			manual:           true,
			enableAutoNaming: true,
			policy:           "auto_first",
			want:             "Manual source",
		},
		{
			name:             "auto naming off ignores auto title",
			sourceTitle:      "Morning Ride",
			autoTitle:        "Generated route title",
			manual:           false,
			enableAutoNaming: false,
			policy:           "auto_first",
			want:             "Morning Ride",
		},
		{
			name:             "smart keeps meaningful source title",
			sourceTitle:      "Kampenwand mit Max",
			autoTitle:        "Hochplatte ab Marquartstein",
			manual:           false,
			enableAutoNaming: true,
			policy:           "source_unless_generic",
			want:             "Kampenwand mit Max",
		},
		{
			name:             "smart keeps meaningful english run source title",
			sourceTitle:      "Alpine Trail Run",
			autoTitle:        "Hochplatte ab Marquartstein",
			manual:           false,
			enableAutoNaming: true,
			policy:           "source_unless_generic",
			want:             "Alpine Trail Run",
		},
		{
			name:             "smart keeps meaningful german run source title",
			sourceTitle:      "Kampenwand-Lauf",
			autoTitle:        "Hochplatte ab Marquartstein",
			manual:           false,
			enableAutoNaming: true,
			policy:           "source_unless_generic",
			want:             "Kampenwand-Lauf",
		},
		{
			name:             "smart keeps source title that may become generic later",
			sourceTitle:      "Radfahren am Nachmittag",
			autoTitle:        "Hochplatte ab Marquartstein",
			manual:           false,
			enableAutoNaming: true,
			policy:           "source_unless_generic",
			want:             "Radfahren am Nachmittag",
		},
		{
			name:             "smart keeps localized sport time source title without active generic rules",
			sourceTitle:      "E-Mountainbike-Fahrt am Morgen",
			autoTitle:        "Hochplatte ab Marquartstein",
			manual:           false,
			enableAutoNaming: true,
			policy:           "source_unless_generic",
			want:             "E-Mountainbike-Fahrt am Morgen",
		},
		{
			name:             "smart keeps localized compound time source title without active generic rules",
			sourceTitle:      "Mittagslauf",
			autoTitle:        "Hochplatte ab Marquartstein",
			manual:           false,
			enableAutoNaming: true,
			policy:           "source_unless_generic",
			want:             "Mittagslauf",
		},
		{
			name:             "smart keeps english sport time source title without active generic rules",
			sourceTitle:      "Morning Mountain Bike Ride",
			autoTitle:        "Hochplatte ab Marquartstein",
			manual:           false,
			enableAutoNaming: true,
			policy:           "source_unless_generic",
			want:             "Morning Mountain Bike Ride",
		},
		{
			name:             "smart uses auto title when source title is empty",
			sourceTitle:      "   ",
			autoTitle:        "Hochplatte ab Marquartstein",
			manual:           false,
			enableAutoNaming: true,
			policy:           "source_unless_generic",
			want:             "Hochplatte ab Marquartstein",
		},
		{
			name:             "source first keeps source title",
			sourceTitle:      "Morning Ride",
			autoTitle:        "Generated route title",
			manual:           false,
			enableAutoNaming: true,
			policy:           "source_first",
			want:             "Morning Ride",
		},
		{
			name:             "auto first keeps generated title",
			sourceTitle:      "Kampenwand mit Max",
			autoTitle:        "Generated route title",
			manual:           false,
			enableAutoNaming: true,
			policy:           "auto_first",
			want:             "Generated route title",
		},
	}

	titleSQL := ActivityDisplayTitleSQL("a")
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := pool.Exec(ctx, `
				UPDATE user_enrichment_settings
				SET enable_auto_naming = $2,
				    auto_title_display_policy = $3
				WHERE user_id = $1`,
				userID, tc.enableAutoNaming, tc.policy); err != nil {
				t.Fatalf("update settings: %v", err)
			}

			var activityID string
			if err := pool.QueryRow(ctx, `
				INSERT INTO activities (user_id, sport_type, title, title_auto, title_is_manual, started_at)
				VALUES ($1, 'cycling', $2, $3, $4, now())
				RETURNING id`,
				userID, tc.sourceTitle, tc.autoTitle, tc.manual).Scan(&activityID); err != nil {
				t.Fatalf("insert activity: %v", err)
			}
			t.Cleanup(func() {
				_, _ = pool.Exec(context.Background(), `DELETE FROM activities WHERE id = $1`, activityID)
			})

			var got string
			query := `SELECT ` + titleSQL + ` FROM activities a WHERE a.id = $1`
			if err := pool.QueryRow(ctx, query, activityID).Scan(&got); err != nil {
				t.Fatalf("resolve title: %v", err)
			}
			if got != tc.want {
				t.Fatalf("display title = %q, want %q", got, tc.want)
			}
		})
	}
}
