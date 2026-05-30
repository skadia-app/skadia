package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/skadia-app/skadia/services/api/internal/model"
)

func TestRouteSnapshotHandlerCreatesAndRejectsStaleVersion(t *testing.T) {
	pool := fusionTestPool(t)
	h := newTestHandler(t, pool)
	r := chi.NewRouter()
	r.Post("/api/v1/routes/{id}/snapshots", h.CreateRouteSnapshot)

	userID := insertTestUser(t, pool)
	routeID := insertTestRoute(t, h, userID)

	rec := performRoutePublicationRequest(t, r, http.MethodPost, "/api/v1/routes/"+routeID+"/snapshots", `{"source_route_version":1,"operation_id":"snapshot-handler-create"}`, userID)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create status = %d, want 201; body: %s", rec.Code, rec.Body.String())
	}
	var created CreateRouteSnapshotResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatalf("unmarshal create response: %v", err)
	}
	if created.SnapshotID == "" || created.SnapshotHash == "" || created.GeometryHash == "" {
		t.Fatalf("snapshot response missing ids/hashes: %#v", created)
	}

	rec = performRoutePublicationRequest(t, r, http.MethodPost, "/api/v1/routes/"+routeID+"/snapshots", `{"source_route_version":999,"operation_id":"snapshot-handler-stale"}`, userID)
	if rec.Code != http.StatusPreconditionFailed {
		t.Fatalf("stale status = %d, want 412; body: %s", rec.Code, rec.Body.String())
	}

	otherUserID := insertTestUser(t, pool)
	rec = performRoutePublicationRequest(t, r, http.MethodPost, "/api/v1/routes/"+routeID+"/snapshots", `{"source_route_version":1,"operation_id":"snapshot-handler-owner"}`, otherUserID)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("cross-user status = %d, want 404; body: %s", rec.Code, rec.Body.String())
	}
}

func TestRouteSnapshotHandlerRejectsMismatchedIfMatch(t *testing.T) {
	pool := fusionTestPool(t)
	h := newTestHandler(t, pool)
	r := chi.NewRouter()
	r.Post("/api/v1/routes/{id}/snapshots", h.CreateRouteSnapshot)

	userID := insertTestUser(t, pool)
	routeID := insertTestRoute(t, h, userID)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/routes/"+routeID+"/snapshots", bytes.NewReader([]byte(`{"source_route_version":1,"operation_id":"snapshot-if-match"}`)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("If-Match", `"2"`)
	req = req.WithContext(withUserAndAdmin(req.Context(), userID, false))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusPreconditionFailed {
		t.Fatalf("status = %d, want 412; body: %s", rec.Code, rec.Body.String())
	}
}

func TestRoutePublishCourseHandlerCreatesPrivateCourse(t *testing.T) {
	pool := fusionTestPool(t)
	h := newTestHandler(t, pool)
	r := routePublicationRouter(h)

	userID := insertTestUser(t, pool)
	routeID := insertTestRoute(t, h, userID)
	snapshotID := createRouteSnapshotViaHandler(t, r, routeID, userID, "publish-private-snapshot")

	body := `{
		"route_snapshot_id":"` + snapshotID + `",
		"source_route_version":1,
		"publish_policy":"new_course",
		"equivalence_decision":"version_specific",
		"visibility":"private",
		"compatibility_preset":"standard",
		"operation_id":"publish-private-course"
	}`
	rec := performRoutePublicationRequest(t, r, http.MethodPost, "/api/v1/routes/"+routeID+"/publish-course", body, userID)
	if rec.Code != http.StatusCreated {
		t.Fatalf("publish status = %d, want 201; body: %s", rec.Code, rec.Body.String())
	}
	var created PublishRouteCourseResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatalf("unmarshal publish response: %v", err)
	}
	if created.PublicationID == "" || created.CourseSegmentID == "" || created.CourseSegmentVersionID == "" {
		t.Fatalf("publication response missing ids: %#v", created)
	}
}

