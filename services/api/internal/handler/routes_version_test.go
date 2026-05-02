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
// Unit-level coverage for parseIfMatchVersion + routeETag (no DB, no HTTP).
// ----------------------------------------------------------------------------

func TestParseIfMatchVersion_StrongQuoted(t *testing.T) {
	v, state := parseIfMatchVersion(`"42"`)
	if state != ifMatchValid || v != 42 {
		t.Fatalf("got (%d, %v), want (42, ifMatchValid)", v, state)
	}
}

func TestParseIfMatchVersion_WeakValidatorTolerated(t *testing.T) {
	v, state := parseIfMatchVersion(`W/"7"`)
	if state != ifMatchValid || v != 7 {
		t.Fatalf("got (%d, %v), want (7, ifMatchValid)", v, state)
	}
}

func TestParseIfMatchVersion_AbsentReturnsAbsent(t *testing.T) {
	v, state := parseIfMatchVersion("")
	if state != ifMatchAbsent || v != 0 {
		t.Fatalf("empty header: got (%d, %v), want (0, ifMatchAbsent)", v, state)
	}
}

func TestParseIfMatchVersion_WildcardTreatedAsAbsent(t *testing.T) {
	v, state := parseIfMatchVersion("*")
	if state != ifMatchAbsent || v != 0 {
		t.Fatalf("wildcard: got (%d, %v), want (0, ifMatchAbsent)", v, state)
	}
}

// TestParseIfMatch_StrongWithWhitespace verifies OWS stripping per RFC 7230.
func TestParseIfMatch_StrongWithWhitespace(t *testing.T) {
	v, state := parseIfMatchVersion(`  "42"  `)
	if state != ifMatchValid || v != 42 {
		t.Fatalf(`got (%d, %v), want (42, ifMatchValid)`, v, state)
	}
}

// TestParseIfMatch_WeakValid covers the W/"N" prefix form.
func TestParseIfMatch_WeakValid(t *testing.T) {
	v, state := parseIfMatchVersion(`W/"99"`)
	if state != ifMatchValid || v != 99 {
		t.Fatalf("got (%d, %v), want (99, ifMatchValid)", v, state)
	}
}

// TestParseIfMatch_MalformedNoQuotes: bare number without quotes → malformed.
func TestParseIfMatch_MalformedNoQuotes(t *testing.T) {
	_, state := parseIfMatchVersion("42")
	if state != ifMatchMalformed {
		t.Fatalf("bare number: got %v, want ifMatchMalformed", state)
	}
}

// TestParseIfMatch_MalformedNonNumeric: quoted non-numeric → malformed.
func TestParseIfMatch_MalformedNonNumeric(t *testing.T) {
	for _, raw := range []string{`"abc"`, `"not-a-number"`, `"-3"`, `"0"`, `"1.5"`, `"  "`} {
		_, state := parseIfMatchVersion(raw)
		if state != ifMatchMalformed {
			t.Errorf("input %q: got %v, want ifMatchMalformed", raw, state)
		}
	}
}

// TestParseIfMatch_Empty: an empty quoted string `""` → malformed (distinct
// from absent header which is the bare empty string "").
func TestParseIfMatch_Empty(t *testing.T) {
	_, state := parseIfMatchVersion(`""`)
	if state != ifMatchMalformed {
		t.Fatalf(`"": got %v, want ifMatchMalformed`, state)
	}
}

// TestParseIfMatch_MultiTagOneInvalid: comma-separated tag list → malformed.
// We set exactly one ETag per resource; a list cannot be evaluated correctly.
func TestParseIfMatch_MultiTagOneInvalid(t *testing.T) {
	_, state := parseIfMatchVersion(`"42", "abc"`)
	if state != ifMatchMalformed {
		t.Fatalf(`multi-tag: got %v, want ifMatchMalformed`, state)
	}
}

