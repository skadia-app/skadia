package handler

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/model"
	"github.com/skadia-app/skadia/services/api/internal/shutdown"
	"github.com/skadia-app/skadia/services/api/internal/sport"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestCreateSegmentRejectsInvalidSegmentTypeBeforeDB(t *testing.T) {
	h := &Handler{}
	r := chi.NewRouter()
	r.Post("/api/v1/segments", h.CreateSegment)

	body := `{"name":"bad","segment_type":"bogus","geojson_track":"{\"type\":\"LineString\",\"coordinates\":[[0,0],[1,1]]}"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/segments", bytes.NewReader([]byte(body)))
	req = req.WithContext(withUserAndAdmin(req.Context(), "00000000-0000-0000-0000-000000000001", false))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body: %s", rec.Code, rec.Body.String())
	}
}

func TestCreateCheckpointCourseAPI_DB(t *testing.T) {
	pool := fusionTestPool(t)
	h := newTestHandler(t, pool)
	r := chi.NewRouter()
	r.Post("/api/v1/segments", h.CreateSegment)
	r.Get("/api/v1/segments", h.ListSegments)
	r.Get("/api/v1/segments/{id}", h.GetSegment)

	userID := insertTestUser(t, pool)
	body := `{
		"name":"api-checkpoint-course",
		"sport_group":"bike",
		"course_kind":"checkpoint_sequence",
		"visibility":"private",
		"controls":[
			{"sequence_index":0,"role":"start","primitive":"point","geometry_shape":"point","geometry":{"type":"Point","coordinates":[8,49]},"hit_radius_m":30,"required":true},
			{"sequence_index":1,"role":"finish","primitive":"point","geometry_shape":"point","geometry":{"type":"Point","coordinates":[8.02,49.02]},"hit_radius_m":30,"required":true}
		]
	}`
	rec := performSegmentRequest(t, r, http.MethodPost, "/api/v1/segments", body, userID)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create status = %d, want 201; body: %s", rec.Code, rec.Body.String())
	}
	var seg model.Segment
	if err := json.Unmarshal(rec.Body.Bytes(), &seg); err != nil {
		t.Fatalf("unmarshal create response: %v", err)
	}
	if seg.CourseKind != "checkpoint_sequence" {
		t.Fatalf("course_kind = %q", seg.CourseKind)
	}
	if seg.ControlSummary == nil || seg.ControlSummary.ControlCount != 2 {
		t.Fatalf("control_summary = %#v", seg.ControlSummary)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM segments WHERE id = $1`, seg.ID)
	})

	rec = performSegmentRequest(t, r, http.MethodGet, "/api/v1/segments?course_kind=checkpoint_sequence&include_control_map_features=true&limit=-1", "", userID)
	if rec.Code != http.StatusOK {
		t.Fatalf("list status = %d, want 200; body: %s", rec.Code, rec.Body.String())
	}
	var list model.ListSegmentsResult
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil {
		t.Fatalf("unmarshal list response: %v", err)
	}
	if !segmentListContains(list.Segments, seg.ID) {
		t.Fatalf("checkpoint list did not include %s: %#v", seg.ID, list.Segments)
	}

	rec = performSegmentRequest(t, r, http.MethodGet, "/api/v1/segments/"+seg.ID+"?include_geojson=true&include_controls=true", "", userID)
	if rec.Code != http.StatusOK {
		t.Fatalf("detail status = %d, want 200; body: %s", rec.Code, rec.Body.String())
	}
	var detail model.Segment
	if err := json.Unmarshal(rec.Body.Bytes(), &detail); err != nil {
		t.Fatalf("unmarshal detail response: %v", err)
	}
	if detail.CourseKind != "checkpoint_sequence" {
		t.Fatalf("detail course_kind = %q", detail.CourseKind)
	}
	if detail.TrackGeoJSON == nil || !strings.Contains(*detail.TrackGeoJSON, "LineString") {
		t.Fatalf("detail display line missing: %#v", detail.TrackGeoJSON)
	}
	if len(detail.Controls) != 2 {
		t.Fatalf("detail controls count = %d, want 2", len(detail.Controls))
	}
	if detail.Controls[0].Role != model.CheckpointControlStart || detail.Controls[1].Role != model.CheckpointControlFinish {
		t.Fatalf("detail control roles = %#v", detail.Controls)
	}

	rec = performSegmentRequest(t, r, http.MethodGet, "/api/v1/segments?include_controls=true", "", userID)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("list include_controls status = %d, want 400; body: %s", rec.Code, rec.Body.String())
	}
}

