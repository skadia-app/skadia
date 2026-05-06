package handler

// Phase 4b.B.4 — verify that the lift-candidate Confirm/Reject HTTP handlers
// re-fire the rules engine with trigger='on_enrichment_complete' so the new
// lift_detected SystemRule reaches activities whose lift_count changes via
// the heuristic pathway (Method B). The handler patch is the test target;
// without it Confirm leaves lift_count > 0 but the Lift tag never attaches.
//
// Tests run the actual HTTP handler via httptest + chi route mux + a
// fake-auth context that mirrors what middleware.AuthMiddleware sets.

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/skadia-app/skadia/services/api/internal/automation"
	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/middleware"
	"github.com/skadia-app/skadia/services/api/internal/model"
)

func liftRuleTriggerTestSetup(t *testing.T) (*Handler, *database.DB, string, string, string) {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("pgxpool.New: %v", err)
	}
	t.Cleanup(pool.Close)

	userID := uuid.NewString()
	if _, err := pool.Exec(ctx,
		`INSERT INTO users (id, username, email, email_verified, registration_source)
		 VALUES ($1, $2, $3, true, 'test')`,
		userID,
		"lifttrig_"+userID[:8],
		"lifttrig_"+userID[:8]+"@example.com",
	); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	t.Cleanup(func() {
		pool.Exec(context.Background(), `DELETE FROM activities WHERE user_id = $1`, userID)
		pool.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, userID)
	})

	db := &database.DB{Pool: pool}

	if err := automation.EnsureSystemArtifacts(ctx, db, userID); err != nil {
		t.Fatalf("EnsureSystemArtifacts: %v", err)
	}

	act, err := db.InsertActivity(ctx, userID, &model.ActivityData{
		SportType: "alpine_skiing",
		StartedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("InsertActivity: %v", err)
	}

	// One unconfirmed lift candidate. RecomputeLiftStats yields lift_count=0
	// initially; the candidate flips lift_count to 1 only after Confirm.
	var candID string
	startedAt := time.Now().UTC()
	if err := pool.QueryRow(ctx, `
		INSERT INTO activity_lift_candidates
			(activity_id, started_at, ended_at, duration_s, ascent_m, descent_m, distance_m, confidence, confirmed)
		VALUES ($1, $2, $3, 60, 100, 0, 200, 0.9, false)
		RETURNING id::text`,
		act.ID, startedAt, startedAt.Add(time.Minute),
	).Scan(&candID); err != nil {
		t.Fatalf("insert lift candidate: %v", err)
	}

	h := &Handler{DB: db}
	return h, db, userID, act.ID, candID
}

// callConfirmHandler invokes Handler.ConfirmLiftCandidate via chi mux + fake
// auth context, mirroring middleware.AuthMiddleware's UserIDKey injection.
func callConfirmHandler(t *testing.T, h *Handler, userID, activityID, candID string) *httptest.ResponseRecorder {
	t.Helper()
	r := chi.NewRouter()
	r.Post("/api/v1/activities/{id}/lift-candidates/{cid}/confirm", h.ConfirmLiftCandidate)
	req := httptest.NewRequest(http.MethodPost,
		"/api/v1/activities/"+activityID+"/lift-candidates/"+candID+"/confirm", nil)
	req = req.WithContext(context.WithValue(req.Context(), middleware.UserIDKey, userID))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

func callRejectHandler(t *testing.T, h *Handler, userID, activityID, candID string) *httptest.ResponseRecorder {
	t.Helper()
	r := chi.NewRouter()
	r.Delete("/api/v1/activities/{id}/lift-candidates/{cid}", h.RejectLiftCandidate)
	req := httptest.NewRequest(http.MethodDelete,
		"/api/v1/activities/"+activityID+"/lift-candidates/"+candID, nil)
	req = req.WithContext(context.WithValue(req.Context(), middleware.UserIDKey, userID))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

func TestConfirmLiftCandidate_TriggersLiftSystemRule(t *testing.T) {
	h, db, userID, activityID, candID := liftRuleTriggerTestSetup(t)

	if hasLiftTag(t, db, activityID) {
		t.Fatalf("setup: Lift tag should NOT be on activity before confirm")
	}

	rec := callConfirmHandler(t, h, userID, activityID, candID)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("ConfirmLiftCandidate HTTP status = %d, want 204; body=%s", rec.Code, rec.Body.String())
	}

	if !hasLiftTag(t, db, activityID) {
		t.Errorf("after Confirm, Lift tag should be attached to activity %s — handler did not re-fire rules", activityID)
	}
}

func TestRejectLiftCandidate_RemovesLiftSystemTagViaRetraction(t *testing.T) {
	h, db, userID, activityID, candID := liftRuleTriggerTestSetup(t)

	// Confirm first so the Lift tag is on the activity.
	if rec := callConfirmHandler(t, h, userID, activityID, candID); rec.Code != http.StatusNoContent {
		t.Fatalf("setup confirm: status %d, body=%s", rec.Code, rec.Body.String())
	}
	if !hasLiftTag(t, db, activityID) {
		t.Fatalf("setup: tag must be present after Confirm")
	}

	// Now Reject the (only) candidate. lift_count drops to 0 → has_lift=false →
	// retraction semantics on the system rule remove the Lift tag.
	rec := callRejectHandler(t, h, userID, activityID, candID)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("RejectLiftCandidate HTTP status = %d, want 204; body=%s", rec.Code, rec.Body.String())
	}

	if hasLiftTag(t, db, activityID) {
		t.Errorf("after Reject, Lift tag should be retracted — handler did not re-fire rules")
	}
}

