package enrichment

import (
	"context"
	"testing"

	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/database/testdb"
	"github.com/google/uuid"
)

func autoTitleRunnerUser(t *testing.T, db *database.DB) string {
	t.Helper()
	ctx := context.Background()
	username := "run-autotitle-" + uuid.NewString()
	var userID string
	if err := db.Pool.QueryRow(ctx,
		`INSERT INTO users (username, email, registration_source)
		 VALUES ($1, $2, 'test') RETURNING id`,
		username, username+"@example.test",
	).Scan(&userID); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.Pool.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, userID)
	})
	return userID
}

func setAutoNaming(t *testing.T, db *database.DB, userID string, enabled bool) {
	t.Helper()
	ctx := context.Background()
	settings, err := db.GetEnrichmentSettings(ctx, userID)
	if err != nil {
		t.Fatalf("GetEnrichmentSettings: %v", err)
	}
	settings.EnableAutoNaming = enabled
	if err := db.UpdateEnrichmentSettings(ctx, userID, settings); err != nil {
		t.Fatalf("UpdateEnrichmentSettings: %v", err)
	}
}

func TestRunAutoTitleWritesWhenAutoNamingEnabled(t *testing.T) {
	ctx := context.Background()
	db := &database.DB{Pool: testdb.Open(t)}
	userID := autoTitleRunnerUser(t, db)
	setAutoNaming(t, db, userID, true)

	var activityID string
	if err := db.Pool.QueryRow(ctx, `
		INSERT INTO activities (
			user_id, sport_type, sub_type, started_at,
			start_location, end_location, start_city, end_city,
			min_elevation_m, max_elevation_m
		) VALUES (
			$1, 'hiking', NULL, now(),
			ST_SetSRID(ST_MakePoint(12.4620, 47.7580), 4326),
			ST_SetSRID(ST_MakePoint(12.3981, 47.7431), 4326),
			'Marquartstein', 'Hochplatte',
			600, 1586
		) RETURNING id`, userID).Scan(&activityID); err != nil {
		t.Fatalf("insert activity: %v", err)
	}
	if _, err := db.Pool.Exec(ctx, `
		INSERT INTO activity_streams (activity_id, time, lat, lng, altitude_m)
		VALUES
			($1, now(), 47.7580, 12.4620, 600),
			($1, now() + interval '1 hour', 47.7431, 12.3981, 1570)`, activityID); err != nil {
		t.Fatalf("insert streams: %v", err)
	}
	if _, err := db.Pool.Exec(ctx, `
		INSERT INTO activity_geo_tags (activity_id, category, name, lat, lng, ele_m, dist_m)
		VALUES ($1, 'summit', 'Hochplatte', 47.7430, 12.3980, 1586, 20)`, activityID); err != nil {
		t.Fatalf("insert geo tag: %v", err)
	}

	if err := RunAutoTitle(ctx, db, userID, activityID); err != nil {
		t.Fatalf("RunAutoTitle: %v", err)
	}

	var titleAuto string
	if err := db.Pool.QueryRow(ctx, `SELECT COALESCE(title_auto, '') FROM activities WHERE id = $1`, activityID).Scan(&titleAuto); err != nil {
		t.Fatalf("read title_auto: %v", err)
	}
	if titleAuto != "Hochplatte ab Marquartstein" {
		t.Fatalf("title_auto = %q, want generated title", titleAuto)
	}
}

func TestRunAutoTitleSkipsWhenAutoNamingDisabled(t *testing.T) {
	ctx := context.Background()
	db := &database.DB{Pool: testdb.Open(t)}
	userID := autoTitleRunnerUser(t, db)
	setAutoNaming(t, db, userID, false)

	var activityID string
	if err := db.Pool.QueryRow(ctx, `
		INSERT INTO activities (user_id, sport_type, title_auto, started_at, start_city)
		VALUES ($1, 'hiking', 'Existing', now(), 'Marquartstein')
		RETURNING id`, userID).Scan(&activityID); err != nil {
		t.Fatalf("insert activity: %v", err)
	}

	if err := RunAutoTitle(ctx, db, userID, activityID); err != nil {
		t.Fatalf("RunAutoTitle: %v", err)
	}

	var titleAuto string
	if err := db.Pool.QueryRow(ctx, `SELECT COALESCE(title_auto, '') FROM activities WHERE id = $1`, activityID).Scan(&titleAuto); err != nil {
		t.Fatalf("read title_auto: %v", err)
	}
	if titleAuto != "Existing" {
		t.Fatalf("title_auto = %q, want Existing", titleAuto)
	}
}
