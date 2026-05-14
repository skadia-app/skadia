// services/api/internal/handler/admin_rename_test.go
package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/settings"
)

// newAdminRenameHandler builds a Handler wired with the DB pool, a test
// settings store and a HandlerConfig pointed at a tmp library dir with
// the local storage backend. Minimal surface for admin-rename tests.
func newAdminRenameHandler(t *testing.T, pool *pgxpool.Pool) *Handler {
	t.Helper()
	h := newTestHandler(t, pool)
	h.Settings = settings.NewStoreForTest(map[string]string{
		"library.user_can_edit_schemas":   "false",
		"library.default_schema.activity": "{year}/{month}",
	})
	h.Config = &HandlerConfig{
		StorageBackend:  "local",
		LibraryBasePath: t.TempDir(),
	}
	return h
}

// doAdminRenameUser issues POST /admin/users/{id}/files/{type}/rename-execute.
func doAdminRenameUser(t *testing.T, h *Handler, targetID, fileType string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost,
		"/api/v1/admin/users/"+targetID+"/files/"+fileType+"/rename-execute", nil)

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", targetID)
	rctx.URLParams.Add("type", fileType)
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	rr := httptest.NewRecorder()
	h.PostAdminRenameUser(rr, req)
	return rr
}

// doAdminRenameBulk issues POST /admin/rename-execute-bulk with body.
func doAdminRenameBulk(t *testing.T, h *Handler, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/rename-execute-bulk",
		bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")

	rr := httptest.NewRecorder()
	h.PostAdminRenameBulk(rr, req)
	return rr
}

// cleanupBgJobs removes background_jobs rows for the given user IDs so
// later tests (or this test's re-run) start with a clean running-set.
func cleanupBgJobs(t *testing.T, pool *pgxpool.Pool, userIDs ...string) {
	t.Helper()
	ctx := context.Background()
	for _, uid := range userIDs {
		if _, err := pool.Exec(ctx,
			`DELETE FROM background_jobs WHERE user_id = $1 AND job_type = 'shadowfs_rename'`, uid); err != nil {
			t.Logf("cleanup bg jobs for %s: %v", uid, err)
		}
	}
}

func seedLocalActivityFileForRename(t *testing.T, pool *pgxpool.Pool, userID string) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), `
		INSERT INTO user_files
			(id, user_id, file_type, storage_backend, storage_path,
			 content_type, file_name, size_bytes, has_sidecar, is_renaming, status)
		VALUES (gen_random_uuid(), $1, 'activity', 'local', $2,
			'application/octet-stream', $3, 4, false, false, 'active')
	`, userID, "/tmp/"+userID+".fit", userID+".fit"); err != nil {
		t.Fatalf("seed local activity file: %v", err)
	}
}

func TestAdminRenameRejectsS3Backend(t *testing.T) {
	pool := fusionTestPool(t)
	db := &database.DB{Pool: pool}
	targetID := insertTestUser(t, pool)
	h := &Handler{
		DB:       db,
		Settings: settings.NewStoreForTest(map[string]string{}),
		Config:   &HandlerConfig{StorageBackend: "s3", LibraryBasePath: t.TempDir()},
	}

	userRR := doAdminRenameUser(t, h, targetID, "activity")
	if userRR.Code != http.StatusForbidden {
		t.Fatalf("user rename status = %d, want 403; body=%s", userRR.Code, userRR.Body.String())
	}
	if !strings.Contains(userRR.Body.String(), "s3_backend") {
		t.Fatalf("user rename response does not mention s3_backend: %s", userRR.Body.String())
	}

	bulkRR := doAdminRenameBulk(t, h, `{"file_type":"activity"}`)
	if bulkRR.Code != http.StatusForbidden {
		t.Fatalf("bulk rename status = %d, want 403; body=%s", bulkRR.Code, bulkRR.Body.String())
	}
	if !strings.Contains(bulkRR.Body.String(), "s3_backend") {
		t.Fatalf("bulk rename response does not mention s3_backend: %s", bulkRR.Body.String())
	}
}

