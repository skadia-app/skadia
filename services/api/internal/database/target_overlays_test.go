package database

import (
	"context"
	"encoding/json"
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
