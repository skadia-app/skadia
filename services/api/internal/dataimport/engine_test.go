package dataimport

import (
	"archive/zip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/skadia-app/skadia/services/api/internal/shadowfs"
)

// ---------------------------------------------------------------------------
// DB helpers
// ---------------------------------------------------------------------------

func testDB(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Skip("DATABASE_URL not set")
	}
	pool, err := pgxpool.New(context.Background(), dbURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { pool.Close() })
	return pool
}

// createTestUser inserts a minimal user row and returns its UUID.
// A t.Cleanup deletes the row (CASCADE handles child rows).
func createTestUser(t *testing.T, pool *pgxpool.Pool) string {
	t.Helper()
	id := uuid.New().String()
	email := fmt.Sprintf("test-%s@example.com", id[:8])
	username := fmt.Sprintf("testuser_%s", id[:8])

	_, err := pool.Exec(context.Background(), `
		INSERT INTO users (id, username, email, display_name, timezone)
		VALUES ($1, $2, $3, 'Test User', 'UTC')
	`, id, username, email)
	if err != nil {
		t.Fatalf("createTestUser: %v", err)
	}

	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), "DELETE FROM users WHERE id = $1", id)
	})
	return id
}

// assertRowCount checks COUNT(*) FROM table WHERE user_id = userID.
func assertRowCount(t *testing.T, pool *pgxpool.Pool, table, userID string, expected int) {
	t.Helper()
	var count int
	err := pool.QueryRow(context.Background(),
		fmt.Sprintf("SELECT COUNT(*) FROM %q WHERE user_id = $1", table),
		userID,
	).Scan(&count)
	if err != nil {
		t.Fatalf("assertRowCount(%s): %v", table, err)
	}
	if count != expected {
		t.Errorf("assertRowCount(%s, user=%s): want %d, got %d", table, userID, expected, count)
	}
}

// currentSchemaMigration returns the MAX(version) from schema_migrations.
func currentSchemaMigration(t *testing.T, pool *pgxpool.Pool) int {
	t.Helper()
	var v int
	if err := pool.QueryRow(context.Background(),
		"SELECT COALESCE(MAX(version), 0) FROM schema_migrations",
	).Scan(&v); err != nil {
		t.Fatalf("currentSchemaMigration: %v", err)
	}
	return v
}

// ---------------------------------------------------------------------------
// mockShadowFS
// ---------------------------------------------------------------------------

// mockShadowFS implements shadowfs.ShadowFS in memory.
// Put() stores file content and returns a synthetic FileRecord.
// All other methods are no-ops or return reasonable zero values.
type mockShadowFS struct {
	mu    sync.Mutex
	files map[string][]byte // fileID → content
}

func newMockShadowFS() *mockShadowFS {
	return &mockShadowFS{files: make(map[string][]byte)}
}

func (m *mockShadowFS) Put(_ context.Context, entry shadowfs.FileEntry, data io.Reader) (shadowfs.FileRecord, error) {
	content, err := io.ReadAll(data)
	if err != nil {
		return shadowfs.FileRecord{}, err
	}
	id := uuid.New().String()
	m.mu.Lock()
	m.files[id] = content
	m.mu.Unlock()
	return shadowfs.FileRecord{
		ID:        id,
		UserID:    entry.UserID,
		FileType:  entry.FileType,
		SizeBytes: int64(len(content)),
		Status:    "active",
	}, nil
}

func (m *mockShadowFS) PutCompanion(_ context.Context, _, _, _ string, _ []byte) error {
	return nil
}

func (m *mockShadowFS) Get(_ context.Context, _, _ string) (io.ReadCloser, error) {
	return io.NopCloser(strings.NewReader("")), nil
}

func (m *mockShadowFS) Delete(_ context.Context, _, _ string) error         { return nil }
func (m *mockShadowFS) DeleteAllForUser(_ context.Context, _ string) error  { return nil }
func (m *mockShadowFS) DeleteCompanions(_ context.Context, _, _ string) error { return nil }

func (m *mockShadowFS) List(_ context.Context, _ string, _ *string) ([]shadowfs.FileRecord, error) {
	return nil, nil
}