func TestRoutePublishCourseHandlerRejectsMismatchedIfMatch(t *testing.T) {
	pool := fusionTestPool(t)
	h := newTestHandler(t, pool)
	r := routePublicationRouter(h)

	userID := insertTestUser(t, pool)
	routeID := insertTestRoute(t, h, userID)
	snapshotID := createRouteSnapshotViaHandler(t, r, routeID, userID, "publish-if-match-snapshot")

	body := `{
		"route_snapshot_id":"` + snapshotID + `",
		"source_route_version":1,
		"publish_policy":"new_course",
		"equivalence_decision":"version_specific",
		"visibility":"private",
		"compatibility_preset":"standard",
		"operation_id":"publish-if-match-mismatch"
	}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/routes/"+routeID+"/publish-course", bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("If-Match", `"2"`)
	req = req.WithContext(withUserAndAdmin(req.Context(), userID, false))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusPreconditionFailed {
		t.Fatalf("status = %d, want 412; body: %s", rec.Code, rec.Body.String())
	}
}

func TestRoutePublishCourseHandlerRejectsMalformedIfMatch(t *testing.T) {
	pool := fusionTestPool(t)
	h := newTestHandler(t, pool)
	r := routePublicationRouter(h)

	userID := insertTestUser(t, pool)
	routeID := insertTestRoute(t, h, userID)
	snapshotID := createRouteSnapshotViaHandler(t, r, routeID, userID, "publish-if-match-malformed-snapshot")

	body := `{
		"route_snapshot_id":"` + snapshotID + `",
		"source_route_version":1,
		"publish_policy":"new_course",
		"equivalence_decision":"version_specific",
		"visibility":"private",
		"compatibility_preset":"standard",
		"operation_id":"publish-if-match-malformed"
	}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/routes/"+routeID+"/publish-course", bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("If-Match", `not-an-etag`)
	req = req.WithContext(withUserAndAdmin(req.Context(), userID, false))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body: %s", rec.Code, rec.Body.String())
	}
}

func TestRoutePublicationHandlerDoesNotCreateGuideContent(t *testing.T) {
	pool := fusionTestPool(t)
	h := newTestHandler(t, pool)
	r := routePublicationRouter(h)

	userID := insertTestUser(t, pool)
	routeID := insertTestRoute(t, h, userID)
	snapshotID := createRouteSnapshotViaHandler(t, r, routeID, userID, "handler-no-guide-snapshot")

	var routeDescription *string
	if err := pool.QueryRow(context.Background(), `
		SELECT route_description
		FROM planned_route_snapshots
		WHERE id = $1
	`, snapshotID).Scan(&routeDescription); err != nil {
		t.Fatalf("load route_description: %v", err)
	}
	if routeDescription == nil || *routeDescription != "orig-desc" {
		t.Fatalf("route_description = %#v, want route metadata", routeDescription)
	}

	body := `{
		"route_snapshot_id":"` + snapshotID + `",
		"source_route_version":1,
		"publish_policy":"new_course",
		"equivalence_decision":"version_specific",
		"visibility":"private",
		"compatibility_preset":"standard",
		"operation_id":"handler-no-guide-publication"
	}`
	rec := performRoutePublicationRequest(t, r, http.MethodPost, "/api/v1/routes/"+routeID+"/publish-course", body, userID)
	if rec.Code != http.StatusCreated {
		t.Fatalf("publish status = %d, want 201; body: %s", rec.Code, rec.Body.String())
	}
	var created PublishRouteCourseResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatalf("unmarshal publish response: %v", err)
	}

	var attachmentCount int
	if err := pool.QueryRow(context.Background(), `
		SELECT COUNT(*)
		FROM file_attachments
		WHERE entity_id IN ($1, $2)
		   OR entity_type IN ('route_guide', 'route_publication_guide', 'tour_report')
	`, snapshotID, created.PublicationID).Scan(&attachmentCount); err != nil {
		t.Fatalf("count file_attachments: %v", err)
	}
	if attachmentCount != 0 {
		t.Fatalf("guide/report attachment count = %d, want 0", attachmentCount)
	}
}

func TestRoutePublishCourseHandlerRejectsForeignSnapshotAndPublicWithoutReview(t *testing.T) {
	pool := fusionTestPool(t)
	h := newTestHandler(t, pool)
	r := routePublicationRouter(h)

	userID := insertTestUser(t, pool)
	routeID := insertTestRoute(t, h, userID)
	otherRouteID := insertTestRoute(t, h, userID)
	otherSnapshotID := createRouteSnapshotViaHandler(t, r, otherRouteID, userID, "publish-foreign-snapshot")

	body := `{
		"route_snapshot_id":"` + otherSnapshotID + `",
		"source_route_version":1,
		"publish_policy":"new_course",
		"equivalence_decision":"version_specific",
		"visibility":"private",
		"compatibility_preset":"standard",
		"operation_id":"publish-foreign"
	}`
	rec := performRoutePublicationRequest(t, r, http.MethodPost, "/api/v1/routes/"+routeID+"/publish-course", body, userID)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("foreign snapshot status = %d, want 400; body: %s", rec.Code, rec.Body.String())
	}

	snapshotID := createRouteSnapshotViaHandler(t, r, routeID, userID, "publish-public-snapshot")
	body = `{
		"route_snapshot_id":"` + snapshotID + `",
		"source_route_version":1,
		"publish_policy":"new_course",
		"equivalence_decision":"version_specific",
		"visibility":"public",
		"compatibility_preset":"standard",
		"operation_id":"publish-public-without-review"
	}`
	rec = performRoutePublicationRequest(t, r, http.MethodPost, "/api/v1/routes/"+routeID+"/publish-course", body, userID)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("public without review status = %d, want 400; body: %s", rec.Code, rec.Body.String())
	}
}

