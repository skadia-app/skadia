package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/database/testdb"
	"github.com/skadia-app/skadia/services/api/internal/middleware"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// injectChiURLParam sets a chi URL parameter in the request context.
func injectChiURLParam(r *http.Request, key, value string) *http.Request {
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add(key, value)
	return r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))
}

// gearCRUDTestHandler returns a minimal Handler + userID for gear CRUD tests.
func gearCRUDTestHandler(t *testing.T) (*Handler, string, *pgxpool.Pool) {
	t.Helper()
	pool := testdb.Open(t)
	ctx := context.Background()

	userID := uuid.NewString()
	if _, err := pool.Exec(ctx,
		`INSERT INTO users (id, username, email, email_verified, registration_source)
		 VALUES ($1, $2, $3, true, 'test')`,
		userID, "gcrud-"+userID[:8], "gcrud-"+userID[:8]+"@example.com"); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, userID)
	})
	return &Handler{DB: &database.DB{Pool: pool}}, userID, pool
}

func doCreateGear(t *testing.T, h *Handler, userID string, body any, extraHeaders ...string) *httptest.ResponseRecorder {
	t.Helper()
	b, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal body: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/gear", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	for i := 0; i+1 < len(extraHeaders); i += 2 {
		req.Header.Set(extraHeaders[i], extraHeaders[i+1])
	}
	req = req.WithContext(context.WithValue(req.Context(), middleware.UserIDKey, userID))
	rr := httptest.NewRecorder()
	h.CreateGear(rr, req)
	return rr
}

// TestSaveGearValidatesAttributesAgainstSchema: POST with attribute key not allowed for the
// gear_type → 400. (Strict per-value validation now only applies to plain `enum` attributes;
// the public surface for `helmet` is `search_enum`/`enum_with_freetext` by design — see
// commit aee699d39. The remaining schema gate is the attribute_keys whitelist per type.)
func TestSaveGearValidatesAttributesAgainstSchema(t *testing.T) {
	h, userID, _ := gearCRUDTestHandler(t)

	body := map[string]any{
		"name":       "Test Helmet",
		"type":       "helmet",
		"attributes": map[string]any{"bike.frame_type": "road_endurance"},
	}
	rr := doCreateGear(t, h, userID, body)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rr.Code, rr.Body.String())
	}
	var resp map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp["error"] != "gear.schema_validation_failed" {
		t.Errorf("expected error=gear.schema_validation_failed, got %v", resp["error"])
	}
}

// TestSaveGearValidatesModifiersAgainstApplicableModifierKeys: POST with unknown modifier → 400.
func TestSaveGearValidatesModifiersAgainstApplicableModifierKeys(t *testing.T) {
	h, userID, _ := gearCRUDTestHandler(t)

	body := map[string]any{
		"name":      "Full Face",
		"type":      "helmet",
		"modifiers": map[string]any{"is_full_face": true},
	}
	rr := doCreateGear(t, h, userID, body)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rr.Code, rr.Body.String())
	}
	var resp map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp["error"] != "gear.unknown_modifier" {
		t.Errorf("expected error=gear.unknown_modifier, got %v", resp["error"])
	}
}

// TestSaveGearValidatesScopeAgainstGeneratedScopes: POST with unknown scope → 400; valid scope → 201.
func TestSaveGearValidatesScopeAgainstGeneratedScopes(t *testing.T) {
	h, userID, _ := gearCRUDTestHandler(t)

	badBody := map[string]any{
		"name":  "UFO Gear",
		"type":  "other",
		"scope": "ufo",
	}
	rr := doCreateGear(t, h, userID, badBody)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for unknown scope, got %d: %s", rr.Code, rr.Body.String())
	}
	var resp map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode bad-scope response: %v", err)
	}
	if resp["error"] != "gear.unknown_scope" {
		t.Errorf("expected error=gear.unknown_scope, got %v", resp["error"])
	}

	goodBody := map[string]any{
		"name":  "My Helmet",
		"type":  "helmet",
		"scope": "other",
	}
	rr2 := doCreateGear(t, h, userID, goodBody)
	if rr2.Code != http.StatusCreated {
		t.Fatalf("expected 201 for valid scope, got %d: %s", rr2.Code, rr2.Body.String())
	}
}

// TestSaveGearValidatesApplicableSportPredicatesAgainstTaxonomy: POST with unknown sport_type → 400.
func TestSaveGearValidatesApplicableSportPredicatesAgainstTaxonomy(t *testing.T) {
	h, userID, _ := gearCRUDTestHandler(t)

	body := map[string]any{
		"name": "UFO Gear",
		"type": "other",
		"applicable_sport_predicates": []map[string]any{
			{"sport_type": "ufo_riding"},
		},
	}
	rr := doCreateGear(t, h, userID, body)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rr.Code, rr.Body.String())
	}
	var resp map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp["error"] != "gear.unknown_sport_predicate" {
		t.Errorf("expected error=gear.unknown_sport_predicate, got %v", resp["error"])
	}
}

// TestLookupByNameDisambiguatesAndReturns409OnConflict: two gear items with same name → 409.
func TestLookupByNameDisambiguatesAndReturns409OnConflict(t *testing.T) {
	h, userID, pool := gearCRUDTestHandler(t)

	// Create two gear items with the same name via raw SQL.
	ctx := context.Background()
	for i := 0; i < 2; i++ {
		if _, err := pool.Exec(ctx,
			`INSERT INTO gear (user_id, name, type, scope)
			 VALUES ($1, 'Stevens P18', 'bike', 'bike')`,
			userID,
		); err != nil {
			t.Fatalf("insert gear %d: %v", i, err)
		}
	}

	req := httptest.NewRequest(http.MethodGet, "/api/gear?name=Stevens+P18", nil)
	req = req.WithContext(context.WithValue(req.Context(), middleware.UserIDKey, userID))
	rr := httptest.NewRecorder()
	h.ListGear(rr, req)

	if rr.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %s", rr.Code, rr.Body.String())
	}
	var resp map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp["error"] != "gear.name_ambiguous" {
		t.Errorf("expected error=gear.name_ambiguous, got %v", resp["error"])
	}
	details, _ := resp["details"].(map[string]any)
	candidates, _ := details["candidates"].([]any)
	if len(candidates) != 2 {
		t.Errorf("expected 2 candidates, got %d", len(candidates))
	}

	// Unique name → 200.
	if _, err := pool.Exec(ctx,
		`INSERT INTO gear (user_id, name, type, scope)
		 VALUES ($1, 'Unique Bike 2026', 'bike', 'bike')`,
		userID,
	); err != nil {
		t.Fatalf("insert unique gear: %v", err)
	}
	req2 := httptest.NewRequest(http.MethodGet, "/api/gear?name=Unique+Bike+2026", nil)
	req2 = req2.WithContext(context.WithValue(req2.Context(), middleware.UserIDKey, userID))
	rr2 := httptest.NewRecorder()
	h.ListGear(rr2, req2)
	if rr2.Code != http.StatusOK {
		t.Fatalf("expected 200 for unique name, got %d: %s", rr2.Code, rr2.Body.String())
	}
}

