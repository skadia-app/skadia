package handler

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/database/testdb"
	"github.com/skadia-app/skadia/services/api/internal/shadowfs"
	"github.com/google/uuid"
)

// TestImportFileBytes_MonitoringFITRoutedToHealthTables verifies that monitoring
// FITs imported via URL get routed to health tables and tracked with
// import_type='monitoring' (or the refined per-FIT-type value), instead of
// being treated as broken activities.
func TestImportFileBytes_MonitoringFITRoutedToHealthTables(t *testing.T) {
	// Use fixtures that are known to exist in the garmin testdata directory.
	candidates := []string{
		"../garmin/testdata/sample_type68_monitor_main.fit",
		"../garmin/testdata/sample_type32_monitoring_b.fit",
		"testdata/monitoring_sample.fit",
	}
	var fixturePath string
	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			fixturePath = p
			break
		}
	}
	if fixturePath == "" {
		t.Fatal("no monitoring FIT fixture found — expected sample_type68_monitor_main.fit or sample_type32_monitoring_b.fit in services/api/internal/garmin/testdata/. Skip statt Fatal würde den URL-Bugfix-Regressions-Test stillschweigend deaktivieren.")
	}

	fileBytes, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}

	pool := testdb.Open(t)
	db := &database.DB{Pool: pool}
	ctx := context.Background()

	suffix := uuid.NewString()[:8]
	var userID string
	if err := pool.QueryRow(ctx,
		`INSERT INTO users (username, email, registration_source)
		 VALUES ($1, $2, 'test') RETURNING id`,
		"url-mon-"+suffix, "url-mon-"+suffix+"@test.localhost",
	).Scan(&userID); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	t.Cleanup(func() {
		pool.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, userID) //nolint:errcheck
	})

	tmpDir := t.TempDir()
	// Wire ShadowFS with a local backend — mirrors the pattern from
	// services/api/internal/shadowfs/shadowfs_test.go.
	localBackend := shadowfs.NewLocalBackend(filepath.Join(tmpDir, "shadow"))
	fs := shadowfs.NewUnifiedShadowFS(pool, localBackend, nil, "local", tmpDir, shadowfs.NewSchemaRenderer(), nil, 1, 0, "", "", "")

	h := &Handler{DB: db, ShadowFS: fs}

	result, importErr := h.importFileBytes(ctx, userID, fileBytes, "monitoring.fit")
	if importErr != nil {
		t.Fatalf("importFileBytes: %v", importErr)
	}
	if result == nil || !result.MonitoringHandled {
		t.Fatalf("want MonitoringHandled=true, got %+v", result)
	}

	var importType, status string
	err = pool.QueryRow(ctx,
		`SELECT import_type, status FROM import_log WHERE user_id = $1 ORDER BY created_at DESC LIMIT 1`,
		userID,
	).Scan(&importType, &status)
	if err != nil {
		t.Fatalf("query import_log: %v", err)
	}
	if importType != "monitoring" {
		t.Fatalf("import_type: want monitoring, got %s", importType)
	}
	if status != "completed" {
		t.Fatalf("status: want completed, got %s", status)
	}
}
