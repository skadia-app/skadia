package handler

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"

	"github.com/skadia-app/skadia/services/api/internal/automation"
	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/enrichment"
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
	if err := h.DB.ConfirmLiftCandidate(r.Context(), userID, candID, activityID); err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "", r.URL.Path)
		return
	}
	h.reapplyRulesAfterLiftChange(r, activityID, userID)
	w.WriteHeader(http.StatusNoContent)
}

// UnconfirmLiftCandidate reverts a heuristic candidate's confirmed flag to false.
// PATCH /api/v1/activities/{id}/lift-candidates/{cid}/unconfirm
//
// Restricted to source='heuristic' AND NOT rejected_by_user — see
// database.UnconfirmLiftCandidate for the rationale. A 0-row UPDATE (wrong
// source, already-rejected row, wrong owner, wrong activity) returns 404.
func (h *Handler) UnconfirmLiftCandidate(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	activityID := chi.URLParam(r, "id")
	activity, err := h.DB.GetActivity(r.Context(), activityID)
	if err != nil {
		slog.Error("get activity for unconfirm lift candidate failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "", r.URL.Path)
		return
	}
	if activity == nil || activity.UserID != userID {
		httputil.WriteError(w, http.StatusNotFound, "Not Found", "activity not found", r.URL.Path)
		return
	}

	candID := chi.URLParam(r, "cid")
	if err := h.DB.UnconfirmLiftCandidate(r.Context(), userID, candID, activityID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			httputil.WriteError(w, http.StatusNotFound, "Not Found",
				"candidate not found, not heuristic, or already rejected", r.URL.Path)
			return
		}
		slog.Error("unconfirm lift candidate failed", "error", err)
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
	if err := h.DB.RejectLiftCandidate(r.Context(), userID, candID, activityID); err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "", r.URL.Path)
		return
	}
	h.reapplyRulesAfterLiftChange(r, activityID, userID)
	w.WriteHeader(http.StatusNoContent)
}

