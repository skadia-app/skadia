package handler

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/model"
	"github.com/go-chi/chi/v5"
)

func TestUpdateActivityNormalizesLegacySportTuple(t *testing.T) {
	pool := fusionTestPool(t)
	db := &database.DB{Pool: pool}
	h := &Handler{DB: db}
	userID := insertTestUser(t, pool)
	activityID := insertMinimalActivity(t, db, userID)

	req := httptest.NewRequest(http.MethodPatch, "/api/v1/activities/"+activityID, bytes.NewReader([]byte(`{"sport_type":"mountain_biking"}`)))
	req = req.WithContext(withUserAndAdmin(req.Context(), userID, false))
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", activityID)
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	w := httptest.NewRecorder()
	h.UpdateActivity(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("PATCH status = %d, body = %s", w.Code, w.Body.String())
	}

	activity, err := db.GetActivity(context.Background(), activityID)
	if err != nil {
		t.Fatalf("GetActivity: %v", err)
	}
	if activity.SportType != "cycling" || activity.SubType == nil || *activity.SubType != "mountain" {
		t.Fatalf("activity tuple = (%s,%v), want (cycling,mountain)", activity.SportType, activity.SubType)
	}
}

func TestUpdateActivityClearsHeatmapExclusionWhenVirtualCleared(t *testing.T) {
	pool := fusionTestPool(t)
	db := &database.DB{Pool: pool}
	h := &Handler{DB: db}
	userID := insertTestUser(t, pool)
	activityID := insertMinimalActivity(t, db, userID)
	if _, err := pool.Exec(context.Background(),
		`UPDATE activities SET is_virtual = true, exclude_from_heatmap = true WHERE id = $1`, activityID); err != nil {
		t.Fatalf("seed virtual activity: %v", err)
	}

	req := httptest.NewRequest(http.MethodPatch, "/api/v1/activities/"+activityID, bytes.NewReader([]byte(`{"is_virtual":false,"exclude_from_heatmap":false}`)))
	req = req.WithContext(withUserAndAdmin(req.Context(), userID, false))
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", activityID)
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	w := httptest.NewRecorder()
	h.UpdateActivity(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("PATCH status = %d, body = %s", w.Code, w.Body.String())
	}

	activity, err := db.GetActivity(context.Background(), activityID)
	if err != nil {
		t.Fatalf("GetActivity: %v", err)
	}
	if activity.IsVirtual || activity.ExcludeFromHeatmap {
		t.Fatalf("virtual=%v heatmap=%v, want false/false", activity.IsVirtual, activity.ExcludeFromHeatmap)
	}
}

func TestUpdateActivityVirtualToggleRefreshesHeatmapRows(t *testing.T) {
	pool := fusionTestPool(t)
	db := &database.DB{Pool: pool}
	h := &Handler{DB: db}
	userID := insertTestUser(t, pool)
	activityID := insertMinimalActivity(t, db, userID)
	if err := db.InsertTrack(context.Background(), activityID, []model.TrackPoint{
		{Lat: 48.0, Lng: 11.0},
		{Lat: 48.001, Lng: 11.001},
		{Lat: 48.002, Lng: 11.002},
	}); err != nil {
		t.Fatalf("insert track: %v", err)
	}
	activity, err := db.GetActivity(context.Background(), activityID)
	if err != nil {
		t.Fatalf("GetActivity: %v", err)
	}
	if err := db.InsertHeatmapData(context.Background(), userID, activityID, activity.SportType, activity.SubType, activity.StartedAt, 50); err != nil {
		t.Fatalf("insert heatmap: %v", err)
	}
	var heatmapLines int
	if err := pool.QueryRow(context.Background(), `SELECT COUNT(*) FROM heatmap_lines WHERE activity_id = $1`, activityID).Scan(&heatmapLines); err != nil {
		t.Fatalf("count seeded heatmap lines: %v", err)
	}
	if heatmapLines != 1 {
		t.Fatalf("seeded heatmap line count = %d, want 1", heatmapLines)
	}

	req := httptest.NewRequest(http.MethodPatch, "/api/v1/activities/"+activityID, bytes.NewReader([]byte(`{"is_virtual":true}`)))
	req = req.WithContext(withUserAndAdmin(req.Context(), userID, false))
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", activityID)
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	w := httptest.NewRecorder()
	h.UpdateActivity(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("PATCH virtual=true status = %d, body = %s", w.Code, w.Body.String())
	}
	if err := pool.QueryRow(context.Background(), `SELECT COUNT(*) FROM heatmap_lines WHERE activity_id = $1`, activityID).Scan(&heatmapLines); err != nil {
		t.Fatalf("count virtual heatmap lines: %v", err)
	}
	if heatmapLines != 0 {
		t.Fatalf("virtual heatmap line count = %d, want 0", heatmapLines)
	}

	req = httptest.NewRequest(http.MethodPatch, "/api/v1/activities/"+activityID, bytes.NewReader([]byte(`{"is_virtual":false}`)))
	req = req.WithContext(withUserAndAdmin(req.Context(), userID, false))
	rctx = chi.NewRouteContext()
	rctx.URLParams.Add("id", activityID)
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	w = httptest.NewRecorder()
	h.UpdateActivity(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("PATCH virtual=false status = %d, body = %s", w.Code, w.Body.String())
	}
	if err := pool.QueryRow(context.Background(), `SELECT COUNT(*) FROM heatmap_lines WHERE activity_id = $1`, activityID).Scan(&heatmapLines); err != nil {
		t.Fatalf("count restored heatmap lines: %v", err)
	}
	if heatmapLines != 1 {
		t.Fatalf("restored heatmap line count = %d, want 1", heatmapLines)
	}
}

