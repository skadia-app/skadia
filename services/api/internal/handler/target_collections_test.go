package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/skadia-app/skadia/services/api/internal/model"
)

func TestTargetCollectionHandlersHidePrivateMembers(t *testing.T) {
	pool := fusionTestPool(t)
	h := newTestHandler(t, pool)
	ownerID := insertTestUser(t, pool)
	collection, err := h.DB.CreateTargetCollection(context.Background(), ownerID, model.TargetCollectionCreate{
		Name:           "public collection",
		CollectionKind: "private_map",
		Visibility:     "public",
		OriginKey:      "manual",
	})
	if err != nil {
		t.Fatalf("CreateTargetCollection: %v", err)
	}
	t.Cleanup(func() {
		_, _ = h.DB.Pool.Exec(context.Background(), `DELETE FROM target_collections WHERE id = $1`, collection.ID)
	})
	privatePlace := createHandlerPlace(t, h, ownerID, "private", nil)
	publicPlace := createHandlerPlace(t, h, ownerID, "public", nil)
	addHandlerCollectionMember(t, h, ownerID, collection.ID, privatePlace.ID)
	addHandlerCollectionMember(t, h, ownerID, collection.ID, publicPlace.ID)

	rr := httptest.NewRecorder()
	h.GetTargetCollection(rr, targetRequestWithParam(http.MethodGet, "/target-collections/"+collection.ID+"?include_members=true", nil, "", "id", collection.ID))
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rr.Code, rr.Body.String())
	}
	var resp struct {
		Collection model.TargetCollectionDetail `json:"collection"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode collection: %v", err)
	}
	if len(resp.Collection.Members) != 1 || resp.Collection.Members[0].MemberID == nil || *resp.Collection.Members[0].MemberID != publicPlace.ID {
		t.Fatalf("public collection members = %#v, want only public member", resp.Collection.Members)
	}
}

func TestTargetCollectionHandlersRejectDeepNesting(t *testing.T) {
	pool := fusionTestPool(t)
	h := newTestHandler(t, pool)
	ownerID := insertTestUser(t, pool)
	parent := createHandlerCollection(t, h, ownerID, "parent")
	child := createHandlerCollection(t, h, ownerID, "child")
	if _, err := h.DB.AddTargetCollectionMember(context.Background(), ownerID, parent.ID, model.TargetCollectionMemberCreate{
		MemberKind: "collection",
		MemberID:   &child.ID,
		OriginKey:  "manual",
	}); err != nil {
		t.Fatalf("AddTargetCollectionMember: %v", err)
	}

	body := []byte(`{"member_kind":"collection","member_id":"` + parent.ID + `","origin_key":"manual"}`)
	rr := httptest.NewRecorder()
	h.AddTargetCollectionMember(rr, targetRequestWithParam(http.MethodPost, "/target-collections/"+child.ID+"/members", body, ownerID, "id", child.ID))
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d body=%s, want 400", rr.Code, rr.Body.String())
	}
}

func createHandlerCollection(t *testing.T, h *Handler, userID, name string) *model.TargetCollection {
	t.Helper()
	collection, err := h.DB.CreateTargetCollection(context.Background(), userID, model.TargetCollectionCreate{
		Name:           name,
		CollectionKind: "private_map",
		Visibility:     "private",
		OriginKey:      "manual",
	})
	if err != nil {
		t.Fatalf("CreateTargetCollection: %v", err)
	}
	t.Cleanup(func() {
		_, _ = h.DB.Pool.Exec(context.Background(), `DELETE FROM target_collections WHERE id = $1`, collection.ID)
	})
	return collection
}

func addHandlerCollectionMember(t *testing.T, h *Handler, userID, collectionID, placeID string) {
	t.Helper()
	if _, err := h.DB.AddTargetCollectionMember(context.Background(), userID, collectionID, model.TargetCollectionMemberCreate{
		MemberKind: "place",
		MemberID:   &placeID,
		OriginKey:  "manual",
	}); err != nil {
		t.Fatalf("AddTargetCollectionMember: %v", err)
	}
}

func targetRequestWithParam(method, path string, body []byte, userID, key, value string) *http.Request {
	req := targetRequest(method, path, body, userID)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add(key, value)
	return req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
}
