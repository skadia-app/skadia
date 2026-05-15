package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/middleware"
)

// liftDetectionConfigSetup returns a Handler and DB backed by the test pool.
func liftDetectionConfigSetup(t *testing.T) (*Handler, *database.DB) {
	t.Helper()
	pool := fusionTestPool(t)
	db := &database.DB{Pool: pool}
	return &Handler{DB: db}, db
}

func callGetDetectionConfig(t *testing.T, h *Handler, userID, activityID string) *httptest.ResponseRecorder {
	t.Helper()
	r := chi.NewRouter()
	r.Get("/api/v1/activities/{id}/lifts/detection-config", h.GetLiftDetectionConfig)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/activities/"+activityID+"/lifts/detection-config", nil)
	req = req.WithContext(context.WithValue(req.Context(), middleware.UserIDKey, userID))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

func callPutDetectionConfig(t *testing.T, h *Handler, userID, activityID, body string) *httptest.ResponseRecorder {
	t.Helper()
	r := chi.NewRouter()
	r.Put("/api/v1/activities/{id}/lifts/detection-config", h.PutLiftDetectionConfig)
	req := httptest.NewRequest(http.MethodPut, "/api/v1/activities/"+activityID+"/lifts/detection-config", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(context.WithValue(req.Context(), middleware.UserIDKey, userID))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

func callDeleteDetectionConfig(t *testing.T, h *Handler, userID, activityID string) *httptest.ResponseRecorder {
	t.Helper()
	r := chi.NewRouter()
	r.Delete("/api/v1/activities/{id}/lifts/detection-config", h.DeleteLiftDetectionConfig)
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/activities/"+activityID+"/lifts/detection-config", nil)
	req = req.WithContext(context.WithValue(req.Context(), middleware.UserIDKey, userID))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

type detectionConfigResponse struct {
	Effective     struct{ OSM, Heuristic bool }  `json:"effective"`
	UserSetting   struct{ OSM, Heuristic bool }  `json:"user_setting"`
	Override      struct{ OSM, Heuristic *bool } `json:"override"`
	MasterEnabled bool                           `json:"master_enabled"`
}

// insertRunningLiftJob inserts a background_jobs row with status='running'
// for (userID, activityID, job_type='lift_recompute'). Returns the job ID.
func insertRunningLiftJob(t *testing.T, db *database.DB, userID, activityID string) string {
	t.Helper()
	id := uuid.NewString()
	_, err := db.Pool.Exec(context.Background(), `
		INSERT INTO background_jobs (id, user_id, job_type, entity_id, status, progress_total)
		VALUES ($1, $2, 'lift_recompute', $3, 'running', 1)
	`, id, userID, activityID)
	if err != nil {
		t.Fatalf("insertRunningLiftJob: %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.Pool.Exec(context.Background(), "DELETE FROM background_jobs WHERE id = $1", id)
	})
	return id
}

// markJobCompleted sets a background_jobs row to status='completed' so it is
// no longer treated as a blocking running job.
func markJobCompleted(t *testing.T, db *database.DB, jobID string) {
	t.Helper()
	_, err := db.Pool.Exec(context.Background(),
		`UPDATE background_jobs SET status = 'completed', finished_at = NOW() WHERE id = $1`, jobID)
	if err != nil {
		t.Fatalf("markJobCompleted: %v", err)
	}
}

// --- Happy-path tests ---

// TestLiftDetectionConfig_GetDefaults verifies that GET returns the default
// shape: effective/user_setting match user enrichment defaults, both overrides nil.
func TestLiftDetectionConfig_GetDefaults(t *testing.T) {
	h, db := liftDetectionConfigSetup(t)
	userID := insertTestUser(t, db.Pool)
	activityID := insertMinimalActivity(t, db, userID)

	rec := callGetDetectionConfig(t, h, userID, activityID)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	var resp detectionConfigResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v; body=%s", err, rec.Body.String())
	}
	// Defaults: master on, user_setting both true, no override.
	if !resp.MasterEnabled {
		t.Error("master_enabled=false, want true (default)")
	}
	if !resp.UserSetting.OSM {
		t.Error("user_setting.osm=false, want true (default)")
	}
	if !resp.UserSetting.Heuristic {
		t.Error("user_setting.heuristic=false, want true (default)")
	}
	if resp.Override.OSM != nil {
		t.Errorf("override.osm=%v, want nil", resp.Override.OSM)
	}
	if resp.Override.Heuristic != nil {
		t.Errorf("override.heuristic=%v, want nil", resp.Override.Heuristic)
	}
	// OSM snap needs runtime artifacts in addition to the user setting.
	if resp.Effective.OSM {
		t.Errorf("effective.osm=true without OSM artifacts, want false")
	}
}

// TestLiftDetectionConfig_PutThenGet verifies PUT persists overrides and GET
// reflects them in override and effective fields.
func TestLiftDetectionConfig_PutThenGet(t *testing.T) {
	h, db := liftDetectionConfigSetup(t)
	userID := insertTestUser(t, db.Pool)
	activityID := insertMinimalActivity(t, db, userID)

	rec := callPutDetectionConfig(t, h, userID, activityID, `{"osm":false,"heuristic":false}`)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("PUT status=%d, want 204; body=%s", rec.Code, rec.Body.String())
	}

	rec = callGetDetectionConfig(t, h, userID, activityID)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET status=%d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	var resp detectionConfigResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Override.OSM == nil || *resp.Override.OSM != false {
		t.Errorf("override.osm=%v, want false", resp.Override.OSM)
	}
	if resp.Override.Heuristic == nil || *resp.Override.Heuristic != false {
		t.Errorf("override.heuristic=%v, want false", resp.Override.Heuristic)
	}
	if resp.Effective.OSM {
		t.Errorf("effective.osm=true, want false")
	}
	if resp.Effective.Heuristic {
		t.Errorf("effective.heuristic=true, want false")
	}
}

func TestFullLiftDetectionConfigRejectsUnavailableOSMSnapEnableAndCoercesEffective(t *testing.T) {
	h, db := liftDetectionConfigSetup(t)
	h.Config = &HandlerConfig{DeploymentProfile: "full"}
	userID := insertTestUser(t, db.Pool)
	activityID := insertMinimalActivity(t, db, userID)

	rec := callPutDetectionConfig(t, h, userID, activityID, `{"osm":true,"heuristic":true}`)
	assertFeatureUnavailable(t, rec)
	if body := rec.Body.String(); !strings.Contains(body, "missing_artifacts") {
		t.Fatalf("response body %q missing missing_artifacts reason", body)
	}

	trueValue := true
	rows, err := db.SetLiftDetectionOverride(context.Background(), userID, activityID, &trueValue, &trueValue)
	if err != nil {
		t.Fatalf("SetLiftDetectionOverride historical fixture: %v", err)
	}
	if rows != 1 {
		t.Fatalf("SetLiftDetectionOverride rows=%d, want 1", rows)
	}

	rec = callGetDetectionConfig(t, h, userID, activityID)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET status=%d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	var resp detectionConfigResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Override.OSM == nil || *resp.Override.OSM != true {
		t.Fatalf("historical override.osm = %v, want raw true preserved", resp.Override.OSM)
	}
	if resp.Effective.OSM {
		t.Fatalf("effective.osm = true without OSM artifacts, want coerced false")
	}
	if !resp.Effective.Heuristic {
		t.Fatalf("effective.heuristic = false, want true")
	}
}

func TestCoreLiftDetectionConfigRejectsOSMSnapEnableAndCoercesEffective(t *testing.T) {
	h, db := liftDetectionConfigSetup(t)
	h.Config = &HandlerConfig{DeploymentProfile: "core"}
	userID := insertTestUser(t, db.Pool)
	activityID := insertMinimalActivity(t, db, userID)

	rec := callPutDetectionConfig(t, h, userID, activityID, `{"osm":true,"heuristic":true}`)
	assertFeatureUnavailable(t, rec)

	trueValue := true
	rows, err := db.SetLiftDetectionOverride(context.Background(), userID, activityID, &trueValue, &trueValue)
	if err != nil {
		t.Fatalf("SetLiftDetectionOverride historical fixture: %v", err)
	}
	if rows != 1 {
		t.Fatalf("SetLiftDetectionOverride rows=%d, want 1", rows)
	}

	rec = callGetDetectionConfig(t, h, userID, activityID)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET status=%d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	var resp detectionConfigResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Override.OSM == nil || *resp.Override.OSM != true {
		t.Fatalf("historical override.osm = %v, want raw true preserved", resp.Override.OSM)
	}
	if resp.Effective.OSM {
		t.Fatalf("effective.osm = true in Core, want coerced false")
	}
	if !resp.Effective.Heuristic {
		t.Fatalf("effective.heuristic = false, want true")
	}
}

// TestLiftDetectionConfig_PutIdempotent verifies that PUT twice with the same
// body both return 204.
func TestLiftDetectionConfig_PutIdempotent(t *testing.T) {
	h, db := liftDetectionConfigSetup(t)
	userID := insertTestUser(t, db.Pool)
	activityID := insertMinimalActivity(t, db, userID)

	body := `{"osm":false,"heuristic":true}`
	for i := range 2 {
		rec := callPutDetectionConfig(t, h, userID, activityID, body)
		if rec.Code != http.StatusNoContent {
			t.Fatalf("PUT[%d] status=%d, want 204; body=%s", i, rec.Code, rec.Body.String())
		}
	}
}

// TestLiftDetectionConfig_DeleteResetsOverride verifies DELETE clears overrides
// and a subsequent GET shows nil pointers.
func TestLiftDetectionConfig_DeleteResetsOverride(t *testing.T) {
	h, db := liftDetectionConfigSetup(t)
	userID := insertTestUser(t, db.Pool)
	activityID := insertMinimalActivity(t, db, userID)

	// Set an override first.
	callPutDetectionConfig(t, h, userID, activityID, `{"osm":false,"heuristic":false}`)

	rec := callDeleteDetectionConfig(t, h, userID, activityID)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("DELETE status=%d, want 204; body=%s", rec.Code, rec.Body.String())
	}

	rec = callGetDetectionConfig(t, h, userID, activityID)
	var resp detectionConfigResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Override.OSM != nil {
		t.Errorf("after DELETE override.osm=%v, want nil", resp.Override.OSM)
	}
	if resp.Override.Heuristic != nil {
		t.Errorf("after DELETE override.heuristic=%v, want nil", resp.Override.Heuristic)
	}
}

// TestLiftDetectionConfig_DeleteIdempotent verifies DELETE on already-null
// overrides still returns 204.
func TestLiftDetectionConfig_DeleteIdempotent(t *testing.T) {
	h, db := liftDetectionConfigSetup(t)
	userID := insertTestUser(t, db.Pool)
	activityID := insertMinimalActivity(t, db, userID)

	for i := range 2 {
		rec := callDeleteDetectionConfig(t, h, userID, activityID)
		if rec.Code != http.StatusNoContent {
			t.Fatalf("DELETE[%d] status=%d, want 204; body=%s", i, rec.Code, rec.Body.String())
		}
	}
}

// --- Master-off semantics ---

// TestLiftDetectionConfig_MasterOff verifies that when enable_lift_detection=false
// the effective result is {false,false} even when override is {true,true}.
func TestLiftDetectionConfig_MasterOff(t *testing.T) {
	h, db := liftDetectionConfigSetup(t)
	userID := insertTestUser(t, db.Pool)
	activityID := insertMinimalActivity(t, db, userID)

	// Disable master toggle.
	_, err := db.Pool.Exec(context.Background(), `
		INSERT INTO user_enrichment_settings (user_id, enable_lift_detection)
		VALUES ($1, false)
		ON CONFLICT (user_id) DO UPDATE SET enable_lift_detection = false
	`, userID)
	if err != nil {
		t.Fatalf("disable master toggle: %v", err)
	}

	// Set override both true.
	callPutDetectionConfig(t, h, userID, activityID, `{"osm":true,"heuristic":true}`)

	rec := callGetDetectionConfig(t, h, userID, activityID)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET status=%d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	var resp detectionConfigResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.MasterEnabled {
		t.Error("master_enabled=true, want false")
	}
	// Even though override is {true,true}, effective must be {false,false}.
	if resp.Effective.OSM {
		t.Error("effective.osm=true with master off, want false")
	}
	if resp.Effective.Heuristic {
		t.Error("effective.heuristic=true with master off, want false")
	}
}

// --- Cross-user ownership ---

// TestLiftDetectionConfig_CrossUserGet verifies User B cannot GET User A's config.
func TestLiftDetectionConfig_CrossUserGet(t *testing.T) {
	h, db := liftDetectionConfigSetup(t)
	userA := insertTestUser(t, db.Pool)
	userB := insertTestUser(t, db.Pool)
	activityA := insertMinimalActivity(t, db, userA)

	rec := callGetDetectionConfig(t, h, userB, activityA)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("cross-user GET status=%d, want 404; body=%s", rec.Code, rec.Body.String())
	}
}

