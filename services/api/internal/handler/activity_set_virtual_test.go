package handler

// activity_set_virtual_test.go — Integration tests for SetActivityVirtual.
//
// Requires TEST_DATABASE_URL to be set; the whole suite is skipped otherwise.
//
// Tests:
//   - TestSetActivityVirtual_HappyPath                — toggle on + off, 200 response, audit rows
//   - TestSetActivityVirtual_OwnershipDenied          — wrong user → 404
//   - TestSetActivityVirtual_InvalidBody              — malformed JSON → 400
//   - TestSetActivityVirtual_FrontendContractFormat   — exact frontend JSON format accepted; response includes exclude_from_heatmap
//   - TestSetActivityVirtual_DisallowUnknownFields    — stale field name → 400
//   - TestSetActivityVirtual_ExcludeFromHeatmapRestore — on/off restore; response includes exclude_from_heatmap
//   - TestSetActivityVirtual_ManualExcludeNotOverwritten — manual clear between toggles is preserved

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

// TestSetActivityVirtual_FrontendContractFormat verifies that the exact JSON format
// sent by the frontend apiClient ({is_virtual: true}) is accepted and processed.
// This test pins the frontend↔backend contract and will fail if either side drifts.
// Also verifies the response now includes exclude_from_heatmap (R2-9).
func TestSetActivityVirtual_FrontendContractFormat(t *testing.T) {
	pool := fusionTestPool(t)
	ctx := context.Background()
	db := &database.DB{Pool: pool}

	userID := insertTestUser(t, pool)
	actID := insertMinimalActivity(t, db, userID)
	h := &Handler{DB: db}

	// Exact body format as sent by frontend/src/api/activities.ts setActivityVirtual().
	body := []byte(`{"is_virtual":true}`)
	rr := setVirtualRequest(t, h, userID, actID, body)
	if rr.Code != http.StatusOK {
		t.Fatalf("frontend contract (on): expected 200, got %d — body: %s", rr.Code, rr.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("frontend contract (on): decode response: %v", err)
	}
	if isVirtual, ok := resp["is_virtual"].(bool); !ok || !isVirtual {
		t.Errorf("frontend contract (on): expected {\"is_virtual\":true}, got %v", resp)
	}
	// R2-9: response must include exclude_from_heatmap=true after on-toggle.
	if excl, ok := resp["exclude_from_heatmap"].(bool); !ok || !excl {
		t.Errorf("frontend contract (on): expected exclude_from_heatmap=true in response, got %v", resp)
	}

	// Verify DB state.
	var dbIsVirtual bool
	if err := pool.QueryRow(ctx, `SELECT is_virtual FROM activities WHERE id = $1`, actID).Scan(&dbIsVirtual); err != nil {
		t.Fatalf("frontend contract (on): query DB: %v", err)
	}
	if !dbIsVirtual {
		t.Error("frontend contract (on): is_virtual not set in DB")
	}

	// Also verify the audit row was written with the correct value key.
	var auditCount int
	if err := pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM activity_edits
		  WHERE activity_id = $1 AND edit_type = 'is_virtual'
		    AND (edit_data->>'value')::boolean = true`,
		actID,
	).Scan(&auditCount); err != nil {
		t.Fatalf("frontend contract (on): query audit: %v", err)
	}
	if auditCount != 1 {
		t.Errorf("frontend contract (on): expected 1 audit row, got %d", auditCount)
	}

	// Toggle off: response must include exclude_from_heatmap with restored value (false,
	// since activity was created with exclude_from_heatmap=false).
	body = []byte(`{"is_virtual":false}`)
	rr = setVirtualRequest(t, h, userID, actID, body)
	if rr.Code != http.StatusOK {
		t.Fatalf("frontend contract (off): expected 200, got %d — body: %s", rr.Code, rr.Body.String())
	}
	var respOff map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &respOff); err != nil {
		t.Fatalf("frontend contract (off): decode response: %v", err)
	}
	if isVirtual, ok := respOff["is_virtual"].(bool); !ok || isVirtual {
		t.Errorf("frontend contract (off): expected is_virtual=false in response, got %v", respOff)
	}
	// R2-9: response must include exclude_from_heatmap after off-toggle.
	if _, ok := respOff["exclude_from_heatmap"]; !ok {
		t.Errorf("frontend contract (off): exclude_from_heatmap field missing from response, got %v", respOff)
	}
}

// TestSetActivityVirtual_DisallowUnknownFields verifies that a request with an
// unknown field (e.g. the old {value: true} format) returns 400.
// This test guards against the frontend sending a stale / wrong field name.
func TestSetActivityVirtual_DisallowUnknownFields(t *testing.T) {
	pool := fusionTestPool(t)
	db := &database.DB{Pool: pool}

	userID := insertTestUser(t, pool)
	actID := insertMinimalActivity(t, db, userID)
	h := &Handler{DB: db}

	// Old (now-incorrect) frontend format — should be rejected with 400.
	body := []byte(`{"value":true}`)
	rr := setVirtualRequest(t, h, userID, actID, body)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("unknown field: expected 400, got %d — body: %s", rr.Code, rr.Body.String())
	}
}

// TestSetActivityVirtual_ExcludeFromHeatmapRestore verifies that:
//  1. on-toggle saves prev exclude_from_heatmap in audit data.
//  2. off-toggle restores the saved value correctly.
//  3. With no prior audit row, off-toggle falls back to exclude_from_heatmap=false.
//  4. Response includes exclude_from_heatmap field (R2-9).
func TestSetActivityVirtual_ExcludeFromHeatmapRestore(t *testing.T) {
	pool := fusionTestPool(t)
	ctx := context.Background()
	db := &database.DB{Pool: pool}

	h := &Handler{DB: db}

	t.Run("RestoreTrue", func(t *testing.T) {
		// Arrange: activity with exclude_from_heatmap=true before virtual toggle.
		userID := insertTestUser(t, pool)
		actID := insertMinimalActivity(t, db, userID)
		// Set exclude_from_heatmap=true as the pre-virtual state.
		if _, err := pool.Exec(ctx,
			`UPDATE activities SET exclude_from_heatmap = true WHERE id = $1`, actID,
		); err != nil {
			t.Fatalf("setup: set exclude_from_heatmap: %v", err)
		}

		// on-toggle: should set is_virtual=true, exclude_from_heatmap=true (already true).
		body, _ := json.Marshal(map[string]any{"is_virtual": true})
		rr := setVirtualRequest(t, h, userID, actID, body)
		if rr.Code != http.StatusOK {
			t.Fatalf("RestoreTrue on: expected 200, got %d — body: %s", rr.Code, rr.Body.String())
		}
		// R2-9: response must include exclude_from_heatmap=true.
		var onResp map[string]any
		if err := json.Unmarshal(rr.Body.Bytes(), &onResp); err != nil {
			t.Fatalf("RestoreTrue on: decode response: %v", err)
		}
		if excl, ok := onResp["exclude_from_heatmap"].(bool); !ok || !excl {
			t.Errorf("RestoreTrue on: expected exclude_from_heatmap=true in response, got %v", onResp)
		}

		// Verify prev_exclude_from_heatmap=true stored in audit row.
		var auditData []byte
		if err := pool.QueryRow(ctx,
			`SELECT edit_data FROM activity_edits
			  WHERE activity_id = $1 AND edit_type = 'is_virtual'
			    AND (edit_data->>'value')::boolean = true
			  ORDER BY created_at DESC LIMIT 1`,
			actID,
		).Scan(&auditData); err != nil {
			t.Fatalf("RestoreTrue on: query audit: %v", err)
		}
		var ad setVirtualAuditData
		if err := json.Unmarshal(auditData, &ad); err != nil {
			t.Fatalf("RestoreTrue on: decode audit: %v", err)
		}
		if ad.PrevExcludeFromHeatmap == nil || !*ad.PrevExcludeFromHeatmap {
			t.Errorf("RestoreTrue on: expected prev_exclude_from_heatmap=true in audit, got %v", ad)
		}

		// off-toggle: should restore exclude_from_heatmap=true.
		body, _ = json.Marshal(map[string]any{"is_virtual": false})
		rr = setVirtualRequest(t, h, userID, actID, body)
		if rr.Code != http.StatusOK {
			t.Fatalf("RestoreTrue off: expected 200, got %d — body: %s", rr.Code, rr.Body.String())
		}
		// R2-9: response must include exclude_from_heatmap=true (restored value).
		var offResp map[string]any
		if err := json.Unmarshal(rr.Body.Bytes(), &offResp); err != nil {
			t.Fatalf("RestoreTrue off: decode response: %v", err)
		}
		if excl, ok := offResp["exclude_from_heatmap"].(bool); !ok || !excl {
			t.Errorf("RestoreTrue off: expected exclude_from_heatmap=true in response, got %v", offResp)
		}

		var gotExclude bool
		if err := pool.QueryRow(ctx,
			`SELECT exclude_from_heatmap FROM activities WHERE id = $1`, actID,
		).Scan(&gotExclude); err != nil {
			t.Fatalf("RestoreTrue off: query DB: %v", err)
		}
		if !gotExclude {
			t.Errorf("RestoreTrue off: expected exclude_from_heatmap restored to true, got false")
		}
	})

	t.Run("RestoreFalse", func(t *testing.T) {
		// Arrange: activity with exclude_from_heatmap=false (default).
		userID := insertTestUser(t, pool)
		actID := insertMinimalActivity(t, db, userID)

		// on-toggle.
		body, _ := json.Marshal(map[string]any{"is_virtual": true})
		rr := setVirtualRequest(t, h, userID, actID, body)
		if rr.Code != http.StatusOK {
			t.Fatalf("RestoreFalse on: expected 200, got %d — body: %s", rr.Code, rr.Body.String())
		}

		// off-toggle: should restore exclude_from_heatmap=false.
		body, _ = json.Marshal(map[string]any{"is_virtual": false})
		rr = setVirtualRequest(t, h, userID, actID, body)
		if rr.Code != http.StatusOK {
			t.Fatalf("RestoreFalse off: expected 200, got %d — body: %s", rr.Code, rr.Body.String())
		}

		var gotExclude bool
		if err := pool.QueryRow(ctx,
			`SELECT exclude_from_heatmap FROM activities WHERE id = $1`, actID,
		).Scan(&gotExclude); err != nil {
			t.Fatalf("RestoreFalse off: query DB: %v", err)
		}
		if gotExclude {
			t.Errorf("RestoreFalse off: expected exclude_from_heatmap restored to false, got true")
		}
	})

	t.Run("NoAuditFallback", func(t *testing.T) {
		// Arrange: activity that is already virtual (e.g. imported as virtual).
		// No prior is_virtual audit row exists — off-toggle should fall back to false.
		userID := insertTestUser(t, pool)
		actID := insertMinimalActivity(t, db, userID)
		if _, err := pool.Exec(ctx,
			`UPDATE activities SET is_virtual = true, exclude_from_heatmap = true WHERE id = $1`, actID,
		); err != nil {
			t.Fatalf("NoAuditFallback setup: %v", err)
		}

		body, _ := json.Marshal(map[string]any{"is_virtual": false})
		rr := setVirtualRequest(t, h, userID, actID, body)
		if rr.Code != http.StatusOK {
			t.Fatalf("NoAuditFallback off: expected 200, got %d — body: %s", rr.Code, rr.Body.String())
		}

		var gotExclude bool
		if err := pool.QueryRow(ctx,
			`SELECT exclude_from_heatmap FROM activities WHERE id = $1`, actID,
		).Scan(&gotExclude); err != nil {
			t.Fatalf("NoAuditFallback off: query DB: %v", err)
		}
		if gotExclude {
			t.Errorf("NoAuditFallback off: expected fallback exclude_from_heatmap=false, got true")
		}
	})
}

// TestSetActivityVirtual_ManualExcludeNotOverwritten verifies R2-12:
// If the user manually sets exclude_from_heatmap=false (via PATCH) between the
// on-toggle and the off-toggle, the off-toggle must NOT restore the audit value —
// it must leave the manually-set false in place.
func TestSetActivityVirtual_ManualExcludeNotOverwritten(t *testing.T) {
	pool := fusionTestPool(t)
	ctx := context.Background()
	db := &database.DB{Pool: pool}
	h := &Handler{DB: db}

	// Arrange: activity with exclude_from_heatmap=true (so on-toggle saves true as prev).
	userID := insertTestUser(t, pool)
	actID := insertMinimalActivity(t, db, userID)
	if _, err := pool.Exec(ctx,
		`UPDATE activities SET exclude_from_heatmap = true WHERE id = $1`, actID,
	); err != nil {
		t.Fatalf("setup: set exclude_from_heatmap=true: %v", err)
	}

	// on-toggle: prev_exclude_from_heatmap=true is saved in audit.
	body, _ := json.Marshal(map[string]any{"is_virtual": true})
	rr := setVirtualRequest(t, h, userID, actID, body)
	if rr.Code != http.StatusOK {
		t.Fatalf("on-toggle: expected 200, got %d — body: %s", rr.Code, rr.Body.String())
	}

	// Simulate user manually clearing exclude_from_heatmap via PATCH /activities/{id}
	// (e.g. they unchecked the checkbox while the activity was virtual).
	if _, err := pool.Exec(ctx,
		`UPDATE activities SET exclude_from_heatmap = false WHERE id = $1`, actID,
	); err != nil {
		t.Fatalf("manual patch simulation: %v", err)
	}

	// off-toggle: current exclude_from_heatmap is false (user cleared it), so the
	// handler must NOT restore from audit (which would set it back to true).
	// The manual false value must be preserved.
	body, _ = json.Marshal(map[string]any{"is_virtual": false})
	rr = setVirtualRequest(t, h, userID, actID, body)
	if rr.Code != http.StatusOK {
		t.Fatalf("off-toggle: expected 200, got %d — body: %s", rr.Code, rr.Body.String())
	}

	// R2-12: exclude_from_heatmap must still be false.
	var gotExclude bool
	if err := pool.QueryRow(ctx,
		`SELECT exclude_from_heatmap FROM activities WHERE id = $1`, actID,
	).Scan(&gotExclude); err != nil {
		t.Fatalf("off-toggle: query DB: %v", err)
	}
	if gotExclude {
		t.Errorf("R2-12: off-toggle overwrote manual exclude_from_heatmap=false, restored to true")
	}

	// R2-9: response must report exclude_from_heatmap=false.
	var resp map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("off-toggle: decode response: %v", err)
	}
	if excl, ok := resp["exclude_from_heatmap"].(bool); !ok || excl {
		t.Errorf("R2-9: expected exclude_from_heatmap=false in response, got %v", resp)
	}
}