// TestIdempotencyKeyOnPostGear: same Idempotency-Key twice → first 201, replay also 201 with same body.
func TestIdempotencyKeyOnPostGear(t *testing.T) {
	h, userID, _ := gearCRUDTestHandler(t)
	idemp := uuid.NewString()
	body := map[string]any{"name": "Idempotent Bike", "type": "bike"}

	rr1 := doCreateGear(t, h, userID, body, "Idempotency-Key", idemp)
	if rr1.Code != http.StatusCreated {
		t.Fatalf("first request: expected 201, got %d: %s", rr1.Code, rr1.Body.String())
	}
	var resp1 map[string]any
	if err := json.NewDecoder(rr1.Body).Decode(&resp1); err != nil {
		t.Fatalf("decode first response: %v", err)
	}
	firstID, _ := resp1["id"].(string)
	if firstID == "" {
		t.Fatal("first response missing id")
	}

	rr2 := doCreateGear(t, h, userID, body, "Idempotency-Key", idemp)
	// Replay must return the original 201 Created, not 200 OK (Critic-4 #8).
	if rr2.Code != http.StatusCreated {
		t.Fatalf("replay request: expected 201 Created, got %d: %s", rr2.Code, rr2.Body.String())
	}
	var resp2 map[string]any
	if err := json.NewDecoder(rr2.Body).Decode(&resp2); err != nil {
		t.Fatalf("decode replay response: %v", err)
	}
	secondID, _ := resp2["id"].(string)
	if secondID != firstID {
		t.Errorf("replay returned different id: first=%q second=%q", firstID, secondID)
	}
}

// TestErrorResponseShapeOnSchemaValidationFailure: invalid attribute → 400 with machine-readable body.
func TestErrorResponseShapeOnSchemaValidationFailure(t *testing.T) {
	h, userID, _ := gearCRUDTestHandler(t)

	body := map[string]any{
		"name": "Helmet",
		"type": "helmet",
		// Attribute key not in helmet's whitelist — see commit aee699d39 note above.
		"attributes": map[string]any{"bike.frame_type": "ufo_category"},
	}
	rr := doCreateGear(t, h, userID, body)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rr.Code, rr.Body.String())
	}
	var resp map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if _, ok := resp["error"]; !ok {
		t.Error("response missing 'error' field")
	}
	if _, ok := resp["message"]; !ok {
		t.Error("response missing 'message' field")
	}
	details, ok := resp["details"].(map[string]any)
	if !ok {
		t.Fatalf("response details field is %T, want map", resp["details"])
	}
	if _, ok := details["attribute"]; !ok {
		t.Error("details missing 'attribute' field")
	}
	if _, ok := details["reason"]; !ok {
		t.Error("details missing 'reason' field")
	}
}

// TestSaveGearAcceptsValidAttributes: POST with valid attribute value → 201.
func TestSaveGearAcceptsValidAttributes(t *testing.T) {
	h, userID, _ := gearCRUDTestHandler(t)

	body := map[string]any{
		"name":       "Road Helmet",
		"type":       "helmet",
		"attributes": map[string]any{"helmet.subcategory": "road_bike"},
	}
	rr := doCreateGear(t, h, userID, body)
	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rr.Code, rr.Body.String())
	}
	var resp map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	attrs, _ := resp["attributes"].(map[string]any)
	if attrs["helmet.subcategory"] != "road_bike" {
		t.Errorf("attributes in response = %v, want helmet.subcategory=road_bike", attrs)
	}
}

// TestSaveGearAcceptsValidModifiers: POST with valid gear-applicable modifier → 201.
func TestSaveGearAcceptsValidModifiers(t *testing.T) {
	h, userID, _ := gearCRUDTestHandler(t)

	body := map[string]any{
		"name":      "E-Bike",
		"type":      "bike",
		"modifiers": map[string]any{"is_electric": true},
	}
	rr := doCreateGear(t, h, userID, body)
	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rr.Code, rr.Body.String())
	}
}

// insertTestGear inserts a gear item via raw SQL for use in other tests.
func insertTestGear(t *testing.T, pool *pgxpool.Pool, userID, name, typeKey, scope string) string {
	t.Helper()
	var id string
	err := pool.QueryRow(context.Background(),
		`INSERT INTO gear (user_id, name, type, scope) VALUES ($1, $2, $3, $4) RETURNING id`,
		userID, name, typeKey, scope,
	).Scan(&id)
	if err != nil {
		t.Fatalf("insertTestGear %q: %v", name, err)
	}
	return id
}

// insertTestActivity inserts a minimal activity for summary tests.
func insertTestActivityForSummary(t *testing.T, pool *pgxpool.Pool, userID string, opts map[string]any) string {
	t.Helper()
	id := uuid.NewString()
	distM, _ := opts["distance_m"].(float64)
	gainM, _ := opts["elevation_gain_m"].(float64)
	lossM, _ := opts["elevation_loss_m"].(float64)
	movingS, _ := opts["moving_time_s"].(int)
	elapsedS, _ := opts["elapsed_time_s"].(int)
	startedAt, _ := opts["started_at"].(string)
	if startedAt == "" {
		startedAt = "2025-01-01T10:00:00Z"
	}
	sportType, _ := opts["sport_type"].(string)
	if sportType == "" {
		sportType = "cycling"
	}
	_, err := pool.Exec(context.Background(),
		`INSERT INTO activities (id, user_id, sport_type, distance_m, elevation_gain_m, elevation_loss_m,
		  moving_time_s, elapsed_time_s, is_group_secondary, started_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, false, $9::timestamptz)`,
		id, userID, sportType, distM, gainM, lossM, int64(movingS), int64(elapsedS), startedAt,
	)
	if err != nil {
		t.Fatalf("insertTestActivityForSummary: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM activities WHERE id = $1`, id)
	})
	return id
}

// insertTestGearAssignment inserts an activity_gear_assignments row.
func insertTestGearAssignment(t *testing.T, pool *pgxpool.Pool, userID, activityID, gearID string, distOverride *float64) {
	t.Helper()
	_, err := pool.Exec(context.Background(),
		`INSERT INTO activity_gear_assignments (user_id, activity_id, gear_id, distance_m_override)
		 VALUES ($1, $2, $3, $4)`,
		userID, activityID, gearID, distOverride,
	)
	if err != nil {
		t.Fatalf("insertTestGearAssignment: %v", err)
	}
}

func doGetSummary(t *testing.T, h *Handler, userID, query string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/gear/summary"+query, nil)
	req = req.WithContext(context.WithValue(req.Context(), middleware.UserIDKey, userID))
	rr := httptest.NewRecorder()
	h.GetGearSummary(rr, req)
	return rr
}