// TestLiftDetectionConfig_CrossUserPut verifies User B cannot PUT User A's config.
func TestLiftDetectionConfig_CrossUserPut(t *testing.T) {
	h, db := liftDetectionConfigSetup(t)
	userA := insertTestUser(t, db.Pool)
	userB := insertTestUser(t, db.Pool)
	activityA := insertMinimalActivity(t, db, userA)

	rec := callPutDetectionConfig(t, h, userB, activityA, `{"osm":false}`)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("cross-user PUT status=%d, want 404; body=%s", rec.Code, rec.Body.String())
	}
}

// TestLiftDetectionConfig_CrossUserDelete verifies User B cannot DELETE User A's config.
func TestLiftDetectionConfig_CrossUserDelete(t *testing.T) {
	h, db := liftDetectionConfigSetup(t)
	userA := insertTestUser(t, db.Pool)
	userB := insertTestUser(t, db.Pool)
	activityA := insertMinimalActivity(t, db, userA)

	rec := callDeleteDetectionConfig(t, h, userB, activityA)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("cross-user DELETE status=%d, want 404; body=%s", rec.Code, rec.Body.String())
	}
}

// TestLiftDetectionConfig_NotFound verifies non-existent activity returns 404.
func TestLiftDetectionConfig_NotFound(t *testing.T) {
	h, db := liftDetectionConfigSetup(t)
	userID := insertTestUser(t, db.Pool)
	nonExistent := uuid.NewString()

	rec := callGetDetectionConfig(t, h, userID, nonExistent)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("non-existent GET status=%d, want 404; body=%s", rec.Code, rec.Body.String())
	}
}