func TestExportImportCheckpointCourseAPI_DB(t *testing.T) {
	pool := fusionTestPool(t)
	h := newTestHandler(t, pool)
	r := chi.NewRouter()
	r.Post("/api/v1/segments", h.CreateSegment)
	r.Get("/api/v1/segments/{id}/export", h.ExportSegment)
	r.Post("/api/v1/segments/import", h.ImportSegment)

	userID := insertTestUser(t, pool)
	body := `{
		"name":"api-checkpoint-export",
		"sport_group":"bike",
		"course_kind":"checkpoint_sequence",
		"visibility":"private",
		"controls":[
			{"sequence_index":0,"role":"start","primitive":"point","geometry_shape":"point","label":"A","geometry":{"type":"Point","coordinates":[8,49]},"hit_radius_m":30,"required":true},
			{"sequence_index":1,"role":"finish","primitive":"area","geometry_shape":"circle","label":"B","geometry":{"type":"Point","coordinates":[8.02,49.02]},"area_radius_m":80,"required":true}
		]
	}`
	rec := performSegmentRequest(t, r, http.MethodPost, "/api/v1/segments", body, userID)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create status = %d, want 201; body: %s", rec.Code, rec.Body.String())
	}
	var seg model.Segment
	if err := json.Unmarshal(rec.Body.Bytes(), &seg); err != nil {
		t.Fatalf("unmarshal create response: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM segments WHERE name IN ('api-checkpoint-export', 'api-checkpoint-export-imported')`)
	})

	rec = performSegmentRequest(t, r, http.MethodGet, "/api/v1/segments/"+seg.ID+"/export?format=geojson", "", userID)
	if rec.Code != http.StatusOK {
		t.Fatalf("export status = %d, want 200; body: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"display_only":true`) || !strings.Contains(rec.Body.String(), `"checkpoint_sequence_v1"`) {
		t.Fatalf("export body missing checkpoint contract/display marker: %s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"area_radius_m":80`) {
		t.Fatalf("export body missing area radius: %s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"import_semantics":"private_copy_rebased_on_import"`) ||
		!strings.Contains(rec.Body.String(), `"source_validity_basis"`) ||
		!strings.Contains(rec.Body.String(), `"source_eligibility_policy_key"`) {
		t.Fatalf("export body missing checkpoint import/source metadata: %s", rec.Body.String())
	}

	importBody := strings.Replace(rec.Body.String(), "api-checkpoint-export", "api-checkpoint-export-imported", 1)
	rec = performSegmentRequest(t, r, http.MethodPost, "/api/v1/segments/import", importBody, userID)
	if rec.Code != http.StatusCreated {
		t.Fatalf("import status = %d, want 201; body: %s", rec.Code, rec.Body.String())
	}
	var imported model.Segment
	if err := json.Unmarshal(rec.Body.Bytes(), &imported); err != nil {
		t.Fatalf("unmarshal import response: %v", err)
	}
	if imported.CourseKind != "checkpoint_sequence" {
		t.Fatalf("imported course_kind = %q", imported.CourseKind)
	}
	if imported.ControlSummary == nil || imported.ControlSummary.ControlCount != 2 {
		t.Fatalf("imported control_summary = %#v", imported.ControlSummary)
	}
}

func TestCreateSegmentFromActivityDoesNotWaitForSourceMatch_DB(t *testing.T) {
	pool := fusionTestPool(t)
	db := newTestHandler(t, pool).DB
	h := &Handler{DB: db}
	r := chi.NewRouter()
	r.Post("/api/v1/segments", h.CreateSegment)
	shutdown.Shutdown(time.Millisecond)
	t.Cleanup(shutdown.Reset)

	userID := insertTestUser(t, pool)
	pts := [][2]float64{
		{0, 0.00},
		{0, 0.25},
		{0, 0.50},
		{0, 0.75},
		{0, 1.00},
	}
	actID := insertActivityWithTrack(t, db, userID, "cycling", "LINESTRING(0 0, 0.25 0, 0.5 0, 0.75 0, 1 0)", pts)

	body := `{
		"name":"source-match",
		"sport_group":"bike",
		"segment_type":"standard",
		"geojson_track":"{\"type\":\"LineString\",\"coordinates\":[[0,0],[0.25,0],[0.5,0],[0.75,0],[1,0]]}",
		"source_activity_id":"` + actID + `"
	}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/segments", bytes.NewReader([]byte(body)))
	req = req.WithContext(withUserAndAdmin(req.Context(), userID, false))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body: %s", rec.Code, rec.Body.String())
	}
	var seg model.Segment
	if err := json.Unmarshal(rec.Body.Bytes(), &seg); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if seg.SourceActivityID == nil || *seg.SourceActivityID != actID {
		t.Fatalf("source_activity_id = %v, want %s", seg.SourceActivityID, actID)
	}

	var effortCount int
	if err := pool.QueryRow(context.Background(),
		`SELECT COUNT(*) FROM segment_efforts WHERE activity_id = $1 AND segment_id = $2`,
		actID, seg.ID,
	).Scan(&effortCount); err != nil {
		t.Fatalf("count source effort: %v", err)
	}
	if effortCount != 0 {
		t.Fatalf("source effort count before background match = %d, want 0", effortCount)
	}
}

func TestCreateSegmentFromActivityRequiresPrivacyZoneConfirmation_DB(t *testing.T) {
	pool := fusionTestPool(t)
	db := newTestHandler(t, pool).DB
	h := &Handler{DB: db}
	r := chi.NewRouter()
	r.Post("/api/v1/segments", h.CreateSegment)
	shutdown.Shutdown(time.Millisecond)
	t.Cleanup(shutdown.Reset)

	userID := insertTestUser(t, pool)
	pts := [][2]float64{
		{49.0000, 8.0000},
		{49.0005, 8.0005},
		{49.0010, 8.0010},
		{49.0015, 8.0015},
		{49.0020, 8.0020},
	}
	actID := insertActivityWithTrack(t, db, userID, "cycling", "LINESTRING(8 49, 8.0005 49.0005, 8.001 49.001, 8.0015 49.0015, 8.002 49.002)", pts)
	insertActivePrivacyZone(t, pool, userID, "POINT(8 49)", 100)

	lineBody := `{
		"name":"privacy-confirm-line",
		"sport_group":"bike",
		"segment_type":"standard",
		"visibility":"public",
		"geojson_track":"{\"type\":\"LineString\",\"coordinates\":[[8,49],[8.001,49.001],[8.002,49.002]]}",
		"source_activity_id":"` + actID + `"
	}`
	rec := performSegmentRequest(t, r, http.MethodPost, "/api/v1/segments", lineBody, userID)
	if rec.Code != http.StatusConflict {
		t.Fatalf("line create without confirmation status = %d, want 409; body: %s", rec.Code, rec.Body.String())
	}
	lineBody = strings.Replace(lineBody, `"source_activity_id":"`+actID+`"`, `"source_activity_id":"`+actID+`","source_activity_publish_confirmed":true`, 1)
	rec = performSegmentRequest(t, r, http.MethodPost, "/api/v1/segments", lineBody, userID)
	if rec.Code != http.StatusCreated {
		t.Fatalf("line create with confirmation status = %d, want 201; body: %s", rec.Code, rec.Body.String())
	}
	var lineSeg model.Segment
	if err := json.Unmarshal(rec.Body.Bytes(), &lineSeg); err != nil {
		t.Fatalf("unmarshal line create response: %v", err)
	}
	assertSourceActivityPublishConfirmationStored(t, pool, lineSeg.ID, userID)

	checkpointBody := `{
		"name":"privacy-confirm-checkpoint",
		"sport_group":"bike",
		"course_kind":"checkpoint_sequence",
		"visibility":"public",
		"source_activity_id":"` + actID + `",
		"controls":[
			{"sequence_index":0,"role":"start","primitive":"point","geometry_shape":"point","geometry":{"type":"Point","coordinates":[8,49]},"hit_radius_m":30,"required":true},
			{"sequence_index":1,"role":"finish","primitive":"point","geometry_shape":"point","geometry":{"type":"Point","coordinates":[8.002,49.002]},"hit_radius_m":30,"required":true}
		]
	}`
	rec = performSegmentRequest(t, r, http.MethodPost, "/api/v1/segments", checkpointBody, userID)
	if rec.Code != http.StatusConflict {
		t.Fatalf("checkpoint create without confirmation status = %d, want 409; body: %s", rec.Code, rec.Body.String())
	}
	checkpointBody = strings.Replace(checkpointBody, `"source_activity_id":"`+actID+`",`, `"source_activity_id":"`+actID+`","source_activity_publish_confirmed":true,`, 1)
	rec = performSegmentRequest(t, r, http.MethodPost, "/api/v1/segments", checkpointBody, userID)
	if rec.Code != http.StatusCreated {
		t.Fatalf("checkpoint create with confirmation status = %d, want 201; body: %s", rec.Code, rec.Body.String())
	}
	var checkpointSeg model.Segment
	if err := json.Unmarshal(rec.Body.Bytes(), &checkpointSeg); err != nil {
		t.Fatalf("unmarshal checkpoint create response: %v", err)
	}
	assertSourceActivityPublishConfirmationStored(t, pool, checkpointSeg.ID, userID)
}

func TestPublishSegmentFromPrivacyZoneSourceRequiresCurrentVersionConfirmation_DB(t *testing.T) {
	pool := fusionTestPool(t)
	db := newTestHandler(t, pool).DB
	h := &Handler{DB: db}
	r := chi.NewRouter()
	r.Post("/api/v1/segments", h.CreateSegment)
	shutdown.Shutdown(time.Millisecond)
	t.Cleanup(shutdown.Reset)

	userID := insertTestUser(t, pool)
	pts := [][2]float64{
		{49.0000, 8.0000},
		{49.0005, 8.0005},
		{49.0010, 8.0010},
	}
	actID := insertActivityWithTrack(t, db, userID, "cycling", "LINESTRING(8 49, 8.0005 49.0005, 8.001 49.001)", pts)
	insertActivePrivacyZone(t, pool, userID, "POINT(8 49)", 100)

	createBody := `{
		"name":"publish-privacy-confirm",
		"sport_group":"bike",
		"segment_type":"standard",
		"visibility":"private",
		"geojson_track":"{\"type\":\"LineString\",\"coordinates\":[[8,49],[8.001,49.001]]}",
		"source_activity_id":"` + actID + `"
	}`
	rec := performSegmentRequest(t, r, http.MethodPost, "/api/v1/segments", createBody, userID)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create private source segment status = %d, want 201; body: %s", rec.Code, rec.Body.String())
	}
	var seg model.Segment
	if err := json.Unmarshal(rec.Body.Bytes(), &seg); err != nil {
		t.Fatalf("unmarshal create response: %v", err)
	}

	published, _, _, err := db.PublishSegmentAndSeedRecalculationRun(context.Background(), seg.ID, userID, false, "publish-privacy-red", false, nil)
	if !errors.Is(err, database.ErrSegmentPublishPrivacyConfirmationRequired) {
		t.Fatalf("publish without confirmation error = %v, want ErrSegmentPublishPrivacyConfirmationRequired", err)
	}
	if published {
		t.Fatal("publish without confirmation returned published=true")
	}
	published, _, _, err = db.PublishSegmentAndSeedRecalculationRun(context.Background(), seg.ID, userID, false, "publish-privacy-confirmed", true, nil)
	if err != nil {
		t.Fatalf("publish with confirmation: %v", err)
	}
	if !published {
		t.Fatal("publish with confirmation returned published=false")
	}
	var confirmedVersionID, activeVersionID *string
	if err := pool.QueryRow(context.Background(), `
		SELECT s.source_activity_publish_confirmed_segment_version_id::text, sv.id::text
		FROM segments s
		JOIN LATERAL (
			SELECT id
			FROM segment_versions
			WHERE segment_id = s.id
			  AND matching_contract_key = 'course_gps_v1'
			  AND matching_contract_version = 1
			  AND validity_basis <> 'rollback_retired'
			  AND (valid_from_at IS NULL OR valid_from_at <= NOW())
			  AND (valid_until_at IS NULL OR NOW() < valid_until_at)
			ORDER BY COALESCE(valid_from_at, '-infinity'::timestamptz) DESC, version_number DESC
			LIMIT 1
		) sv ON true
		WHERE s.id = $1
	`, seg.ID).Scan(&confirmedVersionID, &activeVersionID); err != nil {
		t.Fatalf("load publish confirmation version: %v", err)
	}
	if confirmedVersionID == nil || activeVersionID == nil || *confirmedVersionID != *activeVersionID {
		t.Fatalf("confirmed version = %v, active version = %v", confirmedVersionID, activeVersionID)
	}
}

func TestUpdateSegmentReturnsValidationErrorForIneligiblePredicate_DB(t *testing.T) {
	pool := fusionTestPool(t)
	h := newTestHandler(t, pool)
	r := chi.NewRouter()
	r.Put("/api/v1/segments/{id}", h.UpdateSegment)

	userID := insertTestUser(t, pool)
	segID := insertTestSegment(t, pool, userID, TestSegmentOpts{SegmentType: "wassersport"})
	body := `{"allowed_predicates":[{"sport":"diving"}]}`
	req := httptest.NewRequest(http.MethodPut, "/api/v1/segments/"+segID, bytes.NewReader([]byte(body)))
	req = req.WithContext(withUserAndAdmin(req.Context(), userID, false))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422; body: %s", rec.Code, rec.Body.String())
	}
}

func TestUpdateSegmentRejectsTypeChangeWithExistingIneligiblePredicate_DB(t *testing.T) {
	pool := fusionTestPool(t)
	h := newTestHandler(t, pool)
	r := chi.NewRouter()
	r.Put("/api/v1/segments/{id}", h.UpdateSegment)

	userID := insertTestUser(t, pool)
	segID := insertTestSegment(t, pool, userID, TestSegmentOpts{SegmentType: "standard"})
	if _, err := pool.Exec(context.Background(), `
		UPDATE segments
		SET sport_group = 'bike',
		    allowed_predicates = '[{"sport":"cycling","sub_type":"mountain"}]'::jsonb
		WHERE id = $1`, segID); err != nil {
		t.Fatalf("seed segment predicates: %v", err)
	}

	body := `{"segment_type":"wintersport"}`
	req := httptest.NewRequest(http.MethodPut, "/api/v1/segments/"+segID, bytes.NewReader([]byte(body)))
	req = req.WithContext(withUserAndAdmin(req.Context(), userID, false))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422; body: %s", rec.Code, rec.Body.String())
	}
}

func TestSegmentsHandlerParityExplicitQueryParsing(t *testing.T) {
	pool := fusionTestPool(t)
	h := newTestHandler(t, pool)
	r := chi.NewRouter()
	r.Get("/api/v1/segments", h.ListSegments)

	userID := insertTestUser(t, pool)
	segID := insertTestSegment(t, pool, userID, TestSegmentOpts{SegmentType: "standard"})
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM segments WHERE id = $1`, segID)
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/segments?segment_types=standard,trail&limit=-1&include_geojson=true&completed_only=false&bbox=-1,-1,2,2&sac_scale=T1&mtb_scale=S1&surface=Asphalt&visibility=public", nil)
	req = req.WithContext(withUserAndAdmin(req.Context(), userID, false))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body.String())
	}
	var resp model.ListSegmentsResult
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if resp.Segments == nil {
		t.Fatalf("segments = nil, want response envelope with slice")
	}
	if resp.HasMore {
		t.Fatalf("has_more = true, want false for limit=-1")
	}
}

