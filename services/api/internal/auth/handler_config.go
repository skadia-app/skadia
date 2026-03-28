package auth

import (
	"net/http"

	"github.com/skadia-app/skadia/services/api/internal/httputil"
)

// AuthConfig handles GET /auth/config (public, no authentication required).
func (h *Handler) AuthConfig(w http.ResponseWriter, r *http.Request) {
	providers := make([]string, 0, 2)
	if h.Config.StravaClientID != "" {
		providers = append(providers, "strava")
	}
	if h.Config.GoogleClientID != "" {
		providers = append(providers, "google")
	}

	httputil.WriteJSONNoCache(w, http.StatusOK, map[string]any{
		"registration_mode": h.Config.RegistrationMode,
		"social_providers":  providers,
		"smtp_configured":   h.Config.SMTPHost != "",
	})
}