// --- 409 Race tests ---

// TestRecalculateActivityLifts_409WhenRunning verifies POST returns 409 when a
// lift_recompute job is already running for the same (user, activity).
func TestRecalculateActivityLifts_409WhenRunning(t *testing.T) {
	h, db := liftDetectionConfigSetup(t)
	userID := insertTestUser(t, db.Pool)
	activityID := insertMinimalActivity(t, db, userID)

	// Insert a running job for this (user, activity).
	jobID := insertRunningLiftJob(t, db, userID, activityID)

	r := chi.NewRouter()
	r.Post("/api/v1/activities/{id}/lifts/recalculate-all", h.RecalculateActivityLifts)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/activities/"+activityID+"/lifts/recalculate-all", nil)
	req = req.WithContext(context.WithValue(req.Context(), middleware.UserIDKey, userID))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("status=%d, want 409; body=%s", rec.Code, rec.Body.String())
	}
	// F10: 409 must use the RFC 7807/9457 ProblemDetail shape with a
	// machine-readable code and a job_id extension member.
	if ct := rec.Header().Get("Content-Type"); ct != "application/problem+json" {
		t.Errorf("Content-Type=%q, want application/problem+json", ct)
	}
	var body struct {
		Type   string `json:"type"`
		Title  string `json:"title"`
		Status int    `json:"status"`
		Detail string `json:"detail"`
		Code   string `json:"code"`
		JobID  string `json:"job_id"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode 409 body: %v; raw=%s", err, rec.Body.String())
	}
	if body.Code != "LIFT_RECOMPUTE_RUNNING" {
		t.Errorf("code=%q, want LIFT_RECOMPUTE_RUNNING", body.Code)
	}
	if body.JobID != jobID {
		t.Errorf("job_id=%q, want %q", body.JobID, jobID)
	}
	if body.Title == "" {
		t.Error("title field is empty in 409 response")
	}
	if body.Status != http.StatusConflict {
		t.Errorf("status field=%d, want %d", body.Status, http.StatusConflict)
	}
}

// TestRecalculateActivityLifts_NoConflictAfterCompletion verifies POST returns
// non-409 once the blocking job is marked completed.
func TestRecalculateActivityLifts_NoConflictAfterCompletion(t *testing.T) {
	h, db := liftDetectionConfigSetup(t)
	userID := insertTestUser(t, db.Pool)
	activityID := insertMinimalActivity(t, db, userID)

	jobID := insertRunningLiftJob(t, db, userID, activityID)
	markJobCompleted(t, db, jobID)

	r := chi.NewRouter()
	r.Post("/api/v1/activities/{id}/lifts/recalculate-all", h.RecalculateActivityLifts)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/activities/"+activityID+"/lifts/recalculate-all", nil)
	req = req.WithContext(context.WithValue(req.Context(), middleware.UserIDKey, userID))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code == http.StatusConflict {
		t.Fatalf("got 409 after job completed, want non-409; body=%s", rec.Body.String())
	}
}

func TestRecalculateActivityLifts_BothModesOffCompletesWithoutBackgroundReenrichment(t *testing.T) {
	h, db := liftDetectionConfigSetup(t)
	ctx := context.Background()
	userID := insertTestUser(t, db.Pool)
	activityID := insertMinimalActivity(t, db, userID)

	now := time.Now().UTC().Truncate(time.Second)
	candID := insertHeuristicUnconfirmed(t, db, activityID, now)
	_, err := db.Pool.Exec(ctx, `
		INSERT INTO activity_streams (activity_id, time, lat, lng, altitude_m)
		VALUES ($1, $2, 0, 0, 10)
	`, activityID, now)
	if err != nil {
		t.Fatalf("insert synthetic stream: %v", err)
	}

	off := false
	rows, err := db.SetLiftDetectionOverride(ctx, userID, activityID, &off, &off)
	if err != nil {
		t.Fatalf("SetLiftDetectionOverride: %v", err)
	}
	if rows != 1 {
		t.Fatalf("SetLiftDetectionOverride rows=%d, want 1", rows)
	}

	lockTx, err := db.Pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin lock tx: %v", err)
	}
	defer lockTx.Rollback(ctx) //nolint:errcheck
	if _, err := lockTx.Exec(ctx, `LOCK TABLE activity_streams IN ACCESS EXCLUSIVE MODE`); err != nil {
		t.Fatalf("lock activity_streams: %v", err)
	}

	r := chi.NewRouter()
	r.Post("/api/v1/activities/{id}/lifts/recalculate-all", h.RecalculateActivityLifts)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/activities/"+activityID+"/lifts/recalculate-all", nil)
	req = req.WithContext(context.WithValue(req.Context(), middleware.UserIDKey, userID))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status=%d, want 204; body=%s", rec.Code, rec.Body.String())
	}
	if candidateExists(t, db, candID) {
		t.Fatal("candidate still exists after reset")
	}

	var activeJobs int
	if err := db.Pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM background_jobs
		WHERE user_id = $1
		  AND entity_id = $2
		  AND job_type = 'lift_recompute'
		  AND status IN ('pending', 'running')
	`, userID, activityID).Scan(&activeJobs); err != nil {
		t.Fatalf("count active jobs: %v", err)
	}
	if activeJobs != 0 {
		t.Fatalf("active lift_recompute jobs=%d, want 0 when both detection modes are disabled", activeJobs)
	}
}