func (m *mockShadowFS) StreamToZip(_ context.Context, _ string, _ *zip.Writer) ([]string, error) {
	return nil, nil
}

func (m *mockShadowFS) Resolve(_ context.Context, _, _ string) (shadowfs.ResolvedFile, error) {
	return shadowfs.ResolvedFile{}, nil
}

func (m *mockShadowFS) DiskUsage(_ context.Context, _ string) (int64, error) { return 0, nil }

func (m *mockShadowFS) Archive(_ context.Context, _, _, _ string) error { return nil }

func (m *mockShadowFS) GetCompanion(_ context.Context, _, _, _ string) ([]byte, error) {
	return nil, nil
}

// ---------------------------------------------------------------------------
// buildExportZip
// ---------------------------------------------------------------------------

// zipBuildOpts configures what tables/rows go into the test export ZIP.
type zipBuildOpts struct {
	// sourceUserID is embedded in manifest.json user_id.
	// Defaults to a random UUID if empty.
	sourceUserID string

	// schemaMigration overrides the manifest's schema_migration.
	// Defaults to 293 if zero.
	schemaMigration int

	// profile is written to data/profile.json.
	// nil = omit the file entirely.
	profile map[string]any

	// tags is written to data/tags.json. nil = omit.
	tags []map[string]any

	// trainingZones is written to data/training_zones.json. nil = omit.
	trainingZones []map[string]any

	// activities is written to data/activities.json. nil = omit.
	activities []map[string]any

	// gear is written to data/gear.json. nil = omit.
	gear []map[string]any

	// segmentFavorites is written to data/segment_favorites.json. nil = omit.
	segmentFavorites []map[string]any

	// segments is written to data/segments.json. nil = omit.
	segments []map[string]any

	// oauthConnections is written to data/oauth_connections.json. nil = omit.
	oauthConnections []map[string]any

	// activityTags is written to data/activity_tags.json. nil = omit.
	activityTags []map[string]any

	// columns is written to columns.json. nil = omit.
	columns map[string][]string

	// extraFiles allows injecting arbitrary entries (path → bytes).
	extraFiles map[string][]byte
}

// buildExportZip creates a temp ZIP file according to opts and returns its path.
// t.Cleanup removes the file.
func buildExportZip(t *testing.T, opts zipBuildOpts) string {
	t.Helper()

	if opts.sourceUserID == "" {
		opts.sourceUserID = uuid.New().String()
	}
	if opts.schemaMigration == 0 {
		opts.schemaMigration = 293
	}

	f, err := os.CreateTemp(t.TempDir(), "gdpr-export-*.zip")
	if err != nil {
		t.Fatalf("buildExportZip: create temp: %v", err)
	}
	defer f.Close()

	zw := zip.NewWriter(f)

	writeJSON := func(name string, v any) {
		t.Helper()
		w, err := zw.Create(name)
		if err != nil {
			t.Fatalf("buildExportZip: create entry %q: %v", name, err)
		}
		if err := json.NewEncoder(w).Encode(v); err != nil {
			t.Fatalf("buildExportZip: encode %q: %v", name, err)
		}
	}

	// manifest.json — always written
	writeJSON("manifest.json", map[string]any{
		"format_version":   1,
		"skadia_version":  "2026.04",
		"schema_migration": opts.schemaMigration,
		"exported_at":      time.Now().UTC().Format(time.RFC3339),
		"user_id":          opts.sourceUserID,
	})

	// columns.json — optional
	if opts.columns != nil {
		writeJSON("columns.json", opts.columns)
	}

	// profile.json — optional
	if opts.profile != nil {
		writeJSON("data/profile.json", opts.profile)
	}

	// data tables — optional
	type tableEntry struct {
		path string
		rows []map[string]any
	}
	tables := []tableEntry{
		{"data/tags.json", opts.tags},
		{"data/training_zones.json", opts.trainingZones},
		{"data/activities.json", opts.activities},
		{"data/gear.json", opts.gear},
		{"data/segment_favorites.json", opts.segmentFavorites},
		{"data/segments.json", opts.segments},
		{"data/oauth_connections.json", opts.oauthConnections},
		{"data/activity_tags.json", opts.activityTags},
	}
	for _, te := range tables {
		if te.rows != nil {
			writeJSON(te.path, te.rows)
		}
	}

	// Extra arbitrary files.
	for path, content := range opts.extraFiles {
		w, err := zw.Create(path)
		if err != nil {
			t.Fatalf("buildExportZip: create extra entry %q: %v", path, err)
		}
		if _, err := w.Write(content); err != nil {
			t.Fatalf("buildExportZip: write extra entry %q: %v", path, err)
		}
	}

	if err := zw.Close(); err != nil {
		t.Fatalf("buildExportZip: close zip: %v", err)
	}

	return f.Name()
}

