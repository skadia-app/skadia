package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/skadia-app/skadia/services/api/internal/model"
	"github.com/go-chi/chi/v5"
)

// ----------------------------------------------------------------------------
// Unit-level coverage of applyRoutePatch (no DB, no HTTP).
// ----------------------------------------------------------------------------

func TestApplyRoutePatch_PreservesAbsentFields(t *testing.T) {
	desc := "original description"
	diff := "T3"
	dist := 12345.6
	est := 90
	r := &model.PlannedRoute{
		Name:             "Original Name",
		Description:      &desc,
		Difficulty:       &diff,
		TotalDistanceM:   &dist,
		EstimatedTimeMin: &est,
		TimeOverride:     true,
		AutoTag:          true,
	}
	// Client PATCH payload mirrors the silent-save path: only waypoints/segments/geometry.
	body := []byte(`{"waypoints": [], "segments": []}`)
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(body, &fields); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if err := applyRoutePatch(r, fields); err != nil {
		t.Fatalf("applyRoutePatch: %v", err)
	}
	if r.Name != "Original Name" {
		t.Errorf("Name wiped: got %q", r.Name)
	}
	if r.Description == nil || *r.Description != "original description" {
		t.Errorf("Description wiped: got %v", r.Description)
	}
	if r.Difficulty == nil || *r.Difficulty != "T3" {
		t.Errorf("Difficulty wiped: got %v", r.Difficulty)
	}
	if r.TotalDistanceM == nil || *r.TotalDistanceM != 12345.6 {
		t.Errorf("TotalDistanceM wiped: got %v", r.TotalDistanceM)
	}
	if r.EstimatedTimeMin == nil || *r.EstimatedTimeMin != 90 {
		t.Errorf("EstimatedTimeMin wiped: got %v", r.EstimatedTimeMin)
	}
	if !r.TimeOverride {
		t.Errorf("TimeOverride wiped to false")
	}
	if !r.AutoTag {
		t.Errorf("AutoTag wiped to false")
	}
}

func TestApplyRoutePatch_PresentEmptyStringBeatsPreservation(t *testing.T) {
	r := &model.PlannedRoute{Name: "Original"}
	body := []byte(`{"name": ""}`)
	var fields map[string]json.RawMessage
	_ = json.Unmarshal(body, &fields)
	if err := applyRoutePatch(r, fields); err != nil {
		t.Fatalf("applyRoutePatch: %v", err)
	}
	if r.Name != "" {
		t.Errorf("Name: expected empty-string override, got %q", r.Name)
	}
}

func TestApplyRoutePatch_ExplicitNullClearsNullable(t *testing.T) {
	desc := "original"
	r := &model.PlannedRoute{Description: &desc}
	body := []byte(`{"description": null}`)
	var fields map[string]json.RawMessage
	_ = json.Unmarshal(body, &fields)
	if err := applyRoutePatch(r, fields); err != nil {
		t.Fatalf("applyRoutePatch: %v", err)
	}
	if r.Description != nil {
		t.Errorf("Description: expected nil after explicit null, got %v", *r.Description)
	}
}

func TestApplyRoutePatch_IgnoresImmutableAndUnknownKeys(t *testing.T) {
	r := &model.PlannedRoute{ID: "real-id", UserID: "real-user"}
	body := []byte(`{"id":"evil","user_id":"other","created_at":"2000-01-01T00:00:00Z","totally_unknown":42}`)
	var fields map[string]json.RawMessage
	_ = json.Unmarshal(body, &fields)
	if err := applyRoutePatch(r, fields); err != nil {
		t.Fatalf("applyRoutePatch: %v", err)
	}
	if r.ID != "real-id" {
		t.Errorf("ID mutated: got %q", r.ID)
	}
	if r.UserID != "real-user" {
		t.Errorf("UserID mutated: got %q", r.UserID)
	}
}

func TestApplyRoutePatch_ElevationProfileExplicitNullClears(t *testing.T) {
	r := &model.PlannedRoute{ElevationProfile: json.RawMessage(`{"old":true}`)}
	body := []byte(`{"elevation_profile": null}`)
	var fields map[string]json.RawMessage
	_ = json.Unmarshal(body, &fields)
	if err := applyRoutePatch(r, fields); err != nil {
		t.Fatalf("applyRoutePatch: %v", err)
	}
	// explicit null → RawMessage "null"; DB column receives JSON null, which pgx
	// stores as SQL NULL for nullable JSONB columns.
	if string(r.ElevationProfile) != "null" {
		t.Errorf("ElevationProfile: expected \"null\" RawMessage, got %q", string(r.ElevationProfile))
	}
}

// ----------------------------------------------------------------------------
// Integration-level coverage against a real DB. Skipped unless TEST_DATABASE_URL is set.
// ----------------------------------------------------------------------------

func insertTestRoute(t *testing.T, h *Handler, userID string) string {
	t.Helper()
	desc := "orig-desc"
	diff := "T3"
	dist := 1234.5
	est := 60
	r := &model.PlannedRoute{
		UserID:           userID,
		Name:             "orig-name",
		Description:      &desc,
		SportType:        "hiking",
		Difficulty:       &diff,
		Profile:          "hiking-sac2",
		GeometryGeoJSON:  `{"type":"LineString","coordinates":[[11.0,47.0],[11.1,47.1]]}`,
		Waypoints:        []model.RouteWaypoint{},
		Segments:         []model.RouteSegment{},
		TotalDistanceM:   &dist,
		EstimatedTimeMin: &est,
		TimeOverride:     true,
		AutoTag:          true,
	}
	id, err := h.DB.CreateRoute(context.Background(), r)
	if err != nil {
		t.Fatalf("CreateRoute: %v", err)
	}
	t.Cleanup(func() {
		_, _ = h.DB.Pool.Exec(context.Background(), "DELETE FROM planned_routes WHERE id = $1", id)
	})
	return id
}