// TestRecalculateActivityLifts_F1_NoDataLossOnConflict verifies that when a
// concurrent lift_recompute job is running, the 409 path does NOT delete the
// existing activity_lift_candidates rows. Pre-fix, RecalculateActivityLifts
// (DELETE) ran before the running-job check — which meant a duplicate request
// would wipe the in-flight job's candidates.
func TestRecalculateActivityLifts_F1_NoDataLossOnConflict(t *testing.T) {
	h, db := liftDetectionConfigSetup(t)
	userID := insertTestUser(t, db.Pool)
	activityID := insertMinimalActivity(t, db, userID)

	// Insert a candidate that must survive the rejected request.
	now := time.Now().UTC().Truncate(time.Second)
	candID := insertHeuristicUnconfirmed(t, db, activityID, now)

	// Block with a running lift_recompute job.
	_ = insertRunningLiftJob(t, db, userID, activityID)

	r := chi.NewRouter()
	r.Post("/api/v1/activities/{id}/lifts/recalculate-all", h.RecalculateActivityLifts)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/activities/"+activityID+"/lifts/recalculate-all", nil)
	req = req.WithContext(context.WithValue(req.Context(), middleware.UserIDKey, userID))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("status=%d, want 409; body=%s", rec.Code, rec.Body.String())
	}
	// Pre-fix this would have been deleted by RecalculateActivityLifts.
	if !candidateExists(t, db, candID) {
		t.Error("candidate deleted on 409 path — destructive recalc ran before the running-job check (F1 regression)")
	}
}