// newEngine is a convenience constructor for tests.
func newEngine(pool *pgxpool.Pool) *Engine {
	return &Engine{
		DB:       pool,
		ShadowFS: newMockShadowFS(),
		Config:   EngineConfig{},
	}
}

// openZipSource opens a ZipSource from path, failing the test on error.
func openZipSource(t *testing.T, path string) *ZipSource {
	t.Helper()
	src, err := NewZipSource(path)
	if err != nil {
		t.Fatalf("NewZipSource(%s): %v", path, err)
	}
	t.Cleanup(func() { src.Close() })
	return src
}

// ---------------------------------------------------------------------------
// Test 1: TestProfileAllowList
// ---------------------------------------------------------------------------

// TestProfileAllowList verifies that profile import overwrites only allow-listed
// fields and leaves protected fields (email, username) unchanged.
func TestProfileAllowList(t *testing.T) {
	pool := testDB(t)
	userID := createTestUser(t, pool)

	// Capture original email + username before import.
	var origEmail, origUsername string
	if err := pool.QueryRow(context.Background(),
		"SELECT COALESCE(email,''), username FROM users WHERE id = $1", userID,
	).Scan(&origEmail, &origUsername); err != nil {
		t.Fatalf("select original user: %v", err)
	}

	zipPath := buildExportZip(t, zipBuildOpts{
		profile: map[string]any{
			"email":        "hacker@evil.com",
			"username":     "hacked",
			"timezone":     "Europe/Berlin",
			"display_name": "Imported Name",
		},
	})
	src := openZipSource(t, zipPath)

	eng := newEngine(pool)
	_, err := eng.RunImport(context.Background(), ImportInput{
		Source:       src,
		TargetUserID: userID,
	})
	if err != nil {
		t.Fatalf("RunImport: %v", err)
	}

	var gotEmail, gotUsername, gotTimezone, gotDisplayName string
	if err := pool.QueryRow(context.Background(),
		"SELECT COALESCE(email,''), username, COALESCE(timezone,''), COALESCE(display_name,'') FROM users WHERE id = $1",
		userID,
	).Scan(&gotEmail, &gotUsername, &gotTimezone, &gotDisplayName); err != nil {
		t.Fatalf("select updated user: %v", err)
	}

	if gotEmail != origEmail {
		t.Errorf("email should be unchanged: want %q, got %q", origEmail, gotEmail)
	}
	if gotUsername != origUsername {
		t.Errorf("username should be unchanged: want %q, got %q", origUsername, gotUsername)
	}
	if gotTimezone != "Europe/Berlin" {
		t.Errorf("timezone should be imported: want %q, got %q", "Europe/Berlin", gotTimezone)
	}
	if gotDisplayName != "Imported Name" {
		t.Errorf("display_name should be imported: want %q, got %q", "Imported Name", gotDisplayName)
	}
}

// ---------------------------------------------------------------------------
// Test 2: TestConflictMergeID
// ---------------------------------------------------------------------------