// ConfirmAllLiftCandidates handles POST /api/v1/activities/{id}/lifts/confirm-all.
//
// Bulk-confirms every unconfirmed, non-rejected heuristic candidate of the
// activity, sets lifts_acknowledged_at to NOW(), and recomputes lift_*
// aggregates atomically inside the DB layer. Method-A (source='osm') rows
// are not touched. Idempotent.
//
// Phase 6.3.1.2 bullet 1.
func (h *Handler) ConfirmAllLiftCandidates(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	activityID := chi.URLParam(r, "id")
	activity, err := h.DB.GetActivity(r.Context(), activityID)
	if err != nil {
		slog.Error("get activity for confirm-all lift candidates failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "", r.URL.Path)
		return
	}
	if activity == nil || activity.UserID != userID {
		httputil.WriteError(w, http.StatusNotFound, "Not Found", "activity not found", r.URL.Path)
		return
	}

	if err := h.DB.ConfirmAllLiftCandidates(r.Context(), userID, activityID); err != nil {
		// 0-row UPDATE on activities (race against deletion) → ErrNoRows → 404.
		if errors.Is(err, pgx.ErrNoRows) {
			httputil.WriteError(w, http.StatusNotFound, "Not Found", "activity not found", r.URL.Path)
			return
		}
		slog.Error("confirm-all lift candidates failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "", r.URL.Path)
		return
	}
	h.reapplyRulesAfterLiftChange(r, activityID, userID)
	w.WriteHeader(http.StatusNoContent)
}

// RejectAllLiftCandidates handles POST /api/v1/activities/{id}/lifts/reject-all.
//
// Bulk-soft-rejects every unconfirmed, non-rejected heuristic candidate of
// the activity, sets lifts_acknowledged_at to NOW(), and recomputes
// aggregates atomically. Method-A rows are not touched. Idempotent.
//
// Phase 6.3.1.2 bullet 2.
func (h *Handler) RejectAllLiftCandidates(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	activityID := chi.URLParam(r, "id")
	activity, err := h.DB.GetActivity(r.Context(), activityID)
	if err != nil {
		slog.Error("get activity for reject-all lift candidates failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "", r.URL.Path)
		return
	}
	if activity == nil || activity.UserID != userID {
		httputil.WriteError(w, http.StatusNotFound, "Not Found", "activity not found", r.URL.Path)
		return
	}

	if err := h.DB.RejectAllLiftCandidates(r.Context(), userID, activityID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			httputil.WriteError(w, http.StatusNotFound, "Not Found", "activity not found", r.URL.Path)
			return
		}
		slog.Error("reject-all lift candidates failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "", r.URL.Path)
		return
	}
	h.reapplyRulesAfterLiftChange(r, activityID, userID)
	w.WriteHeader(http.StatusNoContent)
}

// AcknowledgeLifts handles POST /api/v1/activities/{id}/lifts/acknowledge.
//
// Sets activities.lifts_acknowledged_at = NOW() without touching candidates.
// The X-close path of the review modal calls this so the modal does not
// auto-trigger again on the next visit (User-Quote 2026-05-08 #6).
//
// Phase 6.3.1.2 bullet 3.
func (h *Handler) AcknowledgeLifts(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	activityID := chi.URLParam(r, "id")
	activity, err := h.DB.GetActivity(r.Context(), activityID)
	if err != nil {
		slog.Error("get activity for acknowledge lifts failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "", r.URL.Path)
		return
	}
	if activity == nil || activity.UserID != userID {
		httputil.WriteError(w, http.StatusNotFound, "Not Found", "activity not found", r.URL.Path)
		return
	}

	if err := h.DB.AcknowledgeLifts(r.Context(), userID, activityID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			httputil.WriteError(w, http.StatusNotFound, "Not Found", "activity not found", r.URL.Path)
			return
		}
		slog.Error("acknowledge lifts failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "", r.URL.Path)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// RecalculateActivityLifts handles
// POST /api/v1/activities/{id}/lifts/recalculate-all.
//
// Hard-resets lift detection state for one activity (User-Quote 2026-05-08
// #5): DELETE all candidates (including rejected), DELETE coupled lift
// segment_efforts, clear lifts_acknowledged_at, then trigger re-enrichment
// AFTER the transaction commits. The post-commit re-enrichment respects
// user_enrichment_settings.enable_lift_detection — a user who has lift
// detection disabled gets the reset only, no re-detection.
//
// Disjoint from the user-wide POST /api/v1/lifts/recalculate-all
// (RecalculateLifts) which only clears the rejected_by_user flag across all
// of the user's activities and does not delete or re-enrich.
//
// Phase 6.3.1.2 bullet 4.
func (h *Handler) RecalculateActivityLifts(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	activityID := chi.URLParam(r, "id")
	activity, err := h.DB.GetActivity(r.Context(), activityID)
	if err != nil {
		slog.Error("get activity for recalculate lifts failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "", r.URL.Path)
		return
	}
	if activity == nil || activity.UserID != userID {
		httputil.WriteError(w, http.StatusNotFound, "Not Found", "activity not found", r.URL.Path)
		return
	}

	if err := h.DB.RecalculateActivityLifts(r.Context(), userID, activityID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			httputil.WriteError(w, http.StatusNotFound, "Not Found", "activity not found", r.URL.Path)
			return
		}
		slog.Error("recalculate activity lifts failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "", r.URL.Path)
		return
	}

	// Re-enrichment runs AFTER commit to avoid deadlock against the
	// per-activity lift advisory lock taken by enrichStreamLifts. The user
	// has explicitly asked for a recalculate, so we run the lift module
	// (gated by their per-user enable_lift_detection toggle).
	h.triggerLiftReEnrichmentAfterRecalculate(r.Context(), userID, activityID)

	w.WriteHeader(http.StatusNoContent)
}

// triggerLiftReEnrichmentAfterRecalculate fires lift-only stream enrichment
// for the activity. Errors are logged-and-swallowed because the 204
// response is already correct (the DELETE/reset committed); re-enrichment
// is a best-effort secondary effect that can also be retried via the
// user's normal recompute path. No-op when EnableLiftDetection is false.
func (h *Handler) triggerLiftReEnrichmentAfterRecalculate(ctx context.Context, userID, activityID string) {
	settings, err := h.DB.GetEnrichmentSettings(ctx, userID)
	if err != nil || settings == nil {
		slog.Warn("recalculate lifts: load enrichment settings failed; skipping re-enrichment",
			"activity_id", activityID, "error", err)
		return
	}
	if !settings.EnableLiftDetection {
		slog.Info("recalculate lifts: lift detection disabled for user; skipping re-enrichment",
			"activity_id", activityID, "user_id", userID)
		return
	}
	streamSettings := enrichment.StreamEnrichmentSettings{
		EnableLiftDetection: true,
	}
	if err := enrichment.RunStreamEnrichment(ctx, h.DB, userID, activityID, streamSettings); err != nil {
		slog.Warn("recalculate lifts: re-enrichment failed (non-fatal)",
			"activity_id", activityID, "error", err)
	}
}

// RestoreLiftCandidate handles POST /api/v1/lift-candidates/{id}/restore.
//
// Clears the rejected_by_user flag on a soft-rejected candidate; the source
// determines the post-restore confirmed state:
//   - source='heuristic' → confirmed=false (user must re-act).
//   - source='osm'       → confirmed=true (auto-confirmed on the OSM match).
//
// Owner check is enforced inside the DB layer via a JOIN to activities.
// 0-row UPDATE (wrong owner, already-non-rejected, missing row) → 404.
//
// Adversarial-Review-Finding #19. Phase 6.3.1.2 bullet 5.
func (h *Handler) RestoreLiftCandidate(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	candID := chi.URLParam(r, "id")
	activityID, err := h.DB.RestoreLiftCandidate(r.Context(), userID, candID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			httputil.WriteError(w, http.StatusNotFound, "Not Found",
				"candidate not found, not rejected, or owned by another user", r.URL.Path)
			return
		}
		slog.Error("restore lift candidate failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "", r.URL.Path)
		return
	}
	h.reapplyRulesAfterLiftChange(r, activityID, userID)
	w.WriteHeader(http.StatusNoContent)
}