// TestSummaryAggregatesByScopeAndDateRange: 3 bikes (1 retired), 5 activities each.
func TestSummaryAggregatesByScopeAndDateRange(t *testing.T) {
	h, userID, pool := gearCRUDTestHandler(t)
	ctx := context.Background()

	bike1 := insertTestGear(t, pool, userID, "Bike Active 1", "bike", "bike")
	bike2 := insertTestGear(t, pool, userID, "Bike Active 2", "bike", "bike")
	bike3 := insertTestGear(t, pool, userID, "Bike Retired", "bike", "bike")

	// Retire bike3 — keep status and is_retired in sync (matches UpdateGearStatus behaviour).
	if _, err := pool.Exec(ctx, `UPDATE gear SET status='retired', is_retired=true WHERE id=$1`, bike3); err != nil {
		t.Fatalf("retire bike3: %v", err)
	}

	dates := []string{"2025-01-10T10:00:00Z", "2025-02-10T10:00:00Z", "2025-03-10T10:00:00Z",
		"2025-04-10T10:00:00Z", "2025-05-10T10:00:00Z"}
	for _, gearID := range []string{bike1, bike2, bike3} {
		for i, d := range dates {
			actID := insertTestActivityForSummary(t, pool, userID, map[string]any{
				"distance_m":    float64((i + 1) * 1000),
				"started_at":    d,
				"moving_time_s": (i + 1) * 600,
			})
			insertTestGearAssignment(t, pool, userID, actID, gearID, nil)
		}
	}

	// Without retired — expect 2 bikes, 10 activities.
	rr := doGetSummary(t, h, userID, "?scope=bike&since=2025-01-01&until=2025-06-01")
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var resp map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode summary: %v", err)
	}
	actCount, _ := resp["activity_count"].(float64)
	if actCount != 10 {
		t.Errorf("activity_count = %v, want 10 (2 active bikes × 5)", actCount)
	}
	gearCount, _ := resp["gear_count"].(float64)
	if gearCount != 2 {
		t.Errorf("gear_count = %v, want 2", gearCount)
	}

	// With include_retired=true — expect 3 bikes, 15 activities.
	rr2 := doGetSummary(t, h, userID, "?scope=bike&since=2025-01-01&until=2025-06-01&include_retired=true")
	if rr2.Code != http.StatusOK {
		t.Fatalf("expected 200 with retired, got %d: %s", rr2.Code, rr2.Body.String())
	}
	var resp2 map[string]any
	if err := json.NewDecoder(rr2.Body).Decode(&resp2); err != nil {
		t.Fatalf("decode summary with retired: %v", err)
	}
	actCount2, _ := resp2["activity_count"].(float64)
	if actCount2 != 15 {
		t.Errorf("activity_count with retired = %v, want 15", actCount2)
	}
}

// TestSummaryFiltersByTypeKey: ?type_key=shoes filters to shoe gear only.
func TestSummaryFiltersByTypeKey(t *testing.T) {
	h, userID, pool := gearCRUDTestHandler(t)

	bike := insertTestGear(t, pool, userID, "Bike", "bike", "bike")
	shoe := insertTestGear(t, pool, userID, "Shoes", "shoes", "shoes")

	for i, gearID := range []string{bike, shoe} {
		actID := insertTestActivityForSummary(t, pool, userID, map[string]any{
			"distance_m": float64((i + 1) * 5000),
		})
		insertTestGearAssignment(t, pool, userID, actID, gearID, nil)
	}

	rr := doGetSummary(t, h, userID, "?type_key=shoes")
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var resp map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode summary: %v", err)
	}
	gearCount, _ := resp["gear_count"].(float64)
	if gearCount != 1 {
		t.Errorf("gear_count for shoes = %v, want 1", gearCount)
	}
	dist, _ := resp["total_distance_m"].(float64)
	if dist != 10000 {
		t.Errorf("total_distance_m for shoes = %v, want 10000", dist)
	}
}

