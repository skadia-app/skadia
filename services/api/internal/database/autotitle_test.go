package database

import (
	"context"
	"testing"

	"github.com/skadia-app/skadia/services/api/internal/database/testdb"
	"github.com/google/uuid"
)

func TestGetAutoTitleActivityInputScopesByUser(t *testing.T) {
	ctx := context.Background()
	pool := testdb.Open(t)
	db := &DB{Pool: pool}

	var ownerID, otherID, activityID string
	ownerName := "autotitle-owner-" + uuid.NewString()
	if err := pool.QueryRow(ctx,
		`INSERT INTO users (username, email, registration_source)
		 VALUES ($1, $2, 'test') RETURNING id`,
		ownerName, ownerName+"@example.test",
	).Scan(&ownerID); err != nil {
		t.Fatalf("insert owner: %v", err)
	}
	otherName := "autotitle-other-" + uuid.NewString()
	if err := pool.QueryRow(ctx,
		`INSERT INTO users (username, email, registration_source)
		 VALUES ($1, $2, 'test') RETURNING id`,
		otherName, otherName+"@example.test",
	).Scan(&otherID); err != nil {
		t.Fatalf("insert other: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE id = ANY($1)`, []string{ownerID, otherID})
	})

	err := pool.QueryRow(ctx, `
		INSERT INTO activities (
			user_id, sport_type, sub_type, title, title_auto, title_is_manual,
			started_at, start_location, end_location, start_city, end_city,
			min_elevation_m, max_elevation_m
		) VALUES (
			$1, 'hiking', NULL, NULL, NULL, false,
			now(),
			ST_SetSRID(ST_MakePoint(12.4620, 47.7580), 4326),
			ST_SetSRID(ST_MakePoint(12.3981, 47.7431), 4326),
			'Marquartstein', 'Hochplatte',
			600, 1586
		) RETURNING id`, ownerID).Scan(&activityID)
	if err != nil {
		t.Fatalf("insert activity: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO activity_streams (activity_id, time, lat, lng, altitude_m)
		VALUES
			($1, now(), 47.7580, 12.4620, 600),
			($1, now() + interval '1 hour', 47.7431, 12.3981, 1570)`, activityID); err != nil {
		t.Fatalf("insert streams: %v", err)
	}

	got, err := db.GetAutoTitleActivityInput(ctx, ownerID, activityID)
	if err != nil {
		t.Fatalf("GetAutoTitleActivityInput owner: %v", err)
	}
	if got.SportType != "hiking" || got.StartCity != "Marquartstein" || got.EndCity != "Hochplatte" {
		t.Fatalf("unexpected input: %+v", got)
	}
	if got.StartElevationM == nil || *got.StartElevationM != 600 || got.EndElevationM == nil || *got.EndElevationM != 1570 {
		t.Fatalf("endpoint elevations = %v/%v, want 600/1570", got.StartElevationM, got.EndElevationM)
	}

	if _, err := db.GetAutoTitleActivityInput(ctx, otherID, activityID); err == nil {
		t.Fatal("GetAutoTitleActivityInput with other user succeeded, want ownership error")
	}
}

func TestUpdateActivityAutoTitleScopesByUser(t *testing.T) {
	ctx := context.Background()
	pool := testdb.Open(t)
	db := &DB{Pool: pool}

	var ownerID, otherID, activityID string
	ownerName := "autotitle-write-owner-" + uuid.NewString()
	if err := pool.QueryRow(ctx,
		`INSERT INTO users (username, email, registration_source)
		 VALUES ($1, $2, 'test') RETURNING id`,
		ownerName, ownerName+"@example.test",
	).Scan(&ownerID); err != nil {
		t.Fatalf("insert owner: %v", err)
	}
	otherName := "autotitle-write-other-" + uuid.NewString()
	if err := pool.QueryRow(ctx,
		`INSERT INTO users (username, email, registration_source)
		 VALUES ($1, $2, 'test') RETURNING id`,
		otherName, otherName+"@example.test",
	).Scan(&otherID); err != nil {
		t.Fatalf("insert other: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE id = ANY($1)`, []string{ownerID, otherID})
	})

	if err := pool.QueryRow(ctx, `
		INSERT INTO activities (user_id, sport_type, started_at)
		VALUES ($1, 'hiking', now())
		RETURNING id`, ownerID).Scan(&activityID); err != nil {
		t.Fatalf("insert activity: %v", err)
	}

	if err := db.UpdateActivityAutoTitle(ctx, otherID, activityID, "Wrong"); err == nil {
		t.Fatal("UpdateActivityAutoTitle with other user succeeded, want ownership error")
	}
	if err := db.UpdateActivityAutoTitle(ctx, ownerID, activityID, "Hochplatte ab Marquartstein"); err != nil {
		t.Fatalf("UpdateActivityAutoTitle owner: %v", err)
	}

	var titleAuto string
	if err := pool.QueryRow(ctx, `SELECT COALESCE(title_auto, '') FROM activities WHERE id = $1`, activityID).Scan(&titleAuto); err != nil {
		t.Fatalf("read title_auto: %v", err)
	}
	if titleAuto != "Hochplatte ab Marquartstein" {
		t.Fatalf("title_auto = %q, want generated title", titleAuto)
	}
}