func TestRoutePublicationPrivacyReviewHandlerPersistsReviewForPublicPublish(t *testing.T) {
	pool := fusionTestPool(t)
	h := newTestHandler(t, pool)
	r := routePublicationRouter(h)

	userID := insertTestUser(t, pool)
	routeID := insertRoutePublicationPrivacyTestRoute(t, h, userID)
	snapshotID := createRouteSnapshotViaHandler(t, r, routeID, userID, "publish-public-review-snapshot")

	reviewBody := `{
		"route_snapshot_id":"` + snapshotID + `",
		"source_route_version":1,
		"audience_class":"public",
		"owner_confirmed":true
	}`
	rec := performRoutePublicationRequest(t, r, http.MethodPost, "/api/v1/routes/"+routeID+"/publication-privacy-review", reviewBody, userID)
	if rec.Code != http.StatusCreated {
		t.Fatalf("review status = %d, want 201; body: %s", rec.Code, rec.Body.String())
	}

	var ownerApproved bool
	if err := pool.QueryRow(context.Background(), `
		SELECT owner_approved_at IS NOT NULL
		FROM route_publication_privacy_reviews
		WHERE route_snapshot_id = $1
		  AND audience_class = 'public'
	`, snapshotID).Scan(&ownerApproved); err != nil {
		t.Fatalf("load persisted privacy review: %v", err)
	}
	if !ownerApproved {
		t.Fatal("privacy review owner_approved_at is NULL")
	}

	publishBody := `{
		"route_snapshot_id":"` + snapshotID + `",
		"source_route_version":1,
		"publish_policy":"new_course",
		"equivalence_decision":"version_specific",
		"visibility":"public",
		"compatibility_preset":"standard",
		"operation_id":"publish-public-after-review"
	}`
	rec = performRoutePublicationRequest(t, r, http.MethodPost, "/api/v1/routes/"+routeID+"/publish-course", publishBody, userID)
	if rec.Code != http.StatusCreated {
		t.Fatalf("publish status = %d, want 201; body: %s", rec.Code, rec.Body.String())
	}
}

func TestRoutePublishCourseHandlerValidatesSharedAccessScope(t *testing.T) {
	pool := fusionTestPool(t)
	h := newTestHandler(t, pool)
	r := routePublicationRouter(h)

	userID := insertTestUser(t, pool)
	viewerID := insertTestUser(t, pool)
	routeID := insertTestRoute(t, h, userID)
	snapshotID := createRouteSnapshotViaHandler(t, r, routeID, userID, "publish-shared-snapshot")
	scope, err := h.DB.CreateObjectAccessScope(context.Background(), routeObjectAccessScopeInput(userID, viewerID))
	if err != nil {
		t.Fatalf("CreateObjectAccessScope: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM segments WHERE access_scope_id = $1`, scope.ID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM object_access_scopes WHERE id = $1`, scope.ID)
	})
	allowRoutePublicationPrivacyReview(t, pool, snapshotID, "shared")

	body := `{
		"route_snapshot_id":"` + snapshotID + `",
		"source_route_version":1,
		"publish_policy":"new_course",
		"equivalence_decision":"version_specific",
		"visibility":"shared",
		"access_scope_id":"` + scope.ID + `",
		"compatibility_preset":"standard",
		"operation_id":"publish-shared-course"
	}`
	rec := performRoutePublicationRequest(t, r, http.MethodPost, "/api/v1/routes/"+routeID+"/publish-course", body, userID)
	if rec.Code != http.StatusCreated {
		t.Fatalf("shared status = %d, want 201; body: %s", rec.Code, rec.Body.String())
	}

	routeID = insertTestRoute(t, h, userID)
	snapshotID = createRouteSnapshotViaHandler(t, r, routeID, userID, "publish-private-scope-snapshot")
	body = `{
		"route_snapshot_id":"` + snapshotID + `",
		"source_route_version":1,
		"publish_policy":"new_course",
		"equivalence_decision":"version_specific",
		"visibility":"private",
		"access_scope_id":"` + scope.ID + `",
		"compatibility_preset":"standard",
		"operation_id":"publish-private-with-scope"
	}`
	rec = performRoutePublicationRequest(t, r, http.MethodPost, "/api/v1/routes/"+routeID+"/publish-course", body, userID)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("private with scope status = %d, want 400; body: %s", rec.Code, rec.Body.String())
	}
}

