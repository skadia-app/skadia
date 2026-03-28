package auth

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"

	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/httputil"
	"github.com/skadia-app/skadia/services/api/internal/middleware"
)

// --- OAuth2 Config Builders ---

func (h *Handler) stravaOAuthConfig() *oauth2.Config {
	return &oauth2.Config{
		ClientID:     h.Config.StravaClientID,
		ClientSecret: h.Config.StravaClientSecret,
		Endpoint: oauth2.Endpoint{
			AuthURL:  "https://www.strava.com/oauth/authorize",
			TokenURL: "https://www.strava.com/oauth/token",
		},
		RedirectURL: h.Config.AppURL + "/api/v1/auth/social/strava/callback",
		Scopes:      []string{"read"},
	}
}

func (h *Handler) googleOAuthConfig() *oauth2.Config {
	return &oauth2.Config{
		ClientID:     h.Config.GoogleClientID,
		ClientSecret: h.Config.GoogleClientSecret,
		Endpoint:     google.Endpoint,
		RedirectURL:  h.Config.AppURL + "/api/v1/auth/social/google/callback",
		Scopes:       []string{"openid", "email", "profile"},
	}
}

// --- Handlers ---

// SocialLogin handles GET /auth/social/{provider}.
// Redirects the browser to the provider's OAuth consent page.
func (h *Handler) SocialLogin(w http.ResponseWriter, r *http.Request) {
	provider := chi.URLParam(r, "provider")

	var oauthCfg *oauth2.Config
	switch provider {
	case "strava":
		if h.Config.StravaClientID == "" {
			httputil.WriteJSONNoCache(w, http.StatusNotFound, map[string]string{"error": "provider not configured"})
			return
		}
		oauthCfg = h.stravaOAuthConfig()
	case "google":
		if h.Config.GoogleClientID == "" {
			httputil.WriteJSONNoCache(w, http.StatusNotFound, map[string]string{"error": "provider not configured"})
			return
		}
		oauthCfg = h.googleOAuthConfig()
	default:
		httputil.WriteJSONNoCache(w, http.StatusNotFound, map[string]string{"error": "unknown provider"})
		return
	}

	// Generate CSRF state token and store in cookie.
	state, err := GenerateToken()
	if err != nil {
		slog.Error("generate oauth state failed", "error", err)
		httputil.WriteJSONNoCache(w, http.StatusInternalServerError, map[string]string{"error": "internal_error"})
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "oauth_state",
		Value:    state,
		Path:     "/api/v1/auth/social",
		HttpOnly: true,
		Secure:   h.isHTTPS,
		SameSite: http.SameSiteLaxMode, // Lax required for OAuth redirects
		MaxAge:   300,                  // 5 minutes
	})

	url := oauthCfg.AuthCodeURL(state)
	http.Redirect(w, r, url, http.StatusTemporaryRedirect)
}

// SocialCallback handles GET /auth/social/{provider}/callback.
// Exchanges the OAuth code for tokens, gets/creates the user, and redirects
// to the frontend with a short-lived auth code.
func (h *Handler) SocialCallback(w http.ResponseWriter, r *http.Request) {
	// Clear state cookie immediately — r.Cookie() reads from the request (unaffected
	// by Set-Cookie on the response), so clearing first is safe and ensures cleanup
	// on all code paths including errors.
	http.SetCookie(w, &http.Cookie{
		Name:     "oauth_state",
		Value:    "",
		Path:     "/api/v1/auth/social",
		HttpOnly: true,
		Secure:   h.isHTTPS,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})

	provider := chi.URLParam(r, "provider")

	// Validate CSRF state.
	stateCookie, err := r.Cookie("oauth_state")
	if err != nil || stateCookie.Value == "" {
		h.socialError(w, r, "invalid_state")
		return
	}
	if subtle.ConstantTimeCompare([]byte(r.URL.Query().Get("state")), []byte(stateCookie.Value)) != 1 {
		h.socialError(w, r, "invalid_state")
		return
	}

	// Check for error from provider (e.g. user denied consent).
	if errParam := r.URL.Query().Get("error"); errParam != "" {
		slog.Warn("OAuth error from provider", "provider", provider, "error", errParam)
		h.socialError(w, r, "provider_error")
		return
	}

	code := r.URL.Query().Get("code")
	if code == "" {
		h.socialError(w, r, "missing_code")
		return
	}

	// Resolve OAuth config.
	var oauthCfg *oauth2.Config
	switch provider {
	case "strava":
		oauthCfg = h.stravaOAuthConfig()
	case "google":
		oauthCfg = h.googleOAuthConfig()
	default:
		h.socialError(w, r, "unknown_provider")
		return
	}

	// Exchange authorization code for access/refresh tokens.
	token, err := oauthCfg.Exchange(r.Context(), code)
	if err != nil {
		slog.Error("OAuth token exchange failed", "provider", provider, "error", err)
		h.socialError(w, r, "exchange_failed")
		return
	}

	// Fetch user profile from the provider.
	providerID, providerEmail, username, err := h.fetchProviderProfile(r.Context(), provider, token)
	if err != nil {
		slog.Error("fetch provider profile failed", "provider", provider, "error", err)
		h.socialError(w, r, "profile_failed")
		return
	}

	// Get or create user (handles email collision internally).
	user, _, err := h.DB.GetOrCreateUserByOAuth(r.Context(),
		provider, providerID, providerEmail, username,
		[]byte(token.AccessToken), []byte(token.RefreshToken),
		&token.Expiry, h.Config.OAuthEncryptionKey)
	if err != nil {
		if errors.Is(err, database.ErrEmailCollision) {
			// TODO: Account linking endpoint — currently users are told to "link in settings"
			// but no such endpoint exists. See: docs/internal/specs/2026-03-24-auth-migration-go-native-design.md Section 11
			h.socialError(w, r, "email_exists")
			return
		}
		slog.Error("get or create user by OAuth failed", "error", err)
		h.socialError(w, r, "internal_error")
		return
	}

	// Check is_active (C6).
	if !user.IsActive {
		h.socialError(w, r, "account_inactive")
		return
	}
	// Update last login timestamp.
	_ = h.DB.UpdateLoginSuccess(r.Context(), user.ID)

	slog.Info("auth.login_success", "user_id", user.ID, "method", "social", "provider", provider)
	go h.DB.LogAuthEvent(h.shutdownCtx, user.ID, "login_success", middleware.ClientIP(r), r.UserAgent(), map[string]string{"method": "social", "provider": provider})

	// Generate a short-lived auth code (30s, single-use) for the frontend.
	authCode, err := GenerateToken()
	if err != nil {
		slog.Error("generate auth code failed", "error", err)
		h.socialError(w, r, "internal_error")
		return
	}

	codeHash := HashToken(authCode)
	if err := h.DB.CreateAuthCode(r.Context(), uuid.MustParse(user.ID), codeHash, "", "",
		time.Now().Add(30*time.Second)); err != nil {
		slog.Error("create auth code failed", "error", err)
		h.socialError(w, r, "internal_error")
		return
	}

	// Redirect to frontend callback page with the auth code (URL-encoded for defense-in-depth).
	http.Redirect(w, r, h.Config.FrontendURL+"/callback?code="+url.QueryEscape(authCode), http.StatusTemporaryRedirect)
}

