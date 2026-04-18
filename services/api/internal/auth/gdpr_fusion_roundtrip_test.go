package auth

// TestFusionGDPRRoundtrip exercises the full GDPR cycle for a user whose
// account contains all three activity categories that the Fusion Consistency
// Refactor (Mig 000302–000308) cares about:
//
//  1. A plain regular activity (no fusion, no manual entry).
//  2. A fusion-merged activity with two source activities and memberships
//     in activity_fusion_memberships.  A .meta sidecar is written for the
//     merged activity via the mockShadowFS.
//  3. A manual-entry activity (is_manual_entry=true) with its own .meta.
//
// The test then:
//   - Calls executeExportJobOnce directly (avoids the async HTTP layer).
//   - Reads the exported ZIP from background_jobs.metadata.file_path.
//   - Asserts structural invariants of the ZIP (memberships present, sidecars
//     present, no legacy fusion_sources field).
//   - Marks the user deleted and calls DeleteUserData.
//   - Asserts post-delete row counts.
//   - Re-imports the ZIP into a fresh target user.
//   - Asserts activities, memberships and UUIDs are correctly remapped.
//
// The test requires DATABASE_URL — it skips cleanly when not set.

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/config"
	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/dataimport"
	"github.com/skadia-app/skadia/services/api/internal/middleware"
	"github.com/skadia-app/skadia/services/api/internal/settings"
	"github.com/skadia-app/skadia/services/api/internal/shadowfs"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ---------------------------------------------------------------------------
// DB helper (follows engine_test.go pattern — uses DATABASE_URL)
// ---------------------------------------------------------------------------

func testDBForRoundtrip(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Skip("DATABASE_URL not set — skipping GDPR fusion roundtrip integration test")
	}
	pool, err := pgxpool.New(context.Background(), dbURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { pool.Close() })
	return pool
}

// createTestUserRT inserts a minimal user row and returns its UUID.
// t.Cleanup deletes the row (CASCADE handles dependants).
func createTestUserRT(t *testing.T, pool *pgxpool.Pool) string {
	t.Helper()
	id := uuid.New().String()
	email := fmt.Sprintf("rt-%s@example.com", id[:8])
	username := fmt.Sprintf("rtuser_%s", id[:8])
	_, err := pool.Exec(context.Background(), `
		INSERT INTO users (id, username, email, display_name, timezone, email_verified, registration_source)
		VALUES ($1, $2, $3, 'RT User', 'UTC', true, 'test')
	`, id, username, email)
	if err != nil {
		t.Fatalf("createTestUserRT: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), "DELETE FROM users WHERE id = $1", id)
	})
	return id
}

// insertActivity inserts a minimal activity row and returns its UUID.
func insertActivity(t *testing.T, pool *pgxpool.Pool, userID, title string, isManual bool) string {
	t.Helper()
	id := uuid.New().String()
	now := time.Now().UTC()
	_, err := pool.Exec(context.Background(), `
		INSERT INTO activities (id, user_id, title, sport_type, started_at, ended_at,
		                        visibility, is_manual_entry, created_at, updated_at)
		VALUES ($1, $2, $3, 'cycling', $4, $5, 'private', $6, $4, $4)
	`, id, userID, title, now, now.Add(time.Hour), isManual)
	if err != nil {
		t.Fatalf("insertActivity %q: %v", title, err)
	}
	return id
}

// insertFusionMembership inserts one row in activity_fusion_memberships.
func insertFusionMembership(t *testing.T, pool *pgxpool.Pool, userID, mergedID, sourceID, fieldKey string) {
	t.Helper()
	_, err := pool.Exec(context.Background(), `
		INSERT INTO activity_fusion_memberships (merged_id, source_id, field_key, user_id)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT DO NOTHING
	`, mergedID, sourceID, fieldKey, userID)
	if err != nil {
		t.Fatalf("insertFusionMembership: %v", err)
	}
}

// ---------------------------------------------------------------------------
// roundtripShadowFS — a test ShadowFS that:
//   - Stores PutCompanionAt data in memory keyed by path.
//   - StreamToZip writes each stored companion under "files/<path>" inside
//     the ZIP, mirroring what UnifiedShadowFS does for fusion sidecars.
//
// All other methods are no-ops that satisfy the interface.
// ---------------------------------------------------------------------------

type roundtripShadowFS struct {
	companions map[string][]byte // path → bytes
}

func newRoundtripShadowFS() *roundtripShadowFS {
	return &roundtripShadowFS{companions: make(map[string][]byte)}
}

func (r *roundtripShadowFS) PutCompanionAt(_ context.Context, path string, data []byte) error {
	r.companions[path] = data
	return nil
}

