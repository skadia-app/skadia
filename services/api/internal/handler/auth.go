package handler

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/lestrrat-go/jwx/v2/jwa"
	"github.com/lestrrat-go/jwx/v2/jwt"

	"github.com/skadia-app/skadia/services/api/internal/httputil"
)

const (
	refreshCookieName = "skadia_refresh"
	stateCookieName   = "skadia_state"
	jwtDuration       = 15 * time.Minute
	refreshDuration   = 7 * 24 * time.Hour
)

// AuthConfig holds OIDC OIDC settings needed by auth handlers.
type AuthConfig struct {
	OIDCBaseURL      string // internal URL (container-to-container)
	OIDCExternalURL  string // browser-facing URL (for redirects)
	OIDCClientID     string
	OIDCClientSecret string
	OIDCAppSlug      string
	AppRedirectURI        string
	FrontendURL           string
	JWTSecret             []byte
}

// AuthLogin redirects the browser to OIDC provider's OIDC authorization endpoint.
//
// GET /api/v1/auth/login
func (h *Handler) AuthLogin(w http.ResponseWriter, r *http.Request) {
	if h.Auth == nil {
		httputil.WriteError(w, http.StatusServiceUnavailable, "Service Unavailable", "auth not configured", r.URL.Path)
		return
	}

	state, err := generateState()
	if err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to generate state", r.URL.Path)
		return
	}

	isSecure := strings.HasPrefix(h.Auth.AppRedirectURI, "https://")

	http.SetCookie(w, &http.Cookie{
		Name:     stateCookieName,
		Value:    state,
		Path:     "/",
		MaxAge:   int((5 * time.Minute).Seconds()),
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   isSecure,
	})

	params := url.Values{}
	params.Set("response_type", "code")
	params.Set("client_id", h.Auth.OIDCClientID)
	params.Set("redirect_uri", h.Auth.AppRedirectURI)
	params.Set("scope", "openid profile email offline_access")
	params.Set("state", state)

	authorizeURL := fmt.Sprintf("%s/application/o/authorize/?%s",
		h.Auth.OIDCExternalURL, params.Encode())

	http.Redirect(w, r, authorizeURL, http.StatusFound)
}