func TestRouteForkHandlerCreatesAndReplaysFork(t *testing.T) {
	pool := fusionTestPool(t)
	h := newTestHandler(t, pool)
	r := routePublicationRouter(h)

	userID := insertTestUser(t, pool)
	routeID := insertTestRoute(t, h, userID)
	body := `{"name":"forked route","source_route_version":1,"operation_id":"fork-handler"}`

	rec := performRoutePublicationRequest(t, r, http.MethodPost, "/api/v1/routes/"+routeID+"/fork", body, userID)
	if rec.Code != http.StatusCreated {
		t.Fatalf("fork status = %d, want 201; body: %s", rec.Code, rec.Body.String())
	}
	var first ForkRouteResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &first); err != nil {
		t.Fatalf("unmarshal first fork: %v", err)
	}
	if first.RouteID == "" || first.RelationshipID == "" {
		t.Fatalf("fork response missing ids: %#v", first)
	}

	rec = performRoutePublicationRequest(t, r, http.MethodPost, "/api/v1/routes/"+routeID+"/fork", body, userID)
	if rec.Code != http.StatusCreated {
		t.Fatalf("fork replay status = %d, want 201; body: %s", rec.Code, rec.Body.String())
	}
	var second ForkRouteResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &second); err != nil {
		t.Fatalf("unmarshal second fork: %v", err)
	}
	if second.RouteID != first.RouteID || second.RelationshipID != first.RelationshipID {
		t.Fatalf("fork replay = %#v, want %#v", second, first)
	}
}

func TestRouteRelationshipHandlerCreatesRouteAndCourseVariants(t *testing.T) {
	pool := fusionTestPool(t)
	h := newTestHandler(t, pool)
	r := routePublicationRouter(h)

	userID := insertTestUser(t, pool)
	sourceRouteID := insertTestRoute(t, h, userID)
	targetRouteID := insertTestRoute(t, h, userID)
	body := `{"target_route_id":"` + targetRouteID + `","relation_type":"variant_of","operation_id":"route-variant"}`
	rec := performRoutePublicationRequest(t, r, http.MethodPost, "/api/v1/routes/"+sourceRouteID+"/relationships", body, userID)
	if rec.Code != http.StatusCreated {
		t.Fatalf("route relationship status = %d, want 201; body: %s", rec.Code, rec.Body.String())
	}

	sourcePublicationID := publishPrivateRouteCourseViaHandler(t, r, sourceRouteID, userID, "source-course")
	targetPublicationID := publishPrivateRouteCourseViaHandler(t, r, targetRouteID, userID, "target-course")
	body = `{"target_publication_id":"` + targetPublicationID + `","relation_type":"variant_of","operation_id":"course-variant"}`
	rec = performRoutePublicationRequest(t, r, http.MethodPost, "/api/v1/route-publications/"+sourcePublicationID+"/relationships", body, userID)
	if rec.Code != http.StatusCreated {
		t.Fatalf("course relationship status = %d, want 201; body: %s", rec.Code, rec.Body.String())
	}
}

func routePublicationRouter(h *Handler) *chi.Mux {
	r := chi.NewRouter()
	r.Post("/api/v1/routes/{id}/snapshots", h.CreateRouteSnapshot)
	r.Post("/api/v1/routes/{id}/publication-privacy-review", h.CreateRoutePublicationPrivacyReview)
	r.Post("/api/v1/routes/{id}/publish-course", h.PublishRouteCourse)
	r.Post("/api/v1/routes/{id}/fork", h.ForkRoute)
	r.Post("/api/v1/routes/{id}/relationships", h.CreateRouteRelationship)
	r.Post("/api/v1/route-publications/{id}/relationships", h.CreateRouteCourseRelationship)
	return r
}