// TestConfirmLiftCandidate_DoesNotRetractFrostTag — Phase 4b.B.5.1b.
// The handler patch passes trigger='on_enrichment_complete' specifically so
// Frost/Nacht/Rundtour rules (trigger='on_import') are NOT re-evaluated.
// Without the narrow trigger, confirming a lift candidate would retract the
// Frost tag whenever min_temperature_c is now >= 0.
func TestConfirmLiftCandidate_DoesNotRetractFrostTag(t *testing.T) {
	h, db, userID, activityID, candID := liftRuleTriggerTestSetup(t)
	ctx := context.Background()

	// Activity temperature ABOVE Frost threshold — the Frost rule, if
	// re-evaluated, would retract the tag.
	if _, err := db.Pool.Exec(ctx,
		`UPDATE activities SET min_temperature_c = 5 WHERE id = $1`, activityID,
	); err != nil {
		t.Fatalf("update min_temperature_c: %v", err)
	}

	// Manually attach the Frost tag (simulates a previous on_import evaluation
	// that fired when the temperature WAS below 0, since corrected).
	var frostTagID string
	if err := db.Pool.QueryRow(ctx,
		`SELECT id::text FROM tags WHERE user_id = $1 AND name = 'Frost' AND is_system = true`,
		userID,
	).Scan(&frostTagID); err != nil {
		t.Fatalf("lookup Frost tag: %v", err)
	}
	if _, err := db.Pool.Exec(ctx,
		`INSERT INTO activity_tags (activity_id, tag_id) VALUES ($1, $2)
		   ON CONFLICT DO NOTHING`,
		activityID, frostTagID,
	); err != nil {
		t.Fatalf("attach Frost tag: %v", err)
	}

	if rec := callConfirmHandler(t, h, userID, activityID, candID); rec.Code != http.StatusNoContent {
		t.Fatalf("Confirm: status %d, body=%s", rec.Code, rec.Body.String())
	}

	var frostStillThere bool
	if err := db.Pool.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM activity_tags WHERE activity_id = $1 AND tag_id = $2)`,
		activityID, frostTagID,
	).Scan(&frostStillThere); err != nil {
		t.Fatalf("check frost tag: %v", err)
	}
	if !frostStillThere {
		t.Errorf("Frost tag was retracted by the lift Confirm path — narrow trigger guard failed")
	}
}

// hasLiftTag returns true iff the activity has the system 'Lift' tag attached.
func hasLiftTag(t *testing.T, db *database.DB, activityID string) bool {
	t.Helper()
	ctx := context.Background()
	var present bool
	if err := db.Pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM activity_tags at
			JOIN tags tg ON tg.id = at.tag_id
			WHERE at.activity_id = $1 AND tg.name = 'Lift' AND tg.is_system = true
		)`, activityID,
	).Scan(&present); err != nil {
		t.Fatalf("query activity_tags: %v", err)
	}
	return present
}
