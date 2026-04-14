package auth

import (
	"log/slog"
	"net/http"

	"github.com/skadia-app/skadia/services/api/internal/httputil"
)

// AuthConfig handles GET /auth/config (public, no authentication required).
func (h *Handler) AuthConfig(w http.ResponseWriter, r *http.Request) {
	providers := make([]string, 0, 2)
	if h.Settings.Get("social.strava_client_id").Value != "" {
		providers = append(providers, "strava")
	}
	if h.Settings.Get("social.google_client_id").Value != "" {
		providers = append(providers, "google")
	}

	// bootstrap_mode signals the frontend to show the "create first admin" UI.
	// Delegates to the same helper the Register handler uses so the query lives
	// in one place. On DB error we log and fall back to false — the public
	// config endpoint must stay responsive, and exposing the bootstrap UI on a
	// transient DB outage would be worse UX than a login form that can't
	// temporarily authenticate.
	bootstrapMode, err := h.isBootstrapMode(r.Context())
	if err != nil {
		slog.Error("auth.config: bootstrap mode check failed", "error", err)
	}

	httputil.WriteJSONNoCache(w, http.StatusOK, map[string]any{
		"registration_mode": h.Settings.Get("registration.mode").Value,
		"social_providers":  providers,
		"smtp_configured":   h.Settings.Get("smtp.host").Value != "",
		"bootstrap_mode":    bootstrapMode,
	})
}
