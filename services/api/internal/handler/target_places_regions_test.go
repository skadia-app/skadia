package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/skadia-app/skadia/services/api/internal/middleware"
	"github.com/skadia-app/skadia/services/api/internal/model"
)

func TestTargetPlaceHandlersEnforceVisibility(t *testing.T) {
	pool := fusionTestPool(t)
	h := newTestHandler(t, pool)
	ownerID := insertTestUser(t, pool)
	viewerID := insertTestUser(t, pool)
	otherID := insertTestUser(t, pool)
	scope := createHandlerAccessScope(t, h, ownerID, viewerID)

	privatePlace := createHandlerPlace(t, h, ownerID, "private", nil)
	publicPlace := createHandlerPlace(t, h, ownerID, "public", nil)
	sharedPlace := createHandlerPlace(t, h, ownerID, "shared", &scope.ID)

	rr := httptest.NewRecorder()
	h.ListTargetPlaces(rr, targetRequest(http.MethodGet, "/target-places", nil, viewerID))
	if rr.Code != http.StatusOK {
		t.Fatalf("viewer list status = %d body=%s", rr.Code, rr.Body.String())
	}
	var viewerResp struct {
		Places []model.TargetPlace `json:"places"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &viewerResp); err != nil {
		t.Fatalf("decode viewer response: %v", err)
	}
	assertHandlerPlaceIDs(t, viewerResp.Places, publicPlace.ID, sharedPlace.ID)
	assertHandlerPlaceIDsAbsent(t, viewerResp.Places, privatePlace.ID)

	rr = httptest.NewRecorder()
	h.ListTargetPlaces(rr, targetRequest(http.MethodGet, "/target-places", nil, otherID))
	if rr.Code != http.StatusOK {
		t.Fatalf("other list status = %d body=%s", rr.Code, rr.Body.String())
	}
	var otherResp struct {
		Places []model.TargetPlace `json:"places"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &otherResp); err != nil {
		t.Fatalf("decode other response: %v", err)
	}
	assertHandlerPlaceIDs(t, otherResp.Places, publicPlace.ID)
	assertHandlerPlaceIDsAbsent(t, otherResp.Places, privatePlace.ID, sharedPlace.ID)
}

func TestTargetRegionHandlersValidateGeometry(t *testing.T) {
	pool := fusionTestPool(t)
	h := newTestHandler(t, pool)
	userID := insertTestUser(t, pool)

	body := []byte(`{"name":"bad region","region_kind":"custom","visibility":"public","origin_key":"manual"}`)
	rr := httptest.NewRecorder()
	h.CreateTargetRegion(rr, targetRequest(http.MethodPost, "/target-regions", body, userID))
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d body=%s, want 400", rr.Code, rr.Body.String())
	}
}

func createHandlerPlace(t *testing.T, h *Handler, userID, visibility string, scopeID *string) *model.TargetPlace {
	t.Helper()
	place, err := h.DB.CreateTargetPlace(context.Background(), userID, model.TargetPlaceCreate{
		Name:            visibility + " place",
		PlaceKind:       "custom",
		GeometryGeoJSON: `{"type":"Point","coordinates":[8,49]}`,
		Visibility:      visibility,
		AccessScopeID:   scopeID,
		OriginKey:       "manual",
	})
	if err != nil {
		t.Fatalf("CreateTargetPlace: %v", err)
	}
	t.Cleanup(func() {
		_, _ = h.DB.Pool.Exec(context.Background(), `DELETE FROM target_places WHERE id = $1`, place.ID)
	})
	return place
}

func createHandlerAccessScope(t *testing.T, h *Handler, ownerID, viewerID string) *model.ObjectAccessScope {
	t.Helper()
	scope, err := h.DB.CreateObjectAccessScope(context.Background(), model.ObjectAccessScopeInput{
		OwnerUserID: ownerID,
		ScopeKind:   "manual",
		CreatedBy:   ownerID,
		Grants: []model.ObjectAccessGrantInput{
			{PrincipalKind: "user", PrincipalUserID: &viewerID},
		},
	})
	if err != nil {
		t.Fatalf("CreateObjectAccessScope: %v", err)
	}
	t.Cleanup(func() {
		_, _ = h.DB.Pool.Exec(context.Background(), `DELETE FROM object_access_scopes WHERE id = $1`, scope.ID)
	})
	return scope
}

func targetRequest(method, path string, body []byte, userID string) *http.Request {
	req := httptest.NewRequest(method, path, bytes.NewReader(body))
	if userID != "" {
		req = req.WithContext(context.WithValue(req.Context(), middleware.UserIDKey, userID))
	}
	return req
}

func assertHandlerPlaceIDs(t *testing.T, places []model.TargetPlace, ids ...string) {
	t.Helper()
	for _, id := range ids {
		found := false
		for _, place := range places {
			if place.ID == id {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("places missing %s in %#v", id, places)
		}
	}
}

func assertHandlerPlaceIDsAbsent(t *testing.T, places []model.TargetPlace, ids ...string) {
	t.Helper()
	for _, id := range ids {
		for _, place := range places {
			if place.ID == id {
				t.Fatalf("places unexpectedly contain %s in %#v", id, places)
			}
		}
	}
}
