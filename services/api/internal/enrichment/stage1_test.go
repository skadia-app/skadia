//go:build integration

package enrichment

// Integration tests for RunStage1 using Stage1Deps fakes.
//
// These tests require a running Postgres instance (with migrations applied)
// accessible via TEST_DATABASE_URL. They are tagged `integration` and skipped
// when the env var is absent.
//
// What the fakes buy us: no network calls to the weather API or PostGIS OSM
// boundaries table — the fakes track call counts and return controlled data.
// RunStage1's direct DB operations (GetUserProfile, UpdateEnrichmentFields,
// GetEnrichmentSettings, etc.) still use the real pool, which is why a live
// database is required.
//
// Call graph shortcuts used in every test:
//   - pub = nil          → no NATS; skips Stage-2 event publish
//   - graphReader = nil  → skips surface/stream enrichment graph look-ups
//   - skipGeo = true     → skips RunGeoEnrichment (no osm_boundaries needed)
//   - skipStreamEnrichment = true → skips RunStreamEnrichment
//   - asyncWeather = false → fetchAndInsertWeather runs synchronously (testable)

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/model"
)

// ---------------------------------------------------------------------------
// Fakes
// ---------------------------------------------------------------------------

// fakeWeather records how many times FetchReadings was called and returns
// a configurable slice of readings (or a configurable error).
type fakeWeather struct {
	called   int
	readings []model.WeatherReading
	err      error
}

func (f *fakeWeather) FetchReadings(ctx context.Context, reqs []WeatherRequest) ([]model.WeatherReading, error) {
	f.called++
	if f.err != nil {
		return nil, f.err
	}
	return f.readings, nil
}

// fakeGeocoder records calls and returns a configurable GeoResult (or nil).
type fakeGeocoder struct {
	calls  int       // total Reverse calls
	lats   []float64 // lat arguments in call order
	result *GeoResult
	err    error
}

func (f *fakeGeocoder) Reverse(ctx context.Context, db *database.DB, userID string, lat, lng float64) (*GeoResult, error) {
	f.calls++
	f.lats = append(f.lats, lat)
	if f.err != nil {
		return nil, f.err
	}
	return f.result, nil
}

// ---------------------------------------------------------------------------
// Test helpers (follow the pattern in intervals_retract_integration_test.go)
// ---------------------------------------------------------------------------

// stage1TestPool opens a pgxpool from TEST_DATABASE_URL; skips if not set.
func stage1TestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set — skipping integration test")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("connect to test DB: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

// stage1TestUser inserts a minimal user row and registers cleanup.
func stage1TestUser(t *testing.T, pool *pgxpool.Pool) string {
	t.Helper()
	ctx := context.Background()
	userID := uuid.New().String()
	_, err := pool.Exec(ctx,
		`INSERT INTO users (id, username, email, email_verified, registration_source)
		 VALUES ($1, $2, $3, true, 'test')`,
		userID,
		"stage1_test_"+userID[:8],
		"stage1_test_"+userID[:8]+"@example.com",
	)
	if err != nil {
		t.Fatalf("insert user: %v", err)
	}
	t.Cleanup(func() {
		bg := context.Background()
		pool.Exec(bg, "DELETE FROM activity_weather WHERE activity_id IN (SELECT id FROM activities WHERE user_id = $1)", userID) //nolint:errcheck
		pool.Exec(bg, "DELETE FROM activity_tags WHERE activity_id IN (SELECT id FROM activities WHERE user_id = $1)", userID)   //nolint:errcheck
		pool.Exec(bg, "DELETE FROM activities WHERE user_id = $1", userID)                                                       //nolint:errcheck
		pool.Exec(bg, "DELETE FROM tags WHERE user_id = $1", userID)                                                             //nolint:errcheck
		pool.Exec(bg, "DELETE FROM users WHERE id = $1", userID)                                                                 //nolint:errcheck
	})
	return userID
}

// stage1TestActivity inserts a minimal activity row and returns its ID.
func stage1TestActivity(t *testing.T, pool *pgxpool.Pool, userID string) string {
	t.Helper()
	ctx := context.Background()
	activityID := uuid.New().String()
	now := time.Now().UTC().Truncate(time.Second)
	_, err := pool.Exec(ctx, `
		INSERT INTO activities (id, user_id, sport_type, started_at, ended_at, file_hash, has_gps)
		VALUES ($1, $2, 'cycling', $3, $4, $5, false)
	`, activityID, userID, now, now.Add(30*time.Minute), uuid.New().String())
	if err != nil {
		t.Fatalf("insert activity: %v", err)
	}
	return activityID
}

// minimalData builds the minimal ActivityData for a GPS-equipped activity.
// Adjust fields per test case.
func minimalData(startedAt time.Time) *model.ActivityData {
	return &model.ActivityData{
		SportType:    "cycling",
		StartedAt:    startedAt,
		EndedAt:      startedAt.Add(30 * time.Minute),
		ElapsedTimeS: 1800,
		HasGPS:       true,
		StartLat:     48.137154, // Munich
		StartLng:     11.576124,
		EndLat:       48.140000,
		EndLng:       11.580000,
	}
}