func TestSegmentSharedVisibilityHandlersEnforceAccessScope(t *testing.T) {
	pool := fusionTestPool(t)
	h := newTestHandler(t, pool)
	r := chi.NewRouter()
	r.Get("/api/v1/segments", h.ListSegments)
	r.Post("/api/v1/segments", h.CreateSegment)
	r.Get("/api/v1/segments/{id}", h.GetSegment)
	r.Get("/api/v1/segments/{id}/favorite", h.GetSegmentFavoriteStatus)
	r.Get("/api/v1/segments/{id}/performances", h.GetSegmentPerformances)
	r.Get("/api/v1/segments/{id}/export", h.ExportSegment)
	r.Post("/api/v1/segments/export", h.ExportSegments)
	shutdown.Shutdown(time.Millisecond)
	t.Cleanup(shutdown.Reset)

	ownerID := insertTestUser(t, pool)
	viewerID := insertTestUser(t, pool)
	strangerID := insertTestUser(t, pool)
	scope, err := h.DB.CreateObjectAccessScope(context.Background(), routeObjectAccessScopeInput(ownerID, viewerID))
	if err != nil {
		t.Fatalf("CreateObjectAccessScope: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM segments WHERE access_scope_id = $1`, scope.ID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM object_access_scopes WHERE id = $1`, scope.ID)
	})

	createBody := `{
		"name":"shared-segment",
		"sport_group":"bike",
		"segment_type":"standard",
		"visibility":"shared",
		"access_scope_id":"` + scope.ID + `",
		"geojson_track":"{\"type\":\"LineString\",\"coordinates\":[[0,0],[1,1]]}"
	}`
	rec := performSegmentRequest(t, r, http.MethodPost, "/api/v1/segments", createBody, ownerID)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create status = %d, want 201; body: %s", rec.Code, rec.Body.String())
	}
	var seg model.Segment
	if err := json.Unmarshal(rec.Body.Bytes(), &seg); err != nil {
		t.Fatalf("unmarshal create response: %v", err)
	}

	rec = performSegmentRequest(t, r, http.MethodGet, "/api/v1/segments?visibility=shared&limit=-1", "", viewerID)
	if rec.Code != http.StatusOK {
		t.Fatalf("shared list status = %d, want 200; body: %s", rec.Code, rec.Body.String())
	}
	var list model.ListSegmentsResult
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil {
		t.Fatalf("unmarshal list response: %v", err)
	}
	if !segmentListContains(list.Segments, seg.ID) {
		t.Fatalf("shared list did not include granted segment %s", seg.ID)
	}

	for _, target := range []string{
		"/api/v1/segments/" + seg.ID,
		"/api/v1/segments/" + seg.ID + "/favorite",
		"/api/v1/segments/" + seg.ID + "/performances",
		"/api/v1/segments/" + seg.ID + "/export?format=geojson",
	} {
		rec = performSegmentRequest(t, r, http.MethodGet, target, "", viewerID)
		if rec.Code != http.StatusOK {
			t.Fatalf("viewer GET %s status = %d, want 200; body: %s", target, rec.Code, rec.Body.String())
		}
		rec = performSegmentRequest(t, r, http.MethodGet, target, "", strangerID)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("stranger GET %s status = %d, want 404; body: %s", target, rec.Code, rec.Body.String())
		}
	}

	exportBody := `{"ids":["` + seg.ID + `"],"format":"geojson"}`
	rec = performSegmentRequest(t, r, http.MethodPost, "/api/v1/segments/export", exportBody, viewerID)
	if rec.Code != http.StatusOK {
		t.Fatalf("viewer bulk export status = %d, want 200; body: %s", rec.Code, rec.Body.String())
	}
	if got := zipEntryCount(t, rec.Body.Bytes()); got != 1 {
		t.Fatalf("viewer bulk export entries = %d, want 1", got)
	}
	rec = performSegmentRequest(t, r, http.MethodPost, "/api/v1/segments/export", exportBody, strangerID)
	if rec.Code != http.StatusOK {
		t.Fatalf("stranger bulk export status = %d, want 200; body: %s", rec.Code, rec.Body.String())
	}
	if got := zipEntryCount(t, rec.Body.Bytes()); got != 0 {
		t.Fatalf("stranger bulk export entries = %d, want 0", got)
	}
}