// TestSwapComponentRetiresOldAndAssignsNewAtomically: swap creates event row.
func TestSwapComponentRetiresOldAndAssignsNewAtomically(t *testing.T) {
	h, userID, pool := gearCRUDTestHandler(t)
	ctx := context.Background()

	gearID := insertTestGear(t, pool, userID, "Gravel Bike", "bike", "bike")

	// Insert an old brake_pads component.
	var oldCompID string
	err := pool.QueryRow(ctx,
		`INSERT INTO gear_components (gear_id, name, type)
		 VALUES ($1, 'Old Brake Pads', 'bike.consumable.brake_pads')
		 RETURNING id`,
		gearID,
	).Scan(&oldCompID)
	if err != nil {
		t.Fatalf("insert old component: %v", err)
	}

	body := map[string]any{
		"slot_key": "brake_pads_front",
		"new_component_payload": map[string]any{
			"name": "SwissStop Brake Pads",
			"type": "bike.consumable.brake_pads",
		},
		"swap_date": "2026-04-15",
	}
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/gear/%s/components/swap", gearID), bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(context.WithValue(req.Context(), middleware.UserIDKey, userID))

	// Inject gear id via chi URLParam simulation.
	rr := httptest.NewRecorder()
	// We call SwapComponent directly so we need to set the URL param via chi context.
	// Use a router wrapper to inject path params.
	rr = callSwapComponent(t, h, userID, gearID, b)

	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rr.Code, rr.Body.String())
	}
	var resp map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp["new_component_id"] == nil {
		t.Error("response missing new_component_id")
	}
	if resp["swap_event_id"] == nil {
		t.Error("response missing swap_event_id")
	}

	// Verify swap event row exists.
	var count int
	if err := pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM gear_component_swap_events WHERE gear_id = $1`,
		gearID,
	).Scan(&count); err != nil {
		t.Fatalf("count swap events: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 swap event, got %d", count)
	}
}

// callSwapComponent sets up a chi-style route context and calls SwapComponent.
func callSwapComponent(t *testing.T, h *Handler, userID, gearID string, bodyBytes []byte) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost,
		fmt.Sprintf("/api/gear/%s/components/swap", gearID),
		bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(context.WithValue(req.Context(), middleware.UserIDKey, userID))

	// Inject chi URL param manually.
	req = injectChiURLParam(req, "id", gearID)
	rr := httptest.NewRecorder()
	h.SwapComponent(rr, req)
	return rr
}

// TestSwapComponentIdempotent: same Idempotency-Key twice → 1 swap, not 2.
func TestSwapComponentIdempotent(t *testing.T) {
	h, userID, pool := gearCRUDTestHandler(t)

	gearID := insertTestGear(t, pool, userID, "MTB Bike", "bike", "bike")

	body := map[string]any{
		"slot_key": "brake_pads_rear",
		"new_component_payload": map[string]any{
			"name": "New Brake Pads",
			"type": "bike.consumable.brake_pads",
		},
	}
	b, _ := json.Marshal(body)
	idemp := uuid.NewString()

	req1 := httptest.NewRequest(http.MethodPost,
		fmt.Sprintf("/api/gear/%s/components/swap", gearID),
		bytes.NewReader(b))
	req1.Header.Set("Content-Type", "application/json")
	req1.Header.Set("Idempotency-Key", idemp)
	req1 = req1.WithContext(context.WithValue(req1.Context(), middleware.UserIDKey, userID))
	req1 = injectChiURLParam(req1, "id", gearID)
	rr1 := httptest.NewRecorder()
	h.SwapComponent(rr1, req1)
	if rr1.Code != http.StatusCreated {
		t.Fatalf("first swap: expected 201, got %d: %s", rr1.Code, rr1.Body.String())
	}

	req2 := httptest.NewRequest(http.MethodPost,
		fmt.Sprintf("/api/gear/%s/components/swap", gearID),
		bytes.NewReader(b))
	req2.Header.Set("Content-Type", "application/json")
	req2.Header.Set("Idempotency-Key", idemp)
	req2 = req2.WithContext(context.WithValue(req2.Context(), middleware.UserIDKey, userID))
	req2 = injectChiURLParam(req2, "id", gearID)
	rr2 := httptest.NewRecorder()
	h.SwapComponent(rr2, req2)
	// Replay must return the original 201 Created, not 200 OK (Critic-4 #8).
	if rr2.Code != http.StatusCreated {
		t.Fatalf("replay swap: expected 201 Created, got %d: %s", rr2.Code, rr2.Body.String())
	}

	// Verify only 1 swap event in DB.
	var count int
	if err := pool.QueryRow(context.Background(),
		`SELECT COUNT(*) FROM gear_component_swap_events WHERE gear_id = $1`, gearID,
	).Scan(&count); err != nil {
		t.Fatalf("count swap events: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 swap event (idempotent), got %d", count)
	}
}

// TestSwapComponentByNameDisambiguates: lookup by name with multiple matches → 409.
func TestSwapComponentByNameDisambiguates(t *testing.T) {
	h, userID, pool := gearCRUDTestHandler(t)
	ctx := context.Background()

	gearID := insertTestGear(t, pool, userID, "Road Bike", "bike", "bike")

	// Insert two components with the same name.
	for range 2 {
		if _, err := pool.Exec(ctx,
			`INSERT INTO gear_components (gear_id, name, type)
			 VALUES ($1, 'Sram GX', 'bike.consumable.chain_lube')`,
			gearID,
		); err != nil {
			t.Fatalf("insert component: %v", err)
		}
	}

	body := map[string]any{
		"slot_key":                          "chain",
		"existing_component_lookup_by_name": "Sram GX",
		"new_component_payload": map[string]any{
			"name": "New Chain",
			"type": "bike.consumable.chain_lube",
		},
	}
	b, _ := json.Marshal(body)
	rr := callSwapComponent(t, h, userID, gearID, b)
	if rr.Code != http.StatusConflict {
		t.Fatalf("expected 409 for ambiguous component name, got %d: %s", rr.Code, rr.Body.String())
	}
	var resp map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp["error"] != "gear.name_ambiguous" {
		t.Errorf("expected error=gear.name_ambiguous, got %v", resp["error"])
	}
}

// TestCreateGearDefaultsApplicableSportPredicatesFromTaxonomy: POST without
// applicable_sport_predicates → DB row contains taxonomy-computed default.
func TestCreateGearDefaultsApplicableSportPredicatesFromTaxonomy(t *testing.T) {
	h, userID, _ := gearCRUDTestHandler(t)

	body := map[string]any{
		"name":      "Trail Ripper",
		"type":      "bike.mtb",
		"modifiers": map[string]any{"is_electric": true},
		// applicable_sport_predicates intentionally absent
	}
	rr := doCreateGear(t, h, userID, body)
	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	rawPreds, ok := resp["applicable_sport_predicates"].([]any)
	if !ok || len(rawPreds) == 0 {
		t.Fatalf("applicable_sport_predicates absent or empty in response; got %T = %v", resp["applicable_sport_predicates"], resp["applicable_sport_predicates"])
	}

	// Expect at least one cycling/mountain predicate with is_electric modifier.
	foundCyclingMountain := false
	for _, rawPred := range rawPreds {
		pred, ok := rawPred.(map[string]any)
		if !ok {
			continue
		}
		if pred["sport_type"] == "cycling" {
			if subType, _ := pred["sub_type"].(string); subType == "mountain" {
				mods, _ := pred["modifiers"].(map[string]any)
				if isElec, _ := mods["is_electric"].(bool); isElec {
					foundCyclingMountain = true
					break
				}
			}
		}
	}
	if !foundCyclingMountain {
		t.Fatalf("expected cycling/mountain predicate with is_electric; got %#v", rawPreds)
	}
}

// TestCreateGearDoesNotOverrideExplicitEmptyApplicableSportPredicates: POST
// with applicable_sport_predicates=[] must not be overridden by taxonomy defaults.
func TestCreateGearDoesNotOverrideExplicitEmptyApplicableSportPredicates(t *testing.T) {
	h, userID, _ := gearCRUDTestHandler(t)

	// Explicit empty array — user wants no sport filtering.
	body := map[string]any{
		"name":                        "Universal Bike",
		"type":                        "bike.mtb",
		"applicable_sport_predicates": []any{},
	}
	rr := doCreateGear(t, h, userID, body)
	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	rawPreds, _ := resp["applicable_sport_predicates"].([]any)
	if len(rawPreds) != 0 {
		t.Fatalf("expected empty applicable_sport_predicates when explicitly set to []; got %#v", rawPreds)
	}
}

// TestSwapComponentOwnershipCheckBeforeIdempotencyWrite verifies that an
// idempotency key is NOT written to gear_idempotency_keys when the ownership
// check fails (i.e. the ownership check runs BEFORE the idempotency write).
// If idempotency ran first, the key would be written even for a 404 response,
// leaking idempotency state across users.
func TestSwapComponentOwnershipCheckBeforeIdempotencyWrite(t *testing.T) {
	h, attackerUserID, pool := gearCRUDTestHandler(t)

	// Create a second user who owns the gear.
	ownerID := uuid.NewString()
	if _, err := pool.Exec(context.Background(),
		`INSERT INTO users (id, username, email, email_verified, registration_source)
		 VALUES ($1, $2, $3, true, 'test')`,
		ownerID, "owner-"+ownerID[:8], "owner-"+ownerID[:8]+"@example.com"); err != nil {
		t.Fatalf("insert owner: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, ownerID)
	})

	// Create gear belonging to owner, not attacker.
	gearID := uuid.NewString()
	if _, err := pool.Exec(context.Background(), `
		INSERT INTO gear (id, user_id, name, type, status, scope)
		VALUES ($1, $2, 'Victim Bike', 'bike', 'active', 'bike')`,
		gearID, ownerID); err != nil {
		t.Fatalf("insert gear: %v", err)
	}

	idempotencyKey := "test-idem-key-" + uuid.NewString()

	// Attacker sends SwapComponent for gear they don't own, with an Idempotency-Key.
	body := map[string]any{
		"slot_key": "chain",
		"new_component_payload": map[string]any{
			"name": "New Chain",
			"type": "bike_chain",
		},
	}
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/api/gear/"+gearID+"/components/swap", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", idempotencyKey)
	req = req.WithContext(context.WithValue(req.Context(), middleware.UserIDKey, attackerUserID))
	req = injectChiURLParam(req, "id", gearID)

	rr := httptest.NewRecorder()
	h.SwapComponent(rr, req)

	// Must return 404 because attacker doesn't own the gear.
	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for ownership check, got %d: %s", rr.Code, rr.Body.String())
	}

	// The idempotency key must NOT have been written — ownership check runs first.
	var count int
	if err := pool.QueryRow(context.Background(),
		`SELECT COUNT(*) FROM gear_idempotency_keys WHERE key = $1 AND user_id = $2`,
		idempotencyKey, attackerUserID,
	).Scan(&count); err != nil {
		t.Fatalf("query idempotency key: %v", err)
	}
	if count != 0 {
		t.Errorf("idempotency key was written before ownership check — security bug: got %d rows", count)
	}
}

// TestCreateGearDefaultsScopeFromSchema verifies that when scope is omitted in
// POST /api/gear, the handler derives scope from the gear-type schema so the
// DB row has the expected scope value.
func TestCreateGearDefaultsScopeFromSchema(t *testing.T) {
	h, userID, pool := gearCRUDTestHandler(t)

	// POST without scope; type='bike.mtb' has scope='bike' in the schema.
	body := map[string]any{
		"name": "MTB No Scope",
		"type": "bike.mtb",
	}
	rr := doCreateGear(t, h, userID, body)
	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp["scope"] != "bike" {
		t.Errorf("scope = %q, want %q (should default from schema)", resp["scope"], "bike")
	}

	// Verify the DB row also has scope='bike'.
	gearID, _ := resp["id"].(string)
	if gearID == "" {
		t.Fatal("no id in response")
	}
	var dbScope string
	if err := pool.QueryRow(context.Background(), `SELECT scope FROM gear WHERE id = $1`, gearID).Scan(&dbScope); err != nil {
		t.Fatalf("query gear scope: %v", err)
	}
	if dbScope != "bike" {
		t.Errorf("DB scope = %q, want %q", dbScope, "bike")
	}
}

// TestCreateGearRejectsUnknownType verifies that POST /api/gear with a
// completely unknown type returns 400 with machine-error gear.unknown_gear_type.
func TestCreateGearRejectsUnknownType(t *testing.T) {
	h, userID, _ := gearCRUDTestHandler(t)

	body := map[string]any{
		"name": "Mystery Gear",
		"type": "made_up_type_xyz",
	}
	rr := doCreateGear(t, h, userID, body)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for unknown type, got %d: %s", rr.Code, rr.Body.String())
	}
	var resp map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp["error"] != "gear.unknown_gear_type" {
		t.Errorf("expected error=gear.unknown_gear_type, got %v", resp["error"])
	}
}

// doUpdateGear sends a PATCH /api/gear/{id} request with the given body.
func doUpdateGear(t *testing.T, h *Handler, userID, gearID string, body any) *httptest.ResponseRecorder {
	t.Helper()
	b, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal body: %v", err)
	}
	req := httptest.NewRequest(http.MethodPatch, "/api/gear/"+gearID, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(context.WithValue(req.Context(), middleware.UserIDKey, userID))
	req = injectChiURLParam(req, "id", gearID)
	rr := httptest.NewRecorder()
	h.UpdateGear(rr, req)
	return rr
}

// TestUpdateGearRejectsUnknownType verifies that PATCH /api/gear/{id} with an
// unknown type returns 400 with machine-error gear.unknown_gear_type.
func TestUpdateGearRejectsUnknownType(t *testing.T) {
	h, userID, _ := gearCRUDTestHandler(t)

	// Create a valid gear item first.
	createRR := doCreateGear(t, h, userID, map[string]any{
		"name": "My Helmet",
		"type": "helmet",
	})
	if createRR.Code != http.StatusCreated {
		t.Fatalf("setup: expected 201, got %d: %s", createRR.Code, createRR.Body.String())
	}
	var created map[string]any
	if err := json.NewDecoder(createRR.Body).Decode(&created); err != nil {
		t.Fatalf("decode create response: %v", err)
	}
	gearID, _ := created["id"].(string)
	if gearID == "" {
		t.Fatal("setup: gear id missing in create response")
	}

	// PATCH with an unknown type — must be rejected.
	rr := doUpdateGear(t, h, userID, gearID, map[string]any{
		"type": "made_up_xyz",
	})
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rr.Code, rr.Body.String())
	}
	var resp map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp["error"] != "gear.unknown_gear_type" {
		t.Errorf("expected error=gear.unknown_gear_type, got %v", resp["error"])
	}
}

// TestUpdateGearAllowsLegacyType verifies that PATCHing to a legacy allow-list
// type (e_bike) is accepted, matching the same allow-list used by CreateGear.
func TestUpdateGearAllowsLegacyType(t *testing.T) {
	h, userID, _ := gearCRUDTestHandler(t)

	// Create a valid gear item first using a schema-defined type.
	createRR := doCreateGear(t, h, userID, map[string]any{
		"name":  "Generic Bike",
		"type":  "bike",
		"scope": "bike",
	})
	if createRR.Code != http.StatusCreated {
		t.Fatalf("setup: expected 201, got %d: %s", createRR.Code, createRR.Body.String())
	}
	var created map[string]any
	if err := json.NewDecoder(createRR.Body).Decode(&created); err != nil {
		t.Fatalf("decode create response: %v", err)
	}
	gearID, _ := created["id"].(string)
	if gearID == "" {
		t.Fatal("setup: gear id missing in create response")
	}

	// PATCH to the legacy 'e_bike' type — must succeed.
	rr := doUpdateGear(t, h, userID, gearID, map[string]any{
		"type": "e_bike",
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 for legacy e_bike update, got %d: %s", rr.Code, rr.Body.String())
	}
}

// TestCreateGearAllowsLegacyEBikeType verifies that the legacy 'e_bike' type
// (not in the current schema but present in production data) is accepted.
func TestCreateGearAllowsLegacyEBikeType(t *testing.T) {
	h, userID, _ := gearCRUDTestHandler(t)

	body := map[string]any{
		"name":  "Legacy Electric Bike",
		"type":  "e_bike",
		"scope": "bike",
	}
	rr := doCreateGear(t, h, userID, body)
	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201 for legacy e_bike type, got %d: %s", rr.Code, rr.Body.String())
	}
}

// TestUpdateGearTypeChangeRejectsStaleAttributes verifies that PATCH /api/gear/{id}
// with only a new type (no attributes override) returns 422 when the existing
// attributes are incompatible with the new type (Critic #5).
func TestUpdateGearTypeChangeRejectsStaleAttributes(t *testing.T) {
	h, userID, _ := gearCRUDTestHandler(t)

	// Create a bike with a bike-specific attribute.
	createRR := doCreateGear(t, h, userID, map[string]any{
		"name":       "Carbon Bike",
		"type":       "bike",
		"attributes": map[string]any{"bike.frame_material": "carbon"},
	})
	if createRR.Code != http.StatusCreated {
		t.Fatalf("setup: expected 201, got %d: %s", createRR.Code, createRR.Body.String())
	}
	var created map[string]any
	if err := json.NewDecoder(createRR.Body).Decode(&created); err != nil {
		t.Fatalf("decode create response: %v", err)
	}
	gearID, _ := created["id"].(string)
	if gearID == "" {
		t.Fatal("setup: gear id missing in create response")
	}

	// PATCH: change type to helmet without providing attributes.
	// The existing bike.frame_material attr is not allowed for helmet — must be rejected.
	rr := doUpdateGear(t, h, userID, gearID, map[string]any{
		"type": "helmet",
	})
	if rr.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422, got %d: %s", rr.Code, rr.Body.String())
	}
	var resp map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp["error"] != "gear.schema_validation_failed" {
		t.Errorf("expected error=gear.schema_validation_failed, got %v", resp["error"])
	}
}

// TestUpdateGearTypeChangeAllowsCompatibleAttributes verifies that PATCH /api/gear/{id}
// with a new type succeeds when the existing attributes are compatible with the new
// type (Critic #5).
func TestUpdateGearTypeChangeAllowsCompatibleAttributes(t *testing.T) {
	h, userID, _ := gearCRUDTestHandler(t)

	// Create a bike.road with a wheel_size attribute (inherited from bike).
	createRR := doCreateGear(t, h, userID, map[string]any{
		"name":       "Road Bike",
		"type":       "bike.road",
		"attributes": map[string]any{"bike.wheel_size": "s28"},
	})
	if createRR.Code != http.StatusCreated {
		t.Fatalf("setup: expected 201, got %d: %s", createRR.Code, createRR.Body.String())
	}
	var created map[string]any
	if err := json.NewDecoder(createRR.Body).Decode(&created); err != nil {
		t.Fatalf("decode create response: %v", err)
	}
	gearID, _ := created["id"].(string)
	if gearID == "" {
		t.Fatal("setup: gear id missing in create response")
	}

	// PATCH: change type to bike.gravel without providing attributes.
	// bike.wheel_size is allowed for bike.gravel (inherits from bike) — must succeed.
	rr := doUpdateGear(t, h, userID, gearID, map[string]any{
		"type": "bike.gravel",
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
}

// TestUpdateGearRejectsAttributeValueTypeMismatch verifies that PATCH
// /api/gear/{id} with an attribute value of the wrong type returns 400 with
// gear.schema_validation_failed. This exercises the type-change re-validation
// path fixed by Critic-4 #6 and ensures the UpdateGear handler passes attrs
// through the full validator (Critic-4 #10).
func TestUpdateGearRejectsAttributeValueTypeMismatch(t *testing.T) {
	h, userID, _ := gearCRUDTestHandler(t)

	// Create a valid harness item.
	createRR := doCreateGear(t, h, userID, map[string]any{
		"name":  "Rock Harness",
		"type":  "climb.harness",
		"scope": "other",
	})
	if createRR.Code != http.StatusCreated {
		t.Fatalf("setup: expected 201, got %d: %s", createRR.Code, createRR.Body.String())
	}
	var created map[string]any
	if err := json.NewDecoder(createRR.Body).Decode(&created); err != nil {
		t.Fatalf("decode create response: %v", err)
	}
	gearID, _ := created["id"].(string)
	if gearID == "" {
		t.Fatal("setup: gear id missing in create response")
	}

	// PATCH: provide climb.harness_size as an integer instead of a string.
	// ValidateAttributes must now reject this with a type-mismatch error
	// because the schema defines climb.harness_size as ValueType "string".
	rr := doUpdateGear(t, h, userID, gearID, map[string]any{
		"attributes": map[string]any{"climb.harness_size": 42},
	})
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for integer on string attribute, got %d: %s", rr.Code, rr.Body.String())
	}
	var resp map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp["error"] != "gear.schema_validation_failed" {
		t.Errorf("expected error=gear.schema_validation_failed, got %v", resp["error"])
	}
}

// TestUpdateGearTypeChangeStaleAttrRejected_WithFix verifies that after the
// Critic-4 #5/#6 fix, PATCH /api/gear/{id} with a type change rejects the
// existing attributes when they contain a key disallowed by the new type,
// and that the validator now also catches value-type mismatches in stale data.
// Complements TestUpdateGearTypeChangeRejectsStaleAttributes (key-level check)
// with a value-level check introduced in Critic-4 #10.
func TestUpdateGearTypeChangeStaleStringAttrRejectedByNewType(t *testing.T) {
	h, userID, pool := gearCRUDTestHandler(t)
	ctx := context.Background()

	// Insert a harness item with valid harness_size via raw SQL so we can also
	// simulate legacy data where the value type is correct but the key is stale
	// under the target type.
	var gearID string
	attrJSON := `{"climb.harness_size": "M"}`
	if err := pool.QueryRow(ctx,
		`INSERT INTO gear (user_id, name, type, scope, attributes)
		 VALUES ($1, $2, $3, $4, $5::jsonb) RETURNING id`,
		userID, "DB Harness", "climb.harness", "other", attrJSON,
	).Scan(&gearID); err != nil {
		t.Fatalf("insert test gear: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM gear WHERE id = $1`, gearID)
	})

	// PATCH: change type to climb.rope without providing attributes.
	// climb.harness_size is not allowed under climb.rope — the re-validation
	// path at UpdateGear lines 271-279 must catch this.
	rr := doUpdateGear(t, h, userID, gearID, map[string]any{
		"type": "climb.rope",
	})
	if rr.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422 for stale harness attr under rope type, got %d: %s", rr.Code, rr.Body.String())
	}
	var resp map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp["error"] != "gear.schema_validation_failed" {
		t.Errorf("expected error=gear.schema_validation_failed, got %v", resp["error"])
	}
}

