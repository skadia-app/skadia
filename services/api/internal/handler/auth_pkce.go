package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
)

// MobileTokenRequest represents a PKCE token exchange request from mobile.
type MobileTokenRequest struct {
	Code         string `json:"code"`
	CodeVerifier string `json:"code_verifier"`
	RedirectURI  string `json:"redirect_uri"`
}

// MobileRefreshRequest represents a token refresh request from mobile (body-based, no cookie).
type MobileRefreshRequest struct {
	RefreshToken string `json:"refresh_token"`
}

// ExchangeToken exchanges a PKCE authorization code for tokens from OIDC.
// The mobile app sends code + code_verifier; we forward to OIDC and relay the response.
//
// POST /api/v1/auth/token
func (h *Handler) ExchangeToken(w http.ResponseWriter, r *http.Request) {
	if h.Auth == nil {
		writeError(w, http.StatusServiceUnavailable, "Service Unavailable", "auth not configured", r.URL.Path)
		return
	}

	var req MobileTokenRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Bad Request", "invalid request body", r.URL.Path)
		return
	}
	if req.Code == "" || req.CodeVerifier == "" {
		writeError(w, http.StatusBadRequest, "Bad Request", "code and code_verifier required", r.URL.Path)
		return
	}

	redirectURI := req.RedirectURI
	if redirectURI == "" {
		redirectURI = h.Auth.AppRedirectURI
	}

	data := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {req.Code},
		"code_verifier": {req.CodeVerifier},
		"client_id":     {h.Auth.OIDCClientID},
		"redirect_uri":  {redirectURI},
	}

	tokenResp, status, err := h.postTokenRaw(r.Context(), data)
	if err != nil {
		slog.Error("PKCE token exchange failed", "error", err)
		writeError(w, http.StatusBadGateway, "Bad Gateway", "token exchange failed", r.URL.Path)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	w.Write(tokenResp) //nolint:errcheck
}

// MobileRefreshToken refreshes tokens using a refresh_token supplied in the JSON body.
// For mobile: no cookie, tokens travel in the response body.
//
// POST /api/v1/auth/mobile-refresh
func (h *Handler) MobileRefreshToken(w http.ResponseWriter, r *http.Request) {
	if h.Auth == nil {
		writeError(w, http.StatusServiceUnavailable, "Service Unavailable", "auth not configured", r.URL.Path)
		return
	}

	var req MobileRefreshRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Bad Request", "invalid request body", r.URL.Path)
		return
	}
	if req.RefreshToken == "" {
		writeError(w, http.StatusBadRequest, "Bad Request", "refresh_token required", r.URL.Path)
		return
	}

	data := url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {req.RefreshToken},
		"client_id":     {h.Auth.OIDCClientID},
	}
	if h.Auth.OIDCClientSecret != "" {
		data.Set("client_secret", h.Auth.OIDCClientSecret)
	}

	tokenResp, status, err := h.postTokenRaw(r.Context(), data)
	if err != nil {
		slog.Error("mobile token refresh failed", "error", err)
		writeError(w, http.StatusBadGateway, "Bad Gateway", "token refresh failed", r.URL.Path)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	w.Write(tokenResp) //nolint:errcheck
}

// postTokenRaw posts to OIDC provider's token endpoint and returns the raw response body + status code.
func (h *Handler) postTokenRaw(ctx context.Context, data url.Values) ([]byte, int, error) {
	tokenURL := fmt.Sprintf("%s/application/o/token/", h.Auth.OIDCBaseURL)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL,
		strings.NewReader(data.Encode()))
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("token request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, 0, fmt.Errorf("read token response: %w", err)
	}
	return body, resp.StatusCode, nil
}