// AuthCallback handles the OIDC authorization code callback from OIDC.
//
// GET /api/v1/auth/callback?code=...&state=...
func (h *Handler) AuthCallback(w http.ResponseWriter, r *http.Request) {
	if h.Auth == nil {
		httputil.WriteError(w, http.StatusServiceUnavailable, "Service Unavailable", "auth not configured", r.URL.Path)
		return
	}

	// Validate state
	stateCookie, err := r.Cookie(stateCookieName)
	if err != nil || stateCookie.Value == "" {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "missing state cookie", r.URL.Path)
		return
	}
	if r.URL.Query().Get("state") != stateCookie.Value {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "state mismatch", r.URL.Path)
		return
	}

	// Clear state cookie
	http.SetCookie(w, &http.Cookie{
		Name:    stateCookieName,
		Value:   "",
		Path:    "/",
		MaxAge:  -1,
		HttpOnly: true,
	})

	code := r.URL.Query().Get("code")
	if code == "" {
		errMsg := r.URL.Query().Get("error_description")
		if errMsg == "" {
			errMsg = r.URL.Query().Get("error")
		}
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "no authorization code: "+errMsg, r.URL.Path)
		return
	}

	// Exchange code for tokens
	tokens, err := h.exchangeCode(r.Context(), code)
	if err != nil {
		slog.Error("OIDC code exchange failed", "error", err)
		httputil.WriteError(w, http.StatusUnauthorized, "Unauthorized", "code exchange failed", r.URL.Path)
		return
	}

	// Fetch userinfo
	userInfo, err := h.fetchUserInfo(r.Context(), tokens.AccessToken)
	if err != nil {
		slog.Error("userinfo fetch failed", "error", err)
		httputil.WriteError(w, http.StatusUnauthorized, "Unauthorized", "userinfo unavailable", r.URL.Path)
		return
	}

	// Find or create user in DB
	user, err := h.DB.GetOrCreateUserByOIDCSubject(r.Context(), userInfo.Sub, userInfo.PreferredUsername, userInfo.Email)
	if err != nil {
		slog.Error("user lookup/create failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "user setup failed", r.URL.Path)
		return
	}

	// Issue our own HS256 JWT (15 min) with internal user UUID
	ourToken, err := h.issueJWT(user.ID)
	if err != nil {
		slog.Error("JWT issue failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "token generation failed", r.URL.Path)
		return
	}

	// Store OIDC refresh token as HttpOnly cookie (7 days)
	if tokens.RefreshToken != "" {
		isSecureCb := strings.HasPrefix(h.Auth.AppRedirectURI, "https://")
		http.SetCookie(w, &http.Cookie{
			Name:     refreshCookieName,
			Value:    tokens.RefreshToken,
			Path:     "/api/v1/auth",
			MaxAge:   int(refreshDuration.Seconds()),
			HttpOnly: true,
			SameSite: http.SameSiteStrictMode,
			Secure:   isSecureCb,
		})
	}

	// Redirect frontend to /callback?token=<jwt>
	redirectURL := fmt.Sprintf("%s/callback?token=%s", h.Auth.FrontendURL, url.QueryEscape(ourToken))
	http.Redirect(w, r, redirectURL, http.StatusFound)
}

// AuthRefresh issues a new short-lived JWT using the OIDC refresh token from the cookie.
//
// POST /api/v1/auth/refresh
func (h *Handler) AuthRefresh(w http.ResponseWriter, r *http.Request) {
	if h.Auth == nil {
		httputil.WriteError(w, http.StatusServiceUnavailable, "Service Unavailable", "auth not configured", r.URL.Path)
		return
	}

	cookie, err := r.Cookie(refreshCookieName)
	if err != nil || cookie.Value == "" {
		httputil.WriteError(w, http.StatusUnauthorized, "Unauthorized", "no refresh token", r.URL.Path)
		return
	}

	tokens, err := h.refreshTokens(r.Context(), cookie.Value)
	if err != nil {
		slog.Warn("token refresh failed", "error", err)
		// Clear stale cookie
		http.SetCookie(w, &http.Cookie{
			Name: refreshCookieName, Value: "", Path: "/api/v1/auth", MaxAge: -1, HttpOnly: true,
		})
		httputil.WriteError(w, http.StatusUnauthorized, "Unauthorized", "refresh failed", r.URL.Path)
		return
	}

	userInfo, err := h.fetchUserInfo(r.Context(), tokens.AccessToken)
	if err != nil {
		httputil.WriteError(w, http.StatusUnauthorized, "Unauthorized", "userinfo unavailable", r.URL.Path)
		return
	}

	user, err := h.DB.GetOrCreateUserByOIDCSubject(r.Context(), userInfo.Sub, userInfo.PreferredUsername, userInfo.Email)
	if err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "user lookup failed", r.URL.Path)
		return
	}

	ourToken, err := h.issueJWT(user.ID)
	if err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "token generation failed", r.URL.Path)
		return
	}

	// Rotate refresh cookie if OIDC returned a new one
	if tokens.RefreshToken != "" {
		isSecureRef := strings.HasPrefix(h.Auth.AppRedirectURI, "https://")
		http.SetCookie(w, &http.Cookie{
			Name:     refreshCookieName,
			Value:    tokens.RefreshToken,
			Path:     "/api/v1/auth",
			MaxAge:   int(refreshDuration.Seconds()),
			HttpOnly: true,
			SameSite: http.SameSiteStrictMode,
			Secure:   isSecureRef,
		})
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]string{"token": ourToken})
}

