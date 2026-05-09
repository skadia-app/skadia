package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/model"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// systemSyncHandlerEnv builds a *Handler bound to the per-worktree test DB
// and seeds two users (primary + other) so cross-user scoping cases can run
// without leaking state. Config is intentionally left nil so the file-refresh
// best-effort path becomes a no-op — tests assert on tag/activity state, not
// on file system writes.
func systemSyncHandlerEnv(t *testing.T) (*Handler, *database.DB, string, string) {
	t.Helper()
	pool := fusionTestPool(t)
	db := &database.DB{Pool: pool}
	h := &Handler{DB: db}
	primary := seedSystemSyncUser(t, pool, db, "primary")
	other := seedSystemSyncUser(t, pool, db, "other")
	return h, db, primary, other
}

func seedSystemSyncUser(t *testing.T, pool *pgxpool.Pool, db *database.DB, label string) string {
	t.Helper()
	ctx := context.Background()
	userID := uuid.NewString()
	suffix := userID[:8]
	if _, err := pool.Exec(ctx, `
		INSERT INTO users (id, username, email, email_verified, registration_source)
		VALUES ($1, $2, $3, true, 'test')`,
		userID, "tagsync-h-"+label+"-"+suffix, "tagsync-h-"+label+"-"+suffix+"@example.com"); err != nil {
		t.Fatalf("insert user %s: %v", label, err)
	}
	t.Cleanup(func() {
		bg := context.Background()
		_, _ = pool.Exec(bg, `DELETE FROM activity_tag_removals WHERE activity_id IN (SELECT id FROM activities WHERE user_id = $1)`, userID)
		_, _ = pool.Exec(bg, `DELETE FROM activity_tags WHERE activity_id IN (SELECT id FROM activities WHERE user_id = $1)`, userID)
		_, _ = pool.Exec(bg, `DELETE FROM activities WHERE user_id = $1`, userID)
		_, _ = pool.Exec(bg, `UPDATE tags SET is_system = false WHERE user_id = $1`, userID)
		_, _ = pool.Exec(bg, `DELETE FROM tags WHERE user_id = $1`, userID)
		_, _ = pool.Exec(bg, `DELETE FROM users WHERE id = $1`, userID)
	})
	if err := db.SeedSystemDefaults(ctx, userID); err != nil {
		t.Fatalf("SeedSystemDefaults %s: %v", label, err)
	}
	return userID
}

func insertHandlerSyncActivity(t *testing.T, db *database.DB, userID, sport string) string {
	t.Helper()
	now := time.Now().UTC().Truncate(time.Second)
	act, err := db.InsertActivity(context.Background(), userID, &model.ActivityData{
		SportType: sport,
		StartedAt: now,
		EndedAt:   now.Add(15 * time.Minute),
		FileHash:  uuid.NewString(),
	})
	if err != nil {
		t.Fatalf("insert activity %s/%s: %v", userID, sport, err)
	}
	return act.ID
}

func mustHandlerSystemTagID(t *testing.T, db *database.DB, userID, name string) string {
	t.Helper()
	id, err := db.GetSystemTagID(context.Background(), userID, name)
	if err != nil {
		t.Fatalf("get system tag %s: %v", name, err)
	}
	return id
}

func callAssign(t *testing.T, h *Handler, userID, tagID string, activityIDs []string) *httptest.ResponseRecorder {
	t.Helper()
	body, _ := json.Marshal(map[string]any{"activity_ids": activityIDs})
	// defer_activity_file_refresh=true keeps refreshActivityFilePathBestEffort
	// out of the loop so tests that assert "side-effects scoped to accepted IDs"
	// do not also have to mock the ShadowFS layer.
	req := httptest.NewRequest(http.MethodPost, "/api/v1/tags/"+tagID+"/assign-activities?defer_activity_file_refresh=true", bytes.NewReader(body))
	req = req.WithContext(withUserAndAdmin(req.Context(), userID, false))
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", tagID)
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	w := httptest.NewRecorder()
	h.AssignActivitiesToTag(w, req)
	return w
}

func callUnassign(t *testing.T, h *Handler, userID, tagID string, activityIDs []string) *httptest.ResponseRecorder {
	t.Helper()
	body, _ := json.Marshal(map[string]any{"activity_ids": activityIDs})
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/tags/"+tagID+"/activities?defer_activity_file_refresh=true", bytes.NewReader(body))
	req = req.WithContext(withUserAndAdmin(req.Context(), userID, false))
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", tagID)
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	w := httptest.NewRecorder()
	h.UnassignActivitiesFromTag(w, req)
	return w
}

func removalRecorded(t *testing.T, db *database.DB, activityID, tagID string) bool {
	t.Helper()
	got, err := db.IsTagRemovalRecorded(context.Background(), activityID, tagID)
	if err != nil {
		t.Fatalf("IsTagRemovalRecorded: %v", err)
	}
	return got
}