func (r *roundtripShadowFS) GetCompanionAt(_ context.Context, path string) ([]byte, error) {
	if d, ok := r.companions[path]; ok {
		return d, nil
	}
	return nil, shadowfs.ErrNotFound
}

func (r *roundtripShadowFS) DeleteCompanionAt(_ context.Context, _ string) error { return nil }

// StreamToZip writes every stored companion to "files/<path>" in the ZIP.
// This mirrors what UnifiedShadowFS does (it writes the main file first,
// then its .meta at <zipPath>.meta). For our test, fusion/manual-entry
// activities have NO main source file, only a .meta companion written via
// PutCompanionAt. We write those directly under "files/<path>" so that
// the re-import's importCompanion() call can find them.
func (r *roundtripShadowFS) StreamToZip(_ context.Context, userID string, zw *zip.Writer) ([]string, error) {
	for path, data := range r.companions {
		// Only emit companions that belong to this user.
		if !strings.HasPrefix(path, userID+"/") {
			continue
		}
		w, err := zw.Create("files/" + path)
		if err != nil {
			return nil, fmt.Errorf("roundtripShadowFS: create zip entry %q: %w", path, err)
		}
		if _, err := io.Copy(w, bytes.NewReader(data)); err != nil {
			return nil, fmt.Errorf("roundtripShadowFS: write zip entry %q: %w", path, err)
		}
	}
	return nil, nil
}

// Remaining interface methods — no-ops.

func (r *roundtripShadowFS) Put(_ context.Context, _ shadowfs.FileEntry, _ io.Reader) (shadowfs.FileRecord, error) {
	return shadowfs.FileRecord{}, nil
}
func (r *roundtripShadowFS) Get(_ context.Context, _, _ string) (io.ReadCloser, error) {
	return io.NopCloser(strings.NewReader("")), nil
}
func (r *roundtripShadowFS) Delete(_ context.Context, _, _ string) error        { return nil }
func (r *roundtripShadowFS) DeleteAllForUser(_ context.Context, _ string) error { return nil }
func (r *roundtripShadowFS) List(_ context.Context, _ string, _ *string) ([]shadowfs.FileRecord, error) {
	return nil, nil
}
func (r *roundtripShadowFS) Resolve(_ context.Context, _, _ string) (shadowfs.ResolvedFile, error) {
	return shadowfs.ResolvedFile{}, nil
}
func (r *roundtripShadowFS) DiskUsage(_ context.Context, _ string) (int64, error) { return 0, nil }
func (r *roundtripShadowFS) Archive(_ context.Context, _, _, _ string) error      { return nil }
func (r *roundtripShadowFS) PutCompanion(_ context.Context, _, _, _ string, _ []byte) error {
	return nil
}
func (r *roundtripShadowFS) GetCompanion(_ context.Context, _, _, _ string) ([]byte, error) {
	return nil, nil
}
func (r *roundtripShadowFS) DeleteCompanions(_ context.Context, _, _ string) error { return nil }