// readActivity fetches the enrichment fields we care about from the activities table.
func readActivity(t *testing.T, pool *pgxpool.Pool, activityID string) (timezone string, titleAuto string) {
	t.Helper()
	err := pool.QueryRow(context.Background(),
		`SELECT COALESCE(timezone,''), COALESCE(title_auto,'') FROM activities WHERE id = $1`,
		activityID,
	).Scan(&timezone, &titleAuto)
	if err != nil {
		t.Fatalf("read activity fields: %v", err)
	}
	return
}

// weatherReadingCount returns how many rows are in activity_weather for an activity.
func weatherReadingCount(t *testing.T, pool *pgxpool.Pool, activityID string) int {
	t.Helper()
	var n int
	err := pool.QueryRow(context.Background(),
		`SELECT COUNT(*) FROM activity_weather WHERE activity_id = $1`,
		activityID,
	).Scan(&n)
	if err != nil {
		t.Fatalf("count weather rows: %v", err)
	}
	return n
}

// ---------------------------------------------------------------------------
// Test cases
// ---------------------------------------------------------------------------

// TestRunStage1_HappyPath_GPS exercises the nominal path: GPS activity, weather
// and geocoder both return data. Asserts that:
//   - weatherFetcher was called exactly once
//   - geocoder was called for both start AND end coordinates
//   - timezone field is set (tzf lookup from GPS)
//   - title_auto is generated
func TestRunStage1_HappyPath_GPS(t *testing.T) {
	pool := stage1TestPool(t)
	db := &database.DB{Pool: pool}
	userID := stage1TestUser(t, pool)
	activityID := stage1TestActivity(t, pool, userID)

	city := "München"
	country := "Deutschland"
	geo := &GeoResult{City: city, Country: country}

	fw := &fakeWeather{
		readings: []model.WeatherReading{
			{ReadingType: "single", RecordedAt: time.Now()},
		},
	}
	fg := &fakeGeocoder{result: geo}
	deps := &Stage1Deps{Weather: fw, Geocoder: fg}

	startedAt := time.Now().UTC().Add(-time.Hour)
	data := minimalData(startedAt)

	RunStage1(context.Background(), db, userID, activityID, data,
		nil,   // pub — no NATS
		false, // asyncWeather — synchronous for testability
		true,  // skipStreamEnrichment
		nil,   // graphReader
		true,  // skipGeo — no osm table required
		deps,
	)

	// Weather was called once (synchronous path).
	if fw.called != 1 {
		t.Errorf("weatherFetcher.called = %d, want 1", fw.called)
	}

	// Geocoder called for start AND end (two calls).
	if fg.calls != 2 {
		t.Errorf("geocoder.calls = %d, want 2 (start + end)", fg.calls)
	}

	// Timezone should be set from tzf GPS lookup.
	tz, titleAuto := readActivity(t, pool, activityID)
	if tz == "" {
		t.Errorf("timezone is empty, expected IANA timezone from GPS coords")
	}

	// Auto title should be generated (cycling + city → non-empty).
	if titleAuto == "" {
		t.Errorf("title_auto is empty, expected auto-generated title")
	}
}

// TestRunStage1_WeatherAPIFailure verifies that a weather fetch error does not
// cause RunStage1 to panic or skip other fields. Timezone and title_auto must
// still be set; weather readings must NOT be inserted (clean abort).
func TestRunStage1_WeatherAPIFailure(t *testing.T) {
	pool := stage1TestPool(t)
	db := &database.DB{Pool: pool}
	userID := stage1TestUser(t, pool)
	activityID := stage1TestActivity(t, pool, userID)

	fw := &fakeWeather{err: errWeatherSimulated}
	fg := &fakeGeocoder{result: &GeoResult{City: "Teststadt"}}
	deps := &Stage1Deps{Weather: fw, Geocoder: fg}

	startedAt := time.Now().UTC().Add(-time.Hour)
	data := minimalData(startedAt)

	// Must not panic.
	RunStage1(context.Background(), db, userID, activityID, data,
		nil, false, true, nil, true, deps)

	// Weather fetcher was called (it errored, but was attempted).
	if fw.called != 1 {
		t.Errorf("weatherFetcher.called = %d, want 1", fw.called)
	}

	// No weather rows inserted (fetchAndInsertWeather returns early on error).
	if n := weatherReadingCount(t, pool, activityID); n != 0 {
		t.Errorf("activity_weather rows = %d, want 0 after fetch error", n)
	}

	// Other enrichment (timezone, title_auto) must still be set.
	tz, titleAuto := readActivity(t, pool, activityID)
	if tz == "" {
		t.Errorf("timezone empty after weather failure — other fields should still be set")
	}
	if titleAuto == "" {
		t.Errorf("title_auto empty after weather failure — other fields should still be set")
	}
}

// errWeatherSimulated is the sentinel error for TestRunStage1_WeatherAPIFailure.
// Declared at package level so it is accessible from the file-level var block.
var errWeatherSimulated = simWeatherErr("simulated weather API failure")

type simWeatherErr string

