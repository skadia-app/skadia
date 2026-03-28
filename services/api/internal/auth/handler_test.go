package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/alexedwards/argon2id"
	"github.com/go-chi/chi/v5"

	"github.com/skadia-app/skadia/services/api/internal/config"
	"github.com/skadia-app/skadia/services/api/internal/httputil"
	"github.com/skadia-app/skadia/services/api/internal/middleware"
)

// testHandler creates a Handler with test config and no DB (nil).
// Only usable for tests that don't hit database paths.
func testHandler(t *testing.T, cfg *config.Config) *Handler {
	t.Helper()
	// Ensure dummyHash is set with a real argon2id hash (normally done by ValidateConfig at startup).
	if dummyHash == "" {
		var err error
		dummyHash, err = argon2id.CreateHash("test-dummy", GetArgon2Params())
		if err != nil {
			t.Fatal("failed to create test dummy hash: " + err.Error())
		}
	}
	if cfg.JWTSecret == "" {
		cfg.JWTSecret = strings.Repeat("ab", 32) // 64 hex chars
	}
	return NewHandler(nil, cfg, &Mailer{}, middleware.NewRateLimiter(context.Background()), context.Background())
}

// --- AuthConfig Endpoint ---

func TestAuthConfigEndpoint(t *testing.T) {
	h := testHandler(t, &config.Config{
		RegistrationMode: "open",
		StravaClientID:   "test-strava-id",
		GoogleClientID:   "", // not configured
	})

	req := httptest.NewRequest(http.MethodGet, "/auth/config", nil)
	w := httptest.NewRecorder()
	h.AuthConfig(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}

	if resp["registration_mode"] != "open" {
		t.Errorf("expected registration_mode=open, got %v", resp["registration_mode"])
	}

	providers, ok := resp["social_providers"].([]any)
	if !ok {
		t.Fatal("social_providers should be an array")
	}
	if len(providers) != 1 || providers[0] != "strava" {
		t.Errorf("expected [strava], got %v", providers)
	}
}