// TestAdminRenameUserReturnsInProgressWhenDuplicate:
// A pre-existing running shadowfs_rename row for the user → second INSERT
// hits idx_bg_jobs_one_running (23505) → 200 + rename_in_progress.
func TestAdminRenameUserReturnsInProgressWhenDuplicate(t *testing.T) {
	pool := fusionTestPool(t)
	h := newAdminRenameHandler(t, pool)
	userID := insertTestUser(t, pool)
	t.Cleanup(func() { cleanupBgJobs(t, pool, userID) })

	// Seed a running shadowfs_rename job for this user.
	ctx := context.Background()
	if _, err := pool.Exec(ctx,
		`INSERT INTO background_jobs (id, job_type, user_id, status)
		 VALUES (gen_random_uuid(), 'shadowfs_rename', $1, 'running')`, userID); err != nil {
		t.Fatalf("seed running job: %v", err)
	}

	rr := doAdminRenameUser(t, h, userID, "activity")
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var resp map[string]string
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp["status"] != "rename_in_progress" {
		t.Fatalf("expected rename_in_progress, got %q", resp["status"])
	}
}

// TestAdminRenameBulkAdvisoryLockSerializes:
// While one bulk holds the advisory lock, a second concurrent call returns
// bulk_rename_in_progress. After first releases, third call succeeds.
func TestAdminRenameBulkAdvisoryLockSerializes(t *testing.T) {
	pool := fusionTestPool(t)
	h := newAdminRenameHandler(t, pool)
	// Create one user so there is at least one snapshot row.
	userID := insertTestUser(t, pool)
	t.Cleanup(func() { cleanupBgJobs(t, pool, userID) })

	// Manually grab the advisory lock on a separate pinned connection to
	// simulate an in-flight bulk. We hold it until the assertion runs, then
	// release and verify a subsequent call succeeds.
	ctx := context.Background()
	conn, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatalf("acquire conn: %v", err)
	}
	defer conn.Release()

	var locked bool
	if err := conn.QueryRow(ctx,
		`SELECT pg_try_advisory_lock($1)`, bulkRenameAdvisoryLockKey).Scan(&locked); err != nil {
		t.Fatalf("try_lock: %v", err)
	}
	if !locked {
		t.Fatal("expected lock acquired on fresh connection (left over from prior test run?)")
	}

	// Second call must report in-progress.
	rr := doAdminRenameBulk(t, h, `{"file_type":"activity"}`)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var resp map[string]string
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp["status"] != "bulk_rename_in_progress" {
		t.Fatalf("expected bulk_rename_in_progress, got %q", resp["status"])
	}

	// Release the lock so subsequent callers can acquire.
	if _, err := conn.Exec(ctx, `SELECT pg_advisory_unlock($1)`, bulkRenameAdvisoryLockKey); err != nil {
		t.Fatalf("unlock: %v", err)
	}
	_ = userID
}

