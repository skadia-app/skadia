package handler

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/skadia-app/skadia/services/api/internal/automation"
	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/httputil"
)

// reapplyRulesAfterLiftChange re-fires the rules engine with the narrow
// trigger 'on_enrichment_complete' so the lift_detected SystemRule (and any
// user rule that depends on has_lift / lift_count and chose that trigger)
// re-evaluates against the new lift_count. Phase 4b.B.4.
//
// We intentionally do NOT pass "" (= match every trigger) — that would also
// re-fire Frost/Nacht/Rundtour with retraction semantics and could strip
// those tags as a side-effect of confirming a single lift candidate.
//
// Errors are logged-and-swallowed because the user-facing 204 response is
// already correct (the DB write succeeded); the rules re-fire is a
// best-effort secondary effect.
func (h *Handler) reapplyRulesAfterLiftChange(r *http.Request, activityID, userID string) {
	allRules, listErr := h.DB.ListRules(r.Context(), userID)
	if listErr != nil {
		slog.Warn("lift confirm/reject: list rules failed",
			"activity_id", activityID, "error", listErr)
		return
	}
	if rulesErr := automation.ApplyRulesToActivity(
		r.Context(), h.DB, activityID, userID, allRules,
		"on_enrichment_complete",
	); rulesErr != nil {
		if errors.Is(rulesErr, automation.ErrRulesEnginePaused) {
			slog.Warn("lift confirm/reject: rules skipped — engine paused",
				"activity_id", activityID)
		} else {
			slog.Warn("lift confirm/reject: rules failed",
				"activity_id", activityID, "error", rulesErr)
		}
	}
}

// GetLiftCandidates returns unified lift ride candidates for an activity.
// GET /api/v1/activities/{id}/lifts/candidates
func (h *Handler) GetLiftCandidates(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	filter, ok := parseLiftCandidateFilter(w, r)
	if !ok {
		return
	}
	activityID := chi.URLParam(r, "id")
	activity, err := h.DB.GetActivity(r.Context(), activityID)
	if err != nil {
		slog.Error("get activity for lift candidates failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "", r.URL.Path)
		return
	}
	if activity == nil || activity.UserID != userID {
		httputil.WriteError(w, http.StatusNotFound, "Not Found", "activity not found", r.URL.Path)
		return
	}

	candidates, err := h.DB.GetLiftCandidates(r.Context(), activityID, userID, filter)
	if err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "", r.URL.Path)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, candidates)
}

func parseLiftCandidateFilter(w http.ResponseWriter, r *http.Request) (database.LiftCandidateFilter, bool) {
	q := r.URL.Query()
	filter := database.LiftCandidateFilter{}
	switch source := q.Get("source"); source {
	case "", "osm", "heuristic":
		filter.Source = source
	default:
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "source must be osm or heuristic", r.URL.Path)
		return filter, false
	}
	switch includeRejected := q.Get("include_rejected"); includeRejected {
	case "", "false":
		filter.IncludeRejected = false
	case "true":
		filter.IncludeRejected = true
	default:
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "include_rejected must be true or false", r.URL.Path)
		return filter, false
	}
	return filter, true
}

// ConfirmLiftCandidate marks a lift candidate as confirmed.
// POST /api/v1/activities/{id}/lift-candidates/{cid}/confirm
func (h *Handler) ConfirmLiftCandidate(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	activityID := chi.URLParam(r, "id")
	activity, err := h.DB.GetActivity(r.Context(), activityID)
	if err != nil {
		slog.Error("get activity for confirm lift candidate failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "", r.URL.Path)
		return
	}
	if activity == nil || activity.UserID != userID {
		httputil.WriteError(w, http.StatusNotFound, "Not Found", "activity not found", r.URL.Path)
		return
	}

	candID := chi.URLParam(r, "cid")
	if err := h.DB.ConfirmLiftCandidate(r.Context(), candID, activityID); err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "", r.URL.Path)
		return
	}
	h.reapplyRulesAfterLiftChange(r, activityID, userID)
	w.WriteHeader(http.StatusNoContent)
}

// RejectLiftCandidate deletes a lift candidate and recomputes activity lift stats.
// DELETE /api/v1/activities/{id}/lift-candidates/{cid}
func (h *Handler) RejectLiftCandidate(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	activityID := chi.URLParam(r, "id")
	activity, err := h.DB.GetActivity(r.Context(), activityID)
	if err != nil {
		slog.Error("get activity for reject lift candidate failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "", r.URL.Path)
		return
	}
	if activity == nil || activity.UserID != userID {
		httputil.WriteError(w, http.StatusNotFound, "Not Found", "activity not found", r.URL.Path)
		return
	}

	candID := chi.URLParam(r, "cid")
	if err := h.DB.RejectLiftCandidate(r.Context(), candID, activityID); err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "", r.URL.Path)
		return
	}
	h.reapplyRulesAfterLiftChange(r, activityID, userID)
	w.WriteHeader(http.StatusNoContent)
}
