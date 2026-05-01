//go:build integration

package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/event"
	"github.com/skadia-app/skadia/services/api/internal/middleware"
)

// newReadinessPrefHandler constructs a Handler with an injected
// FakeInvalidator so PUT-path invalidation can be asserted. Returns the
// Handler and the FakeInvalidator for direct inspection.
func newReadinessPrefHandler(t *testing.T, pool *pgxpool.Pool) (*Handler, *event.FakeInvalidator) {
	t.Helper()
	fake := &event.FakeInvalidator{}
	h := &Handler{
		DB:                   &database.DB{Pool: pool},
		ReadinessInvalidator: fake,
	}
	return h, fake
}

// putPref builds and dispatches PUT /user/readiness-preferences.
func putPref(t *testing.T, h *Handler, uid string, body string) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(http.MethodPut, "/user/readiness-preferences", bytes.NewReader([]byte(body)))
	r.Header.Set("Content-Type", "application/json")
	r = r.WithContext(context.WithValue(r.Context(), middleware.UserIDKey, uid))
	rr := httptest.NewRecorder()
	h.PutUserReadinessPreferences(rr, r)
	return rr
}

// getPref builds and dispatches GET /user/readiness-preferences.
func getPref(t *testing.T, h *Handler, uid string) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(http.MethodGet, "/user/readiness-preferences", nil)
	r = r.WithContext(context.WithValue(r.Context(), middleware.UserIDKey, uid))
	rr := httptest.NewRecorder()
	h.GetUserReadinessPreferences(rr, r)
	return rr
}

func TestUserReadinessPref_PutManufacturer(t *testing.T) {
	pool := fusionTestPool(t)
	uid := insertTestUser(t, pool)
	h, fake := newReadinessPrefHandler(t, pool)
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(),
			`DELETE FROM user_readiness_preferences WHERE user_id = $1`, uid)
	})

	rr := putPref(t, h, uid, `{"preferred_source":"manufacturer"}`)
	if rr.Code != http.StatusOK {
		t.Fatalf("status %d, want 200; body: %s", rr.Code, rr.Body.String())
	}
	pref, err := h.DB.GetUserReadinessPreference(context.Background(), uid)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if pref == nil || pref.PreferredSource == nil || *pref.PreferredSource != "manufacturer" {
		t.Fatalf("stored pref = %+v, want manufacturer", pref)
	}
	if len(fake.Calls) != 1 || fake.Calls[0] != uid {
		t.Errorf("invalidator calls = %v, want [%s]", fake.Calls, uid)
	}
}

func TestUserReadinessPref_PutSkadia(t *testing.T) {
	pool := fusionTestPool(t)
	uid := insertTestUser(t, pool)
	h, fake := newReadinessPrefHandler(t, pool)
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(),
			`DELETE FROM user_readiness_preferences WHERE user_id = $1`, uid)
	})

	rr := putPref(t, h, uid, `{"preferred_source":"skadia"}`)
	if rr.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rr.Code, rr.Body.String())
	}
	pref, _ := h.DB.GetUserReadinessPreference(context.Background(), uid)
	if pref == nil || pref.PreferredSource == nil || *pref.PreferredSource != "skadia" {
		t.Fatalf("stored pref = %+v, want skadia", pref)
	}
	if len(fake.Calls) != 1 || fake.Calls[0] != uid {
		t.Errorf("invalidator calls = %v, want [%s]", fake.Calls, uid)
	}
}