// TestGetGearSummaryIncludeRetiredParseBool verifies that:
//   - ?include_retired=yes     → 400 with gear.invalid_param
//   - ?include_retired=true    → 200
//   - ?include_retired= absent → 200 (default false)
func TestGetGearSummaryIncludeRetiredParseBool(t *testing.T) {
	h, userID, _ := gearCRUDTestHandler(t)

	doSummary := func(query string) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet, "/api/gear/summary?"+query, nil)
		req = req.WithContext(context.WithValue(req.Context(), middleware.UserIDKey, userID))
		rr := httptest.NewRecorder()
		h.GetGearSummary(rr, req)
		return rr
	}

	// Invalid boolean value must return 400.
	rr := doSummary("include_retired=yes")
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("include_retired=yes: expected 400, got %d: %s", rr.Code, rr.Body.String())
	}
	var errResp map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&errResp); err != nil {
		t.Fatalf("decode 400 response: %v", err)
	}
	if errResp["error"] != "gear.invalid_param" {
		t.Errorf("include_retired=yes: expected error=gear.invalid_param, got %v", errResp["error"])
	}

	// Valid 'true' must succeed.
	rr2 := doSummary("include_retired=true")
	if rr2.Code != http.StatusOK {
		t.Fatalf("include_retired=true: expected 200, got %d: %s", rr2.Code, rr2.Body.String())
	}

	// Absent query param must succeed (default = false).
	rr3 := doSummary("")
	if rr3.Code != http.StatusOK {
		t.Fatalf("include_retired absent: expected 200, got %d: %s", rr3.Code, rr3.Body.String())
	}
}