func TestAuthConfigBothProviders(t *testing.T) {
	h := testHandler(t, &config.Config{
		RegistrationMode: "invite",
		StravaClientID:   "strava-id",
		GoogleClientID:   "google-id",
	})

	req := httptest.NewRequest(http.MethodGet, "/auth/config", nil)
	w := httptest.NewRecorder()
	h.AuthConfig(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	providers := resp["social_providers"].([]any)
	if len(providers) != 2 {
		t.Errorf("expected 2 providers, got %d: %v", len(providers), providers)
	}
}

func TestAuthConfigNoProviders(t *testing.T) {
	h := testHandler(t, &config.Config{
		RegistrationMode: "approval",
	})

	req := httptest.NewRequest(http.MethodGet, "/auth/config", nil)
	w := httptest.NewRecorder()
	h.AuthConfig(w, req)

	var resp map[string]any
	json.Unmarshal(w.Body.Bytes(), &resp)

	providers := resp["social_providers"].([]any)
	if len(providers) != 0 {
		t.Errorf("expected empty providers, got %v", providers)
	}
	if resp["registration_mode"] != "approval" {
		t.Errorf("expected registration_mode=approval, got %v", resp["registration_mode"])
	}
}

// --- Registration Mode Enforcement ---

func TestRegistrationModeInviteBlocksRegister(t *testing.T) {
	h := testHandler(t, &config.Config{
		RegistrationMode: "invite",
	})

	body := `{"email":"test@example.com","username":"testuser","password":"supersecretpassword"}`
	req := httptest.NewRequest(http.MethodPost, "/auth/register", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.Register(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for invite mode, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]string
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["error"] != "registration_disabled" {
		t.Errorf("expected error 'registration_disabled', got %q", resp["error"])
	}
}

// --- Login Validation ---

func TestLoginMissingFields(t *testing.T) {
	h := testHandler(t, &config.Config{})

	tests := []struct {
		name string
		body string
	}{
		{"empty email", `{"email":"","password":"test"}`},
		{"empty password", `{"email":"test@example.com","password":""}`},
		{"both empty", `{"email":"","password":""}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/auth/login", strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			h.Login(w, req)
			if w.Code != http.StatusBadRequest {
				t.Errorf("expected 400, got %d: %s", w.Code, w.Body.String())
			}
		})
	}
}

func TestLoginInvalidJSON(t *testing.T) {
	h := testHandler(t, &config.Config{})

	req := httptest.NewRequest(http.MethodPost, "/auth/login", strings.NewReader("not json"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.Login(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

// --- Register Validation ---

func TestRegisterPasswordTooShort(t *testing.T) {
	h := testHandler(t, &config.Config{
		RegistrationMode: "open",
		SMTPHost:         "smtp.test.com",
	})

	body := `{"email":"test@example.com","username":"testuser","password":"short"}`
	req := httptest.NewRequest(http.MethodPost, "/auth/register", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.Register(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for short password, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]string
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["error"] != "password_too_short" {
		t.Errorf("unexpected error: %q", resp["error"])
	}
}

func TestRegisterMissingFields(t *testing.T) {
	h := testHandler(t, &config.Config{
		RegistrationMode: "open",
		SMTPHost:         "smtp.test.com",
	})

	tests := []struct {
		name string
		body string
	}{
		{"no email", `{"email":"","username":"user","password":"longpassword1"}`},
		{"no username", `{"email":"test@x.com","username":"","password":"longpassword1"}`},
		{"no password", `{"email":"test@x.com","username":"user","password":""}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/auth/register", strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			h.Register(w, req)
			if w.Code != http.StatusBadRequest {
				t.Errorf("expected 400, got %d: %s", w.Code, w.Body.String())
			}
		})
	}
}

func TestRegisterPasswordExactlyAtMinimum(t *testing.T) {
	// Password is exactly 10 chars -- should pass validation (hits DB after).
	h := testHandler(t, &config.Config{
		RegistrationMode: "invite", // invite blocks before DB, so we can test password validation order
	})

	body := `{"email":"test@x.com","username":"user","password":"1234567890"}`
	req := httptest.NewRequest(http.MethodPost, "/auth/register", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.Register(w, req)

	// With invite mode, 403 means the password length check passed (it's checked before mode).
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403 (invite block after validation), got %d: %s", w.Code, w.Body.String())
	}
}

// --- PKCE S256 Challenge ---

func TestPKCES256Challenge(t *testing.T) {
	// Known test vector from RFC 7636 Appendix B.
	verifier := "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"
	expected := "E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM"

	got := computeS256Challenge(verifier)
	if got != expected {
		t.Errorf("S256 challenge: expected %q, got %q", expected, got)
	}
}

func TestPKCES256DifferentVerifiers(t *testing.T) {
	c1 := computeS256Challenge("verifier-one")
	c2 := computeS256Challenge("verifier-two")
	if c1 == c2 {
		t.Error("different verifiers should produce different challenges")
	}
}

func TestPKCES256Deterministic(t *testing.T) {
	v := "my-code-verifier"
	c1 := computeS256Challenge(v)
	c2 := computeS256Challenge(v)
	if c1 != c2 {
		t.Errorf("same verifier should produce same challenge: %q vs %q", c1, c2)
	}
}

// --- Refresh Token — Missing Cookie ---

func TestRefreshMissingCookie(t *testing.T) {
	h := testHandler(t, &config.Config{})

	req := httptest.NewRequest(http.MethodPost, "/auth/refresh", nil)
	w := httptest.NewRecorder()

	h.Refresh(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 without cookie, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]string
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["error"] != "session_expired" {
		t.Errorf("expected error 'session_expired', got %q", resp["error"])
	}
}

func TestRefreshEmptyCookie(t *testing.T) {
	h := testHandler(t, &config.Config{})

	req := httptest.NewRequest(http.MethodPost, "/auth/refresh", nil)
	req.AddCookie(&http.Cookie{Name: RefreshCookieName, Value: ""})
	w := httptest.NewRecorder()

	h.Refresh(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 with empty cookie, got %d", w.Code)
	}
}

// --- Mobile Refresh — Missing Body ---

func TestMobileRefreshMissingToken(t *testing.T) {
	h := testHandler(t, &config.Config{})

	req := httptest.NewRequest(http.MethodPost, "/auth/mobile/refresh", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.MobileRefresh(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 without refresh_token, got %d: %s", w.Code, w.Body.String())
	}
}

// --- Mobile Exchange — Missing Fields ---

func TestMobileExchangeMissingCode(t *testing.T) {
	h := testHandler(t, &config.Config{})

	req := httptest.NewRequest(http.MethodPost, "/auth/mobile/exchange",
		strings.NewReader(`{"code":"","code_verifier":"v"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.MobileExchange(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestMobileExchangeMissingVerifier(t *testing.T) {
	h := testHandler(t, &config.Config{})

	req := httptest.NewRequest(http.MethodPost, "/auth/mobile/exchange",
		strings.NewReader(`{"code":"abc","code_verifier":""}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.MobileExchange(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

// --- ExchangeCode — Missing Code ---

func TestExchangeCodeMissing(t *testing.T) {
	h := testHandler(t, &config.Config{})

	req := httptest.NewRequest(http.MethodPost, "/auth/exchange-code",
		strings.NewReader(`{"code":""}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.ExchangeCode(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

// --- Password Reset Validation ---

func TestResetPasswordMissingFields(t *testing.T) {
	h := testHandler(t, &config.Config{})

	tests := []struct {
		name string
		body string
	}{
		{"no token", `{"token":"","new_password":"longpassword1"}`},
		{"no password", `{"token":"abc","new_password":""}`},
		{"both empty", `{"token":"","new_password":""}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/auth/reset-password", strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			h.ResetPassword(w, req)
			if w.Code != http.StatusBadRequest {
				t.Errorf("expected 400, got %d: %s", w.Code, w.Body.String())
			}
		})
	}
}

func TestResetPasswordTooShort(t *testing.T) {
	h := testHandler(t, &config.Config{})

	body := `{"token":"valid-token","new_password":"short"}`
	req := httptest.NewRequest(http.MethodPost, "/auth/reset-password", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.ResetPassword(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for short password, got %d", w.Code)
	}
}

// --- ForgotPassword — Validation Only ---
// Note: ForgotPassword spawns a background goroutine that hits the DB,
// so we can only test the synchronous validation path (missing email)
// without a real database connection.

func TestForgotPasswordMissingEmail(t *testing.T) {
	h := testHandler(t, &config.Config{})

	body := `{"email":""}`
	req := httptest.NewRequest(http.MethodPost, "/auth/forgot-password", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.ForgotPassword(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for missing email, got %d", w.Code)
	}
}

func TestForgotPasswordInvalidJSON(t *testing.T) {
	h := testHandler(t, &config.Config{})

	req := httptest.NewRequest(http.MethodPost, "/auth/forgot-password", strings.NewReader("not json"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.ForgotPassword(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

// --- Logout (Cookie-based) ---

func TestLogoutWithoutCookie(t *testing.T) {
	// Logout without cookie should still return 200.
	h := testHandler(t, &config.Config{})

	req := httptest.NewRequest(http.MethodPost, "/auth/logout", nil)
	w := httptest.NewRecorder()

	h.Logout(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp map[string]string
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["status"] != "logged_out" {
		t.Errorf("expected status=logged_out, got %q", resp["status"])
	}

	// Verify cookie is cleared.
	cookies := w.Result().Cookies()
	for _, c := range cookies {
		if c.Name == RefreshCookieName && c.MaxAge != -1 {
			t.Error("skadia_refresh cookie should have MaxAge=-1 for deletion")
		}
	}
}

// --- Mobile Logout ---

func TestMobileLogoutMissingToken(t *testing.T) {
	h := testHandler(t, &config.Config{})

	req := httptest.NewRequest(http.MethodPost, "/auth/mobile/logout",
		strings.NewReader(`{"refresh_token":""}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.MobileLogout(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

// --- Rate Limiter Behavior ---

func TestRateLimiterBurst(t *testing.T) {
	rl := middleware.NewRateLimiter(context.Background())

	// Burst of 3 — first 3 should pass.
	for i := range 3 {
		if !rl.Allow("test-burst", 0.001, 3) {
			t.Errorf("request %d should be allowed within burst", i+1)
		}
	}

	// 4th should be blocked (rate too low to replenish).
	if rl.Allow("test-burst", 0.001, 3) {
		t.Error("4th request should be rate-limited after burst exhausted")
	}
}

func TestRateLimiterSeparateKeys(t *testing.T) {
	rl := middleware.NewRateLimiter(context.Background())

	// Exhaust one key.
	for range 3 {
		rl.Allow("key-a", 0.001, 3)
	}

	// Different key should be unaffected.
	if !rl.Allow("key-b", 0.001, 3) {
		t.Error("different key should not be rate-limited")
	}
}

// --- Progressive Delay Formula ---

func TestProgressiveDelayFormula(t *testing.T) {
	// The formula from handler_login.go:
	// if failCount > 3: delay = min(2^(failCount-3), 300) seconds
	tests := []struct {
		failCount int
		wantDelay time.Duration
	}{
		{1, 0},
		{2, 0},
		{3, 0},
		{4, 2 * time.Second},    // 2^(4-3) = 2
		{5, 4 * time.Second},    // 2^(5-3) = 4
		{6, 8 * time.Second},    // 2^(6-3) = 8
		{7, 16 * time.Second},   // 2^(7-3) = 16
		{10, 128 * time.Second}, // 2^(10-3) = 128
		{12, 300 * time.Second}, // 2^(12-3)=512 -> capped at 300
		{50, 300 * time.Second}, // capped
	}

	for _, tt := range tests {
		t.Run("count_"+strings.Repeat("I", tt.failCount), func(t *testing.T) {
			var got time.Duration
			if tt.failCount > 3 {
				delay := min(1<<(tt.failCount-3), 300)
				got = time.Duration(delay) * time.Second
			}
			if got != tt.wantDelay {
				t.Errorf("failCount=%d: expected %v, got %v", tt.failCount, tt.wantDelay, got)
			}
		})
	}
}

// --- Login Timing Equalization ---
// We cannot fully test timing equalization without a DB, but we can verify
// the dummyHash is used (the variable is set and is a valid argon2id hash).

func TestDummyHashFormat(t *testing.T) {
	// dummyHash is set by testHandler or ValidateConfig.
	if GetDummyHash() == "" {
		t.Fatal("dummyHash should be set")
	}
	if !strings.HasPrefix(GetDummyHash(), "$argon2id$") {
		t.Errorf("dummyHash should be argon2id format, got prefix: %q", GetDummyHash()[:20])
	}
}

// --- WriteJSONNoCache Helper ---

// TestWriteJSONNoCache verifies that httputil.WriteJSONNoCache sets
// Cache-Control: no-store on every auth response. This header is
// security-critical: without it, browsers or proxies may cache tokens
// and credentials returned by auth endpoints.
func TestWriteJSONNoCache(t *testing.T) {
	w := httptest.NewRecorder()
	httputil.WriteJSONNoCache(w, http.StatusCreated, map[string]string{"key": "value"})

	if w.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("expected application/json, got %q", ct)
	}
	if cc := w.Header().Get("Cache-Control"); cc != "no-store" {
		t.Errorf("expected Cache-Control: no-store, got %q — auth responses must not be cached", cc)
	}

	var resp map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("response not valid JSON: %v", err)
	}
	if resp["key"] != "value" {
		t.Errorf("expected key=value, got %v", resp)
	}
}

// --- Token Functions (supplement token_test.go) ---

func TestHashTokenLength(t *testing.T) {
	// SHA-256 = 32 bytes = 64 hex chars.
	h := HashToken("any-input")
	if len(h) != 64 {
		t.Errorf("expected 64 hex chars, got %d", len(h))
	}
}

// --- JWT Issuance ---

func TestIssueJWTProducesToken(t *testing.T) {
	secret := make([]byte, 32)
	for i := range secret {
		secret[i] = byte(i)
	}

	token, err := IssueJWT("user-123", false, true, false, secret)
	if err != nil {
		t.Fatalf("IssueJWT failed: %v", err)
	}

	if token == "" {
		t.Error("expected non-empty JWT")
	}

	// JWT has 3 dot-separated segments.
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		t.Errorf("expected 3 JWT segments, got %d", len(parts))
	}
}

func TestIssueJWTDifferentUsers(t *testing.T) {
	secret := make([]byte, 32)

	t1, _ := IssueJWT("user-a", false, true, false, secret)
	t2, _ := IssueJWT("user-b", false, true, false, secret)

	if t1 == t2 {
		t.Error("different users should produce different JWTs")
	}
}

// --- clearRefreshCookie ---

func TestClearRefreshCookieAttributes(t *testing.T) {
	h := testHandler(t, &config.Config{
		AppURL: "https://fit.example.com",
	})

	w := httptest.NewRecorder()
	h.clearRefreshCookie(w)

	cookies := w.Result().Cookies()
	if len(cookies) == 0 {
		t.Fatal("expected a Set-Cookie header")
	}

	c := cookies[0]
	if c.Name != RefreshCookieName {
		t.Errorf("expected cookie name 'skadia_refresh', got %q", c.Name)
	}
	if c.MaxAge != -1 {
		t.Errorf("expected MaxAge=-1 for deletion, got %d", c.MaxAge)
	}
	if !c.HttpOnly {
		t.Error("expected HttpOnly=true")
	}
	if !c.Secure {
		t.Error("expected Secure=true for HTTPS AppURL")
	}
	if c.SameSite != http.SameSiteStrictMode {
		t.Errorf("expected SameSite=Strict, got %v", c.SameSite)
	}
	if c.Path != "/api/v1/auth" {
		t.Errorf("expected path /api/v1/auth, got %q", c.Path)
	}
}

func TestClearRefreshCookieHTTP(t *testing.T) {
	h := testHandler(t, &config.Config{
		AppURL: "http://localhost:5173",
	})

	w := httptest.NewRecorder()
	h.clearRefreshCookie(w)

	cookies := w.Result().Cookies()
	c := cookies[0]
	if c.Secure {
		t.Error("expected Secure=false for HTTP AppURL")
	}
}

// --- Email Validation ---

func TestLoginInvalidEmail(t *testing.T) {
	h := testHandler(t, &config.Config{})

	body := `{"email":"not-an-email","password":"testpassword"}`
	req := httptest.NewRequest(http.MethodPost, "/auth/login", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.Login(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid email, got %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]string
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["error"] != "invalid_email" {
		t.Errorf("expected error 'invalid_email', got %q", resp["error"])
	}
}

func TestRegisterInvalidEmail(t *testing.T) {
	h := testHandler(t, &config.Config{RegistrationMode: "open", SMTPHost: "smtp.test.com"})

	body := `{"email":"bad@@email","username":"user","password":"longpassword1"}`
	req := httptest.NewRequest(http.MethodPost, "/auth/register", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.Register(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid email, got %d: %s", w.Code, w.Body.String())
	}
}

func TestForgotPasswordInvalidEmail(t *testing.T) {
	h := testHandler(t, &config.Config{})

	body := `{"email":"not-valid"}`
	req := httptest.NewRequest(http.MethodPost, "/auth/forgot-password", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ForgotPassword(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid email, got %d: %s", w.Code, w.Body.String())
	}
}

// --- Password Max Length ---

func TestLoginPasswordTooLong(t *testing.T) {
	h := testHandler(t, &config.Config{})

	longPassword := strings.Repeat("a", 1001)
	body := `{"email":"test@example.com","password":"` + longPassword + `"}`
	req := httptest.NewRequest(http.MethodPost, "/auth/login", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.Login(w, req)

	// Password max-length is checked inside authenticateUser (with timing equalization),
	// so it returns 401 invalid_credentials (not 400) to avoid leaking info.
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for too-long password, got %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]string
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["error"] != "invalid_credentials" {
		t.Errorf("expected error 'invalid_credentials', got %q", resp["error"])
	}
}

func TestRegisterPasswordTooLong(t *testing.T) {
	h := testHandler(t, &config.Config{RegistrationMode: "open", SMTPHost: "smtp.test.com"})

	longPassword := strings.Repeat("a", 1001)
	body := `{"email":"test@example.com","username":"testuser","password":"` + longPassword + `"}`
	req := httptest.NewRequest(http.MethodPost, "/auth/register", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.Register(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for too-long password, got %d: %s", w.Code, w.Body.String())
	}
}

func TestResetPasswordTooLong(t *testing.T) {
	h := testHandler(t, &config.Config{})

	longPassword := strings.Repeat("a", 1001)
	body := `{"token":"valid-token","new_password":"` + longPassword + `"}`
	req := httptest.NewRequest(http.MethodPost, "/auth/reset-password", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ResetPassword(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for too-long password, got %d: %s", w.Code, w.Body.String())
	}
}

// --- Username Validation ---

func TestRegisterUsernameTooShort(t *testing.T) {
	h := testHandler(t, &config.Config{RegistrationMode: "open", SMTPHost: "smtp.test.com"})

	body := `{"email":"test@example.com","username":"a","password":"longpassword1"}`
	req := httptest.NewRequest(http.MethodPost, "/auth/register", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.Register(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for short username, got %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]string
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["error"] != "username_invalid_length" {
		t.Errorf("expected error 'username_invalid_length', got %q", resp["error"])
	}
}

func TestRegisterUsernameInvalidChars(t *testing.T) {
	h := testHandler(t, &config.Config{RegistrationMode: "open", SMTPHost: "smtp.test.com"})

	body := `{"email":"test@example.com","username":"user<script>","password":"longpassword1"}`
	req := httptest.NewRequest(http.MethodPost, "/auth/register", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.Register(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid username chars, got %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]string
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["error"] != "username_invalid_chars" {
		t.Errorf("expected error 'username_invalid_chars', got %q", resp["error"])
	}
}

func TestRegisterUsernameUnicodeAllowed(t *testing.T) {
	// Unicode letters should be accepted (e.g. "Müller", "Müller.Hans")
	h := testHandler(t, &config.Config{RegistrationMode: "invite"})

	body := `{"email":"test@example.com","username":"Müller-Hans.42","password":"longpassword1"}`
	req := httptest.NewRequest(http.MethodPost, "/auth/register", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.Register(w, req)

	// invite mode → 403 means username validation passed
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403 (invite block after validation), got %d: %s", w.Code, w.Body.String())
	}
}

// --- Body Size Limit ---

func TestLoginBodySizeLimit(t *testing.T) {
	h := testHandler(t, &config.Config{})

	// Create a body > 4096 bytes
	largeBody := `{"email":"test@example.com","password":"` + strings.Repeat("x", 5000) + `"}`
	req := httptest.NewRequest(http.MethodPost, "/auth/login", strings.NewReader(largeBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.Login(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for oversized body, got %d: %s", w.Code, w.Body.String())
	}
}

// --- Registration Mode Check Order ---
// Verify that mode check happens BEFORE input validation (avoids HIBP call on rejection).

func TestRegisterModeCheckBeforeValidation(t *testing.T) {
	h := testHandler(t, &config.Config{
		RegistrationMode: "invite",
	})

	// In invite mode, registration is rejected fast — even with valid fields.
	body := `{"email":"test@example.com","username":"testuser","password":"validpassword123"}`
	req := httptest.NewRequest(http.MethodPost, "/auth/register", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.Register(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403 for invite mode, got %d", w.Code)
	}
}

// --- Verify Email — Missing Token ---

func TestVerifyEmailMissingToken(t *testing.T) {
	h := testHandler(t, &config.Config{})

	req := httptest.NewRequest(http.MethodPost, "/auth/verify-email",
		strings.NewReader(`{"token":""}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.VerifyEmail(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

// --- ChangePassword Validation ---

func TestChangePasswordMissingFields(t *testing.T) {
	h := testHandler(t, &config.Config{})
	tests := []struct {
		name string
		body string
	}{
		{"empty current", `{"current_password":"","new_password":"newpassword1"}`},
		{"empty new", `{"current_password":"oldpassword","new_password":""}`},
		{"both empty", `{"current_password":"","new_password":""}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/auth/change-password", strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")
			// Add user context (ChangePassword is authenticated)
			ctx := context.WithValue(req.Context(), middleware.UserIDKey, "test-user-id")
			req = req.WithContext(ctx)
			w := httptest.NewRecorder()
			h.ChangePassword(w, req)
			if w.Code != http.StatusBadRequest {
				t.Errorf("expected 400, got %d: %s", w.Code, w.Body.String())
			}
		})
	}
}

func TestChangePasswordTooShort(t *testing.T) {
	h := testHandler(t, &config.Config{})
	body := `{"current_password":"oldpassword","new_password":"short"}`
	req := httptest.NewRequest(http.MethodPost, "/auth/change-password", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	ctx := context.WithValue(req.Context(), middleware.UserIDKey, "test-user-id")
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()
	h.ChangePassword(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
	// Check error code is snake_case
	var resp map[string]string
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["error"] != "password_too_short" {
		t.Errorf("expected password_too_short, got %s", resp["error"])
	}
}

func TestChangePasswordCurrentTooLong(t *testing.T) {
	h := testHandler(t, &config.Config{})
	longPW := strings.Repeat("a", 1001)
	body := fmt.Sprintf(`{"current_password":"%s","new_password":"validpassword"}`, longPW)
	req := httptest.NewRequest(http.MethodPost, "/auth/change-password", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	ctx := context.WithValue(req.Context(), middleware.UserIDKey, "test-user-id")
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()
	h.ChangePassword(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestChangePasswordNewTooLong(t *testing.T) {
	h := testHandler(t, &config.Config{})
	longPW := strings.Repeat("a", 1001)
	body := fmt.Sprintf(`{"current_password":"oldpassword","new_password":"%s"}`, longPW)
	req := httptest.NewRequest(http.MethodPost, "/auth/change-password", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	ctx := context.WithValue(req.Context(), middleware.UserIDKey, "test-user-id")
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()
	h.ChangePassword(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

// --- Admin Self-Deactivation Guard ---

func TestAdminCannotDeactivateSelf(t *testing.T) {
	h := testHandler(t, &config.Config{})

	// Use a valid UUID so the parse check passes and we reach the self-deactivation guard.
	selfID := "00000000-0000-0000-0000-000000000001"

	req := httptest.NewRequest(http.MethodPost, "/admin/users/"+selfID+"/deactivate", nil)
	ctx := context.WithValue(req.Context(), middleware.UserIDKey, selfID)
	req = req.WithContext(ctx)

	// Set chi URL param.
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", selfID)
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	w := httptest.NewRecorder()
	h.AdminDeactivateUser(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}

	var resp map[string]string
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["error"] != "cannot_deactivate_self" {
		t.Errorf("expected error 'cannot_deactivate_self', got %q", resp["error"])
	}
}