// TestConflictMergeID verifies that importing a tag with a name that already
// exists for the user reuses the existing tag's ID and does not create a duplicate.
// The activity that references the tag must end up pointing at the existing tag.
func TestConflictMergeID(t *testing.T) {
	pool := testDB(t)
	userID := createTestUser(t, pool)

	// Create an existing tag "Commute" in the DB.
	existingTagID := uuid.New().String()
	_, err := pool.Exec(context.Background(), `
		INSERT INTO tags (id, user_id, name, color, text_color, category)
		VALUES ($1, $2, 'Commute', '#000000', '#FFFFFF', 'custom')
	`, existingTagID, userID)
	if err != nil {
		t.Fatalf("insert existing tag: %v", err)
	}

	// Build an activity that we'll insert first, then we'll link via activity_tags.
	// The activity needs to exist before phase 6 runs.
	activityOldID := uuid.New().String()
	tagOldID := uuid.New().String() // different UUID than existingTagID

	now := time.Now().UTC()

	zipPath := buildExportZip(t, zipBuildOpts{
		columns: map[string][]string{
			"tags":          {"id", "user_id", "name", "color", "text_color", "category"},
			"activities":    {"id", "user_id", "title", "sport_type", "started_at"},
			"activity_tags": {"activity_id", "tag_id", "assigned_by"},
		},
		tags: []map[string]any{
			{
				"id":         tagOldID,
				"user_id":    uuid.New().String(), // source user — will be remapped
				"name":       "Commute",
				"color":      "#FF0000",
				"text_color": "#000000",
				"category":   "custom",
			},
		},
		activities: []map[string]any{
			{
				"id":         activityOldID,
				"user_id":    uuid.New().String(),
				"title":      "Morning Commute",
				"sport_type": "cycling",
				"started_at": now.Format(time.RFC3339),
			},
		},
		activityTags: []map[string]any{
			{
				"activity_id": activityOldID,
				"tag_id":      tagOldID,
				"assigned_by": "manual",
			},
		},
	})
	src := openZipSource(t, zipPath)

	eng := newEngine(pool)
	result, err := eng.RunImport(context.Background(), ImportInput{
		Source:       src,
		TargetUserID: userID,
	})
	if err != nil {
		t.Fatalf("RunImport: %v", err)
	}
	_ = result

	// Exactly one tag named "Commute" for this user.
	assertRowCount(t, pool, "tags", userID, 1)

	// The activity_tags row must reference the existing tag ID.
	var linkedTagID string
	err = pool.QueryRow(context.Background(), `
		SELECT at.tag_id
		FROM activity_tags at
		JOIN activities a ON a.id = at.activity_id
		WHERE a.user_id = $1
		  AND a.title   = 'Morning Commute'
	`, userID).Scan(&linkedTagID)
	if err != nil {
		t.Fatalf("select activity_tags: %v", err)
	}
	if linkedTagID != existingTagID {
		t.Errorf("activity_tags.tag_id: want existing %q, got %q", existingTagID, linkedTagID)
	}
}

// ---------------------------------------------------------------------------
// Test 3: TestConflictReplace
// ---------------------------------------------------------------------------

// TestConflictReplace verifies that cmReplace tables (training_zones) delete
// pre-existing rows and then insert the imported ones.
func TestConflictReplace(t *testing.T) {
	pool := testDB(t)
	userID := createTestUser(t, pool)

	// Insert an existing training zone that should be wiped.
	_, err := pool.Exec(context.Background(), `
		INSERT INTO training_zones (id, user_id, sport, sub_sport, zone_type, source_type)
		VALUES (gen_random_uuid(), $1, 2, 0, 'heart_rate', 'manual')
	`, userID)
	if err != nil {
		t.Fatalf("insert existing training_zone: %v", err)
	}

	assertRowCount(t, pool, "training_zones", userID, 1)

	newZoneID := uuid.New().String()
	zipPath := buildExportZip(t, zipBuildOpts{
		columns: map[string][]string{
			"training_zones": {"id", "user_id", "sport", "sub_sport", "zone_type", "source_type"},
		},
		trainingZones: []map[string]any{
			{
				"id":          newZoneID,
				"user_id":     uuid.New().String(),
				"sport":       4, // running
				"sub_sport":   0,
				"zone_type":   "power",
				"source_type": "device",
			},
		},
	})
	src := openZipSource(t, zipPath)

	eng := newEngine(pool)
	_, err = eng.RunImport(context.Background(), ImportInput{
		Source:       src,
		TargetUserID: userID,
	})
	if err != nil {
		t.Fatalf("RunImport: %v", err)
	}

	// Old zone gone, new zone imported.
	assertRowCount(t, pool, "training_zones", userID, 1)

	var gotSport int
	var gotZoneType string
	if err := pool.QueryRow(context.Background(),
		"SELECT sport, zone_type FROM training_zones WHERE user_id = $1",
		userID,
	).Scan(&gotSport, &gotZoneType); err != nil {
		t.Fatalf("select training_zone: %v", err)
	}
	if gotSport != 4 {
		t.Errorf("training_zones.sport: want 4 (running), got %d", gotSport)
	}
	if gotZoneType != "power" {
		t.Errorf("training_zones.zone_type: want %q, got %q", "power", gotZoneType)
	}
}