func TestUserReadinessPref_PutNull(t *testing.T) {
	pool := fusionTestPool(t)
	uid := insertTestUser(t, pool)
	h, fake := newReadinessPrefHandler(t, pool)
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(),
			`DELETE FROM user_readiness_preferences WHERE user_id = $1`, uid)
	})

	// Seed a non-null pref first so the null PUT exercises the clear path.
	if rr := putPref(t, h, uid, `{"preferred_source":"skadia"}`); rr.Code != http.StatusOK {
		t.Fatalf("seed put status %d: %s", rr.Code, rr.Body.String())
	}
	rr := putPref(t, h, uid, `{"preferred_source":null}`)
	if rr.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rr.Code, rr.Body.String())
	}
	pref, _ := h.DB.GetUserReadinessPreference(context.Background(), uid)
	if pref == nil {
		t.Fatalf("expected pref row (with null source), got nil")
	}
	if pref.PreferredSource != nil {
		t.Errorf("PreferredSource = %v, want nil", *pref.PreferredSource)
	}
	// Both PUTs must have triggered an invalidation.
	if len(fake.Calls) != 2 || fake.Calls[0] != uid || fake.Calls[1] != uid {
		t.Errorf("invalidator calls = %v, want [%s %s]", fake.Calls, uid, uid)
	}
}

func TestUserReadinessPref_PutBogus(t *testing.T) {
	pool := fusionTestPool(t)
	uid := insertTestUser(t, pool)
	h, fake := newReadinessPrefHandler(t, pool)

	rr := putPref(t, h, uid, `{"preferred_source":"bogus"}`)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status %d, want 400; body: %s", rr.Code, rr.Body.String())
	}
	// Validation failure must NOT trigger an invalidation.
	if len(fake.Calls) != 0 {
		t.Errorf("invalidator calls on 400 = %v, want []", fake.Calls)
	}
}

// TestUserReadinessPref_PutInvalidatorErr verifies the best-effort guarantee:
// the publish failure does not propagate to the response. The DB write
// succeeded, so the user gets a 200.
func TestUserReadinessPref_PutInvalidatorErr(t *testing.T) {
	pool := fusionTestPool(t)
	uid := insertTestUser(t, pool)
	fake := &event.FakeInvalidator{NextErr: errors.New("boom")}
	h := &Handler{
		DB:                   &database.DB{Pool: pool},
		ReadinessInvalidator: fake,
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(),
			`DELETE FROM user_readiness_preferences WHERE user_id = $1`, uid)
	})

	rr := putPref(t, h, uid, `{"preferred_source":"skadia"}`)
	if rr.Code != http.StatusOK {
		t.Fatalf("status %d, want 200 (best-effort); body: %s", rr.Code, rr.Body.String())
	}
	pref, _ := h.DB.GetUserReadinessPreference(context.Background(), uid)
	if pref == nil || pref.PreferredSource == nil || *pref.PreferredSource != "skadia" {
		t.Fatalf("stored pref = %+v, want skadia (DB write must persist even when publish fails)", pref)
	}
	if len(fake.Calls) != 1 {
		t.Errorf("invalidator calls = %v, want 1", fake.Calls)
	}
}

func TestUserReadinessPref_GetAfterPut(t *testing.T) {
	pool := fusionTestPool(t)
	uid := insertTestUser(t, pool)
	h, _ := newReadinessPrefHandler(t, pool)
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(),
			`DELETE FROM user_readiness_preferences WHERE user_id = $1`, uid)
	})

	if rr := putPref(t, h, uid, `{"preferred_source":"skadia"}`); rr.Code != http.StatusOK {
		t.Fatalf("put status %d: %s", rr.Code, rr.Body.String())
	}
	rr := getPref(t, h, uid)
	if rr.Code != http.StatusOK {
		t.Fatalf("get status %d: %s", rr.Code, rr.Body.String())
	}
	var got struct {
		PreferredSource *string `json:"preferred_source"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.PreferredSource == nil || *got.PreferredSource != "skadia" {
		t.Errorf("preferred_source = %v, want skadia", got.PreferredSource)
	}
}

func TestUserReadinessPref_GetFreshUser(t *testing.T) {
	pool := fusionTestPool(t)
	uid := insertTestUser(t, pool)
	h, _ := newReadinessPrefHandler(t, pool)

	rr := getPref(t, h, uid)
	if rr.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rr.Code, rr.Body.String())
	}
	var got map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if v, ok := got["preferred_source"]; !ok || v != nil {
		t.Errorf("preferred_source = %v (ok=%v), want null/nil", v, ok)
	}
}