func TestSegmentSharedVisibilityRejectsInvalidAccessScopePairs(t *testing.T) {
	pool := fusionTestPool(t)
	h := newTestHandler(t, pool)
	r := chi.NewRouter()
	r.Post("/api/v1/segments", h.CreateSegment)
	r.Put("/api/v1/segments/{id}", h.UpdateSegment)
	shutdown.Shutdown(time.Millisecond)
	t.Cleanup(shutdown.Reset)

	ownerID := insertTestUser(t, pool)
	viewerID := insertTestUser(t, pool)
	scope, err := h.DB.CreateObjectAccessScope(context.Background(), routeObjectAccessScopeInput(ownerID, viewerID))
	if err != nil {
		t.Fatalf("CreateObjectAccessScope: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM segments WHERE access_scope_id = $1`, scope.ID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM object_access_scopes WHERE id = $1`, scope.ID)
	})

	base := `"name":"invalid-share","sport_group":"bike","segment_type":"standard","geojson_track":"{\"type\":\"LineString\",\"coordinates\":[[0,0],[1,1]]}"`
	for _, body := range []string{
		`{` + base + `,"visibility":"shared"}`,
		`{` + base + `,"visibility":"private","access_scope_id":"` + scope.ID + `"}`,
		`{` + base + `,"visibility":"public","access_scope_id":"` + scope.ID + `"}`,
	} {
		rec := performSegmentRequest(t, r, http.MethodPost, "/api/v1/segments", body, ownerID)
		if rec.Code != http.StatusUnprocessableEntity {
			t.Fatalf("create status = %d, want 422; body: %s", rec.Code, rec.Body.String())
		}
	}

	segID := insertTestSegment(t, pool, ownerID, TestSegmentOpts{})
	rec := performSegmentRequest(t, r, http.MethodPut, "/api/v1/segments/"+segID, `{"visibility":"shared"}`, ownerID)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("update shared without scope status = %d, want 422; body: %s", rec.Code, rec.Body.String())
	}
	rec = performSegmentRequest(t, r, http.MethodPut, "/api/v1/segments/"+segID, `{"access_scope_id":"`+scope.ID+`"}`, ownerID)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("update public with scope status = %d, want 422; body: %s", rec.Code, rec.Body.String())
	}
}