// ---------------------------------------------------------------------------
// Test 4: TestConflictSkipWarn
// ---------------------------------------------------------------------------

// TestConflictSkipWarn verifies that segment_favorites referencing a segment
// that does not exist on the target server are skipped with a warning.
func TestConflictSkipWarn(t *testing.T) {
	pool := testDB(t)
	userID := createTestUser(t, pool)

	nonexistentSegmentID := uuid.New().String()

	zipPath := buildExportZip(t, zipBuildOpts{
		columns: map[string][]string{
			"segment_favorites": {"user_id", "segment_id"},
		},
		segmentFavorites: []map[string]any{
			{
				"user_id":    uuid.New().String(),
				"segment_id": nonexistentSegmentID,
			},
		},
	})
	src := openZipSource(t, zipPath)

	eng := newEngine(pool)
	result, err := eng.RunImport(context.Background(), ImportInput{
		Source:       src,
		TargetUserID: userID,
	})
	if err != nil {
		t.Fatalf("RunImport: %v", err)
	}

	// No segment_favorites should be imported.
	var count int
	if err := pool.QueryRow(context.Background(),
		"SELECT COUNT(*) FROM segment_favorites WHERE user_id = $1", userID,
	).Scan(&count); err != nil {
		t.Fatalf("count segment_favorites: %v", err)
	}
	if count != 0 {
		t.Errorf("segment_favorites count: want 0, got %d", count)
	}

	// At least one warning should mention the skip.
	hasWarn := false
	for _, w := range result.Warnings {
		if strings.Contains(strings.ToLower(w), "segment_favorites") ||
			strings.Contains(strings.ToLower(w), "skipped") {
			hasWarn = true
			break
		}
	}
	if !hasWarn {
		t.Errorf("expected a skip warning for segment_favorites, got warnings: %v", result.Warnings)
	}
}

// ---------------------------------------------------------------------------
// Test 5: TestDeferredFK
// ---------------------------------------------------------------------------

// TestDeferredFK verifies that deferred FK columns (segments.source_activity_id,
// gear.source_document_id) are correctly remapped after the activity is inserted.
func TestDeferredFK(t *testing.T) {
	pool := testDB(t)
	userID := createTestUser(t, pool)

	srcUser := uuid.New().String()
	actOldID := uuid.New().String()
	segOldID := uuid.New().String()

	now := time.Now().UTC()

	zipPath := buildExportZip(t, zipBuildOpts{
		columns: map[string][]string{
			"activities": {"id", "user_id", "title", "sport_type", "started_at"},
			"segments": {
				"id", "created_by", "name", "sport_group", "segment_type",
				"source_activity_id",
			},
		},
		activities: []map[string]any{
			{
				"id":         actOldID,
				"user_id":    srcUser,
				"title":      "Segment Source Run",
				"sport_type": "running",
				"started_at": now.Format(time.RFC3339),
			},
		},
		segments: []map[string]any{
			{
				"id":                 segOldID,
				"created_by":         srcUser,
				"name":               "Test Segment",
				"sport_group":        "running",
				"segment_type":       "standard",
				"source_activity_id": actOldID, // deferred FK
			},
		},
	})
	src := openZipSource(t, zipPath)

	eng := &Engine{
		DB:       pool,
		ShadowFS: newMockShadowFS(),
		Config:   EngineConfig{},
	}
	eng.deferredFKs = nil // reset for clarity

	_, err := eng.RunImport(context.Background(), ImportInput{
		Source:       src,
		TargetUserID: userID,
	})
	if err != nil {
		t.Fatalf("RunImport: %v", err)
	}

	// segments.source_activity_id must not be NULL.
	var srcActID *string
	if err := pool.QueryRow(context.Background(),
		"SELECT source_activity_id FROM segments WHERE created_by = $1",
		userID,
	).Scan(&srcActID); err != nil {
		t.Fatalf("select segment: %v", err)
	}
	if srcActID == nil {
		t.Error("segments.source_activity_id should not be NULL after deferred FK resolution")
	}
}

