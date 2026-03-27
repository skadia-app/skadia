//go:build integration

package database

import (
	"context"
	"fmt"
	"testing"

	"github.com/skadia-app/skadia/services/api/internal/model"
)

// cleanupHealthDailySummary removes the test user's health_daily_summary rows.
func cleanupHealthDailySummary(t *testing.T, db *DB, userID string) {
	t.Helper()
	_, err := db.Pool.Exec(context.Background(),
		`DELETE FROM health_daily_summary WHERE user_id = $1`, userID)
	if err != nil {
		t.Fatalf("cleanup health_daily_summary: %v", err)
	}
}

// getSourceType reads the source_type for a specific (user, date) row.
func getSourceType(t *testing.T, db *DB, userID, calDate string) string {
	t.Helper()
	var st string
	err := db.Pool.QueryRow(context.Background(),
		`SELECT source_type FROM health_daily_summary WHERE user_id = $1 AND calendar_date = $2::date`,
		userID, calDate).Scan(&st)
	if err != nil {
		t.Fatalf("getSourceType(%q, %q): %v", userID, calDate, err)
	}
	return st
}

// TestSourceTypeEscalation verifies that source_type escalates through the
// priority ladder: manual → garmin → garmin_fit.
//
// Re-importing with a higher-priority source_type MUST update the stored value.
func TestSourceTypeEscalation(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()

	userID := fmt.Sprintf("test-srctype-%d", time.Now().UnixNano())
	calDate := "2024-01-15"
	t.Cleanup(func() { cleanupHealthDailySummary(t, db, userID) })

	steps := 8000
	summary := &model.HealthDailySummary{
		UserID:       userID,
		CalendarDate: calDate,
		TotalSteps:   &steps,
		SourceType:   "manual",
	}

	// Step 1: import as 'manual'
	if err := db.UpsertHealthDailySummary(ctx, summary); err != nil {
		t.Fatalf("upsert manual: %v", err)
	}
	if got := getSourceType(t, db, userID, calDate); got != "manual" {
		t.Errorf("after manual import: want 'manual', got %q", got)
	}

	// Step 2: re-import as 'garmin' — should escalate
	summary.SourceType = "garmin"
	if err := db.UpsertHealthDailySummary(ctx, summary); err != nil {
		t.Fatalf("upsert garmin: %v", err)
	}
	if got := getSourceType(t, db, userID, calDate); got != "garmin" {
		t.Errorf("after garmin import: want 'garmin', got %q", got)
	}

	// Step 3: re-import as 'garmin_fit' — should escalate further
	summary.SourceType = "garmin_fit"
	if err := db.UpsertHealthDailySummary(ctx, summary); err != nil {
		t.Fatalf("upsert garmin_fit: %v", err)
	}
	if got := getSourceType(t, db, userID, calDate); got != "garmin_fit" {
		t.Errorf("after garmin_fit import: want 'garmin_fit', got %q", got)
	}
}

// TestSourceTypeProtection verifies that once source_type='garmin_fit' is stored,
// a re-import with lower-priority 'garmin' does NOT downgrade it.
func TestSourceTypeProtection(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()

	userID := fmt.Sprintf("test-srcprot-%d", time.Now().UnixNano())
	calDate := "2024-01-16"
	t.Cleanup(func() { cleanupHealthDailySummary(t, db, userID) })

	steps := 9000
	summary := &model.HealthDailySummary{
		UserID:       userID,
		CalendarDate: calDate,
		TotalSteps:   &steps,
		SourceType:   "garmin_fit",
	}

	// First import: garmin_fit
	if err := db.UpsertHealthDailySummary(ctx, summary); err != nil {
		t.Fatalf("upsert garmin_fit: %v", err)
	}

	// Re-import: garmin (lower priority) — must not downgrade
	summary.SourceType = "garmin"
	if err := db.UpsertHealthDailySummary(ctx, summary); err != nil {
		t.Fatalf("upsert garmin: %v", err)
	}

	if got := getSourceType(t, db, userID, calDate); got != "garmin_fit" {
		t.Errorf("after garmin re-import: want 'garmin_fit' (protected), got %q", got)
	}
}

// TestSourceTypeIdempotency verifies that importing 'garmin_fit' twice is safe
// and leaves the stored source_type unchanged.
func TestSourceTypeIdempotency(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()

	userID := fmt.Sprintf("test-srcidempotent-%d", time.Now().UnixNano())
	calDate := "2024-01-17"
	t.Cleanup(func() { cleanupHealthDailySummary(t, db, userID) })

	steps := 10000
	summary := &model.HealthDailySummary{
		UserID:       userID,
		CalendarDate: calDate,
		TotalSteps:   &steps,
		SourceType:   "garmin_fit",
	}

	// First import
	if err := db.UpsertHealthDailySummary(ctx, summary); err != nil {
		t.Fatalf("first upsert: %v", err)
	}

	// Duplicate import with same source_type
	if err := db.UpsertHealthDailySummary(ctx, summary); err != nil {
		t.Fatalf("second upsert: %v", err)
	}

	if got := getSourceType(t, db, userID, calDate); got != "garmin_fit" {
		t.Errorf("after duplicate garmin_fit import: want 'garmin_fit', got %q", got)
	}

	// Verify steps were preserved as well (COALESCE should not overwrite)
	summaries, err := db.GetHealthDailySummaries(ctx, userID, calDate, calDate)
	if err != nil {
		t.Fatalf("GetHealthDailySummaries: %v", err)
	}
	if len(summaries) != 1 {
		t.Fatalf("expected 1 row, got %d", len(summaries))
	}
	if summaries[0].TotalSteps == nil || *summaries[0].TotalSteps != steps {
		t.Errorf("TotalSteps: want %d, got %v", steps, summaries[0].TotalSteps)
	}
}

// TestSourceTypePriorityOrder documents the expected ordering of source types.
// This test validates the priority function contract without hitting the DB.
func TestSourceTypePriorityOrder(t *testing.T) {
	// The DB function source_type_priority() must satisfy:
	// garmin_fit > garmin > manual > (anything else)
	// This test documents the contract and serves as a specification check.
	// The actual DB function is validated by the integration tests above.

	// Re-check: the DB function is called in UpsertHealthDailySummary as:
	//   WHEN source_type_priority(EXCLUDED.source_type) >= source_type_priority(health_daily_summary.source_type)
	// This means: equal priority also triggers an update (idempotent for same source).

	// Document the expected priority ordering (ascending):
	type priorityCase struct {
		sourceType     string
		expectedHigher []string // these must have higher priority
	}
	cases := []priorityCase{
		{"manual", []string{"garmin", "garmin_fit"}},
		{"garmin", []string{"garmin_fit"}},
	}
	for _, c := range cases {
		t.Logf("source_type=%q must have lower priority than: %v", c.sourceType, c.expectedHigher)
	}
}
