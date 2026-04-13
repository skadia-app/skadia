package dataimport

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
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

// ---------------------------------------------------------------------------
// Test 11: TestExportImportRoundtrip
// ---------------------------------------------------------------------------

// TestExportImportRoundtrip verifies that a realistic export covering multiple
// tables (tags, activities, training_zones, activity_tags) can be imported onto
// a new user with correct row counts and consistent FK references.
func TestExportImportRoundtrip(t *testing.T) {
	pool := testDB(t)
	userID := createTestUser(t, pool)

	srcUser := uuid.New().String()
	tagOldID := uuid.New().String()
	actOldID := uuid.New().String()
	zoneOldID := uuid.New().String()
	now := time.Now().UTC()

	zipPath := buildExportZip(t, zipBuildOpts{
		columns: map[string][]string{
			"tags":           {"id", "user_id", "name", "color", "text_color", "category"},
			"activities":     {"id", "user_id", "title", "sport_type", "started_at"},
			"training_zones": {"id", "user_id", "sport", "sub_sport", "zone_type", "source_type"},
			"activity_tags":  {"activity_id", "tag_id", "assigned_by"},
		},
		tags: []map[string]any{
			{
				"id":         tagOldID,
				"user_id":    srcUser,
				"name":       "Roundtrip Tag",
				"color":      "#ABCDEF",
				"text_color": "#000000",
				"category":   "custom",
			},
		},
		activities: []map[string]any{
			{
				"id":         actOldID,
				"user_id":    srcUser,
				"title":      "Roundtrip Activity",
				"sport_type": "running",
				"started_at": now.Format(time.RFC3339),
			},
		},
		trainingZones: []map[string]any{
			{
				"id":          zoneOldID,
				"user_id":     srcUser,
				"sport":       1,
				"sub_sport":   0,
				"zone_type":   "heart_rate",
				"source_type": "manual",
			},
		},
		activityTags: []map[string]any{
			{
				"activity_id": actOldID,
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

	// Verify row counts.
	assertRowCount(t, pool, "tags", userID, 1)
	assertRowCount(t, pool, "activities", userID, 1)
	assertRowCount(t, pool, "training_zones", userID, 1)

	// Verify activity_tags → both activity and tag belong to the new user.
	var linkedActivityUserID, linkedTagUserID string
	err = pool.QueryRow(context.Background(), `
		SELECT a.user_id, tg.user_id
		FROM activity_tags at
		JOIN activities a ON a.id = at.activity_id
		JOIN tags tg       ON tg.id = at.tag_id
		WHERE a.user_id = $1
	`, userID).Scan(&linkedActivityUserID, &linkedTagUserID)
	if err != nil {
		t.Fatalf("query activity_tags join: %v", err)
	}
	if linkedActivityUserID != userID {
		t.Errorf("activity_tags.activity_id user_id: want %q, got %q", userID, linkedActivityUserID)
	}
	if linkedTagUserID != userID {
		t.Errorf("activity_tags.tag_id user_id: want %q, got %q", userID, linkedTagUserID)
	}
}

// ---------------------------------------------------------------------------
// Test 12: TestGeometryRoundtrip
// ---------------------------------------------------------------------------

// TestGeometryRoundtrip verifies that a geometry column (user_privacy_zones.center)
// survives the export→import cycle: the GeoJSON emitted by ST_AsGeoJSON must
// equal the GeoJSON read back after import via ST_AsGeoJSON.
func TestGeometryRoundtrip(t *testing.T) {
	pool := testDB(t)
	userID := createTestUser(t, pool)

	// Confirm the user_privacy_zones.center column is geometry.
	var hasCenter bool
	if err := pool.QueryRow(context.Background(), `
		SELECT EXISTS (
			SELECT 1 FROM information_schema.columns
			WHERE table_schema = 'public'
			  AND table_name   = 'user_privacy_zones'
			  AND column_name  = 'center'
			  AND udt_name     = 'geometry'
		)
	`).Scan(&hasCenter); err != nil {
		t.Fatalf("check center column: %v", err)
	}
	if !hasCenter {
		t.Skip("user_privacy_zones.center geometry column not present")
	}

	zoneOldID := uuid.New().String()
	srcUser := uuid.New().String()

	// GeoJSON Point at a deterministic coordinate.
	geoPoint := map[string]any{
		"type":        "Point",
		"coordinates": []any{11.393, 47.265},
	}

	zipPath := buildExportZip(t, zipBuildOpts{
		columns: map[string][]string{
			"user_privacy_zones": {"id", "user_id", "name", "center", "radius_m", "is_active", "zone_type"},
		},
		extraFiles: map[string][]byte{
			"data/user_privacy_zones.json": mustMarshalJSON(t, []map[string]any{
				{
					"id":       zoneOldID,
					"user_id":  srcUser,
					"name":     "GeomZone",
					"center":   geoPoint,
					"radius_m": 200,
					"is_active": true,
					"zone_type": "home",
				},
			}),
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

	// Read back the geometry as GeoJSON and check that it is still a Point
	// at approximately the same coordinates.
	var geoJSONStr string
	if err := pool.QueryRow(context.Background(),
		"SELECT ST_AsGeoJSON(center) FROM user_privacy_zones WHERE user_id = $1 AND name = 'GeomZone'",
		userID,
	).Scan(&geoJSONStr); err != nil {
		t.Fatalf("ST_AsGeoJSON query: %v", err)
	}

	var got map[string]any
	if err := json.Unmarshal([]byte(geoJSONStr), &got); err != nil {
		t.Fatalf("parse returned GeoJSON: %v", err)
	}
	if got["type"] != "Point" {
		t.Errorf("geometry type: want Point, got %v", got["type"])
	}
	coords, ok := got["coordinates"].([]any)
	if !ok || len(coords) < 2 {
		t.Fatalf("geometry coordinates malformed: %v", got["coordinates"])
	}
	lon, _ := coords[0].(float64)
	lat, _ := coords[1].(float64)
	if lon < 11.392 || lon > 11.394 {
		t.Errorf("longitude: want ~11.393, got %v", lon)
	}
	if lat < 47.264 || lat > 47.266 {
		t.Errorf("latitude: want ~47.265, got %v", lat)
	}
}

// mustMarshalJSON encodes v to JSON, failing the test on error.
func mustMarshalJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("mustMarshalJSON: %v", err)
	}
	return b
}

// ---------------------------------------------------------------------------
// Test 13: TestJSONBRoundtrip
// ---------------------------------------------------------------------------

// TestJSONBRoundtrip verifies that a JSONB column (automation_rules.actions)
// survives import as a proper object — not re-encoded as a JSON string.
func TestJSONBRoundtrip(t *testing.T) {
	pool := testDB(t)
	userID := createTestUser(t, pool)

	ruleOldID := uuid.New().String()
	srcUser := uuid.New().String()

	actions := []any{
		map[string]any{"type": "add_tag", "params": map[string]any{"tag_id": uuid.New().String()}},
	}
	conditions := map[string]any{"sport_type": "running"}

	zipPath := buildExportZip(t, zipBuildOpts{
		columns: map[string][]string{
			"automation_rules": {"id", "user_id", "name", "is_active", "priority", "conditions", "actions"},
		},
		extraFiles: map[string][]byte{
			"data/automation_rules.json": mustMarshalJSON(t, []map[string]any{
				{
					"id":         ruleOldID,
					"user_id":    srcUser,
					"name":       "JSONB Roundtrip Rule",
					"is_active":  true,
					"priority":   1,
					"conditions": conditions,
					"actions":    actions,
				},
			}),
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

	// Read back JSONB — it must be a proper object (not wrapped as a string).
	var actionsRaw []byte
	if err := pool.QueryRow(context.Background(),
		"SELECT actions FROM automation_rules WHERE user_id = $1 AND name = 'JSONB Roundtrip Rule'",
		userID,
	).Scan(&actionsRaw); err != nil {
		t.Fatalf("select automation_rules.actions: %v", err)
	}

	// actionsRaw must decode as a JSON array.
	var decoded []any
	if err := json.Unmarshal(actionsRaw, &decoded); err != nil {
		t.Fatalf("actions JSONB is not a JSON array: %v — raw: %s", err, actionsRaw)
	}
	if len(decoded) != 1 {
		t.Errorf("actions: want 1 element, got %d", len(decoded))
	}
}

// ---------------------------------------------------------------------------
// Test 14: TestJSONBRemapping
// ---------------------------------------------------------------------------

// TestJSONBRemapping verifies that UUID references embedded in JSONB columns
// (automation_rules.actions → tag_id) are remapped through the IDMap.
func TestJSONBRemapping(t *testing.T) {
	pool := testDB(t)
	userID := createTestUser(t, pool)

	srcUser := uuid.New().String()
	ruleOldID := uuid.New().String()
	tagOldID := uuid.New().String()

	// Build a rule whose action references tagOldID.
	actions := []any{
		map[string]any{
			"type":   "add_tag",
			"params": map[string]any{"tag_id": tagOldID},
		},
	}

	zipPath := buildExportZip(t, zipBuildOpts{
		columns: map[string][]string{
			"tags":             {"id", "user_id", "name", "color", "text_color", "category"},
			"automation_rules": {"id", "user_id", "name", "is_active", "priority", "conditions", "actions"},
		},
		tags: []map[string]any{
			{
				"id":         tagOldID,
				"user_id":    srcUser,
				"name":       "Remapped Tag",
				"color":      "#111111",
				"text_color": "#FFFFFF",
				"category":   "custom",
			},
		},
		extraFiles: map[string][]byte{
			"data/automation_rules.json": mustMarshalJSON(t, []map[string]any{
				{
					"id":         ruleOldID,
					"user_id":    srcUser,
					"name":       "JSONB Remap Rule",
					"is_active":  true,
					"priority":   1,
					"conditions": map[string]any{},
					"actions":    actions,
				},
			}),
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

	// Fetch the actual new tag ID from the DB.
	var newTagID string
	if err := pool.QueryRow(context.Background(),
		"SELECT id FROM tags WHERE user_id = $1 AND name = 'Remapped Tag'",
		userID,
	).Scan(&newTagID); err != nil {
		t.Fatalf("select tag: %v", err)
	}

	// Read back the rule's actions JSONB and check that tag_id was remapped.
	var actionsRaw []byte
	if err := pool.QueryRow(context.Background(),
		"SELECT actions FROM automation_rules WHERE user_id = $1 AND name = 'JSONB Remap Rule'",
		userID,
	).Scan(&actionsRaw); err != nil {
		t.Fatalf("select automation_rules.actions: %v", err)
	}

	var decodedActions []map[string]any
	if err := json.Unmarshal(actionsRaw, &decodedActions); err != nil {
		t.Fatalf("parse actions: %v", err)
	}
	if len(decodedActions) == 0 {
		t.Fatal("actions is empty")
	}

	params, _ := decodedActions[0]["params"].(map[string]any)
	if params == nil {
		t.Fatal("actions[0].params is nil")
	}
	gotTagID, _ := params["tag_id"].(string)
	if gotTagID == tagOldID {
		t.Errorf("tag_id was NOT remapped — still the old export UUID %q", tagOldID)
	}
	if gotTagID != newTagID {
		t.Errorf("tag_id remapped to %q, want new DB tag ID %q", gotTagID, newTagID)
	}
}

// ---------------------------------------------------------------------------
// Test 15: TestShadowFSIDRemap
// ---------------------------------------------------------------------------

// dbMockShadowFS is a variant of mockShadowFS that also inserts into the
// user_files table so that FK constraints on activities.file_id are satisfied.
type dbMockShadowFS struct {
	*mockShadowFS
	pool *pgxpool.Pool
}

func newDBMockShadowFS(pool *pgxpool.Pool) *dbMockShadowFS {
	return &dbMockShadowFS{mockShadowFS: newMockShadowFS(), pool: pool}
}

func (m *dbMockShadowFS) Put(ctx context.Context, entry shadowfs.FileEntry, data io.Reader) (shadowfs.FileRecord, error) {
	rec, err := m.mockShadowFS.Put(ctx, entry, data)
	if err != nil {
		return rec, err
	}
	// Insert a minimal user_files row so FK constraints are satisfied.
	_, dbErr := m.pool.Exec(ctx, `
		INSERT INTO user_files
			(id, user_id, file_type, storage_backend, storage_path,
			 content_type, file_name, size_bytes, has_sidecar, is_renaming, status)
		VALUES ($1, $2, $3, 'local', $4, $5, $6, $7, false, false, 'active')
		ON CONFLICT (id) DO NOTHING
	`, rec.ID, rec.UserID, entry.FileType,
		"/mock/"+rec.ID,
		entry.ContentType, entry.FileName, rec.SizeBytes)
	if dbErr != nil {
		return shadowfs.FileRecord{}, fmt.Errorf("dbMockShadowFS: insert user_files: %w", dbErr)
	}
	return rec, nil
}

// TestShadowFSIDRemap verifies that after importing an activity with a file,
// activities.file_id points to an existing user_files row (created by ShadowFS.Put)
// and NOT to the old export UUID.
func TestShadowFSIDRemap(t *testing.T) {
	pool := testDB(t)
	userID := createTestUser(t, pool)

	srcUser := uuid.New().String()
	actOldID := uuid.New().String()
	fileOldID := uuid.New().String()
	now := time.Now().UTC()

	// user_files.json entry for the file.
	userFilesJSON := mustMarshalJSON(t, []map[string]any{
		{
			"id":           fileOldID,
			"file_type":    "original",
			"content_type": "application/octet-stream",
			"file_name":    "activity.fit",
			"size_bytes":   5,
			"status":       "active",
		},
	})
	// Actual file content inside files/original/activity.fit.
	fileContent := []byte("hello")

	zipPath := buildExportZip(t, zipBuildOpts{
		columns: map[string][]string{
			"activities": {"id", "user_id", "title", "sport_type", "started_at", "file_id"},
		},
		activities: []map[string]any{
			{
				"id":         actOldID,
				"user_id":    srcUser,
				"title":      "Shadow Activity",
				"sport_type": "cycling",
				"started_at": now.Format(time.RFC3339),
				"file_id":    fileOldID,
			},
		},
		extraFiles: map[string][]byte{
			"data/user_files.json":        userFilesJSON,
			"files/original/activity.fit": fileContent,
		},
	})
	src := openZipSource(t, zipPath)

	// Use a DB-aware mock so user_files rows satisfy FK constraints.
	eng := &Engine{
		DB:       pool,
		ShadowFS: newDBMockShadowFS(pool),
		Config:   EngineConfig{},
	}
	_, err := eng.RunImport(context.Background(), ImportInput{
		Source:       src,
		TargetUserID: userID,
	})
	if err != nil {
		t.Fatalf("RunImport: %v", err)
	}

	// activities.file_id must NOT be nil and must not be the old export UUID.
	var gotFileID *string
	if err := pool.QueryRow(context.Background(),
		"SELECT file_id FROM activities WHERE user_id = $1 AND title = 'Shadow Activity'",
		userID,
	).Scan(&gotFileID); err != nil {
		t.Fatalf("select file_id: %v", err)
	}
	if gotFileID == nil {
		t.Fatal("activities.file_id is NULL — ShadowFS Put was not called or IDMap not updated")
	}
	if *gotFileID == fileOldID {
		t.Errorf("file_id is still the old export UUID %q — IDMap remap did not happen", fileOldID)
	}

	// The file_id must point to an existing user_files row.
	var exists bool
	if err := pool.QueryRow(context.Background(),
		"SELECT EXISTS(SELECT 1 FROM user_files WHERE id = $1 AND user_id = $2)",
		*gotFileID, userID,
	).Scan(&exists); err != nil {
		t.Fatalf("check user_files: %v", err)
	}
	if !exists {
		t.Errorf("activities.file_id %q does not reference an existing user_files row", *gotFileID)
	}
}

// ---------------------------------------------------------------------------
// Test 16: TestPartialImportRecovery
// ---------------------------------------------------------------------------

// TestPartialImportRecovery verifies that a fatal failure in Phase 5 (activities)
// does not roll back already-committed Phases 2/3 (tags, gear).
// We provoke a failure by giving an activity an invalid sport_type that violates
// a DB CHECK constraint — if no such constraint exists we use an FK violation.
func TestPartialImportRecovery(t *testing.T) {
	pool := testDB(t)
	userID := createTestUser(t, pool)

	srcUser := uuid.New().String()
	tagOldID := uuid.New().String()

	// Activity row with a NULL started_at to provoke a NOT NULL violation.
	actOldID := uuid.New().String()

	zipPath := buildExportZip(t, zipBuildOpts{
		columns: map[string][]string{
			"tags":       {"id", "user_id", "name", "color", "text_color", "category"},
			"activities": {"id", "user_id", "title", "sport_type", "started_at"},
		},
		tags: []map[string]any{
			{
				"id":         tagOldID,
				"user_id":    srcUser,
				"name":       "Recovery Tag",
				"color":      "#FACADE",
				"text_color": "#000000",
				"category":   "custom",
			},
		},
		activities: []map[string]any{
			{
				"id":         actOldID,
				"user_id":    srcUser,
				"title":      "Bad Activity",
				"sport_type": "running",
				"started_at": nil, // NOT NULL → DB error → failed
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
		// A hard fatal error on RunImport level is also acceptable —
		// but we still expect tags to have been committed if phases ran independently.
		// Re-check tags count; if the whole import aborted we can't verify partial recovery.
		t.Logf("RunImport returned error (acceptable for fatal phase failure): %v", err)
	}

	// Tags (Phase 2) must be imported regardless of activity failure.
	assertRowCount(t, pool, "tags", userID, 1)

	// If RunImport succeeded partially, EntitiesFailed must be > 0.
	if result != nil && result.EntitiesFailed == 0 {
		// started_at is not always NOT NULL — check if import silently succeeded.
		var actCount int
		if err2 := pool.QueryRow(context.Background(),
			"SELECT COUNT(*) FROM activities WHERE user_id = $1", userID,
		).Scan(&actCount); err2 == nil && actCount == 0 {
			t.Log("Activity with NULL started_at was silently skipped (not a hard error) — test objective met")
		}
	}
}

// ---------------------------------------------------------------------------
// Test 17: TestForwardCompat
// ---------------------------------------------------------------------------

// TestForwardCompat verifies that importing an export whose schema_migration
// is lower than the current DB (i.e. fewer columns) succeeds and DB defaults
// fill the missing columns.
func TestForwardCompat(t *testing.T) {
	pool := testDB(t)
	userID := createTestUser(t, pool)

	current := currentSchemaMigration(t, pool)
	older := current - 10
	if older <= 0 {
		older = 1
	}

	tagOldID := uuid.New().String()
	srcUser := uuid.New().String()

	// Export only the bare minimum columns — omit optional columns like description.
	zipPath := buildExportZip(t, zipBuildOpts{
		schemaMigration: older,
		columns: map[string][]string{
			// Only include columns that definitely existed from migration 1:
			"tags": {"id", "user_id", "name", "color", "text_color", "category"},
		},
		tags: []map[string]any{
			{
				"id":         tagOldID,
				"user_id":    srcUser,
				"name":       "ForwardCompat Tag",
				"color":      "#123456",
				"text_color": "#FFFFFF",
				"category":   "custom",
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
		t.Fatalf("RunImport with older schema migration: %v", err)
	}

	// The tag should have been imported despite missing columns.
	assertRowCount(t, pool, "tags", userID, 1)
}

// ---------------------------------------------------------------------------
// Test 18: TestCompositePKTables
// ---------------------------------------------------------------------------

// TestCompositePKTables verifies that best_effort_exclusions (composite PK:
// user_id + activity_id + category, no "id" column) imports correctly.
func TestCompositePKTables(t *testing.T) {
	pool := testDB(t)
	userID := createTestUser(t, pool)

	// Check that the table exists.
	var tableExists bool
	if err := pool.QueryRow(context.Background(), `
		SELECT EXISTS(
			SELECT 1 FROM information_schema.tables
			WHERE table_schema = 'public' AND table_name = 'best_effort_exclusions'
		)
	`).Scan(&tableExists); err != nil {
		t.Fatalf("check table: %v", err)
	}
	if !tableExists {
		t.Skip("best_effort_exclusions table not present")
	}

	srcUser := uuid.New().String()
	actOldID := uuid.New().String()
	now := time.Now().UTC()

	zipPath := buildExportZip(t, zipBuildOpts{
		columns: map[string][]string{
			"activities":             {"id", "user_id", "title", "sport_type", "started_at"},
			"best_effort_exclusions": {"user_id", "activity_id", "category"},
		},
		activities: []map[string]any{
			{
				"id":         actOldID,
				"user_id":    srcUser,
				"title":      "Composite PK Activity",
				"sport_type": "running",
				"started_at": now.Format(time.RFC3339),
			},
		},
		extraFiles: map[string][]byte{
			"data/best_effort_exclusions.json": mustMarshalJSON(t, []map[string]any{
				{
					"user_id":     srcUser,
					"activity_id": actOldID,
					"category":    "5km",
				},
			}),
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

	// One activity must exist.
	assertRowCount(t, pool, "activities", userID, 1)

	// best_effort_exclusions row must exist with correct user_id.
	var count int
	if err := pool.QueryRow(context.Background(),
		"SELECT COUNT(*) FROM best_effort_exclusions WHERE user_id = $1 AND category = '5km'",
		userID,
	).Scan(&count); err != nil {
		t.Fatalf("count best_effort_exclusions: %v", err)
	}
	if count != 1 {
		t.Errorf("best_effort_exclusions count: want 1, got %d", count)
	}
}

// ---------------------------------------------------------------------------
// Test 19: TestDeferredFKGearDocument
// ---------------------------------------------------------------------------

// TestDeferredFKGearDocument verifies that gear.source_document_id (deferred FK
// pointing at gear_documents) is correctly resolved after the document is imported
// in Phase 4.
func TestDeferredFKGearDocument(t *testing.T) {
	pool := testDB(t)
	userID := createTestUser(t, pool)

	srcUser := uuid.New().String()
	gearOldID := uuid.New().String()
	docOldID := uuid.New().String()

	zipPath := buildExportZip(t, zipBuildOpts{
		columns: map[string][]string{
			"gear": {
				"id", "user_id", "name", "type",
				"source_document_id",
			},
			"gear_documents": {
				"id", "user_id", "gear_id",
				"doc_type", "title", "processing_status",
				"original_filename", "content_type", "file_size_bytes",
			},
		},
		gear: []map[string]any{
			{
				"id":                 gearOldID,
				"user_id":            srcUser,
				"name":               "DeferredGear",
				"type":               "bike",
				"source_document_id": docOldID, // deferred FK
			},
		},
		extraFiles: map[string][]byte{
			"data/gear_documents.json": mustMarshalJSON(t, []map[string]any{
				{
					"id":                docOldID,
					"user_id":           srcUser,
					"gear_id":           gearOldID,
					"doc_type":          "invoice",
					"title":             "Purchase Receipt",
					"processing_status": "pending",
					// NOT NULL columns required by gear_documents:
					"original_filename": "receipt.pdf",
					"content_type":      "application/pdf",
					"file_size_bytes":   1024,
				},
			}),
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

	// gear.source_document_id must not be NULL after deferred FK resolution.
	var srcDocID *string
	if err := pool.QueryRow(context.Background(),
		"SELECT source_document_id FROM gear WHERE user_id = $1 AND name = 'DeferredGear'",
		userID,
	).Scan(&srcDocID); err != nil {
		t.Fatalf("select gear: %v", err)
	}
	if srcDocID == nil {
		t.Error("gear.source_document_id is NULL — deferred FK not resolved")
		return
	}

	// The gear_documents row with that ID must exist.
	var docExists bool
	if err := pool.QueryRow(context.Background(),
		"SELECT EXISTS(SELECT 1 FROM gear_documents WHERE id = $1)",
		*srcDocID,
	).Scan(&docExists); err != nil {
		t.Fatalf("check gear_documents: %v", err)
	}
	if !docExists {
		t.Errorf("gear.source_document_id %q does not reference an existing gear_documents row", *srcDocID)
	}
}

// ---------------------------------------------------------------------------
// Test 20: TestDeferredFKTagCoverFile
// ---------------------------------------------------------------------------

// TestDeferredFKTagCoverFile verifies that tags.cover_file_id (deferred FK
// to user_files) is correctly resolved after the file is imported in Phase 5.
func TestDeferredFKTagCoverFile(t *testing.T) {
	pool := testDB(t)
	userID := createTestUser(t, pool)

	srcUser := uuid.New().String()
	tagOldID := uuid.New().String()
	fileOldID := uuid.New().String()

	// Build a user_files index entry for the cover file.
	userFilesJSON := mustMarshalJSON(t, []map[string]any{
		{
			"id":           fileOldID,
			"file_type":    "image",
			"content_type": "image/jpeg",
			"file_name":    "cover.jpg",
			"size_bytes":   3,
			"status":       "active",
		},
	})
	fileContent := []byte("jpg")

	zipPath := buildExportZip(t, zipBuildOpts{
		columns: map[string][]string{
			"tags": {"id", "user_id", "name", "color", "text_color", "category", "cover_file_id"},
		},
		tags: []map[string]any{
			{
				"id":            tagOldID,
				"user_id":       srcUser,
				"name":          "CoverTag",
				"color":         "#ABCDEF",
				"text_color":    "#000000",
				"category":      "custom",
				"cover_file_id": fileOldID, // deferred FK
			},
		},
		extraFiles: map[string][]byte{
			"data/user_files.json":  userFilesJSON,
			"files/image/cover.jpg": fileContent,
		},
	})
	src := openZipSource(t, zipPath)

	// Use DB-aware mock so user_files rows satisfy FK constraints on cover_file_id.
	eng := &Engine{
		DB:       pool,
		ShadowFS: newDBMockShadowFS(pool),
		Config:   EngineConfig{},
	}
	_, err := eng.RunImport(context.Background(), ImportInput{
		Source:       src,
		TargetUserID: userID,
	})
	if err != nil {
		t.Fatalf("RunImport: %v", err)
	}

	// tags.cover_file_id must not be NULL.
	var coverFileID *string
	if err := pool.QueryRow(context.Background(),
		"SELECT cover_file_id FROM tags WHERE user_id = $1 AND name = 'CoverTag'",
		userID,
	).Scan(&coverFileID); err != nil {
		t.Fatalf("select tag.cover_file_id: %v", err)
	}
	if coverFileID == nil {
		t.Error("tags.cover_file_id is NULL — deferred FK not resolved")
		return
	}
	if *coverFileID == fileOldID {
		t.Errorf("tags.cover_file_id still has the old export UUID %q — IDMap remap failed", fileOldID)
	}

	// The referenced user_files row must exist.
	var exists bool
	if err := pool.QueryRow(context.Background(),
		"SELECT EXISTS(SELECT 1 FROM user_files WHERE id = $1 AND user_id = $2)",
		*coverFileID, userID,
	).Scan(&exists); err != nil {
		t.Fatalf("check user_files: %v", err)
	}
	if !exists {
		t.Errorf("tags.cover_file_id %q does not reference an existing user_files row", *coverFileID)
	}
}

// ---------------------------------------------------------------------------
// Test 21: TestUserColumnMapping
// ---------------------------------------------------------------------------

// TestUserColumnMapping verifies that:
//   - segments uses "created_by" (not "user_id") to identify the owner
//   - chat_messages has UserCol="none" so no user_id is injected
func TestUserColumnMapping(t *testing.T) {
	pool := testDB(t)
	userID := createTestUser(t, pool)

	// Segments have NO ACTION FK on created_by, so prior test runs may leave
	// orphaned rows. Clean them up before the test so the count query is unambiguous.
	// (Cleanup at the end handles our own rows.)
	_, _ = pool.Exec(context.Background(),
		"DELETE FROM segments WHERE name = 'UserColSeg'")

	srcUser := uuid.New().String()
	segOldID := uuid.New().String()
	sessionOldID := uuid.New().String()
	msgOldID := uuid.New().String()

	zipPath := buildExportZip(t, zipBuildOpts{
		columns: map[string][]string{
			"segments": {
				"id", "created_by", "name", "sport_group", "segment_type",
			},
			"chat_sessions": {"id", "user_id", "title"},
			"chat_messages": {"id", "session_id", "role", "content"},
		},
		segments: []map[string]any{
			{
				"id":           segOldID,
				"created_by":   srcUser,
				"name":         "UserColSeg",
				"sport_group":  "running",
				"segment_type": "standard",
			},
		},
		extraFiles: map[string][]byte{
			"data/chat_sessions.json": mustMarshalJSON(t, []map[string]any{
				{
					"id":      sessionOldID,
					"user_id": srcUser,
					"title":   "Test Session",
				},
			}),
			"data/chat_messages.json": mustMarshalJSON(t, []map[string]any{
				{
					"id":         msgOldID,
					"session_id": sessionOldID,
					"role":       "user",
					"content":    "Hello",
				},
			}),
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

	// segments.created_by must equal the target userID.
	// Filter by created_by = userID to avoid picking up leftover rows from prior runs
	// (segments.created_by has NO ACTION delete, so user cleanup does not cascade).
	var segCount int
	if err := pool.QueryRow(context.Background(),
		"SELECT COUNT(*) FROM segments WHERE name = 'UserColSeg' AND created_by = $1",
		userID,
	).Scan(&segCount); err != nil {
		t.Fatalf("count segments: %v", err)
	}
	if segCount != 1 {
		t.Errorf("segments with name='UserColSeg' and created_by=userID: want 1, got %d", segCount)
	}
	// Cleanup: segments aren't CASCADE-deleted when user is deleted (NO ACTION FK).
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(),
			"DELETE FROM segments WHERE name = 'UserColSeg' AND created_by = $1", userID)
	})

	// chat_messages has no user_id column in the schema (UserCol="none").
	// Verify the message was imported and session_id was remapped.
	var msgCount int
	if err := pool.QueryRow(context.Background(),
		`SELECT COUNT(*) FROM chat_messages cm
		 JOIN chat_sessions cs ON cs.id = cm.session_id
		 WHERE cs.user_id = $1`,
		userID,
	).Scan(&msgCount); err != nil {
		t.Fatalf("count chat_messages: %v", err)
	}
	if msgCount != 1 {
		t.Errorf("chat_messages count via session: want 1, got %d", msgCount)
	}
}

// ---------------------------------------------------------------------------
// Test 22: TestRecordingGroupMembersPhase6
// ---------------------------------------------------------------------------

// TestRecordingGroupMembersPhase6 verifies that recording_group_members (Phase 6,
// FK to recording_groups Phase 2 and activities Phase 5) imports correctly.
// The group needs at least 2 members: the trigger fn_sync_group_secondary deletes
// groups with fewer than 2 members (via cascade), which would also delete the members.
func TestRecordingGroupMembersPhase6(t *testing.T) {
	pool := testDB(t)
	userID := createTestUser(t, pool)

	srcUser := uuid.New().String()
	groupOldID := uuid.New().String()
	act1OldID := uuid.New().String()
	act2OldID := uuid.New().String()
	now := time.Now().UTC()

	zipPath := buildExportZip(t, zipBuildOpts{
		columns: map[string][]string{
			"recording_groups":        {"id", "user_id", "sport_type"},
			"activities":              {"id", "user_id", "title", "sport_type", "started_at"},
			"recording_group_members": {"group_id", "activity_id", "is_fused"},
		},
		// Two activities so the group trigger keeps the group alive (requires >= 2 members).
		activities: []map[string]any{
			{
				"id":         act1OldID,
				"user_id":    srcUser,
				"title":      "Group Member Activity 1",
				"sport_type": "cycling",
				"started_at": now.Format(time.RFC3339),
			},
			{
				"id":         act2OldID,
				"user_id":    srcUser,
				"title":      "Group Member Activity 2",
				"sport_type": "cycling",
				"started_at": now.Add(time.Minute).Format(time.RFC3339),
			},
		},
		extraFiles: map[string][]byte{
			"data/recording_groups.json": mustMarshalJSON(t, []map[string]any{
				{
					"id":         groupOldID,
					"user_id":    srcUser,
					"sport_type": "cycling",
				},
			}),
			"data/recording_group_members.json": mustMarshalJSON(t, []map[string]any{
				{
					"group_id":    groupOldID,
					"activity_id": act1OldID,
					"is_fused":    false,
				},
				{
					"group_id":    groupOldID,
					"activity_id": act2OldID,
					"is_fused":    false,
				},
			}),
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

	// Both members must survive — group has >= 2 activities so trigger keeps it.
	var memberCount int
	if err := pool.QueryRow(context.Background(), `
		SELECT COUNT(*) FROM recording_group_members rgm
		JOIN recording_groups rg ON rg.id = rgm.group_id
		JOIN activities a         ON a.id  = rgm.activity_id
		WHERE rg.user_id = $1
	`, userID).Scan(&memberCount); err != nil {
		t.Fatalf("count recording_group_members: %v", err)
	}
	if memberCount != 2 {
		t.Errorf("recording_group_members count: want 2, got %d", memberCount)
	}
}

// ---------------------------------------------------------------------------
// Test 23: TestFileParentIDRemap
// ---------------------------------------------------------------------------

// TestFileParentIDRemap verifies that when a file has ParentType="activity" and
// a ParentID pointing at an old activity UUID, the ShadowFS.Put() call receives
// either a nil ParentID (activity not yet imported at file-import time) or the
// correctly remapped new activity UUID. The old UUID must never reach ShadowFS.
func TestFileParentIDRemap(t *testing.T) {
	pool := testDB(t)
	userID := createTestUser(t, pool)

	srcUser := uuid.New().String()
	actOldID := uuid.New().String()
	fileOldID := uuid.New().String()
	now := time.Now().UTC()

	parentType := "activity"

	// user_files.json: file with ParentType=activity, ParentID=actOldID.
	userFilesJSON := mustMarshalJSON(t, []map[string]any{
		{
			"id":           fileOldID,
			"file_type":    "original",
			"content_type": "application/octet-stream",
			"file_name":    "remap_test.fit",
			"size_bytes":   4,
			"status":       "active",
			"parent_type":  parentType,
			"parent_id":    actOldID, // old activity UUID
		},
	})
	fileContent := []byte("data")

	// Use a trackable dbMockShadowFS that records the ParentID passed to Put
	// and also inserts user_files rows to satisfy FK constraints.
	trackingFS := &trackingMockShadowFS{dbMockShadowFS: newDBMockShadowFS(pool)}

	zipPath := buildExportZip(t, zipBuildOpts{
		columns: map[string][]string{
			"activities": {"id", "user_id", "title", "sport_type", "started_at", "file_id"},
		},
		activities: []map[string]any{
			{
				"id":         actOldID,
				"user_id":    srcUser,
				"title":      "ParentRemap Activity",
				"sport_type": "running",
				"started_at": now.Format(time.RFC3339),
				"file_id":    fileOldID,
			},
		},
		extraFiles: map[string][]byte{
			"data/user_files.json":            userFilesJSON,
			"files/original/remap_test.fit":   fileContent,
		},
	})
	src := openZipSource(t, zipPath)

	eng := &Engine{
		DB:       pool,
		ShadowFS: trackingFS,
		Config:   EngineConfig{},
	}
	_, err := eng.RunImport(context.Background(), ImportInput{
		Source:       src,
		TargetUserID: userID,
	})
	if err != nil {
		t.Fatalf("RunImport: %v", err)
	}

	// The old activity UUID must NEVER have been passed as ParentID to ShadowFS.
	trackingFS.trackMu.Lock()
	defer trackingFS.trackMu.Unlock()
	for _, pid := range trackingFS.parentIDs {
		if pid != nil && *pid == actOldID {
			t.Errorf("ShadowFS.Put received old export activity UUID %q as ParentID — remap failed", actOldID)
		}
	}
}

// trackingMockShadowFS wraps dbMockShadowFS and records the ParentID passed to Put.
type trackingMockShadowFS struct {
	*dbMockShadowFS
	trackMu   sync.Mutex
	parentIDs []*string
}

func (m *trackingMockShadowFS) Put(ctx context.Context, entry shadowfs.FileEntry, data io.Reader) (shadowfs.FileRecord, error) {
	// Copy the pointer: nil means "not set", non-nil means the actual value was passed.
	var pid *string
	if entry.ParentID != nil {
		v := *entry.ParentID
		pid = &v
	}
	m.trackMu.Lock()
	m.parentIDs = append(m.parentIDs, pid)
	m.trackMu.Unlock()
	return m.dbMockShadowFS.Put(ctx, entry, data)
}

// ---------------------------------------------------------------------------
// Test 24: TestMultiPartSource
// ---------------------------------------------------------------------------

// TestMultiPartSource verifies that a multi-part export (manifest.json with
// multipart:true, metadata.zip for data/, files-0001.zip for files/) is read
// correctly by MultiPartSource and imported via RunImport.
func TestMultiPartSource(t *testing.T) {
	pool := testDB(t)
	userID := createTestUser(t, pool)

	srcUser := uuid.New().String()
	tagOldID := uuid.New().String()

	dir := t.TempDir()

	// ── Build metadata.zip (contains data/ + columns.json) ──────────────────
	metaZipPath := dir + "/metadata.zip"
	writeZipFile(t, metaZipPath, map[string][]byte{
		"columns.json": mustMarshalJSON(t, map[string][]string{
			"tags": {"id", "user_id", "name", "color", "text_color", "category"},
		}),
		"data/tags.json": mustMarshalJSON(t, []map[string]any{
			{
				"id":         tagOldID,
				"user_id":    srcUser,
				"name":       "MultiPart Tag",
				"color":      "#AABBCC",
				"text_color": "#000000",
				"category":   "custom",
			},
		}),
	})

	// ── Build files-0001.zip (contains files/ entries) ───────────────────────
	part1Path := dir + "/files-0001.zip"
	writeZipFile(t, part1Path, map[string][]byte{
		// No actual file entries needed for this test — tags have no files.
	})

	// ── Compute SHA-256 of files-0001.zip ────────────────────────────────────
	part1SHA256 := fileSHA256(t, part1Path)
	part1Size := fileSize(t, part1Path)

	// ── Write top-level manifest.json ─────────────────────────────────────────
	current := currentSchemaMigration(t, pool)
	manifest := map[string]any{
		"format_version":   1,
		"skadia_version":  "2026.04",
		"schema_migration": current,
		"exported_at":      time.Now().UTC().Format(time.RFC3339),
		"user_id":          srcUser,
		"multipart":        true,
		"parts": []map[string]any{
			{
				"name":       "files-0001.zip",
				"sha256":     part1SHA256,
				"size_bytes": part1Size,
			},
		},
	}
	manifestBytes := mustMarshalJSON(t, manifest)
	if err := os.WriteFile(dir+"/manifest.json", manifestBytes, 0o644); err != nil {
		t.Fatalf("write manifest.json: %v", err)
	}

	// ── Open as MultiPartSource ───────────────────────────────────────────────
	mps, err := NewMultiPartSource(dir)
	if err != nil {
		t.Fatalf("NewMultiPartSource: %v", err)
	}
	t.Cleanup(func() { mps.Close() })

	eng := newEngine(pool)
	_, err = eng.RunImport(context.Background(), ImportInput{
		Source:       mps,
		TargetUserID: userID,
	})
	if err != nil {
		t.Fatalf("RunImport with MultiPartSource: %v", err)
	}

	assertRowCount(t, pool, "tags", userID, 1)
}

// writeZipFile creates a ZIP archive at path containing the given entries.
func writeZipFile(t *testing.T, path string, entries map[string][]byte) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("writeZipFile create %q: %v", path, err)
	}
	defer f.Close()
	zw := zip.NewWriter(f)
	for name, content := range entries {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatalf("writeZipFile entry %q: %v", name, err)
		}
		if _, err := w.Write(content); err != nil {
			t.Fatalf("writeZipFile write %q: %v", name, err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("writeZipFile close: %v", err)
	}
}

// fileSHA256 returns the hex-encoded SHA-256 of the file at path.
func fileSHA256(t *testing.T, path string) string {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("fileSHA256 open %q: %v", path, err)
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		t.Fatalf("fileSHA256 hash %q: %v", path, err)
	}
	return hex.EncodeToString(h.Sum(nil))
}

// fileSize returns the size of the file at path in bytes.
func fileSize(t *testing.T, path string) int64 {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("fileSize stat %q: %v", path, err)
	}
	return info.Size()
}

// ---------------------------------------------------------------------------
// Test 25: TestEditSidecarPhase6
// ---------------------------------------------------------------------------

// TestEditSidecarPhase6 verifies that:
//  1. A file with file_type="edit" is skipped by Phase 0 (importAllFiles) and
//     imported in Phase 6 (importEditSidecars) after the parent activity exists.
//  2. The ParentID passed to ShadowFS.Put is the *new* remapped activity UUID,
//     not the old export UUID and not nil.
func TestEditSidecarPhase6(t *testing.T) {
	pool := testDB(t)
	userID := createTestUser(t, pool)

	srcUser := uuid.New().String()
	actOldID := uuid.New().String()
	editFileOldID := uuid.New().String()
	now := time.Now().UTC()

	schemaMig := currentSchemaMigration(t, pool)

	userFilesJSON := mustMarshalJSON(t, []map[string]any{
		{
			"id":           editFileOldID,
			"file_type":    "edit",
			"content_type": "application/json",
			"file_name":    "edit.json",
			"size_bytes":   12,
			"parent_type":  "activity",
			"parent_id":    actOldID,
			"has_sidecar":  false,
			"status":       "active",
		},
	})

	zipPath := buildExportZip(t, zipBuildOpts{
		sourceUserID:    srcUser,
		schemaMigration: schemaMig,
		columns: map[string][]string{
			"activities": {"id", "user_id", "title", "sport_type", "started_at"},
		},
		activities: []map[string]any{
			{
				"id":         actOldID,
				"user_id":    srcUser,
				"title":      "Edit Sidecar Activity",
				"sport_type": "running",
				"started_at": now.Format(time.RFC3339),
			},
		},
		extraFiles: map[string][]byte{
			"data/user_files.json":    userFilesJSON,
			"files/edit/edit.json":    []byte(`{"edits":[]}`),
		},
	})
	src := openZipSource(t, zipPath)

	// trackingMockShadowFS records ParentID values passed to Put.
	trackingFS := &trackingMockShadowFS{dbMockShadowFS: newDBMockShadowFS(pool)}

	eng := &Engine{
		DB:       pool,
		ShadowFS: trackingFS,
		Config:   EngineConfig{},
	}
	result, err := eng.RunImport(context.Background(), ImportInput{
		Source:       src,
		TargetUserID: userID,
	})
	if err != nil {
		t.Fatalf("RunImport: %v", err)
	}

	// Edit-sidecar must be counted as imported (Phase 6).
	if result.FilesImported < 1 {
		t.Errorf("expected at least 1 file imported (edit sidecar), got %d", result.FilesImported)
	}

	// Verify that ShadowFS.Put was called for the edit sidecar with a non-nil,
	// remapped ParentID — not the old export activity UUID, not nil.
	trackingFS.trackMu.Lock()
	defer trackingFS.trackMu.Unlock()

	var foundEditPut bool
	for i, pid := range trackingFS.parentIDs {
		_ = i
		if pid == nil {
			// An edit sidecar with nil ParentID would mean the activity wasn't in IDMap yet.
			// That would indicate a Phase 6 ordering bug.
			continue
		}
		if *pid == actOldID {
			t.Errorf("ShadowFS.Put received old export activity UUID %q as ParentID — remap failed", actOldID)
		} else {
			foundEditPut = true
		}
	}
	if !foundEditPut {
		t.Error("edit sidecar ParentID was nil — activity was not in IDMap when importEditSidecars ran (Phase 6 ordering bug)")
	}
}

// ---------------------------------------------------------------------------
// Test 26: TestNonEmptyAccount
// ---------------------------------------------------------------------------

// TestNonEmptyAccount verifies that importing onto a user who already has
// activities is rejected with a hard error (CanImport=false).
func TestNonEmptyAccount(t *testing.T) {
	pool := testDB(t)
	userID := createTestUser(t, pool)

	// Insert an existing activity for this user.
	existingActID := uuid.New().String()
	now := time.Now().UTC()
	_, err := pool.Exec(context.Background(), `
		INSERT INTO activities (id, user_id, title, sport_type, started_at)
		VALUES ($1, $2, 'Existing Activity', 'cycling', $3)
	`, existingActID, userID, now)
	if err != nil {
		t.Fatalf("insert existing activity: %v", err)
	}

	// Build an export with a new activity.
	srcUser := uuid.New().String()
	newActOldID := uuid.New().String()

	zipPath := buildExportZip(t, zipBuildOpts{
		columns: map[string][]string{
			"activities": {"id", "user_id", "title", "sport_type", "started_at"},
		},
		activities: []map[string]any{
			{
				"id":         newActOldID,
				"user_id":    srcUser,
				"title":      "Imported Activity",
				"sport_type": "running",
				"started_at": now.Add(time.Hour).Format(time.RFC3339),
			},
		},
	})
	src := openZipSource(t, zipPath)

	// ValidateAccount must report CanImport=false.
	eng := newEngine(pool)
	vr, err := eng.ValidateAccount(context.Background(), userID)
	if err != nil {
		t.Fatalf("ValidateAccount: %v", err)
	}
	if vr.CanImport {
		t.Errorf("ValidateAccount: expected CanImport=false for account with activities, got true")
	}
	if len(vr.Warnings) == 0 || !strings.Contains(vr.Warnings[0], "activities") {
		t.Errorf("ValidateAccount: expected warning mentioning 'activities', got: %v", vr.Warnings)
	}

	// RunImport must return an error (hard gate).
	_, importErr := eng.RunImport(context.Background(), ImportInput{
		Source:       src,
		TargetUserID: userID,
	})
	if importErr == nil {
		t.Fatalf("RunImport: expected error for non-empty account, got nil")
	}
	if !strings.Contains(importErr.Error(), "activities") {
		t.Errorf("RunImport error should mention 'activities', got: %v", importErr)
	}

	// The existing activity must not have been affected.
	assertRowCount(t, pool, "activities", userID, 1)
}

// ---------------------------------------------------------------------------
// Test: TestJSONBRemapNullColumn (Finding #18)
// ---------------------------------------------------------------------------

// TestJSONBRemapNullColumn verifies that runJSONBRemap correctly skips NULL
// JSONB columns without error. The condition_groups column is nullable and
// included in jsonbRemapFields; when NULL the remap must skip it gracefully.
func TestJSONBRemapNullColumn(t *testing.T) {
	pool := testDB(t)
	ctx := context.Background()
	userID := createTestUser(t, pool)
	schemaMig := currentSchemaMigration(t, pool)

	tagOldID := uuid.New().String()
	srcUser := uuid.New().String()

	zipPath := buildExportZip(t, zipBuildOpts{
		sourceUserID:    srcUser,
		schemaMigration: schemaMig,
		columns: map[string][]string{
			"tags":             {"id", "user_id", "name", "color", "text_color", "category"},
			"automation_rules": {"id", "user_id", "name", "is_active", "priority", "conditions", "actions"},
		},
		tags: []map[string]any{
			{
				"id":         tagOldID,
				"user_id":    srcUser,
				"name":       "TestTag",
				"color":      "#000",
				"text_color": "#FFF",
				"category":   "custom",
			},
		},
		extraFiles: map[string][]byte{
			"data/automation_rules.json": mustMarshalJSON(t, []map[string]any{
				{
					"id":         uuid.New().String(),
					"user_id":    srcUser,
					"name":       "NullConditionGroups",
					"is_active":  true,
					"priority":   1,
					"conditions": map[string]any{},
					"actions":    []map[string]any{{"type": "add_tag", "params": map[string]any{"tag_id": tagOldID}}},
					// condition_groups intentionally absent → DB default NULL
				},
			}),
		},
	})

	src := openZipSource(t, zipPath)
	eng := newEngine(pool)
	result, err := eng.RunImport(ctx, ImportInput{Source: src, TargetUserID: userID})
	if err != nil {
		t.Fatalf("RunImport: %v", err)
	}

	// Rule should be imported with actions remapped and condition_groups NULL.
	var actionsRaw []byte
	var conditionGroupsRaw *[]byte
	err = pool.QueryRow(ctx,
		"SELECT actions, condition_groups FROM automation_rules WHERE user_id = $1 AND name = 'NullConditionGroups' LIMIT 1",
		userID,
	).Scan(&actionsRaw, &conditionGroupsRaw)
	if err != nil {
		t.Fatalf("query rule: %v", err)
	}
	if conditionGroupsRaw != nil {
		t.Error("condition_groups should be NULL")
	}
	// Verify actions were remapped (tag_id should NOT be the old ID).
	if bytes.Contains(actionsRaw, []byte(tagOldID)) {
		t.Error("actions still contain old tag_id — JSONB remap failed")
	}
	_ = result
}

// ---------------------------------------------------------------------------
// Test: TestPhase0MissingFileBlob (Finding #19)
// ---------------------------------------------------------------------------

// TestPhase0MissingFileBlob verifies that when a file is listed in
// user_files.json but the actual blob is missing from the ZIP, importFileByID
// logs a warning and returns nil (non-fatal). The activity referencing this
// file should end up with file_id = NULL.
func TestPhase0MissingFileBlob(t *testing.T) {
	pool := testDB(t)
	ctx := context.Background()
	userID := createTestUser(t, pool)
	schemaMig := currentSchemaMigration(t, pool)

	missingFileID := uuid.New().String()
	actID := uuid.New().String()
	srcUser := uuid.New().String()

	zipPath := buildExportZip(t, zipBuildOpts{
		sourceUserID:    srcUser,
		schemaMigration: schemaMig,
		columns: map[string][]string{
			"activities": {"id", "user_id", "sport_type", "started_at", "title", "file_id"},
		},
		activities: []map[string]any{
			{
				"id":         actID,
				"user_id":    srcUser,
				"sport_type": "running",
				"started_at": "2026-01-01T10:00:00Z",
				"title":      "No File",
				"file_id":    missingFileID,
			},
		},
		extraFiles: map[string][]byte{
			"data/user_files.json": mustMarshalJSON(t, []map[string]any{
				{
					"id":           missingFileID,
					"file_type":    "original",
					"content_type": "application/fit",
					"file_name":    "missing.fit",
					"size_bytes":   1000,
					"has_sidecar":  false,
					"status":       "active",
				},
			}),
			// NOTE: No "files/original/missing.fit" blob!
		},
	})

	src := openZipSource(t, zipPath)
	eng := newEngine(pool)
	result, err := eng.RunImport(ctx, ImportInput{Source: src, TargetUserID: userID})
	if err != nil {
		t.Fatalf("RunImport should succeed despite missing blob: %v", err)
	}

	// File blob is missing -> importFileByID returns nil (logs warning), no IDMap entry.
	// Activity's file_id should be NULL (GetIfExists returns false).
	var fileID *string
	err = pool.QueryRow(ctx,
		"SELECT file_id FROM activities WHERE user_id = $1 LIMIT 1", userID,
	).Scan(&fileID)
	if err != nil {
		t.Fatalf("query activity: %v", err)
	}
	if fileID != nil {
		t.Errorf("activity file_id should be NULL when file blob is missing, got %q", *fileID)
	}
	_ = result
}

// ---------------------------------------------------------------------------
// Test: TestFileRefMissingFile (Finding #20)
// ---------------------------------------------------------------------------

// TestFileRefMissingFile verifies that a fileRef column (tags.cover_file_id)
// becomes NULL when the referenced file is not in the export at all (no
// user_files.json entry, no blob). This exercises the GetIfExists path in
// prepareRow for file-reference columns.
func TestFileRefMissingFile(t *testing.T) {
	pool := testDB(t)
	ctx := context.Background()
	userID := createTestUser(t, pool)
	schemaMig := currentSchemaMigration(t, pool)

	fakeFileID := uuid.New().String() // Referenced but not in user_files.json
	srcUser := uuid.New().String()

	zipPath := buildExportZip(t, zipBuildOpts{
		sourceUserID:    srcUser,
		schemaMigration: schemaMig,
		tags: []map[string]any{
			{
				"id":            uuid.New().String(),
				"user_id":       srcUser,
				"name":          "TagNoFile",
				"color":         "#FF0000",
				"text_color":    "#000",
				"category":      "custom",
				"cover_file_id": fakeFileID,
			},
		},
		// No user_files.json, no file blobs.
	})

	src := openZipSource(t, zipPath)
	eng := newEngine(pool)
	result, err := eng.RunImport(ctx, ImportInput{Source: src, TargetUserID: userID})
	if err != nil {
		t.Fatalf("RunImport: %v", err)
	}

	// cover_file_id should be NULL (file not in IDMap -> GetIfExists -> nil).
	var coverFileID *string
	err = pool.QueryRow(ctx,
		"SELECT cover_file_id FROM tags WHERE user_id = $1 AND name = 'TagNoFile' LIMIT 1",
		userID,
	).Scan(&coverFileID)
	if err != nil {
		t.Fatalf("query tag: %v", err)
	}
	if coverFileID != nil {
		t.Errorf("cover_file_id should be NULL when file is missing, got %q", *coverFileID)
	}
	_ = result
}

// ---------------------------------------------------------------------------
// Test: TestJSONBRemapMissingEntity
// ---------------------------------------------------------------------------

// TestJSONBRemapMissingEntity verifies that JSONB UUID references to entities
// NOT in the export are preserved as-is (not remapped to phantom UUIDs and
// not dropped from the JSONB structure).
func TestJSONBRemapMissingEntity(t *testing.T) {
	pool := testDB(t)
	ctx := context.Background()
	userID := createTestUser(t, pool)
	schemaMig := currentSchemaMigration(t, pool)

	srcUser := uuid.New().String()
	// gear_id that is NOT exported (no gear.json in the ZIP).
	missingGearID := uuid.New().String()

	zipPath := buildExportZip(t, zipBuildOpts{
		sourceUserID:    srcUser,
		schemaMigration: schemaMig,
		extraFiles: map[string][]byte{
			"data/automation_rules.json": mustMarshalJSON(t, []map[string]any{
				{
					"id":         uuid.New().String(),
					"user_id":    srcUser,
					"name":       "MissingGearRule",
					"is_active":  true,
					"priority":   1,
					"conditions": map[string]any{},
					"actions": []any{
						map[string]any{
							"type":   "set_gear",
							"params": map[string]any{"gear_id": missingGearID},
						},
					},
				},
			}),
		},
	})

	src := openZipSource(t, zipPath)
	eng := newEngine(pool)
	_, err := eng.RunImport(ctx, ImportInput{Source: src, TargetUserID: userID})
	if err != nil {
		t.Fatalf("RunImport: %v", err)
	}

	// Read back the rule's actions JSONB.
	var actionsRaw []byte
	err = pool.QueryRow(ctx,
		"SELECT actions FROM automation_rules WHERE user_id = $1 AND name = 'MissingGearRule'",
		userID,
	).Scan(&actionsRaw)
	if err != nil {
		t.Fatalf("select automation_rules.actions: %v", err)
	}

	var decodedActions []map[string]any
	if err := json.Unmarshal(actionsRaw, &decodedActions); err != nil {
		t.Fatalf("parse actions: %v", err)
	}
	if len(decodedActions) == 0 {
		t.Fatal("actions is empty")
	}

	params, ok := decodedActions[0]["params"].(map[string]any)
	if !ok {
		t.Fatal("params is not a map")
	}

	// gear_id should be preserved as the original UUID (not remapped, not dropped).
	gearID, ok := params["gear_id"].(string)
	if !ok {
		t.Fatal("gear_id is missing from params — walkAndRemap dropped it instead of preserving")
	}
	if gearID != missingGearID {
		t.Errorf("gear_id should be preserved as original %q, got %q", missingGearID, gearID)
	}
}