// ---------------------------------------------------------------------------
// Test 6: TestOAuthReconnect
// ---------------------------------------------------------------------------

// TestOAuthReconnect verifies that imported oauth_connections have their
// access_token, refresh_token, and token_expiry nulled out.
func TestOAuthReconnect(t *testing.T) {
	pool := testDB(t)
	userID := createTestUser(t, pool)

	connOldID := uuid.New().String()

	zipPath := buildExportZip(t, zipBuildOpts{
		columns: map[string][]string{
			"oauth_connections": {
				"id", "user_id", "provider", "provider_id",
				"access_token", "refresh_token", "token_expiry",
			},
		},
		oauthConnections: []map[string]any{
			{
				"id":            connOldID,
				"user_id":       uuid.New().String(),
				"provider":      "strava",
				"provider_id":   "strava-123",
				"access_token":  "super_secret_token",
				"refresh_token": "super_secret_refresh",
				"token_expiry":  time.Now().Add(time.Hour).UTC().Format(time.RFC3339),
			},
		},
	})
	src := openZipSource(t, zipPath)

	eng := newEngine(pool)
	_, err := eng.RunImport(context.Background(), ImportInput{
		Source:       src,
		TargetUserID: userID,
	})
	if err != nil {
		t.Fatalf("RunImport: %v", err)
	}

	// access_token and refresh_token must be NULL.
	var accessToken, refreshToken *string
	var tokenExpiry *time.Time
	if err := pool.QueryRow(context.Background(),
		"SELECT access_token, refresh_token, token_expiry FROM oauth_connections WHERE user_id = $1",
		userID,
	).Scan(&accessToken, &refreshToken, &tokenExpiry); err != nil {
		t.Fatalf("select oauth_connection: %v", err)
	}

	if accessToken != nil {
		t.Errorf("access_token should be NULL, got %q", *accessToken)
	}
	if refreshToken != nil {
		t.Errorf("refresh_token should be NULL, got %q", *refreshToken)
	}
	if tokenExpiry != nil {
		t.Errorf("token_expiry should be NULL, got %v", *tokenExpiry)
	}
}

// ---------------------------------------------------------------------------
// Test 7: TestNullFKRemapping
// ---------------------------------------------------------------------------

// TestNullFKRemapping verifies that an activity with gear_id=null retains NULL
// after import (the IDMap must not substitute an empty-string placeholder).
func TestNullFKRemapping(t *testing.T) {
	pool := testDB(t)
	userID := createTestUser(t, pool)

	actOldID := uuid.New().String()
	now := time.Now().UTC()

	zipPath := buildExportZip(t, zipBuildOpts{
		columns: map[string][]string{
			"activities": {"id", "user_id", "title", "sport_type", "started_at", "gear_id"},
		},
		activities: []map[string]any{
			{
				"id":         actOldID,
				"user_id":    uuid.New().String(),
				"title":      "Gear-less Ride",
				"sport_type": "cycling",
				"started_at": now.Format(time.RFC3339),
				"gear_id":    nil, // explicitly null
			},
		},
	})
	src := openZipSource(t, zipPath)

	eng := newEngine(pool)
	_, err := eng.RunImport(context.Background(), ImportInput{
		Source:       src,
		TargetUserID: userID,
	})
	if err != nil {
		t.Fatalf("RunImport: %v", err)
	}

	var gearID *string
	if err := pool.QueryRow(context.Background(),
		"SELECT gear_id FROM activities WHERE user_id = $1 AND title = 'Gear-less Ride'",
		userID,
	).Scan(&gearID); err != nil {
		t.Fatalf("select activity gear_id: %v", err)
	}

	if gearID != nil {
		t.Errorf("gear_id should be NULL for gear-less activity, got %q", *gearID)
	}
}

// ---------------------------------------------------------------------------
// Test 8: TestDeletedAtNull
// ---------------------------------------------------------------------------