// socialError redirects to the frontend login page with an error code.
func (h *Handler) socialError(w http.ResponseWriter, r *http.Request, errCode string) {
	http.Redirect(w, r, h.Config.FrontendURL+"/login?error="+errCode, http.StatusTemporaryRedirect)
}

// --- Provider Profile Fetching ---

func (h *Handler) fetchProviderProfile(ctx context.Context, provider string, token *oauth2.Token) (providerID, email, username string, err error) {
	client := oauth2.NewClient(ctx, oauth2.StaticTokenSource(token))

	switch provider {
	case "strava":
		return h.fetchStravaProfile(client)
	case "google":
		return h.fetchGoogleProfile(client)
	default:
		return "", "", "", fmt.Errorf("unknown provider: %s", provider)
	}
}

func (h *Handler) fetchStravaProfile(client *http.Client) (string, string, string, error) {
	resp, err := client.Get("https://www.strava.com/api/v3/athlete")
	if err != nil {
		return "", "", "", fmt.Errorf("strava athlete request: %w", err)
	}
	defer resp.Body.Close()

	// Check provider API status before parsing (I8).
	if resp.StatusCode != http.StatusOK {
		errBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return "", "", "", fmt.Errorf("strava API returned %d: %s", resp.StatusCode, string(errBody))
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20)) // 1 MB max
	if err != nil {
		return "", "", "", fmt.Errorf("read strava response: %w", err)
	}

	// NOTE: Strava may return empty email (user privacy settings). Users created
	// without email cannot use password login or forgot-password. They are permanently
	// bound to Strava OAuth until they add an email via account settings (future feature).
	var athlete struct {
		ID        int64  `json:"id"`
		FirstName string `json:"firstname"`
		LastName  string `json:"lastname"`
		Email     string `json:"email"` // may be empty (privacy settings)
	}
	if err := json.Unmarshal(body, &athlete); err != nil {
		return "", "", "", fmt.Errorf("decode strava athlete: %w", err)
	}

	username := strings.TrimSpace(athlete.FirstName + " " + athlete.LastName)
	if username == "" {
		username = "strava_" + strconv.FormatInt(athlete.ID, 10)
	}

	return strconv.FormatInt(athlete.ID, 10), athlete.Email, username, nil
}

func (h *Handler) fetchGoogleProfile(client *http.Client) (string, string, string, error) {
	resp, err := client.Get("https://www.googleapis.com/oauth2/v2/userinfo")
	if err != nil {
		return "", "", "", fmt.Errorf("google userinfo request: %w", err)
	}
	defer resp.Body.Close()

	// Check provider API status before parsing (I8).
	if resp.StatusCode != http.StatusOK {
		errBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return "", "", "", fmt.Errorf("google API returned %d: %s", resp.StatusCode, string(errBody))
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20)) // 1 MB max
	if err != nil {
		return "", "", "", fmt.Errorf("read google response: %w", err)
	}

	var profile struct {
		ID    string `json:"id"`
		Email string `json:"email"`
		Name  string `json:"name"`
	}
	if err := json.Unmarshal(body, &profile); err != nil {
		return "", "", "", fmt.Errorf("decode google profile: %w", err)
	}

	return profile.ID, profile.Email, profile.Name, nil
}