// AuthLogout clears the refresh cookie and optionally revokes the token at OIDC.
//
// POST /api/v1/auth/logout
func (h *Handler) AuthLogout(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie(refreshCookieName)

	// Revoke at OIDC if we have a token and credentials are configured
	if err == nil && cookie.Value != "" && h.Auth != nil &&
		h.Auth.OIDCClientID != "" && h.Auth.OIDCClientSecret != "" {
		_ = h.revokeToken(r.Context(), cookie.Value) // best-effort, ignore errors
	}

	http.SetCookie(w, &http.Cookie{
		Name:     refreshCookieName,
		Value:    "",
		Path:     "/api/v1/auth",
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
	})

	httputil.WriteJSON(w, http.StatusOK, map[string]string{"status": "logged out"})
}

// ── Internal helpers ─────────────────────────────────────────────────────────

type oidcTokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
}

type oidcUserInfo struct {
	Sub               string `json:"sub"`
	Email             string `json:"email"`
	PreferredUsername string `json:"preferred_username"`
	Name              string `json:"name"`
}

func (h *Handler) exchangeCode(ctx context.Context, code string) (*oidcTokenResponse, error) {
	body := url.Values{}
	body.Set("grant_type", "authorization_code")
	body.Set("code", code)
	body.Set("redirect_uri", h.Auth.AppRedirectURI)
	body.Set("client_id", h.Auth.OIDCClientID)
	body.Set("client_secret", h.Auth.OIDCClientSecret)

	return h.postTokenEndpoint(ctx, body)
}

func (h *Handler) refreshTokens(ctx context.Context, refreshToken string) (*oidcTokenResponse, error) {
	body := url.Values{}
	body.Set("grant_type", "refresh_token")
	body.Set("refresh_token", refreshToken)
	body.Set("client_id", h.Auth.OIDCClientID)
	body.Set("client_secret", h.Auth.OIDCClientSecret)

	return h.postTokenEndpoint(ctx, body)
}

func (h *Handler) postTokenEndpoint(ctx context.Context, body url.Values) (*oidcTokenResponse, error) {
	tokenURL := fmt.Sprintf("%s/application/o/token/", h.Auth.OIDCBaseURL)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL,
		strings.NewReader(body.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	httpClient := &http.Client{Timeout: 15 * time.Second}
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("token request: %w", err)
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("token endpoint %d: %s", resp.StatusCode, raw)
	}

	var tokens oidcTokenResponse
	if err := json.Unmarshal(raw, &tokens); err != nil {
		return nil, fmt.Errorf("parse token response: %w", err)
	}
	return &tokens, nil
}

func (h *Handler) fetchUserInfo(ctx context.Context, accessToken string) (*oidcUserInfo, error) {
	userinfoURL := fmt.Sprintf("%s/application/o/userinfo/", h.Auth.OIDCBaseURL)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, userinfoURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	httpClient := &http.Client{Timeout: 15 * time.Second}
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("userinfo request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("userinfo returned %d", resp.StatusCode)
	}

	var info oidcUserInfo
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return nil, fmt.Errorf("parse userinfo: %w", err)
	}
	if info.Sub == "" {
		return nil, fmt.Errorf("userinfo missing sub claim")
	}
	// Fall back to sub as username if preferred_username is empty
	if info.PreferredUsername == "" {
		info.PreferredUsername = info.Sub
	}
	return &info, nil
}

func (h *Handler) issueJWT(userID string) (string, error) {
	token, err := jwt.NewBuilder().
		Subject(userID).
		Issuer("skadia").
		IssuedAt(time.Now()).
		Expiration(time.Now().Add(jwtDuration)).
		Build()
	if err != nil {
		return "", err
	}
	signed, err := jwt.Sign(token, jwt.WithKey(jwa.HS256, h.Auth.JWTSecret))
	if err != nil {
		return "", err
	}
	return string(signed), nil
}

func (h *Handler) revokeToken(ctx context.Context, token string) error {
	revokeURL := fmt.Sprintf("%s/application/o/revoke/", h.Auth.OIDCBaseURL)
	body := url.Values{}
	body.Set("token", token)
	body.Set("client_id", h.Auth.OIDCClientID)
	body.Set("client_secret", h.Auth.OIDCClientSecret)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, revokeURL,
		strings.NewReader(body.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

func generateState() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(b), nil
}
