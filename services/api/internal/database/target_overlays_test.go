package database

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/model"
	"github.com/google/uuid"
)

func TestCreateTargetReportAppendsInitialStatusEvent(t *testing.T) {
	db, userID := segmentPRTestDB(t)
	ctx := context.Background()
	place := createTargetPlaceFixture(t, db, userID, "reported place", "public", nil, `[8,49]`)

	report, err := db.CreateTargetReport(ctx, userID, model.CreateTargetReportInput{
		SubjectKind:     "place",
		SubjectID:       place.ID,
		ReportType:      "condition",
		Visibility:      "public",
		AttributionMode: "named",
		Description:     "wet trail",
		ObservedAt:      time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("CreateTargetReport: %v", err)
	}

	var status string
	if err := db.Pool.QueryRow(ctx, `
		SELECT status
		FROM target_report_status_events
		WHERE report_id = $1
	`, report.ID).Scan(&status); err != nil {
		t.Fatalf("query initial status: %v", err)
	}
	if status != "open" {
		t.Fatalf("initial status = %q, want open", status)
	}
}

func TestCreateSharedTargetReportValidatesAccessScopeOwner(t *testing.T) {
	db, ownerID := segmentPRTestDB(t)
	ctx := context.Background()
	reporterID := insertObjectAccessTestUser(t, db)
	nonGranteeID := insertObjectAccessTestUser(t, db)
	otherOwnerID := insertObjectAccessTestUser(t, db)
	place := createTargetPlaceFixture(t, db, ownerID, "shared report place", "public", nil, `[8,49]`)
	ownerScope := createTargetObjectAccessScope(t, db, ownerID, reporterID)
	otherScope := createTargetObjectAccessScope(t, db, otherOwnerID, reporterID)

	base := model.CreateTargetReportInput{
		SubjectKind:     "place",
		SubjectID:       place.ID,
		ReportType:      "condition",
		Visibility:      "shared",
		AttributionMode: "named",
		Description:     "wet trail",
		ObservedAt:      time.Now().UTC(),
	}
	badScopeInput := base
	badScopeInput.AccessScopeID = &otherScope.ID
	if _, err := db.CreateTargetReport(ctx, reporterID, badScopeInput); !errors.Is(err, ErrTargetOverlayValidation) {
		t.Fatalf("CreateTargetReport foreign shared scope err = %v, want ErrTargetOverlayValidation", err)
	}
	nonGranteeInput := base
	nonGranteeInput.AccessScopeID = &ownerScope.ID
	if _, err := db.CreateTargetReport(ctx, nonGranteeID, nonGranteeInput); !errors.Is(err, ErrTargetOverlayValidation) {
		t.Fatalf("CreateTargetReport ungranted target-owned shared scope err = %v, want ErrTargetOverlayValidation", err)
	}
	goodScopeInput := base
	goodScopeInput.AccessScopeID = &ownerScope.ID
	if _, err := db.CreateTargetReport(ctx, reporterID, goodScopeInput); err != nil {
		t.Fatalf("CreateTargetReport target-owned shared scope: %v", err)
	}
}

func TestTargetReportStatusDoesNotRewriteReport(t *testing.T) {
	db, userID := segmentPRTestDB(t)
	ctx := context.Background()
	place := createTargetPlaceFixture(t, db, userID, "status report place", "public", nil, `[8,49]`)
	report, err := db.CreateTargetReport(ctx, userID, model.CreateTargetReportInput{
		SubjectKind:     "place",
		SubjectID:       place.ID,
		ReportType:      "issue",
		Visibility:      "public",
		AttributionMode: "named",
		Description:     "fallen tree",
		ObservedAt:      time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("CreateTargetReport: %v", err)
	}

	if _, err := db.AddTargetReportStatus(ctx, userID, report.ID, model.AddTargetReportStatusInput{
		Status: "resolved",
		Reason: "cleared",
	}); err != nil {
		t.Fatalf("AddTargetReportStatus: %v", err)
	}

	got, err := db.GetTargetReport(ctx, userID, report.ID, nil)
	if err != nil {
		t.Fatalf("GetTargetReport: %v", err)
	}
	if got.Description != "fallen tree" {
		t.Fatalf("report description was rewritten to %q", got.Description)
	}
	statuses, err := db.ListTargetReportStatusEvents(ctx, userID, report.ID)
	if err != nil {
		t.Fatalf("ListTargetReportStatusEvents: %v", err)
	}
	if len(statuses) != 2 || statuses[0].Status != "open" || statuses[1].Status != "resolved" {
		t.Fatalf("statuses = %#v, want open then resolved", statuses)
	}
}

func TestTargetReportRedactedStatusRemovesPublicRead(t *testing.T) {
	db, userID := segmentPRTestDB(t)
	ctx := context.Background()
	viewerID := insertObjectAccessTestUser(t, db)
	place := createTargetPlaceFixture(t, db, userID, "redacted report place", "public", nil, `[8,49]`)
	report, err := db.CreateTargetReport(ctx, userID, model.CreateTargetReportInput{
		SubjectKind:     "place",
		SubjectID:       place.ID,
		ReportType:      "issue",
		Visibility:      "public",
		AttributionMode: "named",
		Description:     "sensitive text",
		ObservedAt:      time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("CreateTargetReport: %v", err)
	}
	if _, err := db.AddTargetReportStatus(ctx, userID, report.ID, model.AddTargetReportStatusInput{Status: "redacted", Reason: "cleanup"}); err != nil {
		t.Fatalf("AddTargetReportStatus redacted: %v", err)
	}

	if _, err := db.GetTargetReport(ctx, viewerID, report.ID, nil); err != ErrTargetNotFound {
		t.Fatalf("GetTargetReport viewer err = %v, want ErrTargetNotFound", err)
	}
	reports, err := db.ListTargetReports(ctx, model.ListTargetReportsParams{
		SubjectKind: "place",
		SubjectID:   place.ID,
		UserID:      viewerID,
		Limit:       20,
	})
	if err != nil {
		t.Fatalf("ListTargetReports: %v", err)
	}
	if len(reports) != 0 {
		t.Fatalf("redacted report listed publicly: %#v", reports)
	}
}

func TestCreateTargetObservationIsIdempotentBySourceOccurrence(t *testing.T) {
	db, userID := segmentPRTestDB(t)
	ctx := context.Background()
	place := createTargetPlaceFixture(t, db, userID, "observation place", "public", nil, `[8,49]`)
	input := model.CreateTargetActivityObservationInput{
		SubjectKind:         "place",
		SubjectID:           place.ID,
		ObservationType:     "visit",
		SourceType:          "manual",
		SourceID:            "visit-1",
		SourceOccurrenceKey: "once",
		Visibility:          "private",
		VisibilitySnapshot:  "private",
		OccurredAt:          time.Now().UTC(),
	}

	first, err := db.CreateTargetActivityObservation(ctx, model.TargetOverlayActor{UserID: userID}, input)
	if err != nil {
		t.Fatalf("CreateTargetActivityObservation first: %v", err)
	}
	second, err := db.CreateTargetActivityObservation(ctx, model.TargetOverlayActor{UserID: userID}, input)
	if err != nil {
		t.Fatalf("CreateTargetActivityObservation second: %v", err)
	}
	if first.ID != second.ID {
		t.Fatalf("idempotent observation ids differ: %s vs %s", first.ID, second.ID)
	}
}

func TestRebuildTargetObservationsFromSegmentEffortsUsesActiveSupersededAt(t *testing.T) {
	db, userID := segmentPRTestDB(t)
	ctx := context.Background()
	activityID := uuid.NewString()
	segmentID := uuid.NewString()
	versionID := uuid.NewString()
	activeEffortID := uuid.NewString()
	staleEffortID := uuid.NewString()
	startedAt := time.Now().UTC().Add(-time.Hour)

	if _, err := db.Pool.Exec(ctx, `
		INSERT INTO activities (id, user_id, sport_type, sub_type, title, started_at, file_hash, visibility, enrichment_status)
		VALUES ($1, $2, 'cycling', 'mountain', 'Observation Rebuild Test', $3, $4, 'private', 'complete')
	`, activityID, userID, startedAt, "observation-rebuild-"+activityID); err != nil {
		t.Fatalf("insert activity: %v", err)
	}
	if _, err := db.Pool.Exec(ctx, `
		INSERT INTO segments (id, created_by, name, sport_group, segment_type, visibility)
		VALUES ($1, $2, 'Observation Rebuild Segment', 'bike', 'standard', 'public')
	`, segmentID, userID); err != nil {
		t.Fatalf("insert segment: %v", err)
	}
	if _, err := db.Pool.Exec(ctx, `
		INSERT INTO segment_versions (id, segment_id, version_number, track, valid_from, created_by)
		VALUES ($1, $2, 1, ST_GeomFromText('LINESTRING(8 49, 8.01 49.01)', 4326), CURRENT_DATE, $3)
	`, versionID, segmentID, userID); err != nil {
		t.Fatalf("insert segment version: %v", err)
	}
	if _, err := db.Pool.Exec(ctx, `
		INSERT INTO segment_efforts (
			id, segment_id, segment_version_id, activity_id, user_id,
			started_at, elapsed_time_s, sport_type, sub_type, in_leaderboard
		)
		VALUES ($1, $2, $3, $4, $5, $6, 60, 'cycling', 'mountain', true)
	`, activeEffortID, segmentID, versionID, activityID, userID, startedAt); err != nil {
		t.Fatalf("insert active effort: %v", err)
	}
	if _, err := db.Pool.Exec(ctx, `
		INSERT INTO segment_efforts (
			id, segment_id, segment_version_id, activity_id, user_id,
			started_at, elapsed_time_s, sport_type, sub_type, in_leaderboard,
			superseded_at, superseded_reason
		)
		VALUES ($1, $2, $3, $4, $5, $6, 55, 'cycling', 'mountain', true, NOW(), 'recalculated_removed')
	`, staleEffortID, segmentID, versionID, activityID, userID, startedAt.Add(-time.Minute)); err != nil {
		t.Fatalf("insert stale effort: %v", err)
	}
	if _, err := db.CreateTargetActivityObservation(ctx, model.TargetOverlayActor{System: true}, model.CreateTargetActivityObservationInput{
		SubjectKind:         "course",
		SubjectID:           segmentID,
		TargetVersionID:     &versionID,
		ActivityID:          &activityID,
		UserID:              &userID,
		ObservationType:     "matched_traversal",
		SourceType:          "segment_effort",
		SourceID:            staleEffortID,
		SourceOccurrenceKey: startedAt.Add(-time.Minute).Format(time.RFC3339Nano),
		Visibility:          "public",
		VisibilitySnapshot:  "private",
		OccurredAt:          startedAt.Add(-time.Minute),
	}); err != nil {
		t.Fatalf("seed stale observation: %v", err)
	}

	count, err := db.RebuildTargetObservationsFromSegmentEfforts(ctx, segmentID, 100)
	if err != nil {
		t.Fatalf("RebuildTargetObservationsFromSegmentEfforts: %v", err)
	}
	if count != 1 {
		t.Fatalf("rebuilt observations = %d, want 1 active effort", count)
	}
	var activeObservations, staleActiveObservations int
	if err := db.Pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM target_activity_observations
		WHERE subject_id = $1
		  AND source_id = $2
		  AND superseded_at IS NULL
	`, segmentID, activeEffortID).Scan(&activeObservations); err != nil {
		t.Fatalf("count active observation: %v", err)
	}
	if err := db.Pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM target_activity_observations
		WHERE subject_id = $1
		  AND source_id = $2
		  AND superseded_at IS NULL
	`, segmentID, staleEffortID).Scan(&staleActiveObservations); err != nil {
		t.Fatalf("count stale observation: %v", err)
	}
	if activeObservations != 1 || staleActiveObservations != 0 {
		t.Fatalf("active observations=%d stale active observations=%d, want 1/0", activeObservations, staleActiveObservations)
	}
}

func TestTargetObservationRequiresLiveActivityAuthorizationForPublicRead(t *testing.T) {
	db, ownerID := segmentPRTestDB(t)
	ctx := context.Background()
	viewerID := insertObjectAccessTestUser(t, db)
	place := createTargetPlaceFixture(t, db, ownerID, "public ridelog place", "public", nil, `[8,49]`)
	activityID := insertTargetOverlayActivityFixture(t, db, ownerID, "private")

	if _, err := db.CreateTargetActivityObservation(ctx, model.TargetOverlayActor{System: true}, model.CreateTargetActivityObservationInput{
		SubjectKind:         "place",
		SubjectID:           place.ID,
		ActivityID:          &activityID,
		UserID:              &ownerID,
		ObservationType:     "visit",
		SourceType:          "activity",
		SourceID:            activityID,
		SourceOccurrenceKey: "visit-1",
		Visibility:          "public",
		VisibilitySnapshot:  "private",
		OccurredAt:          time.Now().UTC(),
	}); err != nil {
		t.Fatalf("CreateTargetActivityObservation: %v", err)
	}

	publicRows, err := db.ListTargetActivityObservations(ctx, model.ListTargetActivityObservationsParams{
		SubjectKind: "place",
		SubjectID:   place.ID,
		UserID:      viewerID,
		Limit:       20,
	})
	if err != nil {
		t.Fatalf("ListTargetActivityObservations public: %v", err)
	}
	if len(publicRows) != 0 {
		t.Fatalf("public rows leaked private source activity: %#v", publicRows)
	}

	ownerRows, err := db.ListTargetActivityObservations(ctx, model.ListTargetActivityObservationsParams{
		SubjectKind: "place",
		SubjectID:   place.ID,
		UserID:      ownerID,
		Limit:       20,
	})
	if err != nil {
		t.Fatalf("ListTargetActivityObservations owner: %v", err)
	}
	if len(ownerRows) != 1 {
		t.Fatalf("owner rows = %d, want 1", len(ownerRows))
	}
}

func TestTargetOverlaySummaryForViewerRequiresTargetVisibility(t *testing.T) {
	db, ownerID := segmentPRTestDB(t)
	ctx := context.Background()
	viewerID := insertObjectAccessTestUser(t, db)
	place := createTargetPlaceFixture(t, db, ownerID, "private summary place", "private", nil, `[8,49]`)

	if _, err := db.CreateTargetReport(ctx, ownerID, model.CreateTargetReportInput{
		SubjectKind:     "place",
		SubjectID:       place.ID,
		ReportType:      "condition",
		Visibility:      "public",
		AttributionMode: "named",
		Description:     "visible only with target",
		ObservedAt:      time.Now().UTC(),
	}); err != nil {
		t.Fatalf("CreateTargetReport: %v", err)
	}
	if _, err := db.GetTargetOverlaySummaryForViewer(ctx, viewerID, "place", place.ID, nil, nil); err != ErrTargetNotFound {
		t.Fatalf("GetTargetOverlaySummaryForViewer err = %v, want ErrTargetNotFound", err)
	}
}

func TestTargetReportSourceActivityIsOwnerBoundAndRedacted(t *testing.T) {
	db, ownerID := segmentPRTestDB(t)
	ctx := context.Background()
	viewerID := insertObjectAccessTestUser(t, db)
	place := createTargetPlaceFixture(t, db, ownerID, "source report place", "public", nil, `[8,49]`)
	privateActivityID := insertTargetOverlayActivityFixture(t, db, ownerID, "private")
	foreignActivityID := insertTargetOverlayActivityFixture(t, db, viewerID, "public")

	if _, err := db.CreateTargetReport(ctx, ownerID, model.CreateTargetReportInput{
		SubjectKind:      "place",
		SubjectID:        place.ID,
		ReportType:       "condition",
		Visibility:       "public",
		AttributionMode:  "named",
		Description:      "foreign source",
		SourceActivityID: &foreignActivityID,
		ObservedAt:       time.Now().UTC(),
	}); err != ErrTargetNotFound {
		t.Fatalf("foreign source activity err = %v, want ErrTargetNotFound", err)
	}

	report, err := db.CreateTargetReport(ctx, ownerID, model.CreateTargetReportInput{
		SubjectKind:      "place",
		SubjectID:        place.ID,
		ReportType:       "condition",
		Visibility:       "public",
		AttributionMode:  "named",
		Description:      "private source",
		SourceActivityID: &privateActivityID,
		ObservedAt:       time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("CreateTargetReport private source: %v", err)
	}
	viewerReport, err := db.GetTargetReport(ctx, viewerID, report.ID, nil)
	if err != nil {
		t.Fatalf("GetTargetReport viewer: %v", err)
	}
	if viewerReport.SourceActivityID != nil {
		t.Fatalf("source activity leaked to viewer: %v", *viewerReport.SourceActivityID)
	}
	ownerReport, err := db.GetTargetReport(ctx, ownerID, report.ID, nil)
	if err != nil {
		t.Fatalf("GetTargetReport owner: %v", err)
	}
	if ownerReport.SourceActivityID == nil || *ownerReport.SourceActivityID != privateActivityID {
		t.Fatalf("owner source activity = %v, want %s", ownerReport.SourceActivityID, privateActivityID)
	}
}

func TestTargetUserContentSaveAndRatingAreUniquePerUserTarget(t *testing.T) {
	db, userID := segmentPRTestDB(t)
	ctx := context.Background()
	place := createTargetPlaceFixture(t, db, userID, "content place", "public", nil, `[8,49]`)

	for range 2 {
		if _, err := db.CreateTargetUserContent(ctx, userID, model.CreateTargetUserContentInput{
			SubjectKind: "place",
			SubjectID:   place.ID,
			ContentKind: "save",
			Visibility:  "private",
			Payload:     json.RawMessage(`{}`),
		}); err != nil {
			t.Fatalf("CreateTargetUserContent save: %v", err)
		}
	}
	for _, rating := range []string{`{"rating":3}`, `{"rating":5}`} {
		if _, err := db.CreateTargetUserContent(ctx, userID, model.CreateTargetUserContentInput{
			SubjectKind: "place",
			SubjectID:   place.ID,
			ContentKind: "rating",
			Visibility:  "public",
			Payload:     json.RawMessage(rating),
		}); err != nil {
			t.Fatalf("CreateTargetUserContent rating: %v", err)
		}
	}

	rows, err := db.ListTargetUserContent(ctx, model.ListTargetUserContentParams{
		SubjectKind: "place",
		SubjectID:   place.ID,
		UserID:      userID,
		Limit:       20,
	})
	if err != nil {
		t.Fatalf("ListTargetUserContent: %v", err)
	}
	var saves, ratings int
	for _, row := range rows {
		if row.ContentKind == "save" {
			saves++
		}
		if row.ContentKind == "rating" {
			ratings++
			var payload struct {
				Rating int `json:"rating"`
			}
			if err := json.Unmarshal(row.Payload, &payload); err != nil {
				t.Fatalf("unmarshal rating payload: %v", err)
			}
			if payload.Rating != 5 {
				t.Fatalf("rating payload = %s, want latest", row.Payload)
			}
		}
	}
	if saves != 1 || ratings != 1 {
		t.Fatalf("saves=%d ratings=%d, want 1/1", saves, ratings)
	}
}

func TestTargetUserContentRejectsInvalidRatingPayload(t *testing.T) {
	db, userID := segmentPRTestDB(t)
	ctx := context.Background()
	place := createTargetPlaceFixture(t, db, userID, "invalid rating place", "public", nil, `[8,49]`)

	_, err := db.CreateTargetUserContent(ctx, userID, model.CreateTargetUserContentInput{
		SubjectKind: "place",
		SubjectID:   place.ID,
		ContentKind: "rating",
		Visibility:  "public",
		Payload:     json.RawMessage(`{"rating":"bad"}`),
	})
	if err == nil {
		t.Fatal("CreateTargetUserContent accepted invalid rating payload")
	}
}

func TestTargetOverlayVisibilityRejectsSharedWithoutScope(t *testing.T) {
	db, userID := segmentPRTestDB(t)
	ctx := context.Background()
	place := createTargetPlaceFixture(t, db, userID, "shared invalid place", "public", nil, `[8,49]`)

	_, err := db.CreateTargetReport(ctx, userID, model.CreateTargetReportInput{
		SubjectKind:     "place",
		SubjectID:       place.ID,
		ReportType:      "condition",
		Visibility:      "shared",
		AttributionMode: "named",
		Description:     "invalid",
		ObservedAt:      time.Now().UTC(),
	})
	if err == nil {
		t.Fatal("CreateTargetReport accepted shared visibility without access scope")
	}
}

func TestTargetOverlayRejectsMismatchedTargetVersion(t *testing.T) {
	db, userID := segmentPRTestDB(t)
	ctx := context.Background()
	segmentID := uuid.NewString()
	versionID := uuid.NewString()
	foreignVersionID := uuid.NewString()
	if _, err := db.Pool.Exec(ctx, `
		INSERT INTO segments (id, created_by, name, sport_group, segment_type, visibility)
		VALUES ($1, $2, 'versioned segment', 'bike', 'standard', 'public')
	`, segmentID, userID); err != nil {
		t.Fatalf("insert segment: %v", err)
	}
	if _, err := db.Pool.Exec(ctx, `
		INSERT INTO segment_versions (id, segment_id, version_number, track, valid_from, created_by)
		VALUES ($1, $2, 1, ST_GeomFromText('LINESTRING(8 49, 8.001 49.001)', 4326), NOW(), $3)
	`, versionID, segmentID, userID); err != nil {
		t.Fatalf("insert segment version: %v", err)
	}
	foreignSegmentID := uuid.NewString()
	if _, err := db.Pool.Exec(ctx, `
		INSERT INTO segments (id, created_by, name, sport_group, segment_type, visibility)
		VALUES ($1, $2, 'other version segment', 'bike', 'standard', 'public')
	`, foreignSegmentID, userID); err != nil {
		t.Fatalf("insert other segment: %v", err)
	}
	if _, err := db.Pool.Exec(ctx, `
		INSERT INTO segment_versions (id, segment_id, version_number, track, valid_from, created_by)
		VALUES ($1, $2, 1, ST_GeomFromText('LINESTRING(8 49, 8.001 49.001)', 4326), NOW(), $3)
	`, foreignVersionID, foreignSegmentID, userID); err != nil {
		t.Fatalf("insert foreign segment version: %v", err)
	}

	if _, err := db.CreateTargetReport(ctx, userID, model.CreateTargetReportInput{
		SubjectKind:     "course",
		SubjectID:       segmentID,
		TargetVersionID: &versionID,
		ReportType:      "condition",
		Visibility:      "public",
		AttributionMode: "named",
		Description:     "valid version",
		ObservedAt:      time.Now().UTC(),
	}); err != nil {
		t.Fatalf("CreateTargetReport valid version: %v", err)
	}
	if _, err := db.CreateTargetReport(ctx, userID, model.CreateTargetReportInput{
		SubjectKind:     "course",
		SubjectID:       segmentID,
		TargetVersionID: &foreignVersionID,
		ReportType:      "condition",
		Visibility:      "public",
		AttributionMode: "named",
		Description:     "bad version",
		ObservedAt:      time.Now().UTC(),
	}); err != ErrTargetNotFound {
		t.Fatalf("CreateTargetReport mismatched version err = %v, want ErrTargetNotFound", err)
	}
}

func TestTargetOverlaySummaryRebuildDoesNotTouchSegmentSearch(t *testing.T) {
	db, userID := segmentPRTestDB(t)
	ctx := context.Background()
	segmentID := uuid.NewString()
	if _, err := db.Pool.Exec(ctx, `
		INSERT INTO segments (id, created_by, name, sport_group, segment_type, visibility)
		VALUES ($1, $2, 'overlay hot path segment', 'bike', 'standard', 'public')
	`, segmentID, userID); err != nil {
		t.Fatalf("insert segment: %v", err)
	}

	if _, err := db.CreateTargetReport(ctx, userID, model.CreateTargetReportInput{
		SubjectKind:     "course",
		SubjectID:       segmentID,
		ReportType:      "condition",
		Visibility:      "public",
		AttributionMode: "named",
		Description:     "dry",
		ObservedAt:      time.Now().UTC(),
	}); err != nil {
		t.Fatalf("CreateTargetReport: %v", err)
	}

	result, err := db.ListSegments(ctx, model.ListSegmentsParams{UserID: userID, Limit: 20})
	if err != nil {
		t.Fatalf("ListSegments: %v", err)
	}
	var found *model.Segment
	for i := range result.Segments {
		if result.Segments[i].ID == segmentID {
			found = &result.Segments[i]
			break
		}
	}
	if found == nil {
		t.Fatal("ListSegments did not return public segment")
	}
	if found.OverlaySummary != nil {
		t.Fatalf("ListSegments included overlay summary without opt-in: %#v", found.OverlaySummary)
	}

	detail, err := db.GetSegmentWithOptions(ctx, segmentID, userID, false, GetSegmentOptions{IncludeOverlaySummary: true})
	if err != nil {
		t.Fatalf("GetSegmentWithOptions with overlay summary: %v", err)
	}
	if detail == nil || detail.OverlaySummary == nil || detail.OverlaySummary.PublicOpenReportCount != 1 {
		t.Fatalf("overlay opt-in summary = %#v", detail)
	}
}

func TestTargetOverlayHotPathSQLDoesNotUseOverlayDetailTables(t *testing.T) {
	for _, path := range []string{
		"segments.go",
		"../handler/segments.go",
		"../handler/segment_matching.go",
		"route_publications.go",
	} {
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		source := string(body)
		for _, forbidden := range []string{"target_reports", "target_activity_observations", "target_user_content"} {
			if strings.Contains(source, forbidden) {
				t.Fatalf("%s references %s in a segment/course hot path", path, forbidden)
			}
		}
		if strings.Contains(source, "target_overlay_summary_projection") {
			t.Fatalf("%s reads target_overlay_summary_projection directly; use explicit overlay summary helpers only", path)
		}
	}
}

func TestTargetOverlaySummaryExcludesPrivateRowsAndActivityBackedObservations(t *testing.T) {
	db, userID := segmentPRTestDB(t)
	ctx := context.Background()
	place := createTargetPlaceFixture(t, db, userID, "summary privacy place", "public", nil, `[8,49]`)
	activityID := insertTargetOverlayActivityFixture(t, db, userID, "public")

	if _, err := db.CreateTargetReport(ctx, userID, model.CreateTargetReportInput{
		SubjectKind:     "place",
		SubjectID:       place.ID,
		ReportType:      "condition",
		Visibility:      "private",
		AttributionMode: "named",
		Description:     "private condition",
		ObservedAt:      time.Now().UTC(),
	}); err != nil {
		t.Fatalf("CreateTargetReport private: %v", err)
	}
	for _, input := range []model.CreateTargetUserContentInput{
		{SubjectKind: "place", SubjectID: place.ID, ContentKind: "comment", Visibility: "private", Payload: json.RawMessage(`{"text":"hidden"}`)},
		{SubjectKind: "place", SubjectID: place.ID, ContentKind: "rating", Visibility: "private", Payload: json.RawMessage(`{"rating":5}`)},
		{SubjectKind: "place", SubjectID: place.ID, ContentKind: "check_in", Visibility: "private", Payload: json.RawMessage(`{}`)},
		{SubjectKind: "place", SubjectID: place.ID, ContentKind: "completion", Visibility: "private", Payload: json.RawMessage(`{}`)},
	} {
		if _, err := db.CreateTargetUserContent(ctx, userID, input); err != nil {
			t.Fatalf("CreateTargetUserContent private %s: %v", input.ContentKind, err)
		}
	}
	if _, err := db.CreateTargetActivityObservation(ctx, model.TargetOverlayActor{System: true}, model.CreateTargetActivityObservationInput{
		SubjectKind:         "place",
		SubjectID:           place.ID,
		ActivityID:          &activityID,
		UserID:              &userID,
		ObservationType:     "visit",
		SourceType:          "activity",
		SourceID:            activityID,
		SourceOccurrenceKey: "activity-backed",
		Visibility:          "public",
		VisibilitySnapshot:  "public",
		OccurredAt:          time.Now().UTC(),
	}); err != nil {
		t.Fatalf("CreateTargetActivityObservation activity-backed: %v", err)
	}

	summary, err := db.GetTargetOverlaySummary(ctx, "place", place.ID, nil)
	if err != nil {
		t.Fatalf("GetTargetOverlaySummary: %v", err)
	}
	if summary == nil {
		t.Fatal("summary is nil")
	}
	if summary.PublicOpenReportCount != 0 || summary.PublicCommentCount != 0 ||
		summary.PublicRatingCount != 0 || summary.PublicCheckInCount != 0 ||
		summary.PublicCompletionCount != 0 || summary.PublicRecentObservationCount30d != 0 ||
		summary.LastObservationAt != nil {
		t.Fatalf("private/activity-backed rows affected public summary: %#v", summary)
	}
}

func TestTargetOverlaySummaryCountsOnlyPublicLowCardinalityRows(t *testing.T) {
	db, userID := segmentPRTestDB(t)
	ctx := context.Background()
	place := createTargetPlaceFixture(t, db, userID, "summary public place", "public", nil, `[8,49]`)

	if _, err := db.CreateTargetReport(ctx, userID, model.CreateTargetReportInput{
		SubjectKind:       "place",
		SubjectID:         place.ID,
		ReportType:        "condition",
		Visibility:        "public",
		AttributionMode:   "anonymous_public",
		ConditionState:    targetOverlayStringPtr("wet"),
		ConditionSeverity: targetOverlayIntPtr(2),
		Description:       "public condition",
		ObservedAt:        time.Now().UTC(),
	}); err != nil {
		t.Fatalf("CreateTargetReport public: %v", err)
	}
	if _, err := db.CreateTargetUserContent(ctx, userID, model.CreateTargetUserContentInput{
		SubjectKind: "place",
		SubjectID:   place.ID,
		ContentKind: "rating",
		Visibility:  "public",
		Payload:     json.RawMessage(`{"rating":4}`),
	}); err != nil {
		t.Fatalf("CreateTargetUserContent rating: %v", err)
	}
	if _, err := db.CreateTargetActivityObservation(ctx, model.TargetOverlayActor{System: true}, model.CreateTargetActivityObservationInput{
		SubjectKind:         "place",
		SubjectID:           place.ID,
		ObservationType:     "visit",
		SourceType:          "manual",
		SourceID:            "manual-visit",
		SourceOccurrenceKey: "once",
		Visibility:          "public",
		VisibilitySnapshot:  "public",
		OccurredAt:          time.Now().UTC(),
	}); err != nil {
		t.Fatalf("CreateTargetActivityObservation manual: %v", err)
	}

	summary, err := db.GetTargetOverlaySummary(ctx, "place", place.ID, nil)
	if err != nil {
		t.Fatalf("GetTargetOverlaySummary: %v", err)
	}
	if summary == nil || summary.PublicOpenReportCount != 1 || summary.PublicRatingCount != 1 ||
		summary.PublicRatingMean == nil || *summary.PublicRatingMean != 4 ||
		summary.PublicRecentObservationCount30d != 1 || summary.LastObservationAt == nil {
		t.Fatalf("summary = %#v, want public report/rating/manual observation counts", summary)
	}
}

func TestTargetOverlayInvalidationSourceActivityChangesStayFailClosed(t *testing.T) {
	db, userID := segmentPRTestDB(t)
	ctx := context.Background()
	place := createTargetPlaceFixture(t, db, userID, "source invalidation place", "public", nil, `[8,49]`)
	activityID := insertTargetOverlayActivityFixture(t, db, userID, "public")

	if _, err := db.CreateTargetActivityObservation(ctx, model.TargetOverlayActor{System: true}, model.CreateTargetActivityObservationInput{
		SubjectKind:         "place",
		SubjectID:           place.ID,
		ActivityID:          &activityID,
		UserID:              &userID,
		ObservationType:     "visit",
		SourceType:          "activity",
		SourceID:            activityID,
		SourceOccurrenceKey: "source-change",
		Visibility:          "public",
		VisibilitySnapshot:  "public",
		OccurredAt:          time.Now().UTC(),
	}); err != nil {
		t.Fatalf("CreateTargetActivityObservation: %v", err)
	}
	if _, err := db.Pool.Exec(ctx, `UPDATE activities SET visibility = 'private' WHERE id = $1`, activityID); err != nil {
		t.Fatalf("update activity visibility: %v", err)
	}
	if _, err := db.Pool.Exec(ctx, `DELETE FROM activities WHERE id = $1`, activityID); err != nil {
		t.Fatalf("delete source activity: %v", err)
	}

	summary, err := db.GetTargetOverlaySummary(ctx, "place", place.ID, nil)
	if err != nil {
		t.Fatalf("GetTargetOverlaySummary: %v", err)
	}
	if summary == nil {
		t.Fatal("summary is nil")
	}
	if summary.PublicRecentObservationCount30d != 0 || summary.LastObservationAt != nil {
		t.Fatalf("source activity derived data leaked through summary: %#v", summary)
	}
}

func TestTargetOverlayPrivateOwnerCountsStayLiveOnly(t *testing.T) {
	db, userID := segmentPRTestDB(t)
	ctx := context.Background()
	place := createTargetPlaceFixture(t, db, userID, "owner live place", "public", nil, `[8,49]`)

	if _, err := db.CreateTargetUserContent(ctx, userID, model.CreateTargetUserContentInput{
		SubjectKind: "place",
		SubjectID:   place.ID,
		ContentKind: "comment",
		Visibility:  "private",
		Payload:     json.RawMessage(`{"text":"nur fuer mich"}`),
	}); err != nil {
		t.Fatalf("CreateTargetUserContent private comment: %v", err)
	}

	rows, err := db.ListTargetUserContent(ctx, model.ListTargetUserContentParams{
		SubjectKind: "place",
		SubjectID:   place.ID,
		UserID:      userID,
		Limit:       20,
	})
	if err != nil {
		t.Fatalf("ListTargetUserContent owner: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("owner live content rows = %d, want 1", len(rows))
	}
	summary, err := db.GetTargetOverlaySummary(ctx, "place", place.ID, nil)
	if err != nil {
		t.Fatalf("GetTargetOverlaySummary: %v", err)
	}
	if summary != nil && summary.PublicCommentCount != 0 {
		t.Fatalf("private owner content was cached in public summary: %#v", summary)
	}
}

func insertTargetOverlayActivityFixture(t *testing.T, db *DB, userID, visibility string) string {
	t.Helper()
	activityID := uuid.NewString()
	if _, err := db.Pool.Exec(context.Background(), `
		INSERT INTO activities (
			id, user_id, sport_type, sub_type, title, started_at,
			file_hash, visibility, enrichment_status
		)
		VALUES ($1, $2, 'cycling', 'mountain', 'overlay activity', NOW(), $3, $4, 'complete')
	`, activityID, userID, "target-overlay-"+activityID, visibility); err != nil {
		t.Fatalf("insert activity fixture: %v", err)
	}
	return activityID
}

func targetOverlayStringPtr(value string) *string {
	return &value
}

func targetOverlayIntPtr(value int) *int {
	return &value
}