// TestParseIfMatch_MultiTagBothValid: even a syntactically valid list is
// rejected as malformed — we don't evaluate tag lists.
func TestParseIfMatch_MultiTagBothValid(t *testing.T) {
	_, state := parseIfMatchVersion(`"42", "43"`)
	if state != ifMatchMalformed {
		t.Fatalf(`multi-tag both valid: got %v, want ifMatchMalformed`, state)
	}
}

func TestRouteETag_StrongFormat(t *testing.T) {
	if got := routeETag(1); got != `"1"` {
		t.Errorf("routeETag(1) = %q, want %q", got, `"1"`)
	}
	if got := routeETag(123456789); got != `"123456789"` {
		t.Errorf("routeETag(123456789) = %q, want %q", got, `"123456789"`)
	}
}

// ----------------------------------------------------------------------------
// Integration-level coverage. Requires TEST_DATABASE_URL.
// ----------------------------------------------------------------------------

// patchRouteRequestWithIfMatch mirrors patchRouteRequest but lets the test
// drive the If-Match header explicitly.
func patchRouteRequestWithIfMatch(t *testing.T, h *Handler, userID, routeID string, body []byte, ifMatch string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/routes/"+routeID, bytes.NewReader(body))
	req = req.WithContext(withUserAndAdmin(req.Context(), userID, false))
	if ifMatch != "" {
		req.Header.Set("If-Match", ifMatch)
	}

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", routeID)
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	w := httptest.NewRecorder()
	h.UpdateRoute(w, req)
	return w
}

func getRouteRequest(t *testing.T, h *Handler, userID, routeID string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/routes/"+routeID, nil)
	req = req.WithContext(withUserAndAdmin(req.Context(), userID, false))

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", routeID)
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	w := httptest.NewRecorder()
	h.GetRoute(w, req)
	return w
}

func TestGetRoute_EmitsStrongETag_Integration(t *testing.T) {
	if os.Getenv("TEST_DATABASE_URL") == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	pool := fusionTestPool(t)
	h := newTestHandler(t, pool)
	userID := insertTestUser(t, pool)
	routeID := insertTestRoute(t, h, userID)

	w := getRouteRequest(t, h, userID, routeID)
	if w.Code != http.StatusOK {
		t.Fatalf("GET status = %d, body = %s", w.Code, w.Body.String())
	}
	etag := w.Header().Get("ETag")
	if etag == "" {
		t.Fatalf("ETag header missing on GET response")
	}
	if etag != `"1"` {
		t.Errorf("expected fresh row to carry ETag %q, got %q", `"1"`, etag)
	}
}

func TestUpdateRoute_IfMatchHappyPath_BumpsVersion_Integration(t *testing.T) {
	if os.Getenv("TEST_DATABASE_URL") == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	pool := fusionTestPool(t)
	h := newTestHandler(t, pool)
	userID := insertTestUser(t, pool)
	routeID := insertTestRoute(t, h, userID)

	// Fresh row → version 1.
	w := patchRouteRequestWithIfMatch(t, h, userID, routeID,
		[]byte(`{"name":"updated"}`), `"1"`)
	if w.Code != http.StatusOK {
		t.Fatalf("PATCH with valid If-Match: status %d, body %s", w.Code, w.Body.String())
	}
	if got := w.Header().Get("ETag"); got != `"2"` {
		t.Errorf("ETag after first PATCH = %q, want %q", got, `"2"`)
	}

	var got model.PlannedRoute
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if got.Version != 2 {
		t.Errorf("body version = %d, want 2", got.Version)
	}
	if got.Name != "updated" {
		t.Errorf("body name = %q, want %q", got.Name, "updated")
	}
}