func insertRoutePublicationPrivacyTestRoute(t *testing.T, h *Handler, userID string) string {
	t.Helper()
	desc := "orig-desc"
	route := &model.PlannedRoute{
		UserID:          userID,
		Name:            "orig-name",
		Description:     &desc,
		SportType:       "hiking",
		Profile:         "hiking-mountain",
		GeometryGeoJSON: `{"type":"LineString","coordinates":[[11.0,47.0],[11.1,47.1]]}`,
		Waypoints: []model.RouteWaypoint{
			{ID: "a", Type: "start", LatLng: [2]float64{47.0, 11.0}},
			{ID: "b", Type: "end", LatLng: [2]float64{47.1, 11.1}},
		},
		Segments: []model.RouteSegment{{
			FromWaypoint: "a",
			ToWaypoint:   "b",
			Mode:         "routed",
			DistanceM:    1000,
			Geometry:     &model.GeoJSONLineString{Type: "LineString", Coordinates: [][]float64{{11.0, 47.0}, {11.1, 47.1}}},
		}},
	}
	id, err := h.DB.CreateRoute(context.Background(), route)
	if err != nil {
		t.Fatalf("CreateRoute: %v", err)
	}
	t.Cleanup(func() {
		_, _ = h.DB.Pool.Exec(context.Background(), "DELETE FROM planned_routes WHERE id = $1", id)
	})
	return id
}

func createRouteSnapshotViaHandler(t *testing.T, r http.Handler, routeID, userID, operationID string) string {
	t.Helper()
	rec := performRoutePublicationRequest(t, r, http.MethodPost, "/api/v1/routes/"+routeID+"/snapshots", `{"source_route_version":1,"operation_id":"`+operationID+`"}`, userID)
	if rec.Code != http.StatusCreated {
		t.Fatalf("snapshot status = %d, want 201; body: %s", rec.Code, rec.Body.String())
	}
	var response CreateRouteSnapshotResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("unmarshal snapshot response: %v", err)
	}
	return response.SnapshotID
}

func allowRoutePublicationPrivacyReview(t *testing.T, pool *pgxpool.Pool, snapshotID, audienceClass string) {
	t.Helper()
	var snapshotHash string
	if err := pool.QueryRow(context.Background(), `
		SELECT snapshot_hash
		FROM planned_route_snapshots
		WHERE id = $1
	`, snapshotID).Scan(&snapshotHash); err != nil {
		t.Fatalf("load snapshot hash: %v", err)
	}
	if _, err := pool.Exec(context.Background(), `
		INSERT INTO route_publication_privacy_reviews (
			route_snapshot_id, snapshot_hash, audience_class,
			route_name_state, description_state, waypoints_state,
			source_metadata_state, geometry_state, enrichment_state, media_pointer_state,
			overall_state, review_hash
		)
		VALUES (
			$1, $2, $3,
			'safe', 'safe', 'safe',
			'safe', 'safe', 'safe', 'safe',
			'safe', $4
		)
		ON CONFLICT (route_snapshot_id, snapshot_hash, audience_class) DO UPDATE
		SET overall_state = EXCLUDED.overall_state,
		    review_hash = EXCLUDED.review_hash,
		    updated_at = NOW()
	`, snapshotID, snapshotHash, audienceClass, "test-"+snapshotID+"-"+audienceClass); err != nil {
		t.Fatalf("insert privacy review: %v", err)
	}
}

func publishPrivateRouteCourseViaHandler(t *testing.T, r http.Handler, routeID, userID, operationPrefix string) string {
	t.Helper()
	snapshotID := createRouteSnapshotViaHandler(t, r, routeID, userID, operationPrefix+"-snapshot")
	body := `{
		"route_snapshot_id":"` + snapshotID + `",
		"source_route_version":1,
		"publish_policy":"new_course",
		"equivalence_decision":"version_specific",
		"visibility":"private",
		"compatibility_preset":"standard",
		"operation_id":"` + operationPrefix + `-publication"
	}`
	rec := performRoutePublicationRequest(t, r, http.MethodPost, "/api/v1/routes/"+routeID+"/publish-course", body, userID)
	if rec.Code != http.StatusCreated {
		t.Fatalf("publish status = %d, want 201; body: %s", rec.Code, rec.Body.String())
	}
	var response PublishRouteCourseResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("unmarshal publication response: %v", err)
	}
	return response.PublicationID
}

func performRoutePublicationRequest(t *testing.T, r http.Handler, method, target, body, userID string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, target, bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(withUserAndAdmin(req.Context(), userID, false))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

func routeObjectAccessScopeInput(ownerID, viewerID string) model.ObjectAccessScopeInput {
	return model.ObjectAccessScopeInput{
		OwnerUserID: ownerID,
		ScopeKind:   "manual",
		CreatedBy:   ownerID,
		Grants: []model.ObjectAccessGrantInput{{
			PrincipalKind:   "user",
			PrincipalUserID: &viewerID,
			Permission:      "view",
		}},
	}
}