// TestDeletedAtNull verifies that deleted_at from the export is always set to
// NULL on import (soft-deleted activities are restored).
func TestDeletedAtNull(t *testing.T) {
	pool := testDB(t)
	userID := createTestUser(t, pool)

	// Only run if activities table has a deleted_at column.
	var hasDeletedAt bool
	if err := pool.QueryRow(context.Background(), `
		SELECT EXISTS (
			SELECT 1 FROM information_schema.columns
			WHERE table_schema = 'public'
			  AND table_name   = 'activities'
			  AND column_name  = 'deleted_at'
		)
	`).Scan(&hasDeletedAt); err != nil {
		t.Fatalf("check deleted_at column: %v", err)
	}
	if !hasDeletedAt {
		t.Skip("activities.deleted_at column not present in this schema version")
	}

	actOldID := uuid.New().String()
	now := time.Now().UTC()

	zipPath := buildExportZip(t, zipBuildOpts{
		columns: map[string][]string{
			"activities": {"id", "user_id", "title", "sport_type", "started_at", "deleted_at"},
		},
		activities: []map[string]any{
			{
				"id":         actOldID,
				"user_id":    uuid.New().String(),
				"title":      "Deleted Activity",
				"sport_type": "running",
				"started_at": now.Format(time.RFC3339),
				"deleted_at": "2026-01-01T00:00:00Z",
			},
		},
	})
	src := openZipSource(t, zipPath)

	eng := newEngine(pool)
	_, err := eng.RunImport(context.Background(), ImportInput{
		Source:       src,
		TargetUserID: userID,
	})
	if err != nil {
		t.Fatalf("RunImport: %v", err)
	}

	var deletedAt *time.Time
	if err := pool.QueryRow(context.Background(),
		"SELECT deleted_at FROM activities WHERE user_id = $1 AND title = 'Deleted Activity'",
		userID,
	).Scan(&deletedAt); err != nil {
		t.Fatalf("select activity deleted_at: %v", err)
	}

	if deletedAt != nil {
		t.Errorf("deleted_at should be NULL after import, got %v", *deletedAt)
	}
}

// ---------------------------------------------------------------------------
// Test 9: TestMutualExclusion
// ---------------------------------------------------------------------------

// TestMutualExclusion verifies that RunImport returns an error when a
// data_export or data_import job is already running for the target user.
func TestMutualExclusion(t *testing.T) {
	pool := testDB(t)
	userID := createTestUser(t, pool)

	// Insert a running data_export job.
	jobID := uuid.New().String()
	_, err := pool.Exec(context.Background(), `
		INSERT INTO background_jobs (id, user_id, job_type, status)
		VALUES ($1, $2, 'data_export', 'running')
	`, jobID, userID)
	if err != nil {
		t.Fatalf("insert background_job: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), "DELETE FROM background_jobs WHERE id = $1", jobID)
	})

	zipPath := buildExportZip(t, zipBuildOpts{})
	src := openZipSource(t, zipPath)

	eng := newEngine(pool)
	_, err = eng.RunImport(context.Background(), ImportInput{
		Source:       src,
		TargetUserID: userID,
	})

	if err == nil {
		t.Fatal("RunImport should have returned an error due to running export job")
	}
	if !strings.Contains(err.Error(), "already running") {
		t.Errorf("error should mention 'already running', got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Test 10: TestSchemaVersionCheck
// ---------------------------------------------------------------------------

// TestSchemaVersionCheck verifies that RunImport rejects exports whose
// schema_migration is higher than the server's current migration.
func TestSchemaVersionCheck(t *testing.T) {
	pool := testDB(t)
	userID := createTestUser(t, pool)

	current := currentSchemaMigration(t, pool)
	future := current + 1000 // definitely higher than current

	zipPath := buildExportZip(t, zipBuildOpts{
		schemaMigration: future,
	})
	src := openZipSource(t, zipPath)

	eng := newEngine(pool)
	_, err := eng.RunImport(context.Background(), ImportInput{
		Source:       src,
		TargetUserID: userID,
	})

	if err == nil {
		t.Fatal("RunImport should have returned an error for future schema_migration")
	}
	if !strings.Contains(err.Error(), "upgrade the server first") {
		t.Errorf("error should mention 'upgrade the server first', got: %v", err)
	}
}
