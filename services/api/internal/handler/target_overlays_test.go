package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/model"
	"github.com/google/uuid"
)

func TestCreateTargetReport(t *testing.T) {
	pool := fusionTestPool(t)
	h := newTestHandler(t, pool)
	userID := insertTestUser(t, pool)
	place := createHandlerPlace(t, h, userID, "public", nil)

	body := `{"subject_kind":"place","subject_id":"` + place.ID + `","report_type":"condition","visibility":"public","attribution_mode":"named","description":"nass","observed_at":"` + time.Now().UTC().Format(time.RFC3339) + `"}`
	rr := httptest.NewRecorder()
	h.CreateTargetReport(rr, targetRequest(http.MethodPost, "/target-overlays/reports", []byte(body), userID))
	if rr.Code != http.StatusCreated {
		t.Fatalf("status = %d body=%s", rr.Code, rr.Body.String())
	}
	var resp struct {
		Report model.TargetReport `json:"report"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Report.ID == "" || resp.Report.LatestStatus == nil || *resp.Report.LatestStatus != "open" {
		t.Fatalf("report response = %#v", resp.Report)
	}
}

func TestListTargetReportsHonorsTargetVisibility(t *testing.T) {
	pool := fusionTestPool(t)
	h := newTestHandler(t, pool)
	ownerID := insertTestUser(t, pool)
	otherID := insertTestUser(t, pool)
	place := createHandlerPlace(t, h, ownerID, "private", nil)
	createHandlerReport(t, h, ownerID, "place", place.ID, "private")

	rr := httptest.NewRecorder()
	h.ListTargetReports(rr, targetRequest(http.MethodGet, "/target-overlays/reports?subject_kind=place&subject_id="+place.ID, nil, otherID))
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rr.Code, rr.Body.String())
	}
	var resp struct {
		Reports []model.TargetReport `json:"reports"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(resp.Reports) != 0 {
		t.Fatalf("private target reports leaked: %#v", resp.Reports)
	}
}

func TestReportStatusAppend(t *testing.T) {
	pool := fusionTestPool(t)
	h := newTestHandler(t, pool)
	userID := insertTestUser(t, pool)
	place := createHandlerPlace(t, h, userID, "public", nil)
	report := createHandlerReport(t, h, userID, "place", place.ID, "public")

	rr := httptest.NewRecorder()
	h.AddTargetReportStatus(rr, targetRequestWithParam(http.MethodPost, "/target-overlays/reports/"+report.ID+"/status", []byte(`{"status":"resolved","reason":"erledigt"}`), userID, "id", report.ID))
	if rr.Code != http.StatusCreated {
		t.Fatalf("status = %d body=%s", rr.Code, rr.Body.String())
	}

	statuses, err := h.DB.ListTargetReportStatusEvents(context.Background(), userID, report.ID)
	if err != nil {
		t.Fatalf("ListTargetReportStatusEvents: %v", err)
	}
	if len(statuses) != 2 || statuses[1].Status != "resolved" {
		t.Fatalf("statuses = %#v", statuses)
	}
}

func TestPrivateTargetReportDoesNotLeakThroughPublicTarget(t *testing.T) {
	pool := fusionTestPool(t)
	h := newTestHandler(t, pool)
	authorID := insertTestUser(t, pool)
	viewerID := insertTestUser(t, pool)
	place := createHandlerPlace(t, h, authorID, "public", nil)
	report := createHandlerReport(t, h, authorID, "place", place.ID, "private")

	rr := httptest.NewRecorder()
	h.GetTargetReport(rr, targetRequestWithParam(http.MethodGet, "/target-overlays/reports/"+report.ID, nil, viewerID, "id", report.ID))
	if rr.Code != http.StatusNotFound {
		t.Fatalf("status = %d body=%s, want private report hidden", rr.Code, rr.Body.String())
	}
}

func TestTargetOwnerCanAppendStatusToPrivateReport(t *testing.T) {
	pool := fusionTestPool(t)
	h := newTestHandler(t, pool)
	ownerID := insertTestUser(t, pool)
	reporterID := insertTestUser(t, pool)
	place := createHandlerPlace(t, h, ownerID, "public", nil)
	report := createHandlerReport(t, h, reporterID, "place", place.ID, "private")

	rr := httptest.NewRecorder()
	h.AddTargetReportStatus(rr, targetRequestWithParam(http.MethodPost, "/target-overlays/reports/"+report.ID+"/status", []byte(`{"status":"informational","reason":"gesehen"}`), ownerID, "id", report.ID))
	if rr.Code != http.StatusCreated {
		t.Fatalf("status = %d body=%s", rr.Code, rr.Body.String())
	}
}

func TestListTargetObservationsHidesPrivateActivity(t *testing.T) {
	pool := fusionTestPool(t)
	h := newTestHandler(t, pool)
	ownerID := insertTestUser(t, pool)
	viewerID := insertTestUser(t, pool)
	place := createHandlerPlace(t, h, ownerID, "public", nil)
	activityID := insertHandlerOverlayActivity(t, h, ownerID, "private")
	if _, err := h.DB.CreateTargetActivityObservation(context.Background(), model.TargetOverlayActor{System: true}, model.CreateTargetActivityObservationInput{
		SubjectKind:         "place",
		SubjectID:           place.ID,
		ActivityID:          &activityID,
		UserID:              &ownerID,
		ObservationType:     "visit",
		SourceType:          "activity",
		SourceID:            activityID,
		SourceOccurrenceKey: "once",
		Visibility:          "public",
		VisibilitySnapshot:  "private",
		OccurredAt:          time.Now().UTC(),
	}); err != nil {
		t.Fatalf("CreateTargetActivityObservation: %v", err)
	}

	rr := httptest.NewRecorder()
	h.ListTargetObservations(rr, targetRequest(http.MethodGet, "/target-overlays/observations?subject_kind=place&subject_id="+place.ID, nil, viewerID))
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rr.Code, rr.Body.String())
	}
	if strings.Contains(rr.Body.String(), activityID) {
		t.Fatalf("private activity id leaked in observations: %s", rr.Body.String())
	}
}