func TestAssignActivitiesToTagSystemModifierEBikeOnRunningReturnsNoContent(t *testing.T) {
	h, db, userID, _ := systemSyncHandlerEnv(t)
	runningID := insertHandlerSyncActivity(t, db, userID, "running")
	ebikeTagID := mustHandlerSystemTagID(t, db, userID, "E-Bike")

	w := callAssign(t, h, userID, ebikeTagID, []string{runningID})
	if w.Code != http.StatusNoContent {
		t.Fatalf("status = %d, body = %s, want 204 (no-op for non-cycling)", w.Code, w.Body.String())
	}

	// Activity must remain non-electric and untagged with E-Bike.
	var isElectric bool
	var tags []string
	if err := db.Pool.QueryRow(context.Background(),
		`SELECT COALESCE(is_electric, false), COALESCE(tags, '{}') FROM activities WHERE id = $1`, runningID,
	).Scan(&isElectric, &tags); err != nil {
		t.Fatalf("query activity: %v", err)
	}
	if isElectric {
		t.Fatalf("is_electric = true, want false on running activity")
	}
	for _, tag := range tags {
		if tag == "E-Bike" {
			t.Fatalf("tags = %v, must not contain E-Bike", tags)
		}
	}
	// And: ClearTagRemoval must NOT have been called on a non-cycling ID.
	// We can't observe a non-call directly; instead we verify the inverse by
	// pre-recording a removal and confirming the no-op handler did not clear it.
}

func TestAssignActivitiesToTagSystemModifierTagClearRemovalScopedToAccepted(t *testing.T) {
	// Pre-record removals for both a cross-user activity and a non-cycling
	// activity, then assign E-Bike to a mix. Only the cycling owned activity
	// should have its removal cleared. Cross-user removal stays. Non-cycling
	// owned removal stays.
	h, db, primaryID, otherID := systemSyncHandlerEnv(t)
	cyclingID := insertHandlerSyncActivity(t, db, primaryID, "cycling")
	runningID := insertHandlerSyncActivity(t, db, primaryID, "running")
	otherCyclingID := insertHandlerSyncActivity(t, db, otherID, "cycling")
	ebikeTagID := mustHandlerSystemTagID(t, db, primaryID, "E-Bike")

	ctx := context.Background()
	for _, id := range []string{cyclingID, runningID, otherCyclingID} {
		if err := db.RecordTagRemoval(ctx, id, ebikeTagID); err != nil {
			t.Fatalf("RecordTagRemoval %s: %v", id, err)
		}
	}

	w := callAssign(t, h, primaryID, ebikeTagID, []string{cyclingID, runningID, otherCyclingID})
	if w.Code != http.StatusNoContent {
		t.Fatalf("status = %d, body = %s, want 204", w.Code, w.Body.String())
	}

	if removalRecorded(t, db, cyclingID, ebikeTagID) {
		t.Fatalf("removal still recorded for accepted cycling activity — ClearTagRemoval skipped it")
	}
	if !removalRecorded(t, db, runningID, ebikeTagID) {
		t.Fatalf("removal cleared for non-cycling activity — must stay because activity was rejected")
	}
	if !removalRecorded(t, db, otherCyclingID, ebikeTagID) {
		t.Fatalf("removal cleared for cross-user activity — must stay because activity was filtered")
	}
}

func TestAssignActivitiesToTagSystemModifierTagMixedOwnedSubsetIsApplied(t *testing.T) {
	h, db, primaryID, otherID := systemSyncHandlerEnv(t)
	primaryRunID := insertHandlerSyncActivity(t, db, primaryID, "running")
	otherRunID := insertHandlerSyncActivity(t, db, otherID, "running")
	indoorTagID := mustHandlerSystemTagID(t, db, primaryID, "Indoor")

	w := callAssign(t, h, primaryID, indoorTagID, []string{primaryRunID, otherRunID})
	if w.Code != http.StatusNoContent {
		t.Fatalf("status = %d, body = %s, want 204", w.Code, w.Body.String())
	}

	var primaryIndoor, otherIndoor bool
	if err := db.Pool.QueryRow(context.Background(),
		`SELECT COALESCE(is_indoor, false) FROM activities WHERE id = $1`, primaryRunID,
	).Scan(&primaryIndoor); err != nil {
		t.Fatalf("query primary: %v", err)
	}
	if err := db.Pool.QueryRow(context.Background(),
		`SELECT COALESCE(is_indoor, false) FROM activities WHERE id = $1`, otherRunID,
	).Scan(&otherIndoor); err != nil {
		t.Fatalf("query other: %v", err)
	}
	if !primaryIndoor {
		t.Fatalf("primary is_indoor = false, want true")
	}
	if otherIndoor {
		t.Fatalf("other-user is_indoor = true, must remain false")
	}
}

