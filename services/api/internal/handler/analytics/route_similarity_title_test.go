package analytics

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/database/testdb"
	"github.com/skadia-app/skadia/services/api/internal/middleware"
	"github.com/google/uuid"
)

func routeSimilarityTitleTestDB(t *testing.T) (*database.DB, string) {
	t.Helper()
	ctx := context.Background()
	pool := testdb.Open(t)
	db := &database.DB{Pool: pool}

	var userID string
	suffix := uuid.NewString()
	if err := pool.QueryRow(ctx, `
		INSERT INTO users (username, email, registration_source)
		VALUES ($1, $2, 'test')
		RETURNING id`, "route-sim-title-"+suffix, "route-sim-title-"+suffix+"@example.test").Scan(&userID); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, userID)
	})

	return db, userID
}

func insertRouteSimilarityActivity(t *testing.T, db *database.DB, userID, title string, autoTitle *string, manual bool, startedAt time.Time) string {
	t.Helper()
	ctx := context.Background()

	var activityID string
	if err := db.Pool.QueryRow(ctx, `
		INSERT INTO activities (user_id, sport_type, title, title_auto, title_is_manual, started_at)
		VALUES ($1, 'running', $2, $3, $4, $5)
		RETURNING id`, userID, title, autoTitle, manual, startedAt).Scan(&activityID); err != nil {
		t.Fatalf("insert activity: %v", err)
	}
	if _, err := db.Pool.Exec(ctx, `
		INSERT INTO activity_tracks (activity_id, track_simplified, point_count)
		VALUES ($1, ST_GeomFromText('LINESTRING(8 49, 8.01 49.01, 8.02 49.02)', 4326), 3)`,
		activityID); err != nil {
		t.Fatalf("insert activity track: %v", err)
	}
	return activityID
}

func TestGetSimilarRoutesUsesGeneratedTitleUnlessManual(t *testing.T) {
	db, userID := routeSimilarityTitleTestDB(t)
	now := time.Now().UTC()

	targetID := insertRouteSimilarityActivity(t, db, userID, "Target", nil, false, now)
	insertRouteSimilarityActivity(t, db, userID, "Manual title", stringPtr("Generated manual"), true, now.Add(-time.Minute))
	insertRouteSimilarityActivity(t, db, userID, "Source title", stringPtr("Generated title"), false, now.Add(-2*time.Minute))
	insertRouteSimilarityActivity(t, db, userID, "Source fallback", nil, false, now.Add(-3*time.Minute))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/analytics/similar-routes?activity_id="+targetID+"&max_results=10", nil)
	req = req.WithContext(context.WithValue(req.Context(), middleware.UserIDKey, userID))
	rec := httptest.NewRecorder()

	(&Handler{DB: db}).GetSimilarRoutes(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}

	var routes []SimilarRoute
	if err := json.Unmarshal(rec.Body.Bytes(), &routes); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(routes) != 3 {
		t.Fatalf("routes len = %d, want 3: %#v", len(routes), routes)
	}
	got := map[string]bool{}
	for _, route := range routes {
		got[route.Title] = true
	}
	for _, title := range []string{"Manual title", "Generated title", "Source fallback"} {
		if !got[title] {
			t.Fatalf("response titles = %#v, missing %q", got, title)
		}
	}
}

func stringPtr(s string) *string {
	return &s
}
