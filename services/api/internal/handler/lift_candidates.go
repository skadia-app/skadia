package handler

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/skadia-app/skadia/services/api/internal/automation"
	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/enrichment"
	"github.com/skadia-app/skadia/services/api/internal/enrichment/lifts"
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
		if errors.Is(err, pgx.ErrNoRows) {
			httputil.WriteProblem(w, http.StatusNotFound, "Cannot confirm",
				"lift candidate not found", "NOT_FOUND", r.URL.Path)
			return
		}
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
// database.UnconfirmLiftCandidate for the rationale. The handler probes the
// row first and returns a structured RFC7807 problem+json with a `code` field:
//   - 409 WRONG_SOURCE     — candidate is OSM-sourced
//   - 409 ALREADY_REJECTED — candidate has rejected_by_user=true
//   - 404 NOT_FOUND        — missing, wrong activity, or wrong owner (collapsed
//     to one status to avoid existence/owner leak)
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

	// F12: probe the candidate state before calling UnconfirmLiftCandidate so we
	// can return a structured error that distinguishes the four failure cases.
	// The probe is a plain SELECT — no lock needed since the subsequent UPDATE
	// re-checks the same predicates atomically inside UnconfirmLiftCandidate.
	var probeSource string
	var probeRejected bool
	var probeActivityID string
	probeErr := h.DB.Pool.QueryRow(r.Context(),
		`SELECT source, rejected_by_user, activity_id::text
		 FROM activity_lift_candidates WHERE id = $1`, candID,
	).Scan(&probeSource, &probeRejected, &probeActivityID)
	switch {
	case errors.Is(probeErr, pgx.ErrNoRows):
		// Row does not exist at all.
		httputil.WriteProblem(w, http.StatusNotFound, "Cannot unconfirm",
			"lift candidate not found", "NOT_FOUND", r.URL.Path)
		return
	case probeErr != nil:
		slog.Error("unconfirm lift candidate: probe failed", "error", probeErr)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "", r.URL.Path)
		return
	}
	// Collapse wrong-activity and wrong-owner into NOT_FOUND to avoid leaking
	// existence of candidates belonging to another user (AGENTS.md multi-user rule).
	if probeActivityID != activityID {
		httputil.WriteProblem(w, http.StatusNotFound, "Cannot unconfirm",
			"lift candidate not found", "NOT_FOUND", r.URL.Path)
		return
	}
	if probeRejected {
		httputil.WriteProblem(w, http.StatusConflict, "Cannot unconfirm",
			"candidate is already soft-rejected; restore it first", "ALREADY_REJECTED", r.URL.Path)
		return
	}
	if probeSource != "heuristic" {
		httputil.WriteProblem(w, http.StatusConflict, "Cannot unconfirm",
			"only heuristic candidates can be unconfirmed; use the reject path for OSM candidates",
			"WRONG_SOURCE", r.URL.Path)
		return
	}

	if err := h.DB.UnconfirmLiftCandidate(r.Context(), userID, candID, activityID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// Race: candidate was modified between probe and update — treat as NOT_FOUND.
			httputil.WriteProblem(w, http.StatusNotFound, "Cannot unconfirm",
				"lift candidate not found", "NOT_FOUND", r.URL.Path)
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
		if errors.Is(err, pgx.ErrNoRows) {
			httputil.WriteProblem(w, http.StatusNotFound, "Cannot reject",
				"lift candidate not found", "NOT_FOUND", r.URL.Path)
			return
		}
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
// from a pre-created background job (F2 fix: async
// dispatch prevents client-disconnect from leaving the activity in a
// half-reset state and avoids blocking long enrichment runs).
//
// F25 fix: non-lift-eligible sports (running, cycling indoor, etc.) are
// rejected with 400 NOT_LIFT_ELIGIBLE before any DELETE — the reset would be
// a no-op anyway, and returning early avoids unnecessary DB churn.
//
// The post-commit re-enrichment respects
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

	// F25: guard against wasting a DELETE+re-enrichment cycle on sports that
	// will never produce lift candidates. IsLiftRelevant mirrors the Python
	// detector's entry check (_is_lift_relevant_activity) 1:1.
	subType := ""
	if activity.SubType != nil {
		subType = *activity.SubType
	}
	if !lifts.IsLiftRelevant(activity.SportType, subType, activity.IsIndoor, activity.IsElectric, activity.IsVirtual) {
		httputil.WriteProblem(w, http.StatusBadRequest, "Cannot recalculate",
			"sport "+activity.SportType+" hat keine Lift-Erkennung",
			"NOT_LIFT_ELIGIBLE", r.URL.Path)
		return
	}

	// F1: 409 guard MUST run BEFORE the destructive RecalculateActivityLifts
	// (DELETE + clear lifts_acknowledged_at). Previously the check ran after,
	// which meant a duplicate request would wipe candidates the still-running
	// job had just produced — exactly the data-loss the 409 was meant to
	// prevent. Reorder: check, then recalc.
	if running, existingJobID, checkErr := h.DB.HasRunningLiftRecomputeJob(r.Context(), userID, activityID); checkErr != nil {
		slog.Warn("recalculate lifts: could not check for running job", "activity_id", activityID, "error", checkErr)
	} else if running && existingJobID != nil {
		httputil.WriteProblemWithJob(w, http.StatusConflict,
			"Re-Detect job already running",
			"a lift_recompute job for this activity is already running",
			"LIFT_RECOMPUTE_RUNNING", *existingJobID, r.URL.Path)
		return
	}

	// Create the tracked job before the destructive reset. If the INSERT cannot
	// be guaranteed, the existing candidates and lift aggregates must remain
	// untouched so the user can retry without data loss.
	jobID, jobErr := h.DB.InsertBackgroundJob(r.Context(), userID, "lift_recompute", &activityID, nil, 1)
	if jobErr != nil {
		if isLiftRecomputeUniqueViolation(jobErr) {
			// Race-loser path: the lift-recompute unique index rejected the
			// insert because another request slipped in between our
			// HasRunningLiftRecomputeJob check and this INSERT.
			if running, existingJobID, _ := h.DB.HasRunningLiftRecomputeJob(r.Context(), userID, activityID); running && existingJobID != nil {
				httputil.WriteProblemWithJob(w, http.StatusConflict,
					"Re-Detect job already running",
					"a lift_recompute job for this activity is already running",
					"LIFT_RECOMPUTE_RUNNING", *existingJobID, r.URL.Path)
				return
			}
			// Unique violation but no row visible — extremely unlikely (would
			// require the conflicting job to complete in the same instant).
			// Fall through to 500 so the caller retries.
		}
		slog.Error("recalculate lifts: failed to insert background job",
			"activity_id", activityID, "error", jobErr)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "", r.URL.Path)
		return
	}

	jobCtx := context.Background()
	if err := h.DB.RecalculateActivityLifts(jobCtx, userID, activityID); err != nil {
		_ = h.DB.CompleteBackgroundJob(jobCtx, jobID, true, err.Error())
		if errors.Is(err, pgx.ErrNoRows) {
			httputil.WriteError(w, http.StatusNotFound, "Not Found", "activity not found", r.URL.Path)
			return
		}
		slog.Error("recalculate activity lifts failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "", r.URL.Path)
		return
	}

	// Capture copies for the goroutine (request values must not be read after
	// ServeHTTP returns — the goroutine must use context.Background()).
	capturedUserID := userID
	capturedActivityID := activityID
	capturedJobID := jobID

	go func() {
		ctx := context.Background()
		if err := h.dispatchLiftReEnrichmentBackground(ctx, capturedUserID, capturedActivityID); err != nil {
			slog.Warn("recalculate lifts: background re-enrichment failed (non-fatal)",
				"activity_id", capturedActivityID, "error", err)
			_ = h.DB.CompleteBackgroundJob(ctx, capturedJobID, true, err.Error())
			return
		}
		_ = h.DB.UpdateBackgroundJobProgress(ctx, capturedJobID, 1)
		_ = h.DB.CompleteBackgroundJob(ctx, capturedJobID, false, "")
	}()

	w.WriteHeader(http.StatusNoContent)
}