func TestSegmentsHandlerLapForcesFlatElevationKind(t *testing.T) {
	pool := fusionTestPool(t)
	h := newTestHandler(t, pool)
	r := chi.NewRouter()
	r.Post("/api/v1/segments", h.CreateSegment)
	r.Put("/api/v1/segments/{id}", h.UpdateSegment)

	userID := insertTestUser(t, pool)
	body := `{
		"name":"lap-elevation",
		"sport_group":"bike",
		"segment_type":"lap",
		"climb_type":"bergauf",
		"elevation_kind":"uphill",
		"geojson_track":"{\"type\":\"LineString\",\"coordinates\":[[0,0],[0.001,0],[0,0]]}"
	}`
	rec := performSegmentRequest(t, r, http.MethodPost, "/api/v1/segments", body, userID)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create status = %d, want 201; body: %s", rec.Code, rec.Body.String())
	}
	var seg model.Segment
	if err := json.Unmarshal(rec.Body.Bytes(), &seg); err != nil {
		t.Fatalf("unmarshal create response: %v", err)
	}
	if seg.ElevationKind != "flat" {
		t.Fatalf("created elevation_kind = %s, want flat", seg.ElevationKind)
	}
	if seg.ClimbType == nil || *seg.ClimbType != "standard" {
		t.Fatalf("created climb_type = %#v, want standard", seg.ClimbType)
	}

	rec = performSegmentRequest(t, r, http.MethodPut, "/api/v1/segments/"+seg.ID, `{"climb_type":"bergab","elevation_kind":"downhill"}`, userID)
	if rec.Code != http.StatusOK {
		t.Fatalf("update status = %d, want 200; body: %s", rec.Code, rec.Body.String())
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &seg); err != nil {
		t.Fatalf("unmarshal update response: %v", err)
	}
	if seg.ElevationKind != "flat" {
		t.Fatalf("updated elevation_kind = %s, want flat", seg.ElevationKind)
	}
	if seg.ClimbType == nil || *seg.ClimbType != "standard" {
		t.Fatalf("updated climb_type = %#v, want standard", seg.ClimbType)
	}
}