func TestCreateTargetUserContent(t *testing.T) {
	pool := fusionTestPool(t)
	h := newTestHandler(t, pool)
	userID := insertTestUser(t, pool)
	place := createHandlerPlace(t, h, userID, "public", nil)

	body := `{"subject_kind":"place","subject_id":"` + place.ID + `","content_kind":"comment","visibility":"public","payload":{"text":"passt"}}`
	rr := httptest.NewRecorder()
	h.CreateTargetUserContent(rr, targetRequest(http.MethodPost, "/target-overlays/content", []byte(body), userID))
	if rr.Code != http.StatusCreated {
		t.Fatalf("status = %d body=%s", rr.Code, rr.Body.String())
	}
	var resp struct {
		Content model.TargetUserContent `json:"content"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Content.ID == "" || resp.Content.ContentKind != "comment" {
		t.Fatalf("content response = %#v", resp.Content)
	}
}

func TestSegmentDetailCanOptIntoOverlaySummary(t *testing.T) {
	pool := fusionTestPool(t)
	h := newTestHandler(t, pool)
	userID := insertTestUser(t, pool)
	segmentID := insertTestSegment(t, pool, userID, TestSegmentOpts{})
	createHandlerReport(t, h, userID, "course", segmentID, "public")

	rr := httptest.NewRecorder()
	h.GetSegment(rr, targetRequestWithParam(http.MethodGet, "/segments/"+segmentID+"?include_overlay_summary=true", nil, userID, "id", segmentID))
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rr.Code, rr.Body.String())
	}
	var seg model.Segment
	if err := json.Unmarshal(rr.Body.Bytes(), &seg); err != nil {
		t.Fatalf("decode segment: %v", err)
	}
	if seg.OverlaySummary == nil || seg.OverlaySummary.PublicOpenReportCount != 1 {
		t.Fatalf("overlay summary = %#v, want report count", seg.OverlaySummary)
	}
}

func TestSegmentListDoesNotLoadOverlaySummaryByDefault(t *testing.T) {
	pool := fusionTestPool(t)
	h := newTestHandler(t, pool)
	userID := insertTestUser(t, pool)
	segmentID := insertTestSegment(t, pool, userID, TestSegmentOpts{})
	createHandlerReport(t, h, userID, "course", segmentID, "public")

	rr := httptest.NewRecorder()
	h.ListSegments(rr, targetRequest(http.MethodGet, "/segments?limit=-1", nil, userID))
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rr.Code, rr.Body.String())
	}
	var resp model.ListSegmentsResult
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode segment list: %v", err)
	}
	for _, segment := range resp.Segments {
		if segment.ID == segmentID && segment.OverlaySummary != nil {
			t.Fatalf("segment list loaded overlay summary by default: %#v", segment.OverlaySummary)
		}
	}
}

func createHandlerReport(t *testing.T, h *Handler, userID, subjectKind, subjectID, visibility string) *model.TargetReport {
	t.Helper()
	report, err := h.DB.CreateTargetReport(context.Background(), userID, model.CreateTargetReportInput{
		SubjectKind:     subjectKind,
		SubjectID:       subjectID,
		ReportType:      "condition",
		Visibility:      visibility,
		AttributionMode: "named",
		Description:     "fixture",
		ObservedAt:      time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("CreateTargetReport: %v", err)
	}
	return report
}

func insertHandlerOverlayActivity(t *testing.T, h *Handler, userID, visibility string) string {
	t.Helper()
	activityID := uuid.NewString()
	if _, err := h.DB.Pool.Exec(context.Background(), `
		INSERT INTO activities (
			id, user_id, sport_type, sub_type, title, started_at,
			file_hash, visibility, enrichment_status
		)
		VALUES ($1, $2, 'cycling', 'mountain', 'handler overlay activity', NOW(), $3, $4, 'complete')
	`, activityID, userID, "handler-target-overlay-"+activityID, visibility); err != nil {
		t.Fatalf("insert overlay activity: %v", err)
	}
	return activityID
}