// isLiftRecomputeUniqueViolation reports whether err is a Postgres unique-
// violation on the lift_recompute background_jobs guard.
func isLiftRecomputeUniqueViolation(err error) bool {
	pgErr, ok := errors.AsType[*pgconn.PgError](err)
	if !ok {
		return false
	}
	return pgErr.Code == "23505" && pgErr.ConstraintName == "idx_bg_jobs_one_lift_recompute"
}

// dispatchLiftReEnrichmentBackground fires lift-only stream enrichment for the
// activity. Errors are returned so the background goroutine can update job
// status. No-op when EnableLiftDetection is false for the user.
func (h *Handler) dispatchLiftReEnrichmentBackground(ctx context.Context, userID, activityID string) error {
	settings, err := h.DB.GetEnrichmentSettings(ctx, userID)
	if err != nil || settings == nil {
		if err == nil {
			err = errors.New("enrichment settings not found")
		}
		return fmt.Errorf("recalculate lifts: load enrichment settings: %w", err)
	}
	if !settings.EnableLiftDetection {
		slog.Info("recalculate lifts: lift detection disabled for user; skipping re-enrichment",
			"activity_id", activityID, "user_id", userID)
		return nil
	}
	streamSettings := enrichment.StreamEnrichmentSettings{
		EnableLiftDetection: true,
		FailOnModuleError:   true,
	}
	return enrichment.RunStreamEnrichment(ctx, h.DB, userID, activityID, streamSettings)
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
