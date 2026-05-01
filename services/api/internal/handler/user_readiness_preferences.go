package handler

import (
	"log/slog"
	"net/http"

	"github.com/skadia-app/skadia/services/api/internal/event"
	"github.com/skadia-app/skadia/services/api/internal/httputil"
)

// readinessInvalidator returns the AnalyticsInvalidator wired on the Handler
// (production: *event.Publisher; tests: FakeInvalidator). Returns nil when
// NATS is not configured so callers can skip the publish entirely.
func (h *Handler) readinessInvalidator() event.AnalyticsInvalidator {
	return h.AnalyticsInvalidator
}

// readinessPreferenceBody is the wire shape for both GET response and PUT body.
// preferred_source is null (omitted) for "automatic", "manufacturer" or
// "skadia" otherwise.
type readinessPreferenceBody struct {
	PreferredSource *string `json:"preferred_source"`
}

// validReadinessPrefSources lists the values accepted on the PUT path. Stays
// in sync with the DB CHECK constraint and database.validReadinessSources.
var validReadinessPrefSources = map[string]struct{}{
	"manufacturer": {},
	"skadia":      {},
}

// GetUserReadinessPreferences returns the current preference (preferred_source
// is null when no manual override is stored).
func (h *Handler) GetUserReadinessPreferences(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}

	pref, err := h.DB.GetUserReadinessPreference(r.Context(), userID)
	if err != nil {
		slog.Error("get readiness preference failed", "user_id", userID, "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "preference read failed", r.URL.Path)
		return
	}
	body := readinessPreferenceBody{}
	if pref != nil {
		body.PreferredSource = pref.PreferredSource
	}
	httputil.WriteJSON(w, http.StatusOK, body)
}

// PutUserReadinessPreferences stores a preference and triggers analytics
// invalidation. The publisher is best-effort here: even if invalidation fails,
// the write succeeded and the user gets a 200 confirming their choice.
func (h *Handler) PutUserReadinessPreferences(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}

	// Empty body {} and {"preferred_source": null} are both treated as "set to NULL" (auto-mode).
	body, ok := httputil.DecodeJSON[readinessPreferenceBody](w, r)
	if !ok {
		return
	}

	if body.PreferredSource != nil {
		if _, valid := validReadinessPrefSources[*body.PreferredSource]; !valid {
			httputil.WriteError(w, http.StatusBadRequest, "Bad Request",
				"preferred_source must be 'manufacturer', 'skadia', or null", r.URL.Path)
			return
		}
	}

	if body.PreferredSource == nil {
		if err := h.DB.UpsertUserReadinessPreferenceNull(r.Context(), userID); err != nil {
			slog.Error("upsert readiness preference (null) failed", "user_id", userID, "error", err)
			httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "preference write failed", r.URL.Path)
			return
		}
	} else {
		if err := h.DB.UpsertUserReadinessPreference(r.Context(), userID, *body.PreferredSource); err != nil {
			slog.Error("upsert readiness preference failed", "user_id", userID, "error", err)
			httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "preference write failed", r.URL.Path)
			return
		}
	}

	if inv := h.readinessInvalidator(); inv != nil {
		if err := inv.PublishAnalyticsInvalidate(userID); err != nil {
			// Best-effort: log + continue. The pref change is durable; the
			// invalidation will be re-tried on the next user-initiated trigger.
			slog.Warn("readiness preference analytics invalidate failed", "user_id", userID, "error", err)
		}
	}

	// Re-read from DB so the response reflects the canonical persisted state
	// rather than echoing the request body (which may differ if the DB applied
	// coercion such as NULL normalization).
	stored, err := h.DB.GetUserReadinessPreference(r.Context(), userID)
	if err != nil {
		slog.Error("get readiness preference after put failed", "user_id", userID, "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "preference read-back failed", r.URL.Path)
		return
	}
	canonical := readinessPreferenceBody{}
	if stored != nil {
		canonical.PreferredSource = stored.PreferredSource
	}
	httputil.WriteJSON(w, http.StatusOK, canonical)
}