func patchRouteRequest(t *testing.T, h *Handler, userID, routeID string, body []byte) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/routes/"+routeID, bytes.NewReader(body))
	req = req.WithContext(withUserAndAdmin(req.Context(), userID, false))

	// Wire chi URL param extraction.
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", routeID)
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	w := httptest.NewRecorder()
	h.UpdateRoute(w, req)
	return w
}

func TestUpdateRoute_PreservesAbsentFields_Integration(t *testing.T) {
	if os.Getenv("TEST_DATABASE_URL") == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	pool := fusionTestPool(t)
	h := newTestHandler(t, pool)
	userID := insertTestUser(t, pool)
	routeID := insertTestRoute(t, h, userID)

	body := []byte(`{"waypoints":[],"segments":[],"total_elevation_gain":99}`)
	w := patchRouteRequest(t, h, userID, routeID, body)
	if w.Code != http.StatusOK {
		t.Fatalf("PATCH status = %d, body = %s", w.Code, w.Body.String())
	}

	got, err := h.DB.GetRoute(context.Background(), routeID, userID)
	if err != nil {
		t.Fatalf("GetRoute: %v", err)
	}
	if got.Name != "orig-name" {
		t.Errorf("Name wiped: %q", got.Name)
	}
	if got.Description == nil || *got.Description != "orig-desc" {
		t.Errorf("Description wiped: %v", got.Description)
	}
	if got.Difficulty == nil || *got.Difficulty != "T3" {
		t.Errorf("Difficulty wiped: %v", got.Difficulty)
	}
	if !got.TimeOverride {
		t.Errorf("TimeOverride wiped")
	}
	if !got.AutoTag {
		t.Errorf("AutoTag wiped")
	}
	if got.TotalElevationGain == nil || *got.TotalElevationGain != 99 {
		t.Errorf("TotalElevationGain not applied: %v", got.TotalElevationGain)
	}
}

func TestUpdateRoute_ExplicitNullClearsDescription_Integration(t *testing.T) {
	if os.Getenv("TEST_DATABASE_URL") == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	pool := fusionTestPool(t)
	h := newTestHandler(t, pool)
	userID := insertTestUser(t, pool)
	routeID := insertTestRoute(t, h, userID)

	body := []byte(`{"description": null}`)
	w := patchRouteRequest(t, h, userID, routeID, body)
	if w.Code != http.StatusOK {
		t.Fatalf("PATCH status = %d, body = %s", w.Code, w.Body.String())
	}
	got, err := h.DB.GetRoute(context.Background(), routeID, userID)
	if err != nil {
		t.Fatalf("GetRoute: %v", err)
	}
	if got.Description != nil {
		t.Errorf("Description should be NULL after explicit null PATCH; got %q", *got.Description)
	}
}

func TestUpdateRoute_EmptyStringName_Integration(t *testing.T) {
	if os.Getenv("TEST_DATABASE_URL") == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	pool := fusionTestPool(t)
	h := newTestHandler(t, pool)
	userID := insertTestUser(t, pool)
	routeID := insertTestRoute(t, h, userID)

	body := []byte(`{"name": ""}`)
	w := patchRouteRequest(t, h, userID, routeID, body)
	if w.Code != http.StatusOK {
		t.Fatalf("PATCH status = %d, body = %s", w.Code, w.Body.String())
	}
	got, err := h.DB.GetRoute(context.Background(), routeID, userID)
	if err != nil {
		t.Fatalf("GetRoute: %v", err)
	}
	if got.Name != "" {
		t.Errorf("Name should be \"\" after explicit empty-string PATCH; got %q", got.Name)
	}
}

func TestUpdateRoute_OtherUser404_Integration(t *testing.T) {
	if os.Getenv("TEST_DATABASE_URL") == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	pool := fusionTestPool(t)
	h := newTestHandler(t, pool)
	ownerID := insertTestUser(t, pool)
	attackerID := insertTestUser(t, pool)
	routeID := insertTestRoute(t, h, ownerID)

	body := []byte(`{"name":"pwned"}`)
	w := patchRouteRequest(t, h, attackerID, routeID, body)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for cross-user PATCH; got %d body=%s", w.Code, w.Body.String())
	}

	got, err := h.DB.GetRoute(context.Background(), routeID, ownerID)
	if err != nil {
		t.Fatalf("GetRoute: %v", err)
	}
	if got.Name != "orig-name" {
		t.Errorf("Name mutated by non-owner: %q", got.Name)
	}
}

func TestUpdateRoute_NonExistentID404_Integration(t *testing.T) {
	if os.Getenv("TEST_DATABASE_URL") == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	pool := fusionTestPool(t)
	h := newTestHandler(t, pool)
	userID := insertTestUser(t, pool)

	body := []byte(`{"name":"new"}`)
	// Valid UUID that does not exist.
	w := patchRouteRequest(t, h, userID, "00000000-0000-0000-0000-000000000000", body)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for missing id; got %d body=%s", w.Code, w.Body.String())
	}
}
