package testdb_test

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/skadia-app/skadia/services/api/internal/database/testdb"
)

func TestOpenSkipsWithoutEnv(t *testing.T) {
	if os.Getenv("SKADIA_TESTDB_SKIP_SUBPROCESS") == "1" {
		t.Setenv("TEST_DATABASE_URL", "")
		testdb.Open(t)
		t.Fatal("testdb.Open returned without skipping")
	}

	cmd := exec.Command(os.Args[0], "-test.run=^TestOpenSkipsWithoutEnv$", "-test.v")
	cmd.Env = withoutEnv(os.Environ(), "TEST_DATABASE_URL")
	cmd.Env = append(cmd.Env, "SKADIA_TESTDB_SKIP_SUBPROCESS=1")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("subprocess: %v\n%s", err, out)
	}
	if !bytes.Contains(out, []byte("TEST_DATABASE_URL not set")) {
		t.Fatalf("subprocess did not report TEST_DATABASE_URL skip:\n%s", out)
	}
}

func TestValidateDSNRejectsProductionDB(t *testing.T) {
	err := testdb.ValidateDSN("postgres://skadia:secret@localhost:5432/skadia?sslmode=disable", "")
	if err == nil || !strings.Contains(err.Error(), "production database") {
		t.Fatalf("ValidateDSN should reject production database, got %v", err)
	}
}

func TestValidateDSNRejectsNonTestDB(t *testing.T) {
	err := testdb.ValidateDSN("postgres://skadia:secret@localhost:5432/analytics_tmp?sslmode=disable", "")
	if err == nil || !strings.Contains(err.Error(), "skadia_test") {
		t.Fatalf("ValidateDSN should reject non-test database, got %v", err)
	}
}

func TestValidateDSNRejectsSharedTestDB(t *testing.T) {
	err := testdb.ValidateDSN("postgres://skadia:secret@localhost:5432/skadia_test?sslmode=disable", "")
	if err == nil || !strings.Contains(err.Error(), "skadia_test_<12-hex-slug>") {
		t.Fatalf("ValidateDSN should reject shared skadia_test database, got %v", err)
	}
}

func TestValidateDSNRejectsInvalidSlug(t *testing.T) {
	for _, name := range []string{
		"skadia_test_",
		"skadia_test_shared",
		"skadia_test_abcdef12345",
		"skadia_test_abcdef12345g",
	} {
		t.Run(name, func(t *testing.T) {
			err := testdb.ValidateDSN("postgres://skadia:secret@localhost:5432/"+name+"?sslmode=disable", "")
			if err == nil || !strings.Contains(err.Error(), "skadia_test_<12-hex-slug>") {
				t.Fatalf("ValidateDSN should reject invalid slug %q, got %v", name, err)
			}
		})
	}
}

func TestValidateDSNRejectsConflictingDatabaseURL(t *testing.T) {
	testDSN := "postgres://skadia:secret@localhost:5432/skadia_test_abcdef123456?sslmode=disable"
	databaseURL := "postgres://skadia:secret@localhost:5432/skadia?sslmode=disable"
	err := testdb.ValidateDSN(testDSN, databaseURL)
	if err == nil || !strings.Contains(err.Error(), "DATABASE_URL must be unset or equal TEST_DATABASE_URL") {
		t.Fatalf("ValidateDSN should reject conflicting DATABASE_URL, got %v", err)
	}
}

func TestValidateDSNAllowsEqualEnvWhenDatabaseIsTestDB(t *testing.T) {
	dsn := "postgres://skadia:secret@localhost:5432/skadia_test_abcdef123456?sslmode=disable"
	if err := testdb.ValidateDSN(dsn, dsn); err != nil {
		t.Fatalf("ValidateDSN should allow DATABASE_URL equality for isolated test DB: %v", err)
	}
}

func TestOpenReturnsPoolWhenEnvSet(t *testing.T) {
	pool := testdb.Open(t)
	if err := pool.Ping(context.Background()); err != nil {
		t.Fatalf("Ping: %v", err)
	}
}

func withoutEnv(env []string, key string) []string {
	prefix := key + "="
	filtered := make([]string, 0, len(env))
	for _, entry := range env {
		if strings.HasPrefix(entry, prefix) {
			continue
		}
		filtered = append(filtered, entry)
	}
	return filtered
}
