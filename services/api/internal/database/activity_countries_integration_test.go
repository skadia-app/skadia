//go:build integration

package database

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/database/testdb"
	"github.com/google/uuid"
)

func TestRefreshActivityVisitedCountriesWithoutOSMBoundariesUsesPointCodes(t *testing.T) {
	pool := testdb.Open(t)
	db := New(pool)
	ctx := context.Background()
	userID := uuid.New().String()
	activityID := uuid.New().String()

	_, err := db.Pool.Exec(ctx, `
		INSERT INTO users (id, username, email, email_verified, registration_source)
		VALUES ($1, $2, $3, true, 'test')
	`, userID, "country_"+userID[:8], "country_"+userID[:8]+"@example.com")
	if err != nil {
		t.Fatalf("insert user: %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.Pool.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, userID)
	})

	_, err = db.Pool.Exec(ctx, `
		INSERT INTO activities (
			id, user_id, sport_type, started_at, start_country_code, end_country_code,
			location_country_code, visited_country_codes
		)
		VALUES ($1, $2, 'cycling', $3, 'CH', 'IT', 'AT', '{}')
	`, activityID, userID, time.Now().UTC())
	if err != nil {
		t.Fatalf("insert activity: %v", err)
	}

	if err := db.RefreshActivityVisitedCountries(ctx, activityID); err != nil {
		t.Fatalf("RefreshActivityVisitedCountries returned error without osm_boundaries: %v", err)
	}

	var got []string
	if err := db.Pool.QueryRow(ctx, `SELECT visited_country_codes FROM activities WHERE id = $1`, activityID).Scan(&got); err != nil {
		t.Fatalf("load visited countries: %v", err)
	}
	want := []string{"AT", "CH", "IT"}
	if !slices.Equal(got, want) {
		t.Fatalf("visited_country_codes = %#v, want %#v", got, want)
	}
}
