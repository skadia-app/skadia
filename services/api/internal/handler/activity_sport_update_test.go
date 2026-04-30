package handler

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/skadia-app/skadia/services/api/internal/database"
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

	req := httptest.NewRequest(http.MethodPatch, "/api/v1/activities/"+activityID, bytes.NewReader([]byte(`{"is_virtual":false}`)))
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
