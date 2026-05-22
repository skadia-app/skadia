package main

import (
	"os"
	"strings"
	"testing"
)

func TestStartupBackfillsCourseEligibilityAfterMigrationsWithoutBlockingHealthz(t *testing.T) {
	source, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatalf("read main.go: %v", err)
	}
	text := string(source)
	migrateIdx := strings.Index(text, "migrate.Run(ctx, db.Pool)")
	readyGateIdx := strings.Index(text, "courseEligibilityProjectionReady := &atomic.Bool{}")
	routerIdx := strings.Index(text, "chi.NewRouter()")
	backfillIdx := strings.LastIndex(text, "EnsureCourseEligibilityProjections(ctx)")
	serverIdx := strings.Index(text, "srv.ListenAndServe()")
	if migrateIdx < 0 {
		t.Fatalf("startup migrate.Run call missing")
	}
	if readyGateIdx < 0 {
		t.Fatalf("startup course eligibility readiness gate missing")
	}
	if backfillIdx < 0 {
		t.Fatalf("startup course eligibility backfill call missing")
	}
	if routerIdx < 0 {
		t.Fatalf("startup router setup anchor missing")
	}
	if serverIdx < 0 {
		t.Fatalf("startup server anchor missing")
	}
	if !(migrateIdx < readyGateIdx && readyGateIdx < routerIdx && routerIdx < backfillIdx && backfillIdx < serverIdx) {
		t.Fatalf("course eligibility startup order invalid: migrate=%d readyGate=%d router=%d backfill=%d server=%d", migrateIdx, readyGateIdx, routerIdx, backfillIdx, serverIdx)
	}
}
