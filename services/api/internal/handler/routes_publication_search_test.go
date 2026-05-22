package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/skadia-app/skadia/services/api/internal/model"
)

func TestPublishedCourseProfileFiltersHandler(t *testing.T) {
	pool := fusionTestPool(t)
	h := newTestHandler(t, pool)
	r := routePublicationRouter(h)
	r.Get("/api/v1/route-publications", h.ListRoutePublications)

	userID := insertTestUser(t, pool)
	routeID := insertTestRoute(t, h, userID)
	publicationID := publishPrivateRouteCourseViaHandler(t, r, routeID, userID, "search-handler-course")
	if _, err := pool.Exec(t.Context(), `
		UPDATE route_course_profile_projection
		SET sac_max_rank = 2, distance_m = 300
		WHERE publication_id = $1
	`, publicationID); err != nil {
		t.Fatalf("seed projection: %v", err)
	}

	rec := performRoutePublicationRequest(t, r, http.MethodGet, "/api/v1/route-publications?visibility_mode=owner&sac_max_rank=3&max_distance_m=400", "", userID)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body.String())
	}
	var response struct {
		Courses []struct {
			PublicationID string `json:"publication_id"`
		} `json:"courses"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if len(response.Courses) != 1 || response.Courses[0].PublicationID != publicationID {
		t.Fatalf("courses = %#v, want publication %s", response.Courses, publicationID)
	}

	rec = performRoutePublicationRequest(t, r, http.MethodGet, "/api/v1/route-publications?sac_max_rank=nope", "", userID)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("invalid rank status = %d, want 400; body: %s", rec.Code, rec.Body.String())
	}
}

func TestPublishedCourseHandlerAllowsUnauthenticatedLinkTokenOnly(t *testing.T) {
	pool := fusionTestPool(t)
	h := newTestHandler(t, pool)
	r := routePublicationRouter(h)
	r.Get("/api/v1/route-publications", h.ListRoutePublications)

	userID := insertTestUser(t, pool)
	linkToken := "route-publication-link-token"
	linkHash := hashObjectAccessLinkToken(linkToken)
	scope, err := h.DB.CreateObjectAccessScope(context.Background(), model.ObjectAccessScopeInput{
		OwnerUserID: userID,
		ScopeKind:   "manual",
		CreatedBy:   userID,
		Grants: []model.ObjectAccessGrantInput{{
			PrincipalKind: "link_token",
			LinkTokenHash: &linkHash,
			Permission:    "view",
		}},
	})
	if err != nil {
		t.Fatalf("CreateObjectAccessScope: %v", err)
	}

	routeID := insertTestRoute(t, h, userID)
	snapshotID := createRouteSnapshotViaHandler(t, r, routeID, userID, "search-handler-link-snapshot")
	allowRoutePublicationPrivacyReview(t, pool, snapshotID, "shared")
	body := `{
		"route_snapshot_id":"` + snapshotID + `",
		"source_route_version":1,
		"publish_policy":"new_course",
		"equivalence_decision":"version_specific",
		"visibility":"shared",
		"access_scope_id":"` + scope.ID + `",
		"compatibility_preset":"standard",
		"operation_id":"search-handler-link-publication"
	}`
	rec := performRoutePublicationRequest(t, r, http.MethodPost, "/api/v1/routes/"+routeID+"/publish-course", body, userID)
	if rec.Code != http.StatusCreated {
		t.Fatalf("publish status = %d, want 201; body: %s", rec.Code, rec.Body.String())
	}
	var published PublishRouteCourseResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &published); err != nil {
		t.Fatalf("unmarshal publish response: %v", err)
	}

	rec = performUnauthenticatedRoutePublicationRequest(r, "/api/v1/route-publications")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("no-token status = %d, want 401; body: %s", rec.Code, rec.Body.String())
	}

	rec = performUnauthenticatedRoutePublicationRequest(r, "/api/v1/route-publications?link_token=wrong")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("invalid-token status = %d, want 401; body: %s", rec.Code, rec.Body.String())
	}

	rec = performUnauthenticatedRoutePublicationRequest(r, "/api/v1/route-publications?visibility_mode=shared_visible_to_me&link_token="+linkToken)
	if rec.Code != http.StatusOK {
		t.Fatalf("link-token status = %d, want 200; body: %s", rec.Code, rec.Body.String())
	}
	var response struct {
		Courses []struct {
			PublicationID string `json:"publication_id"`
		} `json:"courses"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	found := false
	for _, course := range response.Courses {
		if course.PublicationID == published.PublicationID {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("courses = %#v, want shared publication %s", response.Courses, published.PublicationID)
	}
}

func performUnauthenticatedRoutePublicationRequest(r http.Handler, target string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, target, nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}