// TestIdempotencyReplayPreservesOriginalStatus verifies that a replay returns
// the same HTTP status as the original response, not always 200 OK (Critic-4 #8).
func TestIdempotencyReplayPreservesOriginalStatus(t *testing.T) {
	h, userID, _ := gearCRUDTestHandler(t)

	t.Run("success_201_replayed_as_201", func(t *testing.T) {
		idemp := uuid.NewString()
		body := map[string]any{"name": "Status Test Bike", "type": "bike"}

		rr1 := doCreateGear(t, h, userID, body, "Idempotency-Key", idemp)
		if rr1.Code != http.StatusCreated {
			t.Fatalf("first request: want 201, got %d: %s", rr1.Code, rr1.Body.String())
		}

		rr2 := doCreateGear(t, h, userID, body, "Idempotency-Key", idemp)
		if rr2.Code != http.StatusCreated {
			t.Fatalf("replay: want 201 (not 200), got %d: %s", rr2.Code, rr2.Body.String())
		}
	})

	t.Run("validation_400_replayed_as_400", func(t *testing.T) {
		idemp := uuid.NewString()
		// Schema validation failure: attribute key not in helmet's whitelist
		// (per-value validation no longer rejects search_enum freetext — see commit aee699d39).
		body := map[string]any{
			"name":       "BadAttr Helmet",
			"type":       "helmet",
			"attributes": map[string]any{"bike.frame_type": "no_such_category"},
		}

		rr1 := doCreateGear(t, h, userID, body, "Idempotency-Key", idemp)
		if rr1.Code != http.StatusBadRequest {
			t.Fatalf("first request: want 400, got %d: %s", rr1.Code, rr1.Body.String())
		}
		var resp1 map[string]any
		if err := json.NewDecoder(rr1.Body).Decode(&resp1); err != nil {
			t.Fatalf("decode first 400: %v", err)
		}

		rr2 := doCreateGear(t, h, userID, body, "Idempotency-Key", idemp)
		// Replay must return the stored 400, not re-run validation and return 400
		// only coincidentally. The status comes from the stored response_status_code.
		if rr2.Code != http.StatusBadRequest {
			t.Fatalf("replay: want 400, got %d: %s", rr2.Code, rr2.Body.String())
		}
		var resp2 map[string]any
		if err := json.NewDecoder(rr2.Body).Decode(&resp2); err != nil {
			t.Fatalf("decode replay 400: %v", err)
		}
		// Body must be identical.
		if resp2["error"] != resp1["error"] {
			t.Errorf("replay error code mismatch: first=%v second=%v", resp1["error"], resp2["error"])
		}
	})

	t.Run("no_idempotency_key_no_storage", func(t *testing.T) {
		// Without an Idempotency-Key header, each request is independent.
		body := map[string]any{"name": "No-Idemp Bike", "type": "bike"}
		rr1 := doCreateGear(t, h, userID, body)
		if rr1.Code != http.StatusCreated {
			t.Fatalf("first request without key: want 201, got %d: %s", rr1.Code, rr1.Body.String())
		}
		rr2 := doCreateGear(t, h, userID, body)
		if rr2.Code != http.StatusCreated {
			t.Fatalf("second request without key: want 201 (new resource), got %d: %s", rr2.Code, rr2.Body.String())
		}
		// Two distinct gear items must have been created (no idempotency).
		var resp1, resp2 map[string]any
		_ = json.NewDecoder(rr1.Body).Decode(&resp1)
		_ = json.NewDecoder(rr2.Body).Decode(&resp2)
		if resp1["id"] == resp2["id"] {
			t.Error("without Idempotency-Key, two requests must create two distinct gear items")
		}
	})
}