func TestUpdateActivityClearFields(t *testing.T) {
	pool := fusionTestPool(t)
	db := &database.DB{Pool: pool}
	h := &Handler{DB: db}
	userID := insertTestUser(t, pool)
	activityID := insertMinimalActivity(t, db, userID)
	if _, err := pool.Exec(context.Background(),
		`UPDATE activities
		 SET title = 'Manual title', title_is_manual = true,
		     description = 'Notes', workout_rpe = 7, workout_feel = 3
		 WHERE id = $1`, activityID); err != nil {
		t.Fatalf("seed cleared fields: %v", err)
	}

	req := httptest.NewRequest(http.MethodPatch, "/api/v1/activities/"+activityID, bytes.NewReader([]byte(`{
		"clear_title": true,
		"clear_description": true,
		"clear_workout_rpe": true,
		"clear_workout_feel": true
	}`)))
	req = req.WithContext(withUserAndAdmin(req.Context(), userID, false))
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", activityID)
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	w := httptest.NewRecorder()
	h.UpdateActivity(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("PATCH clear fields status = %d, body = %s", w.Code, w.Body.String())
	}

	activity, err := db.GetActivity(context.Background(), activityID)
	if err != nil {
		t.Fatalf("GetActivity: %v", err)
	}
	if activity.Title != nil || activity.TitleIsManual || activity.Description != nil || activity.WorkoutRpe != nil || activity.WorkoutFeel != nil {
		t.Fatalf("cleared fields = title:%v manual:%v description:%v rpe:%v feel:%v, want nil/false/nil/nil/nil",
			activity.Title, activity.TitleIsManual, activity.Description, activity.WorkoutRpe, activity.WorkoutFeel)
	}
}

func TestUpdateActivityClearsPendingSportConflictOnManualSportPatch(t *testing.T) {
	pool := fusionTestPool(t)
	db := &database.DB{Pool: pool}
	h := &Handler{DB: db}
	userID := insertTestUser(t, pool)
	activityID := insertMinimalActivity(t, db, userID)
	if _, err := pool.Exec(context.Background(),
		`UPDATE activities SET sport_conflict_pending = true WHERE id = $1`, activityID); err != nil {
		t.Fatalf("seed sport conflict: %v", err)
	}
	queueID, err := db.CreateImportQueueItem(context.Background(), database.ImportQueueItem{
		UserID:           userID,
		QueueType:        "sport_conflict",
		OriginalFilename: "activity.fit",
		ContentType:      "application/octet-stream",
		Metadata:         map[string]any{"activity_id": activityID},
	})
	if err != nil {
		t.Fatalf("seed sport conflict queue: %v", err)
	}

	req := httptest.NewRequest(http.MethodPatch, "/api/v1/activities/"+activityID, bytes.NewReader([]byte(`{"sport_type":"cycling","sub_type":"mountain","is_electric":true}`)))
	req = req.WithContext(withUserAndAdmin(req.Context(), userID, false))
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", activityID)
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	w := httptest.NewRecorder()
	h.UpdateActivity(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("PATCH status = %d, body = %s", w.Code, w.Body.String())
	}

	activity, err := db.GetActivity(context.Background(), activityID)
	if err != nil {
		t.Fatalf("GetActivity: %v", err)
	}
	if activity.SportConflictPending {
		t.Fatal("sport_conflict_pending = true, want false after manual sport patch")
	}
	var queueStatus string
	var queueResolved bool
	if err := pool.QueryRow(context.Background(),
		`SELECT status, resolved_at IS NOT NULL FROM import_queue WHERE id = $1`, queueID).Scan(&queueStatus, &queueResolved); err != nil {
		t.Fatalf("query sport conflict queue: %v", err)
	}
	if queueStatus != "resolved" || !queueResolved {
		t.Fatalf("queue status/resolved = %s/%v, want resolved/true", queueStatus, queueResolved)
	}
}