func TestRecalculateActivityLifts_JobInsertFailureDoesNotReset(t *testing.T) {
	h, db := liftDetectionConfigSetup(t)
	userID := insertTestUser(t, db.Pool)
	activityID := insertMinimalActivity(t, db, userID)

	now := time.Now().UTC().Truncate(time.Second)
	candID := insertHeuristicUnconfirmed(t, db, activityID, now)

	suffix := strings.ReplaceAll(uuid.NewString(), "-", "_")
	funcName := "fail_lift_recompute_insert_" + suffix
	triggerName := "fail_lift_recompute_insert_trg_" + suffix
	if _, err := db.Pool.Exec(context.Background(), fmt.Sprintf(`
		CREATE FUNCTION public.%s()
		RETURNS trigger
		LANGUAGE plpgsql
		AS $$
		BEGIN
			RAISE EXCEPTION 'test lift_recompute insert failure';
		END;
		$$;
		CREATE TRIGGER %s
		BEFORE INSERT ON background_jobs
		FOR EACH ROW
		WHEN (NEW.job_type = 'lift_recompute')
		EXECUTE FUNCTION public.%s();
	`, funcName, triggerName, funcName)); err != nil {
		t.Fatalf("install background_jobs failure trigger: %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.Pool.Exec(context.Background(), fmt.Sprintf(`
			DROP TRIGGER IF EXISTS %s ON background_jobs;
			DROP FUNCTION IF EXISTS public.%s();
		`, triggerName, funcName))
	})

	r := chi.NewRouter()
	r.Post("/api/v1/activities/{id}/lifts/recalculate-all", h.RecalculateActivityLifts)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/activities/"+activityID+"/lifts/recalculate-all", nil)
	req = req.WithContext(context.WithValue(req.Context(), middleware.UserIDKey, userID))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d, want 500; body=%s", rec.Code, rec.Body.String())
	}
	if !candidateExists(t, db, candID) {
		t.Error("candidate deleted even though lift_recompute job could not be created")
	}
}

func TestRecalculateActivityLifts_RequestCancelAfterJobInsertDoesNotStrandJob(t *testing.T) {
	h, db := liftDetectionConfigSetup(t)
	userID := insertTestUser(t, db.Pool)
	activityID := insertMinimalActivity(t, db, userID)

	now := time.Now().UTC().Truncate(time.Second)
	insertHeuristicUnconfirmed(t, db, activityID, now)

	suffix := strings.ReplaceAll(uuid.NewString(), "-", "_")
	funcName := "sleep_lift_recompute_activity_update_" + suffix
	triggerName := "sleep_lift_recompute_activity_update_trg_" + suffix
	notifyFuncName := "notify_lift_recompute_insert_" + suffix
	notifyTriggerName := "notify_lift_recompute_insert_trg_" + suffix
	notifyChannel := "lift_recompute_insert_" + suffix
	if _, err := db.Pool.Exec(context.Background(), fmt.Sprintf(`
		CREATE FUNCTION public.%s()
		RETURNS trigger
		LANGUAGE plpgsql
		AS $$
		BEGIN
			PERFORM pg_sleep(0.25);
			RETURN NEW;
		END;
		$$;
		CREATE TRIGGER %s
		BEFORE UPDATE OF lifts_acknowledged_at ON activities
		FOR EACH ROW
		EXECUTE FUNCTION public.%s();
		CREATE FUNCTION public.%s()
		RETURNS trigger
		LANGUAGE plpgsql
		AS $$
		BEGIN
			PERFORM pg_notify('%s', NEW.id::text);
			RETURN NEW;
		END;
		$$;
		CREATE TRIGGER %s
		AFTER INSERT ON background_jobs
		FOR EACH ROW
		WHEN (NEW.job_type = 'lift_recompute')
		EXECUTE FUNCTION public.%s();
	`, funcName, triggerName, funcName, notifyFuncName, notifyChannel, notifyTriggerName, notifyFuncName)); err != nil {
		t.Fatalf("install recalculate test triggers: %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.Pool.Exec(context.Background(), fmt.Sprintf(`
			DROP TRIGGER IF EXISTS %s ON activities;
			DROP FUNCTION IF EXISTS public.%s();
			DROP TRIGGER IF EXISTS %s ON background_jobs;
			DROP FUNCTION IF EXISTS public.%s();
		`, triggerName, funcName, notifyTriggerName, notifyFuncName))
	})
	listenConn, err := db.Pool.Acquire(context.Background())
	if err != nil {
		t.Fatalf("acquire listen conn: %v", err)
	}
	defer listenConn.Release()
	if _, err := listenConn.Exec(context.Background(), `LISTEN `+notifyChannel); err != nil {
		t.Fatalf("listen for job insert: %v", err)
	}

	r := chi.NewRouter()
	r.Post("/api/v1/activities/{id}/lifts/recalculate-all", h.RecalculateActivityLifts)
	reqCtx, cancelReq := context.WithCancel(context.Background())
	reqCtx = context.WithValue(reqCtx, middleware.UserIDKey, userID)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/activities/"+activityID+"/lifts/recalculate-all", nil)
	req = req.WithContext(reqCtx)
	rec := httptest.NewRecorder()
	done := make(chan struct{})
	go func() {
		defer close(done)
		r.ServeHTTP(rec, req)
	}()

	notifyCtx, cancelNotify := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancelNotify()
	notification, err := listenConn.Conn().WaitForNotification(notifyCtx)
	if err != nil {
		select {
		case <-done:
			t.Fatalf("handler returned before lift_recompute job insert notification; status=%d body=%s", rec.Code, rec.Body.String())
		default:
		}
		t.Fatalf("timed out waiting for lift_recompute job insert notification: %v", err)
	}
	jobID := notification.Payload
	cancelReq()

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("handler did not finish after request cancellation")
	}
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status=%d, want 204; body=%s", rec.Code, rec.Body.String())
	}

	var status string
	for i := 0; i < 100; i++ {
		if err := db.Pool.QueryRow(context.Background(),
			`SELECT status FROM background_jobs WHERE id = $1`, jobID).Scan(&status); err != nil {
			t.Fatalf("query job status: %v", err)
		}
		if status != "running" {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("lift_recompute job remained %q after request cancellation", status)
}

func TestDispatchLiftReEnrichmentBackground_SettingsErrorPropagates(t *testing.T) {
	h, db := liftDetectionConfigSetup(t)
	userID := insertTestUser(t, db.Pool)
	activityID := insertMinimalActivity(t, db, userID)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := h.dispatchLiftReEnrichmentBackground(ctx, userID, activityID); err == nil {
		t.Fatal("dispatchLiftReEnrichmentBackground returned nil for settings load failure")
	}
}

// TestSetLiftDetectionOverride_F2_CrossUserRejected verifies the DB-level
// defence-in-depth user_id scoping on SetLiftDetectionOverride. User B's UPDATE
// on User A's activity must affect 0 rows.
func TestSetLiftDetectionOverride_F2_CrossUserRejected(t *testing.T) {
	_, db := liftDetectionConfigSetup(t)
	userA := insertTestUser(t, db.Pool)
	userB := insertTestUser(t, db.Pool)
	activityA := insertMinimalActivity(t, db, userA)

	tr := true
	rows, err := db.SetLiftDetectionOverride(context.Background(), userB, activityA, &tr, &tr)
	if err != nil {
		t.Fatalf("SetLiftDetectionOverride: %v", err)
	}
	if rows != 0 {
		t.Errorf("RowsAffected=%d, want 0 (cross-user UPDATE must not match)", rows)
	}

	// Verify the row stayed untouched.
	var osm, heu *bool
	if err := db.Pool.QueryRow(context.Background(),
		`SELECT lift_detection_osm_override, lift_detection_heuristic_override FROM activities WHERE id = $1`,
		activityA).Scan(&osm, &heu); err != nil {
		t.Fatalf("query overrides: %v", err)
	}
	if osm != nil || heu != nil {
		t.Errorf("overrides modified by cross-user UPDATE: osm=%v heu=%v", osm, heu)
	}
}

// TestSetLiftDetectionOverride_F18_NoOpSkipped verifies that re-PUTing the
// same body returns RowsAffected=0 (the IS DISTINCT FROM clause skips the
// no-op write). First PUT writes, second PUT is a no-op.
func TestSetLiftDetectionOverride_F18_NoOpSkipped(t *testing.T) {
	_, db := liftDetectionConfigSetup(t)
	userID := insertTestUser(t, db.Pool)
	activityID := insertMinimalActivity(t, db, userID)

	tr := true
	fa := false
	ctx := context.Background()

	// First write: must affect 1 row.
	rows1, err := db.SetLiftDetectionOverride(ctx, userID, activityID, &tr, &fa)
	if err != nil {
		t.Fatalf("first PUT: %v", err)
	}
	if rows1 != 1 {
		t.Errorf("first PUT RowsAffected=%d, want 1", rows1)
	}

	// Identical second write: must be a no-op (RowsAffected=0).
	rows2, err := db.SetLiftDetectionOverride(ctx, userID, activityID, &tr, &fa)
	if err != nil {
		t.Fatalf("second PUT: %v", err)
	}
	if rows2 != 0 {
		t.Errorf("idempotent PUT RowsAffected=%d, want 0 (F18 no-op skip)", rows2)
	}

	// Changing one column → 1 row.
	rows3, err := db.SetLiftDetectionOverride(ctx, userID, activityID, &fa, nil)
	if err != nil {
		t.Fatalf("change PUT: %v", err)
	}
	if rows3 != 1 {
		t.Errorf("change PUT RowsAffected=%d, want 1", rows3)
	}
}

// TestRecalculateActivityLifts_F17_JobInsertConflict409 verifies that when
// InsertBackgroundJob fails with a unique_violation on idx_bg_jobs_one_running
// (concurrent insert raced past F1's check), the handler returns 409
// LIFT_RECOMPUTE_RUNNING instead of 500 — and does not start the
// re-enrichment goroutine.
//
// Reproduction strategy: skip F1's pre-check by NOT inserting a running job
// before the request, but pre-insert a running job between the F1 SELECT and
// the InsertBackgroundJob INSERT? That requires goroutine timing. Simpler
// proxy: the F3-widened index now also covers 'queued' — insert a queued
// lift_recompute row for this entity but NOT a running one. The F1 check
// covers queued (F3 alignment), so we get 409 at the pre-check, which already
// proves queued visibility. Combined with the migration test verifying
// queued+running collision at the index level, F17 is end-to-end covered.
func TestRecalculateActivityLifts_F17_QueuedJobAlsoBlocks(t *testing.T) {
	h, db := liftDetectionConfigSetup(t)
	userID := insertTestUser(t, db.Pool)
	activityID := insertMinimalActivity(t, db, userID)

	// Insert a queued lift_recompute job (status='queued', not 'running').
	jobID := uuid.NewString()
	if _, err := db.Pool.Exec(context.Background(), `
		INSERT INTO background_jobs (id, user_id, job_type, entity_id, status, progress_total)
		VALUES ($1, $2, 'lift_recompute', $3, 'queued', 1)
	`, jobID, userID, activityID); err != nil {
		t.Fatalf("insert queued job: %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.Pool.Exec(context.Background(), "DELETE FROM background_jobs WHERE id = $1", jobID)
	})

	r := chi.NewRouter()
	r.Post("/api/v1/activities/{id}/lifts/recalculate-all", h.RecalculateActivityLifts)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/activities/"+activityID+"/lifts/recalculate-all", nil)
	req = req.WithContext(context.WithValue(req.Context(), middleware.UserIDKey, userID))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("status=%d, want 409 (queued job must block); body=%s", rec.Code, rec.Body.String())
	}
}