func TestSegmentsHandlerLegacyClimbTypeMapsToElevationKind(t *testing.T) {
	pool := fusionTestPool(t)
	h := newTestHandler(t, pool)
	r := chi.NewRouter()
	r.Post("/api/v1/segments", h.CreateSegment)
	r.Put("/api/v1/segments/{id}", h.UpdateSegment)

	userID := insertTestUser(t, pool)
	for _, tc := range []struct {
		climb string
		want  string
	}{
		{"standard", "flat"},
		{"bergauf", "uphill"},
		{"bergfahrt", "uphill"},
		{"bergab", "downhill"},
		{"talfahrt", "downhill"},
	} {
		t.Run("create_"+tc.climb, func(t *testing.T) {
			body := `{"name":"legacy-map","sport_group":"bike","segment_type":"standard","climb_type":"` + tc.climb + `","geojson_track":"{\"type\":\"LineString\",\"coordinates\":[[0,0],[1,1]]}"}`
			rec := performSegmentRequest(t, r, http.MethodPost, "/api/v1/segments", body, userID)
			if rec.Code != http.StatusCreated {
				t.Fatalf("status = %d, want 201; body: %s", rec.Code, rec.Body.String())
			}
			var seg model.Segment
			if err := json.Unmarshal(rec.Body.Bytes(), &seg); err != nil {
				t.Fatalf("unmarshal response: %v", err)
			}
			if seg.ElevationKind != tc.want {
				t.Fatalf("elevation_kind = %s, want %s", seg.ElevationKind, tc.want)
			}
		})
	}

	seed := `{"name":"legacy-update","sport_group":"bike","segment_type":"standard","geojson_track":"{\"type\":\"LineString\",\"coordinates\":[[0,0],[1,1]]}"}`
	rec := performSegmentRequest(t, r, http.MethodPost, "/api/v1/segments", seed, userID)
	if rec.Code != http.StatusCreated {
		t.Fatalf("seed status = %d, want 201; body: %s", rec.Code, rec.Body.String())
	}
	var seg model.Segment
	if err := json.Unmarshal(rec.Body.Bytes(), &seg); err != nil {
		t.Fatalf("unmarshal seed response: %v", err)
	}
	rec = performSegmentRequest(t, r, http.MethodPut, "/api/v1/segments/"+seg.ID, `{"climb_type":"bergfahrt"}`, userID)
	if rec.Code != http.StatusOK {
		t.Fatalf("update status = %d, want 200; body: %s", rec.Code, rec.Body.String())
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &seg); err != nil {
		t.Fatalf("unmarshal update response: %v", err)
	}
	if seg.ElevationKind != "uphill" {
		t.Fatalf("updated elevation_kind = %s, want uphill", seg.ElevationKind)
	}
}

