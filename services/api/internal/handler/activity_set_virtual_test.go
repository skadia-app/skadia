package handler

// activity_set_virtual_test.go — Integration tests for SetActivityVirtual.
//
// Requires TEST_DATABASE_URL to be set; the whole suite is skipped otherwise.
//
// Tests:
//   - TestSetActivityVirtual_HappyPath     — toggle on + off, 200 response, audit rows
//   - TestSetActivityVirtual_OwnershipDenied — wrong user → 404
//   - TestSetActivityVirtual_InvalidBody   — malformed JSON → 400

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/middleware"
	"github.com/go-chi/chi/v5"
)

// setVirtualRequest calls SetActivityVirtual via its HTTP handler and returns the recorder.
func setVirtualRequest(t *testing.T, h *Handler, userID, activityID string, body []byte) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/activities/"+activityID+"/set-virtual", bytes.NewReader(body))
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", activityID)
	req = req.WithContext(
		context.WithValue(
			context.WithValue(req.Context(), chi.RouteCtxKey, rctx),
			middleware.UserIDKey, userID,
		),
	)
	rr := httptest.NewRecorder()
	h.SetActivityVirtual(rr, req)
	return rr
}

// TestSetActivityVirtual_HappyPath verifies that:
//  1. POST with is_virtual=true sets the flag, returns 200 with {"is_virtual":true}.
//  2. An activity_edits audit row with edit_type='is_virtual' and value=true is written.
//  3. POST with is_virtual=false clears the flag, returns 200 with {"is_virtual":false}.
//  4. A second audit row with value=false is written.
func TestSetActivityVirtual_HappyPath(t *testing.T) {
	pool := fusionTestPool(t)
	ctx := context.Background()
	db := &database.DB{Pool: pool}

	userID := insertTestUser(t, pool)
	actID := insertMinimalActivity(t, db, userID)

	h := &Handler{DB: db}

	// ── Toggle ON ────────────────────────────────────────────────────────────
	body, _ := json.Marshal(map[string]any{"is_virtual": true})
	rr := setVirtualRequest(t, h, userID, actID, body)
	if rr.Code != http.StatusOK {
		t.Fatalf("toggle on: expected 200, got %d — body: %s", rr.Code, rr.Body.String())
	}
	var respOn map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &respOn); err != nil {
		t.Fatalf("toggle on: decode response: %v", err)
	}
	if isVirtual, ok := respOn["is_virtual"].(bool); !ok || !isVirtual {
		t.Errorf("toggle on: expected is_virtual=true in response, got %v", respOn)
	}

	// Verify flag in DB.
	var dbIsVirtual bool
	if err := pool.QueryRow(ctx, `SELECT is_virtual FROM activities WHERE id = $1`, actID).Scan(&dbIsVirtual); err != nil {
		t.Fatalf("toggle on: query is_virtual: %v", err)
	}
	if !dbIsVirtual {
		t.Error("toggle on: is_virtual in DB not set to true after toggle on")
	}

	// Audit row must exist with value=true.
	var auditCount int
	if err := pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM activity_edits WHERE activity_id = $1 AND edit_type = 'is_virtual' AND edit_data->>'value' = 'true'`,
		actID,
	).Scan(&auditCount); err != nil {
		t.Fatalf("toggle on: query audit row: %v", err)
	}
	if auditCount != 1 {
		t.Errorf("toggle on: expected 1 audit row with value=true, got %d", auditCount)
	}

	// ── Toggle OFF ───────────────────────────────────────────────────────────
	body, _ = json.Marshal(map[string]any{"is_virtual": false})
	rr = setVirtualRequest(t, h, userID, actID, body)
	if rr.Code != http.StatusOK {
		t.Fatalf("toggle off: expected 200, got %d — body: %s", rr.Code, rr.Body.String())
	}
	var respOff map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &respOff); err != nil {
		t.Fatalf("toggle off: decode response: %v", err)
	}
	if isVirtual, ok := respOff["is_virtual"].(bool); !ok || isVirtual {
		t.Errorf("toggle off: expected is_virtual=false in response, got %v", respOff)
	}

	// Verify flag cleared in DB.
	if err := pool.QueryRow(ctx, `SELECT is_virtual FROM activities WHERE id = $1`, actID).Scan(&dbIsVirtual); err != nil {
		t.Fatalf("toggle off: query is_virtual: %v", err)
	}
	if dbIsVirtual {
		t.Error("toggle off: is_virtual in DB still true after toggle off")
	}

	// Total of 2 audit rows (one per toggle).
	if err := pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM activity_edits WHERE activity_id = $1 AND edit_type = 'is_virtual'`,
		actID,
	).Scan(&auditCount); err != nil {
		t.Fatalf("toggle off: query total audit rows: %v", err)
	}
	if auditCount != 2 {
		t.Errorf("toggle off: expected 2 total audit rows, got %d", auditCount)
	}
}

// TestSetActivityVirtual_OwnershipDenied verifies that a request from a different
// user cannot toggle the flag on another user's activity and receives 404.
func TestSetActivityVirtual_OwnershipDenied(t *testing.T) {
	pool := fusionTestPool(t)
	db := &database.DB{Pool: pool}

	ownerID := insertTestUser(t, pool)
	actID := insertMinimalActivity(t, db, ownerID)

	// A different user attempts to toggle the owner's activity.
	otherUserID := insertTestUser(t, pool)
	h := &Handler{DB: db}

	body, _ := json.Marshal(map[string]any{"is_virtual": true})
	rr := setVirtualRequest(t, h, otherUserID, actID, body)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("ownership denied: expected 404, got %d — body: %s", rr.Code, rr.Body.String())
	}
}

// TestSetActivityVirtual_InvalidBody verifies that a malformed JSON body returns 400.
func TestSetActivityVirtual_InvalidBody(t *testing.T) {
	pool := fusionTestPool(t)
	db := &database.DB{Pool: pool}

	userID := insertTestUser(t, pool)
	actID := insertMinimalActivity(t, db, userID)

	h := &Handler{DB: db}

	rr := setVirtualRequest(t, h, userID, actID, []byte(`not-json`))
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("invalid body: expected 400, got %d — body: %s", rr.Code, rr.Body.String())
	}
}