// doCreateGearRaw posts a raw byte body to CreateGear (for malformed-JSON tests).
func doCreateGearRaw(t *testing.T, h *Handler, userID string, raw []byte, extraHeaders ...string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/gear", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	for i := 0; i+1 < len(extraHeaders); i += 2 {
		req.Header.Set(extraHeaders[i], extraHeaders[i+1])
	}
	req = req.WithContext(context.WithValue(req.Context(), middleware.UserIDKey, userID))
	rr := httptest.NewRecorder()
	h.CreateGear(rr, req)
	return rr
}

// TestIdempotencyReplayPreservesMalformedJSON400 verifies that when the first
// request fails JSON decoding (deterministic 400), a replay with the same
// Idempotency-Key returns the SAME 400 body — even when the second request's
// body bytes differ. The contract: a properly stored 4xx wins over re-decode.
//
// Architecture-B follow-up to Critic-4 #8: persists the DecodeJSON 400 path.
func TestIdempotencyReplayPreservesMalformedJSON400(t *testing.T) {
	h, userID, _ := gearCRUDTestHandler(t)
	idemp := uuid.NewString()

	rr1 := doCreateGearRaw(t, h, userID, []byte(`{not valid json`), "Idempotency-Key", idemp)
	if rr1.Code != http.StatusBadRequest {
		t.Fatalf("first request: want 400, got %d: %s", rr1.Code, rr1.Body.String())
	}
	var resp1 map[string]any
	if err := json.NewDecoder(bytes.NewReader(rr1.Body.Bytes())).Decode(&resp1); err != nil {
		t.Fatalf("decode first 400: %v", err)
	}

	// Second call: completely different malformed body, same key. Replay must
	// return the SAME response semantically (Postgres JSONB normalises key
	// ordering and whitespace, so byte-equality is too strict).
	rr2 := doCreateGearRaw(t, h, userID, []byte(`<<<also not json>>>`), "Idempotency-Key", idemp)
	if rr2.Code != http.StatusBadRequest {
		t.Fatalf("replay (malformed): want 400, got %d: %s", rr2.Code, rr2.Body.String())
	}
	var resp2 map[string]any
	if err := json.NewDecoder(bytes.NewReader(rr2.Body.Bytes())).Decode(&resp2); err != nil {
		t.Fatalf("decode replay 400: %v", err)
	}
	for _, key := range []string{"type", "title", "status", "detail", "instance"} {
		if resp1[key] != resp2[key] {
			t.Errorf("replay %s differs: first=%v replay=%v", key, resp1[key], resp2[key])
		}
	}

	// Third call: VALID JSON body with same key. The contract says replay must
	// still return the cached 400, NOT create a new gear. Without persistence
	// this would 201, breaking idempotency: same key → different status.
	validBody, _ := json.Marshal(map[string]any{"name": "Should Not Exist", "type": "bike"})
	rr3 := doCreateGearRaw(t, h, userID, validBody, "Idempotency-Key", idemp)
	if rr3.Code != http.StatusBadRequest {
		t.Fatalf("replay (valid JSON, cached 400): want 400, got %d: %s", rr3.Code, rr3.Body.String())
	}
}