func TestSegmentsHandlerRejectsRouteStyleElevationKinds(t *testing.T) {
	pool := fusionTestPool(t)
	h := newTestHandler(t, pool)
	r := chi.NewRouter()
	r.Get("/api/v1/segments", h.ListSegments)
	r.Post("/api/v1/segments", h.CreateSegment)
	r.Put("/api/v1/segments/{id}", h.UpdateSegment)

	userID := insertTestUser(t, pool)
	for _, invalid := range []string{"rolling", "mountainous", "variable", "steep"} {
		t.Run("create_"+invalid, func(t *testing.T) {
			body := `{"name":"invalid-elevation","sport_group":"bike","segment_type":"standard","elevation_kind":"` + invalid + `","geojson_track":"{\"type\":\"LineString\",\"coordinates\":[[0,0],[1,1]]}"}`
			rec := performSegmentRequest(t, r, http.MethodPost, "/api/v1/segments", body, userID)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400; body: %s", rec.Code, rec.Body.String())
			}
		})
		t.Run("list_"+invalid, func(t *testing.T) {
			rec := performSegmentRequest(t, r, http.MethodGet, "/api/v1/segments?elevation_kind="+invalid, "", userID)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400; body: %s", rec.Code, rec.Body.String())
			}
		})
	}

	segID := insertTestSegment(t, pool, userID, TestSegmentOpts{})
	rec := performSegmentRequest(t, r, http.MethodPut, "/api/v1/segments/"+segID, `{"elevation_kind":"variable"}`, userID)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("update status = %d, want 400; body: %s", rec.Code, rec.Body.String())
	}
}

func TestSegmentsHandlerUpdatePreservesElevationKindWhenOmitted(t *testing.T) {
	pool := fusionTestPool(t)
	h := newTestHandler(t, pool)
	r := chi.NewRouter()
	r.Post("/api/v1/segments", h.CreateSegment)
	r.Put("/api/v1/segments/{id}", h.UpdateSegment)

	userID := insertTestUser(t, pool)
	body := `{"name":"hilly-preserve","sport_group":"bike","segment_type":"standard","elevation_kind":"hilly","geojson_track":"{\"type\":\"LineString\",\"coordinates\":[[0,0],[1,1]]}"}`
	rec := performSegmentRequest(t, r, http.MethodPost, "/api/v1/segments", body, userID)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create status = %d, want 201; body: %s", rec.Code, rec.Body.String())
	}
	var seg model.Segment
	if err := json.Unmarshal(rec.Body.Bytes(), &seg); err != nil {
		t.Fatalf("unmarshal create response: %v", err)
	}
	rec = performSegmentRequest(t, r, http.MethodPut, "/api/v1/segments/"+seg.ID, `{"description":"still hilly"}`, userID)
	if rec.Code != http.StatusOK {
		t.Fatalf("update status = %d, want 200; body: %s", rec.Code, rec.Body.String())
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &seg); err != nil {
		t.Fatalf("unmarshal update response: %v", err)
	}
	if seg.ElevationKind != "hilly" {
		t.Fatalf("elevation_kind = %s, want hilly", seg.ElevationKind)
	}
}

func TestSegmentsHandlerRejectsInvalidElevationKindQuery(t *testing.T) {
	h := &Handler{}
	r := chi.NewRouter()
	r.Get("/api/v1/segments", h.ListSegments)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/segments?elevation_kind=rolling", nil)
	req = req.WithContext(withUserAndAdmin(req.Context(), "00000000-0000-0000-0000-000000000001", false))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body: %s", rec.Code, rec.Body.String())
	}
}

func TestSubTypeResponsesAreDeterministic(t *testing.T) {
	got := subTypeResponses(sport.SportDef{
		SportType: "test",
		SubTypes: map[string]sport.SubTypeDef{
			"z_last":  {SubType: "z_last", LabelDE: "Z"},
			"a_first": {SubType: "a_first", LabelDE: "A"},
		},
	})
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2", len(got))
	}
	if got[0].SubType != "a_first" || got[1].SubType != "z_last" {
		t.Fatalf("subtype order = %#v, want sorted by sub_type", got)
	}
}

func segmentListContains(segments []model.Segment, id string) bool {
	for _, segment := range segments {
		if segment.ID == id {
			return true
		}
	}
	return false
}

func zipEntryCount(t *testing.T, raw []byte) int {
	t.Helper()
	reader, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		t.Fatalf("open zip response: %v", err)
	}
	return len(reader.File)
}