func (e simWeatherErr) Error() string { return string(e) }

// TestRunStage1_ReverseGeocodeMiss verifies the nil-result path: geocoder
// returns (nil, nil) → no start_city/start_country fields, but other
// enrichment (timezone, title_auto) still happens.
func TestRunStage1_ReverseGeocodeMiss(t *testing.T) {
	pool := stage1TestPool(t)
	db := &database.DB{Pool: pool}
	userID := stage1TestUser(t, pool)
	activityID := stage1TestActivity(t, pool, userID)

	fw := &fakeWeather{} // returns (nil, nil) — no readings
	fg := &fakeGeocoder{result: nil}
	deps := &Stage1Deps{Weather: fw, Geocoder: fg}

	startedAt := time.Now().UTC().Add(-time.Hour)
	data := minimalData(startedAt)

	RunStage1(context.Background(), db, userID, activityID, data,
		nil, false, true, nil, true, deps)

	// Geocoder was still called (just returned nothing useful).
	if fg.calls == 0 {
		t.Errorf("geocoder.calls = 0, expected at least 1 (start coords)")
	}

	// No start_city stored — verify via direct query.
	var startCity *string
	err := pool.QueryRow(context.Background(),
		`SELECT start_city FROM activities WHERE id = $1`, activityID,
	).Scan(&startCity)
	if err != nil {
		t.Fatalf("read start_city: %v", err)
	}
	if startCity != nil && *startCity != "" {
		t.Errorf("start_city = %q, want empty/nil after geocode miss", *startCity)
	}

	// Timezone still set from GPS.
	tz, _ := readActivity(t, pool, activityID)
	if tz == "" {
		t.Errorf("timezone empty after geocode miss — should still be set from GPS")
	}
}

// TestRunStage1_NoGPS_WeatherAndGeoSkipped verifies that for an activity
// without GPS data (HasGPS=false, StartLat=0), neither the weather fetcher
// nor the geocoder is called.
func TestRunStage1_NoGPS_WeatherAndGeoSkipped(t *testing.T) {
	pool := stage1TestPool(t)
	db := &database.DB{Pool: pool}
	userID := stage1TestUser(t, pool)
	activityID := stage1TestActivity(t, pool, userID)

	fw := &fakeWeather{}
	fg := &fakeGeocoder{}
	deps := &Stage1Deps{Weather: fw, Geocoder: fg}

	startedAt := time.Now().UTC().Add(-time.Hour)
	data := &model.ActivityData{
		SportType:    "cycling",
		StartedAt:    startedAt,
		EndedAt:      startedAt.Add(30 * time.Minute),
		ElapsedTimeS: 1800,
		HasGPS:       false,
		// StartLat / StartLng remain zero
	}

	RunStage1(context.Background(), db, userID, activityID, data,
		nil, false, true, nil, true, deps)

	if fw.called != 0 {
		t.Errorf("weatherFetcher called %d times for no-GPS activity, want 0", fw.called)
	}
	if fg.calls != 0 {
		t.Errorf("geocoder called %d times for no-GPS activity, want 0", fg.calls)
	}
}

// TestRunStage1_VirtualActivity_WeatherAndGeoSkipped verifies that for a
// virtual activity (IsVirtual=true), weather and geocoder are NOT called
// even though StartLat/StartLng are non-zero. Virtual activities have
// simulated GPS; weather at a simulated location is irrelevant.
func TestRunStage1_VirtualActivity_WeatherAndGeoSkipped(t *testing.T) {
	pool := stage1TestPool(t)
	db := &database.DB{Pool: pool}
	userID := stage1TestUser(t, pool)
	activityID := stage1TestActivity(t, pool, userID)

	fw := &fakeWeather{}
	fg := &fakeGeocoder{}
	deps := &Stage1Deps{Weather: fw, Geocoder: fg}

	startedAt := time.Now().UTC().Add(-time.Hour)
	data := &model.ActivityData{
		SportType:    "cycling",
		StartedAt:    startedAt,
		EndedAt:      startedAt.Add(30 * time.Minute),
		ElapsedTimeS: 1800,
		HasGPS:       true,
		IsVirtual:    true,
		StartLat:     48.137154,
		StartLng:     11.576124,
	}

	RunStage1(context.Background(), db, userID, activityID, data,
		nil, false, true, nil, true, deps)

	if fw.called != 0 {
		t.Errorf("weatherFetcher called %d times for virtual activity, want 0", fw.called)
	}
	if fg.calls != 0 {
		t.Errorf("geocoder called %d times for virtual activity, want 0", fg.calls)
	}

	// Virtual activities must set exclude_from_heatmap=true.
	var excludeFromHeatmap bool
	err := pool.QueryRow(context.Background(),
		`SELECT COALESCE(exclude_from_heatmap, false) FROM activities WHERE id = $1`,
		activityID,
	).Scan(&excludeFromHeatmap)
	if err != nil {
		t.Fatalf("read exclude_from_heatmap: %v", err)
	}
	if !excludeFromHeatmap {
		t.Errorf("exclude_from_heatmap = false for virtual activity, want true")
	}
}