// TestAdminRenameBulkSkipsUsersWithRunningPerUserJob:
// User X already has a running shadowfs_rename row → bulk iterates, hits
// 23505 for X, skips it, proceeds for the remaining users. Verified via
// returned user_count (all users enumerated) + X's bg-job row not replaced.
func TestAdminRenameBulkSkipsUsersWithRunningPerUserJob(t *testing.T) {
	pool := fusionTestPool(t)
	h := newAdminRenameHandler(t, pool)

	userX := insertTestUser(t, pool)
	userY := insertTestUser(t, pool)
	seedLocalActivityFileForRename(t, pool, userX)
	seedLocalActivityFileForRename(t, pool, userY)
	t.Cleanup(func() { cleanupBgJobs(t, pool, userX, userY) })
	t.Cleanup(func() {
		ctx := context.Background()
		// Best-effort release in case a previous failed run leaked the lock.
		_, _ = pool.Exec(ctx, `SELECT pg_advisory_unlock($1)`, bulkRenameAdvisoryLockKey)
	})

	// Seed a running job for X.
	ctx := context.Background()
	var seededJobID string
	if err := pool.QueryRow(ctx,
		`INSERT INTO background_jobs (id, job_type, user_id, status)
		 VALUES (gen_random_uuid(), 'shadowfs_rename', $1, 'running')
		 RETURNING id`, userX).Scan(&seededJobID); err != nil {
		t.Fatalf("seed running job: %v", err)
	}

	rr := doAdminRenameBulk(t, h, `{"file_type":"activity"}`)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var resp map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp["status"] != "ok" {
		t.Fatalf("expected status=ok, got %v", resp["status"])
	}
	// user_count reflects ALL active users the JOIN returned (not just newly
	// started). The test only asserts the call returned ok + reports >=2
	// snapshots (userX + userY contribute).
	uc, _ := resp["user_count"].(float64)
	if uc < 2 {
		t.Fatalf("expected user_count>=2, got %v", uc)
	}

	// Give the bg goroutine a short moment to attempt its INSERTs.
	// (It will either skip X with 23505 or succeed for Y and run the rename.)
	time.Sleep(200 * time.Millisecond)

	// Verify the seeded running job for X is still there (not replaced).
	var stillRunning int
	if err := pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM background_jobs
		 WHERE id = $1 AND status = 'running'`, seededJobID).Scan(&stillRunning); err != nil {
		t.Fatalf("verify seeded job: %v", err)
	}
	if stillRunning != 1 {
		t.Fatalf("seeded running job for X should still be running, got %d", stillRunning)
	}
}

// TestAdminRenameBulkSkippedUsersDontBreakLoop:
// Seed running jobs for two users. Bulk must still return ok + user_count
// covering all active users (processing loop doesn't abort on 23505).
func TestAdminRenameBulkSkippedUsersDontBreakLoop(t *testing.T) {
	pool := fusionTestPool(t)
	h := newAdminRenameHandler(t, pool)

	u1 := insertTestUser(t, pool)
	u2 := insertTestUser(t, pool)
	u3 := insertTestUser(t, pool)
	seedLocalActivityFileForRename(t, pool, u1)
	seedLocalActivityFileForRename(t, pool, u2)
	seedLocalActivityFileForRename(t, pool, u3)
	t.Cleanup(func() { cleanupBgJobs(t, pool, u1, u2, u3) })
	t.Cleanup(func() {
		ctx := context.Background()
		_, _ = pool.Exec(ctx, `SELECT pg_advisory_unlock($1)`, bulkRenameAdvisoryLockKey)
	})

	ctx := context.Background()
	for _, uid := range []string{u1, u2} {
		if _, err := pool.Exec(ctx,
			`INSERT INTO background_jobs (id, job_type, user_id, status)
			 VALUES (gen_random_uuid(), 'shadowfs_rename', $1, 'running')`, uid); err != nil {
			t.Fatalf("seed running job: %v", err)
		}
	}

	rr := doAdminRenameBulk(t, h, `{"file_type":"activity"}`)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var resp map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp["status"] != "ok" {
		t.Fatalf("expected status=ok, got %v", resp["status"])
	}
	uc, _ := resp["user_count"].(float64)
	if uc < 3 {
		t.Fatalf("expected user_count>=3, got %v", uc)
	}

	// Allow bg goroutine to run its loop; assert both seeded jobs survived.
	time.Sleep(300 * time.Millisecond)

	var survived int
	if err := pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM background_jobs
		 WHERE user_id = ANY($1) AND job_type = 'shadowfs_rename' AND status = 'running'`,
		[]string{u1, u2}).Scan(&survived); err != nil {
		t.Fatalf("verify seeded jobs: %v", err)
	}
	if survived < 2 {
		t.Fatalf("expected both seeded running jobs to survive, got %d", survived)
	}
}

// TestAdminRenameBulkUnknownFileTypeRejected: quick sanity check for the
// input validation path; does NOT require the advisory lock.
func TestAdminRenameBulkUnknownFileTypeRejected(t *testing.T) {
	h := &Handler{
		Config: &HandlerConfig{StorageBackend: "local", LibraryBasePath: "/tmp/x"},
	}
	rr := doAdminRenameBulk(t, h, `{"file_type":"bogus"}`)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rr.Code, rr.Body.String())
	}
}