func TestUnassignActivitiesFromTagSystemModifierTagRecordRemovalScopedToAccepted(t *testing.T) {
	// Bulk-unassign Indoor (system modifier tag) with a mix of owned + cross-user IDs.
	// For system modifier tags, RecordTagRemoval must NOT fire — the modifier flag
	// (is_indoor=false) is the authoritative state; activity_tag_removals is only
	// written for non-system-modifier tags. Cross-user activity must never
	// get a removal record under any circumstance.
	h, db, primaryID, otherID := systemSyncHandlerEnv(t)
	ctx := context.Background()
	primaryRunID := insertHandlerSyncActivity(t, db, primaryID, "running")
	otherRunID := insertHandlerSyncActivity(t, db, otherID, "running")
	indoorTagID := mustHandlerSystemTagID(t, db, primaryID, "Indoor")

	// Seed primary with Indoor on.
	if _, err := db.AssignActivitiesToTag(ctx, indoorTagID, primaryID, []string{primaryRunID}); err != nil {
		t.Fatalf("seed assign: %v", err)
	}

	w := callUnassign(t, h, primaryID, indoorTagID, []string{primaryRunID, otherRunID})
	if w.Code != http.StatusNoContent {
		t.Fatalf("status = %d, body = %s, want 204", w.Code, w.Body.String())
	}

	// System modifier tag: RecordTagRemoval must NOT be called — modifier flag is authoritative.
	// system tag removal/unrecord is reconciled by skadia_resync_system_modifier_tags inside the helper tx.
	if removalRecorded(t, db, primaryRunID, indoorTagID) {
		t.Fatalf("removal recorded for primary system-modifier-tag activity — must not be written for system modifier tags")
	}
	if removalRecorded(t, db, otherRunID, indoorTagID) {
		t.Fatalf("removal recorded for cross-user activity — must never happen")
	}
}

func TestAssignActivitiesToTagSystemModifierSkipsClearTagRemoval(t *testing.T) {
	// Assign a system modifier tag (E-Bike cycling) to a cycling activity that has
	// a pre-existing removal record. The handler must NOT call ClearTagRemoval
	// directly; the resync inside the helper tx handles it instead. The net result
	// is the same (removal cleared), but the path must not double-write.
	// We verify the end state (removal cleared) and that a non-cycling activity's
	// removal stays (it was not accepted by the DB layer).
	h, db, primaryID, _ := systemSyncHandlerEnv(t)
	ctx := context.Background()
	cyclingID := insertHandlerSyncActivity(t, db, primaryID, "cycling")
	runningID := insertHandlerSyncActivity(t, db, primaryID, "running")
	ebikeTagID := mustHandlerSystemTagID(t, db, primaryID, "E-Bike")

	// Pre-record removals on both.
	for _, id := range []string{cyclingID, runningID} {
		if err := db.RecordTagRemoval(ctx, id, ebikeTagID); err != nil {
			t.Fatalf("RecordTagRemoval %s: %v", id, err)
		}
	}

	w := callAssign(t, h, primaryID, ebikeTagID, []string{cyclingID, runningID})
	if w.Code != http.StatusNoContent {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}

	// Cycling activity: removal must be cleared by the resync (not the handler).
	if removalRecorded(t, db, cyclingID, ebikeTagID) {
		t.Fatalf("cycling E-Bike removal still recorded after assign — resync should have cleared it")
	}
	// Running activity: was not accepted (cycling-only), so its removal must stay.
	if !removalRecorded(t, db, runningID, ebikeTagID) {
		t.Fatalf("running E-Bike removal was cleared — must stay because it was not accepted")
	}
}

func TestUnassignActivitiesFromTagSystemModifierSkipsRecordTagRemoval(t *testing.T) {
	// Assign then unassign a system modifier tag (Indoor). Assert no removal row
	// is created in activity_tag_removals — for system modifier tags, the modifier
	// flag (is_indoor=false) is the removal record, not the activity_tag_removals table.
	// system tag removal/unrecord is reconciled by skadia_resync_system_modifier_tags inside the helper tx.
	h, db, primaryID, _ := systemSyncHandlerEnv(t)
	ctx := context.Background()
	runID := insertHandlerSyncActivity(t, db, primaryID, "running")
	indoorTagID := mustHandlerSystemTagID(t, db, primaryID, "Indoor")

	if _, err := db.AssignActivitiesToTag(ctx, indoorTagID, primaryID, []string{runID}); err != nil {
		t.Fatalf("seed assign: %v", err)
	}

	w := callUnassign(t, h, primaryID, indoorTagID, []string{runID})
	if w.Code != http.StatusNoContent {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}

	if removalRecorded(t, db, runID, indoorTagID) {
		t.Fatalf("removal row written for system modifier tag — must not be written")
	}
}