func performSegmentRequest(t *testing.T, r http.Handler, method, target, body, userID string) *httptest.ResponseRecorder {
	t.Helper()
	var reader *bytes.Reader
	if body == "" {
		reader = bytes.NewReader(nil)
	} else {
		reader = bytes.NewReader([]byte(body))
	}
	req := httptest.NewRequest(method, target, reader)
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(withUserAndAdmin(req.Context(), userID, false))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

func insertActivePrivacyZone(t *testing.T, pool *pgxpool.Pool, userID, centerWKT string, radiusM int) string {
	t.Helper()
	var id string
	err := pool.QueryRow(context.Background(), `
		INSERT INTO user_privacy_zones (user_id, name, center, radius_m, is_active, zone_type)
		VALUES ($1, 'source publish confirmation', ST_GeomFromText($2, 4326), $3, true, 'circle')
		RETURNING id
	`, userID, centerWKT, radiusM).Scan(&id)
	if err != nil {
		t.Fatalf("insert privacy zone: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM user_privacy_zones WHERE id = $1`, id)
	})
	return id
}

func assertSourceActivityPublishConfirmationStored(t *testing.T, pool *pgxpool.Pool, segmentID, userID string) {
	t.Helper()
	var confirmedAt *time.Time
	var confirmedBy *string
	if err := pool.QueryRow(context.Background(), `
		SELECT source_activity_publish_confirmed_at, source_activity_publish_confirmed_by::text
		FROM segments
		WHERE id = $1
	`, segmentID).Scan(&confirmedAt, &confirmedBy); err != nil {
		t.Fatalf("load source activity publish confirmation: %v", err)
	}
	if confirmedAt == nil {
		t.Fatalf("source_activity_publish_confirmed_at is nil for segment %s", segmentID)
	}
	if confirmedBy == nil || *confirmedBy != userID {
		t.Fatalf("source_activity_publish_confirmed_by = %v, want %s", confirmedBy, userID)
	}
}

func TestSubTypeResponsesUseTaxonomyOrder(t *testing.T) {
	cycling, ok := sport.GetSportDef("cycling")
	if !ok {
		t.Fatal("missing cycling sport definition")
	}
	got := subTypeResponses(cycling)
	want := []string{"road", "mountain", "downhill", "recumbent", "cyclocross", "hand_cycling", "track_cycling", "indoor_cycling", "spin", "gravel_cycling", "unicycling", "mixed_surface", "bmx"}
	if len(got) != len(want) {
		t.Fatalf("len = %d, want %d", len(got), len(want))
	}
	for i, wantSubType := range want {
		if got[i].SubType != wantSubType {
			t.Fatalf("subtype[%d] = %q, want taxonomy order %q", i, got[i].SubType, wantSubType)
		}
	}
}

// TestGetSegment_IncludeGeoJSON_FlagControlsResponseField verifies that the
// include_geojson query parameter controls whether TrackGeoJSON is populated
// in the GetSegment response.
//
//   - Without the flag → TrackGeoJSON is nil (field omitted from JSON).
//   - With include_geojson=true → TrackGeoJSON contains a GeoJSON LineString.
//
// Requires DB. The test inserts a segment with a known track geometry so that
// the GeoJSON content can be asserted on.
func TestGetSegment_IncludeGeoJSON_FlagControlsResponseField(t *testing.T) {
	pool := fusionTestPool(t)
	h := newTestHandler(t, pool)

	r := chi.NewRouter()
	r.Get("/api/v1/segments/{id}", h.GetSegment)

	userID := insertTestUser(t, pool)
	segID := insertTestSegment(t, pool, userID, TestSegmentOpts{
		Track: "LINESTRING(10.0 47.0, 10.1 47.1)",
	})

	t.Run("without_flag_track_geojson_is_nil", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/v1/segments/"+segID, nil)
		req = req.WithContext(withUserAndAdmin(req.Context(), userID, false))
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body.String())
		}
		var seg model.Segment
		if err := json.Unmarshal(rec.Body.Bytes(), &seg); err != nil {
			t.Fatalf("unmarshal response: %v", err)
		}
		if seg.TrackGeoJSON != nil {
			t.Errorf("TrackGeoJSON = %q, want nil when include_geojson not set", *seg.TrackGeoJSON)
		}
	})

	t.Run("with_flag_track_geojson_is_linestring", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/v1/segments/"+segID+"?include_geojson=true", nil)
		req = req.WithContext(withUserAndAdmin(req.Context(), userID, false))
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body.String())
		}
		var seg model.Segment
		if err := json.Unmarshal(rec.Body.Bytes(), &seg); err != nil {
			t.Fatalf("unmarshal response: %v", err)
		}
		if seg.TrackGeoJSON == nil {
			t.Fatal("TrackGeoJSON is nil, want GeoJSON LineString string")
		}
		if !strings.Contains(*seg.TrackGeoJSON, `"LineString"`) {
			t.Errorf("TrackGeoJSON = %q, want string containing \"LineString\"", *seg.TrackGeoJSON)
		}
	})
}

func TestNormalizeSurfaceLabel(t *testing.T) {
	cases := []struct {
		input, want string
	}{
		{"", ""},
		{"Asphalt", "Asphalt"},
		{"Pflaster", "Pflaster"},
		{"Schotter", "Schotter (grob)"}, // historic: promoted to grob
		{"Schotter (fein)", "Schotter (fein)"},
		{"Schotter (grob)", "Schotter (grob)"},
		{"Erde", "Naturweg"}, // historic
		{"Naturweg", "Naturweg"},
		{"Naturweg (fest)", "Naturweg (fest)"}, // user-only sub-type: pass through
		{"Holz", "Holz"},
		{"Sportbelag", "Sportbelag"},
		{"Schnee", "Schnee"},
		{"Wechselnd", "Wechselnd"},  // editor-only: pass through
		{"   Asphalt  ", "Asphalt"}, // trim whitespace
		{"Unbekannt", "Unbekannt"},
		{"NonsenseValue", "NonsenseValue"}, // unknown stays raw (UI falls back)
	}
	for _, c := range cases {
		got := normalizeSurfaceLabel(c.input)
		if got != c.want {
			t.Errorf("normalizeSurfaceLabel(%q) = %q, want %q", c.input, got, c.want)
		}
	}
}