func TestUpdateRoute_IfMatchStaleReturns412WithCurrent_Integration(t *testing.T) {
	if os.Getenv("TEST_DATABASE_URL") == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	pool := fusionTestPool(t)
	h := newTestHandler(t, pool)
	userID := insertTestUser(t, pool)
	routeID := insertTestRoute(t, h, userID)

	// Tab A patches first, success.
	wA := patchRouteRequestWithIfMatch(t, h, userID, routeID,
		[]byte(`{"name":"from-tab-A"}`), `"1"`)
	if wA.Code != http.StatusOK {
		t.Fatalf("Tab A: status %d, body %s", wA.Code, wA.Body.String())
	}

	// Tab B still believes it holds version 1.
	wB := patchRouteRequestWithIfMatch(t, h, userID, routeID,
		[]byte(`{"name":"from-tab-B"}`), `"1"`)
	if wB.Code != http.StatusPreconditionFailed {
		t.Fatalf("Tab B: expected 412, got %d body=%s", wB.Code, wB.Body.String())
	}
	if got := wB.Header().Get("ETag"); got != `"2"` {
		t.Errorf("412 response ETag = %q, want %q (current row state)", got, `"2"`)
	}

	var current model.PlannedRoute
	if err := json.Unmarshal(wB.Body.Bytes(), &current); err != nil {
		t.Fatalf("decode 412 body: %v", err)
	}
	if current.Name != "from-tab-A" {
		t.Errorf("412 body should expose Tab-A's winning state; got name=%q", current.Name)
	}
	if current.Version != 2 {
		t.Errorf("412 body version = %d, want 2", current.Version)
	}
}

func TestUpdateRoute_NoIfMatchHeaderStillWorks_Integration(t *testing.T) {
	if os.Getenv("TEST_DATABASE_URL") == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	pool := fusionTestPool(t)
	h := newTestHandler(t, pool)
	userID := insertTestUser(t, pool)
	routeID := insertTestRoute(t, h, userID)

	// Legacy client (no If-Match): silent-overwrite path. Server logs a Warn,
	// returns 200, version still bumps.
	w := patchRouteRequestWithIfMatch(t, h, userID, routeID,
		[]byte(`{"name":"legacy-overwrite"}`), "")
	if w.Code != http.StatusOK {
		t.Fatalf("legacy PATCH: status %d, body %s", w.Code, w.Body.String())
	}
	if got := w.Header().Get("ETag"); got != `"2"` {
		t.Errorf("ETag after legacy PATCH = %q, want %q", got, `"2"`)
	}
}

// TestRoutesPATCH_400OnMalformedIfMatch verifies that a present but
// unparseable If-Match header is rejected with 400 (not silently treated as
// absent, which would disable the concurrency guard).
func TestRoutesPATCH_400OnMalformedIfMatch(t *testing.T) {
	if os.Getenv("TEST_DATABASE_URL") == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	pool := fusionTestPool(t)
	h := newTestHandler(t, pool)
	userID := insertTestUser(t, pool)
	routeID := insertTestRoute(t, h, userID)

	malformed := []string{
		`"not-a-number"`,
		`garbage`,
		`not-a-number`,
		`"abc"`,
		`42`,
		`"42", "43"`,
		`"42", "abc"`,
	}
	for _, hdr := range malformed {
		w := patchRouteRequestWithIfMatch(t, h, userID, routeID,
			[]byte(`{"name":"should-not-apply"}`), hdr)
		if w.Code != http.StatusBadRequest {
			t.Errorf("If-Match %q: got %d, want 400; body=%s", hdr, w.Code, w.Body.String())
		}
	}
}

func TestUpdateRoute_SequentialPatchesIncrementVersion_Integration(t *testing.T) {
	if os.Getenv("TEST_DATABASE_URL") == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	pool := fusionTestPool(t)
	h := newTestHandler(t, pool)
	userID := insertTestUser(t, pool)
	routeID := insertTestRoute(t, h, userID)

	currentETag := `"1"`
	for i := 2; i <= 5; i++ {
		w := patchRouteRequestWithIfMatch(t, h, userID, routeID,
			[]byte(`{"name":"step"}`), currentETag)
		if w.Code != http.StatusOK {
			t.Fatalf("step %d: status %d body %s", i, w.Code, w.Body.String())
		}
		want := `"` + itoaVer(i) + `"`
		if got := w.Header().Get("ETag"); got != want {
			t.Fatalf("step %d ETag = %q, want %q", i, got, want)
		}
		currentETag = want
	}
}

// itoaVer keeps the test allocation-free of strconv import noise.
func itoaVer(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
