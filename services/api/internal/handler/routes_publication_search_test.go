package handler

import (
	"encoding/json"
	"net/http"
	"testing"
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
