package main

import (
	"os"
	"strings"
	"testing"
)

func TestStartupBackfillsCourseEligibilityAfterMigrationsBeforeServing(t *testing.T) {
	source, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatalf("read main.go: %v", err)
	}
	text := string(source)
	migrateIdx := strings.Index(text, "migrate.Run(ctx, db.Pool)")
	backfillIdx := strings.Index(text, "BackfillAndVerifyCourseEligibilityProjections(ctx)")
	routerIdx := strings.Index(text, "chi.NewRouter()")
	if migrateIdx < 0 {
		t.Fatalf("startup migrate.Run call missing")
	}
	if backfillIdx < 0 {
		t.Fatalf("startup course eligibility backfill call missing")
	}
	if routerIdx < 0 {
		t.Fatalf("startup router setup anchor missing")
	}
	if !(migrateIdx < backfillIdx && backfillIdx < routerIdx) {
		t.Fatalf("course eligibility backfill order invalid: migrate=%d backfill=%d router=%d", migrateIdx, backfillIdx, routerIdx)
	}
}