// zipReadJSON reads and unmarshals a JSON entry from an open zip.ReadCloser.
func zipReadJSON(t *testing.T, rc *zip.ReadCloser, entryName string, dest any) bool {
	t.Helper()
	for _, f := range rc.File {
		if f.Name == entryName {
			r, err := f.Open()
			if err != nil {
				t.Fatalf("zipReadJSON: open entry %q: %v", entryName, err)
			}
			defer r.Close()
			if err := json.NewDecoder(r).Decode(dest); err != nil {
				t.Fatalf("zipReadJSON: decode %q: %v", entryName, err)
			}
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// TestFusionGDPRRoundtrip — main test
// ---------------------------------------------------------------------------

func TestFusionGDPRRoundtrip(t *testing.T) {
	pool := testDBForRoundtrip(t)
	ctx := context.Background()

	// ---- 1. Seed source user ------------------------------------------------

	srcUserID := createTestUserRT(t, pool)

	// Regular activity (no fusion, no manual entry).
	regularID := insertActivity(t, pool, srcUserID, "Regular Ride", false)

	// Fusion: two source activities + one merged activity + two memberships.
	srcGpsID := insertActivity(t, pool, srcUserID, "Source GPS", false)
	srcPwrID := insertActivity(t, pool, srcUserID, "Source Power", false)
	fusedID := insertActivity(t, pool, srcUserID, "Fused Ride", false)
	insertFusionMembership(t, pool, srcUserID, fusedID, srcGpsID, "gps")
	insertFusionMembership(t, pool, srcUserID, fusedID, srcPwrID, "power")

	// Manual-entry activity.
	manualID := insertActivity(t, pool, srcUserID, "Manual Entry", true)

	// ---- 2. Write .meta sidecars via roundtripShadowFS ----------------------

	fs := newRoundtripShadowFS()

	// Fusion merged activity sidecar: {userID}/activity-{fusedID}.meta
	fusedMetaPath := fmt.Sprintf("%s/activity-%s.meta", srcUserID, fusedID)
	fusedMetaContent := []byte(`{"version":3,"type":"fusion","activity_id":"` + fusedID + `"}`)
	if err := fs.PutCompanionAt(ctx, fusedMetaPath, fusedMetaContent); err != nil {
		t.Fatalf("PutCompanionAt fused: %v", err)
	}

	// Manual-entry activity sidecar: {userID}/activity-{manualID}.meta
	manualMetaPath := fmt.Sprintf("%s/activity-%s.meta", srcUserID, manualID)
	manualMetaContent := []byte(`{"version":3,"type":"manual","activity_id":"` + manualID + `"}`)
	if err := fs.PutCompanionAt(ctx, manualMetaPath, manualMetaContent); err != nil {
		t.Fatalf("PutCompanionAt manual: %v", err)
	}

	// ---- 3. Build auth.Handler and execute export ---------------------------

	db := &database.DB{Pool: pool}
	cfg := &config.Config{JWTSecret: strings.Repeat("ab", 32)}
	testSettings := settings.NewStoreForTest(map[string]string{})
	h := NewHandler(db, cfg, testSettings, &Mailer{}, middleware.NewRateLimiter(ctx), ctx)
	h.ShadowFS = fs

	// Register an export job (executing directly needs a background_jobs row).
	jobID := uuid.New().String()
	if _, err := pool.Exec(ctx,
		`INSERT INTO background_jobs (id, job_type, user_id, status, started_at, updated_at)
		 VALUES ($1, 'gdpr_export', $2, 'running', NOW(), NOW())`,
		jobID, srcUserID,
	); err != nil {
		t.Fatalf("insert export job: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, "DELETE FROM background_jobs WHERE id = $1", jobID)
	})

	// Execute the export synchronously.
	if err := h.executeExportJobOnce(ctx, jobID, srcUserID); err != nil {
		t.Fatalf("executeExportJobOnce: %v", err)
	}

	// Retrieve the ZIP path from background_jobs metadata.
	var zipPath string
	if err := pool.QueryRow(ctx,
		`SELECT metadata->>'file_path' FROM background_jobs WHERE id = $1`, jobID,
	).Scan(&zipPath); err != nil || zipPath == "" {
		t.Fatalf("could not read export ZIP path from background_jobs: %v", err)
	}
	t.Logf("export ZIP: %s", zipPath)
	t.Cleanup(func() { os.Remove(zipPath) })

	// ---- 4. Inspect the ZIP -------------------------------------------------

	rc, err := zip.OpenReader(zipPath)
	if err != nil {
		t.Fatalf("open ZIP %q: %v", zipPath, err)
	}
	defer rc.Close()

	entrySet := make(map[string]bool)
	for _, f := range rc.File {
		entrySet[f.Name] = true
	}

	// 4a. activity_fusion_memberships.json must be present with ≥1 row.
	var memberships []map[string]any
	if !zipReadJSON(t, rc, "data/activity_fusion_memberships.json", &memberships) {
		t.Error("ZIP missing data/activity_fusion_memberships.json")
	} else if len(memberships) < 1 {
		t.Errorf("data/activity_fusion_memberships.json: expected ≥1 row, got 0")
	} else {
		t.Logf("fusion memberships in ZIP: %d", len(memberships))
	}

	// 4b. .meta sidecars must be present for fusion and manual-entry activities.
	fusedZipPath := fmt.Sprintf("files/%s/activity-%s.meta", srcUserID, fusedID)
	if !entrySet[fusedZipPath] {
		t.Errorf("ZIP missing fused activity .meta: %q (available entries: see log)", fusedZipPath)
		for name := range entrySet {
			if strings.HasSuffix(name, ".meta") {
				t.Logf("  found .meta entry: %s", name)
			}
		}
	}

	manualZipPath := fmt.Sprintf("files/%s/activity-%s.meta", srcUserID, manualID)
	if !entrySet[manualZipPath] {
		t.Errorf("ZIP missing manual-entry activity .meta: %q", manualZipPath)
	}

	// 4c. Legacy regression: no row in activities should contain "fusion_sources".
	var activities []map[string]any
	if zipReadJSON(t, rc, "data/activities.json", &activities) {
		for i, act := range activities {
			if _, hasFusionSources := act["fusion_sources"]; hasFusionSources {
				t.Errorf("activity[%d] (id=%v) has legacy 'fusion_sources' field — regression from Mig 000304", i, act["id"])
			}
		}
	}

	// 4d. Regular activity must be in the export.
	found := false
	for _, act := range activities {
		if act["id"] == regularID {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("regular activity %s not found in data/activities.json", regularID)
	}
	_ = rc.Close() // Close before mutating

	// ---- 5. Delete source user data -----------------------------------------

	// DeleteUserData requires deleted_at IS NOT NULL — mark the user.
	if _, err := pool.Exec(ctx,
		`UPDATE users SET deleted_at = NOW() WHERE id = $1`, srcUserID,
	); err != nil {
		t.Fatalf("mark user deleted_at: %v", err)
	}

	if err := db.DeleteUserData(ctx, srcUserID, nil); err != nil {
		t.Fatalf("DeleteUserData: %v", err)
	}

	// 5a. Post-delete: 0 activities for that user.
	var actCount int
	if err := pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM activities WHERE user_id = $1`, srcUserID,
	).Scan(&actCount); err != nil {
		t.Fatalf("post-delete count activities: %v", err)
	}
	if actCount != 0 {
		t.Errorf("post-delete: expected 0 activities, got %d", actCount)
	}

	// 5b. Post-delete: 0 memberships referencing srcUserID.
	var memCount int
	if err := pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM activity_fusion_memberships WHERE user_id = $1`, srcUserID,
	).Scan(&memCount); err != nil {
		t.Fatalf("post-delete count memberships: %v", err)
	}
	if memCount != 0 {
		t.Errorf("post-delete: expected 0 memberships, got %d", memCount)
	}

	// 5c. Post-delete: user row gone.
	var userExists bool
	if err := pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM users WHERE id = $1)`, srcUserID,
	).Scan(&userExists); err != nil {
		t.Fatalf("post-delete check user: %v", err)
	}
	if userExists {
		t.Error("post-delete: user row still exists — expected CASCADE delete")
	}

	// ---- 6. Re-import into a fresh target user ------------------------------

	tgtUserID := createTestUserRT(t, pool)

	src, err := dataimport.NewZipSource(zipPath)
	if err != nil {
		t.Fatalf("NewZipSource: %v", err)
	}
	defer src.Close()

	importShadowFS := newRoundtripShadowFS()
	eng := &dataimport.Engine{
		DB:       pool,
		ShadowFS: importShadowFS,
		Config:   dataimport.EngineConfig{},
	}
	result, err := eng.RunImport(ctx, dataimport.ImportInput{
		Source:       src,
		TargetUserID: tgtUserID,
	})
	if err != nil {
		t.Fatalf("RunImport: %v", err)
	}
	if len(result.Errors) > 0 {
		t.Errorf("RunImport returned %d errors: %+v", len(result.Errors), result.Errors)
	}

	// 6a. Activity count must match: 5 (regular, srcGps, srcPwr, fused, manual).
	var importedActCount int
	if err := pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM activities WHERE user_id = $1`, tgtUserID,
	).Scan(&importedActCount); err != nil {
		t.Fatalf("post-import count activities: %v", err)
	}
	if importedActCount != 5 {
		t.Errorf("post-import: expected 5 activities, got %d", importedActCount)
	}

	// 6b. Membership count must be 2 (fused←srcGps/power).
	var importedMemCount int
	if err := pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM activity_fusion_memberships
		 WHERE merged_id IN (SELECT id FROM activities WHERE user_id = $1)`, tgtUserID,
	).Scan(&importedMemCount); err != nil {
		t.Fatalf("post-import count memberships: %v", err)
	}
	if importedMemCount != 2 {
		t.Errorf("post-import: expected 2 memberships, got %d", importedMemCount)
	}

	// 6c. All imported activity UUIDs must be NEW (different from the original set).
	origIDs := map[string]bool{
		regularID: true,
		srcGpsID:  true,
		srcPwrID:  true,
		fusedID:   true,
		manualID:  true,
	}
	rows, err := pool.Query(ctx,
		`SELECT id FROM activities WHERE user_id = $1`, tgtUserID)
	if err != nil {
		t.Fatalf("query imported activity IDs: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			t.Fatalf("scan activity id: %v", err)
		}
		if origIDs[id] {
			t.Errorf("imported activity has original UUID %q — UUIDs should be remapped on import", id)
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate imported activity rows: %v", err)
	}

	// 6d. Cross-user integrity: no membership references activities outside tgtUserID.
	var crossUserCount int
	if err := pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM activity_fusion_memberships afm
		 JOIN activities merged ON merged.id = afm.merged_id
		 JOIN activities src    ON src.id    = afm.source_id
		 WHERE merged.user_id != $1 OR src.user_id != $1`, tgtUserID,
	).Scan(&crossUserCount); err != nil {
		t.Fatalf("cross-user membership check: %v", err)
	}
	if crossUserCount != 0 {
		t.Errorf("post-import: %d memberships reference activities from wrong user", crossUserCount)
	}

	t.Logf("TestFusionGDPRRoundtrip: all assertions passed (src=%s, tgt=%s)", srcUserID, tgtUserID)
}