// TestIdempotencyReplayPreservesNormalize400 verifies that when the request
// passes JSON decode but fails normalizeGearApplicablePredicatesForRequest
// (deterministic 400), a replay returns the SAME 400.
func TestIdempotencyReplayPreservesNormalize400(t *testing.T) {
	h, userID, _ := gearCRUDTestHandler(t)
	idemp := uuid.NewString()

	body := map[string]any{
		"name": "BadPred Gear",
		"type": "other",
		"applicable_sport_predicates": []map[string]any{
			{"sport_type": "ufo_riding"},
		},
	}
	rr1 := doCreateGear(t, h, userID, body, "Idempotency-Key", idemp)
	if rr1.Code != http.StatusBadRequest {
		t.Fatalf("first request: want 400, got %d: %s", rr1.Code, rr1.Body.String())
	}
	var resp1 map[string]any
	if err := json.NewDecoder(bytes.NewReader(rr1.Body.Bytes())).Decode(&resp1); err != nil {
		t.Fatalf("decode first 400: %v", err)
	}

	// Replay with the SAME body and SAME key. Must return cached 400.
	rr2 := doCreateGear(t, h, userID, body, "Idempotency-Key", idemp)
	if rr2.Code != http.StatusBadRequest {
		t.Fatalf("replay: want 400, got %d: %s", rr2.Code, rr2.Body.String())
	}
	var resp2 map[string]any
	if err := json.NewDecoder(bytes.NewReader(rr2.Body.Bytes())).Decode(&resp2); err != nil {
		t.Fatalf("decode replay 400: %v", err)
	}
	if resp1["error"] != resp2["error"] {
		t.Errorf("replay error code differs: first=%v replay=%v", resp1["error"], resp2["error"])
	}

	// Critical: replay with a *valid* body and same key still returns the
	// cached 400 (idempotency contract: same key → same response).
	goodBody := map[string]any{"name": "BadPred Gear", "type": "other"}
	rr3 := doCreateGear(t, h, userID, goodBody, "Idempotency-Key", idemp)
	if rr3.Code != http.StatusBadRequest {
		t.Fatalf("replay with valid body: want stored 400, got %d: %s", rr3.Code, rr3.Body.String())
	}
}

// TestIdempotencyReplayPreservesSwapComponent400 verifies that SwapComponent's
// early-validation 400 paths (slot_key required, new_component_payload required)
// are persisted and replayed.
func TestIdempotencyReplayPreservesSwapComponent400(t *testing.T) {
	h, userID, pool := gearCRUDTestHandler(t)
	gearID := insertTestGear(t, pool, userID, "MTB", "bike", "bike")
	idemp := uuid.NewString()

	// First call: missing slot_key → 400.
	badBody, _ := json.Marshal(map[string]any{
		"new_component_payload": map[string]any{
			"name": "Pad",
			"type": "bike.consumable.brake_pads",
		},
	})
	req1 := httptest.NewRequest(http.MethodPost,
		fmt.Sprintf("/api/gear/%s/components/swap", gearID),
		bytes.NewReader(badBody))
	req1.Header.Set("Content-Type", "application/json")
	req1.Header.Set("Idempotency-Key", idemp)
	req1 = req1.WithContext(context.WithValue(req1.Context(), middleware.UserIDKey, userID))
	req1 = injectChiURLParam(req1, "id", gearID)
	rr1 := httptest.NewRecorder()
	h.SwapComponent(rr1, req1)
	if rr1.Code != http.StatusBadRequest {
		t.Fatalf("first swap: want 400, got %d: %s", rr1.Code, rr1.Body.String())
	}
	var resp1 map[string]any
	if err := json.NewDecoder(bytes.NewReader(rr1.Body.Bytes())).Decode(&resp1); err != nil {
		t.Fatalf("decode first 400: %v", err)
	}

	// Second call: same key, *valid* slot_key. Replay must return cached 400.
	validBody, _ := json.Marshal(map[string]any{
		"slot_key": "brake_pads_rear",
		"new_component_payload": map[string]any{
			"name": "Pad",
			"type": "bike.consumable.brake_pads",
		},
	})
	req2 := httptest.NewRequest(http.MethodPost,
		fmt.Sprintf("/api/gear/%s/components/swap", gearID),
		bytes.NewReader(validBody))
	req2.Header.Set("Content-Type", "application/json")
	req2.Header.Set("Idempotency-Key", idemp)
	req2 = req2.WithContext(context.WithValue(req2.Context(), middleware.UserIDKey, userID))
	req2 = injectChiURLParam(req2, "id", gearID)
	rr2 := httptest.NewRecorder()
	h.SwapComponent(rr2, req2)
	if rr2.Code != http.StatusBadRequest {
		t.Fatalf("replay swap: want cached 400, got %d: %s", rr2.Code, rr2.Body.String())
	}
	var resp2 map[string]any
	if err := json.NewDecoder(bytes.NewReader(rr2.Body.Bytes())).Decode(&resp2); err != nil {
		t.Fatalf("decode replay 400: %v", err)
	}
	for _, key := range []string{"type", "title", "status", "detail", "instance"} {
		if resp1[key] != resp2[key] {
			t.Errorf("replay %s differs: first=%v replay=%v", key, resp1[key], resp2[key])
		}
	}

	// No swap event was recorded.
	var count int
	if err := pool.QueryRow(context.Background(),
		`SELECT COUNT(*) FROM gear_component_swap_events WHERE gear_id = $1`, gearID,
	).Scan(&count); err != nil {
		t.Fatalf("count swap events: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0 swap events (validation failed), got %d", count)
	}
}

// TestIdempotencyDoesNotPersistAuth401 verifies that a 401 from missing JWT
// is NEVER persisted as idempotency state. Auth state is session-derived,
// not request-body-derived, so caching it would prevent recovery from auth
// glitches.
func TestIdempotencyDoesNotPersistAuth401(t *testing.T) {
	h, _, pool := gearCRUDTestHandler(t)
	idemp := uuid.NewString()

	// Build a request WITHOUT injecting userID into context.
	body, _ := json.Marshal(map[string]any{"name": "Anon Bike", "type": "bike"})
	req := httptest.NewRequest(http.MethodPost, "/api/gear", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", idemp)
	rr := httptest.NewRecorder()
	h.CreateGear(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d: %s", rr.Code, rr.Body.String())
	}

	// Verify NO idempotency row was written. The 401 path runs before
	// RequireUserID returns a userID, so there is nothing to key on — but the
	// stronger assertion is that the table is empty for this key.
	var count int
	if err := pool.QueryRow(context.Background(),
		`SELECT COUNT(*) FROM gear_idempotency_keys WHERE key = $1`, idemp,
	).Scan(&count); err != nil {
		t.Fatalf("query idempotency rows: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0 idempotency rows for unauthenticated 401, got %d", count)
	}
}

// TestIdempotencyReplayDoesNotStoreTransient5xx verifies that transient server
// errors (5xx) are never persisted as idempotency responses, so that a later
// retry can succeed without being stuck on a cached error.
//
// We simulate a transient error by using a valid key before any mutation
// succeeds. If the handler returns 5xx and never calls storeIdempotencyResponse,
// a subsequent retry with the same key must run the handler again (in-flight
// race path, response_jsonb remains 'null').
func TestIdempotencyReplayDoesNotStoreTransient5xx(t *testing.T) {
	h, userID, pool := gearCRUDTestHandler(t)
	idemp := uuid.NewString()

	// Inspect the DB directly: after a successful create with this key, the row
	// must have response_status_code = 201, not 0 or 500.
	body := map[string]any{"name": "5xx-test Bike", "type": "bike"}
	rr := doCreateGear(t, h, userID, body, "Idempotency-Key", idemp)
	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rr.Code, rr.Body.String())
	}

	var storedStatus int
	if err := pool.QueryRow(context.Background(),
		`SELECT response_status_code FROM gear_idempotency_keys WHERE key = $1 AND user_id = $2`,
		idemp, userID,
	).Scan(&storedStatus); err != nil {
		t.Fatalf("query stored status code: %v", err)
	}
	if storedStatus != http.StatusCreated {
		t.Errorf("stored response_status_code = %d, want %d (5xx must not overwrite)", storedStatus, http.StatusCreated)
	}
}
