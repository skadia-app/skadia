package migrations_test

// Safety guard for migration integration tests. Refuses to run when
// TEST_DATABASE_URL points at a non-test database, so a buggy test like
// the historical migration_000333_test.go::TestMigration000333DownRaisesNotice
// (which executed down.sql via conn.Exec without a transaction) cannot drop
// real schema again.
//
// The guard rejects two configurations:
//   1. TEST_DATABASE_URL unset -> t.Skip (existing behavior; CI without DB)
//   2. TEST_DATABASE_URL parsed DB name is not skadia_test_<slug>
//
// DATABASE_URL may equal TEST_DATABASE_URL only when both point at the isolated
// test DB provided by scripts/test_db.sh run.

import (
	"bytes"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/skadia-app/skadia/services/api/internal/database/testdb"
)

// requireSafeTestDSN returns TEST_DATABASE_URL after asserting it does not
// reference the production database. Skips the test if unset.
func requireSafeTestDSN(t *testing.T) string {
	t.Helper()
	return testdb.RequireSafeDSN(t)
}

// TestForbidsDirectDatabaseURLInTests grep-checks the repo for direct DB env
// reads in *_test.go files. Runtime DSN validation lives in
// services/api/internal/database/testdb.
func TestForbidsDirectDatabaseURLInTests(t *testing.T) {
	repoRootBytes, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
	if err != nil {
		t.Skipf("not in git: %v", err)
	}
	root := strings.TrimSpace(string(repoRootBytes))

	var hits []string
	err = filepath.WalkDir(filepath.Join(root, "services/api"), func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == "vendor" || d.Name() == "node_modules" {
				return fs.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(p, "_test.go") {
			return nil
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		if rel == "services/api/internal/migrate/migrations/testdb_safety_test.go" {
			return nil
		}
		if strings.HasPrefix(rel, "services/api/internal/database/testdb/") {
			return nil
		}
		body, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		if bytes.Contains(body, []byte(`os.Getenv("DATABASE_URL")`)) ||
			bytes.Contains(body, []byte(`os.Getenv("TEST_DATABASE_URL")`)) ||
			bytes.Contains(body, []byte(`os.LookupEnv("DATABASE_URL")`)) ||
			bytes.Contains(body, []byte(`os.LookupEnv("TEST_DATABASE_URL")`)) {
			hits = append(hits, rel)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk: %v", err)
	}
	if len(hits) > 0 {
		sort.Strings(hits)
		t.Errorf("forbidden direct database env access in %d test file(s):\n  %s\n\nUse services/api/internal/database/testdb.Open(t) or testdb.RequireSafeDSN(t) instead.",
			len(hits), strings.Join(hits, "\n  "))
	}
}
