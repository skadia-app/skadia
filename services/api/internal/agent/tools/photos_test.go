package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/database/testdb"
)

func TestSearchPhotosByContentSchemaValid(t *testing.T) {
	reg := NewRegistry()
	reg.Register(SearchPhotosByContentTool())
	if err := reg.ValidateSchemas(); err != nil {
		t.Fatalf("ValidateSchemas: %v", err)
	}
}

func TestSearchPhotosByContentEmbeddingErrorReturnsEmptyPhotos(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "model unavailable", http.StatusServiceUnavailable)
	}))
	defer server.Close()

	got, err := executeSearchPhotosByContent(context.Background(), &Deps{DB: &database.DB{}, MLBaseURL: server.URL}, "user-1", json.RawMessage(`{"query":"lake"}`))
	if err != nil {
		t.Fatalf("executeSearchPhotosByContent: %v", err)
	}
	result := got.(map[string]any)
	if result["count"] != 0 || result["error"] == nil {
		t.Fatalf("unexpected result: %#v", result)
	}
}

func TestSearchPhotosByContentDB(t *testing.T) {
	ctx := context.Background()
	db := openPhotoSearchTestDB(t)
	userA := "00000000-0000-0000-0000-00000000a101"
	userB := "00000000-0000-0000-0000-00000000b101"
	activityA := "00000000-0000-0000-0000-00000000a201"
	activityB := "00000000-0000-0000-0000-00000000b201"
	photoA := "00000000-0000-0000-0000-00000000a301"
	photoB := "00000000-0000-0000-0000-00000000b301"

	seedPhotoSearchRows(t, db, userA, activityA, photoA, "Lake Run")
	seedPhotoSearchRows(t, db, userB, activityB, photoB, "Foreign Lake")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"embedding": photoSearchVector(), "dim": 1152})
	}))
	defer server.Close()

	got, err := executeSearchPhotosByContent(ctx, &Deps{DB: db, MLBaseURL: server.URL}, userA, json.RawMessage(`{"query":"lake","limit":10}`))
	if err != nil {
		t.Fatalf("executeSearchPhotosByContent: %v", err)
	}
	result := got.(map[string]any)
	photos := result["photos"].([]map[string]any)
	if result["count"] != 1 || len(photos) != 1 {
		t.Fatalf("expected one user-owned photo, got %#v", result)
	}
	if photos[0]["id"] != photoA || photos[0]["activity_title"] != "Lake Run" {
		t.Fatalf("unexpected photo result: %#v", photos[0])
	}
	if photos[0]["date"] != "2026-05-10" {
		t.Fatalf("unexpected date: %#v", photos[0])
	}
}

func openPhotoSearchTestDB(t *testing.T) *database.DB {
	t.Helper()
	ctx := context.Background()
	dsn := testdb.RequireSafeDSN(t)
	db, err := database.New(ctx, dsn)
	if err != nil {
		t.Fatalf("database.New: %v", err)
	}
	t.Cleanup(db.Close)
	return db
}

func seedPhotoSearchRows(t *testing.T, db *database.DB, userID, activityID, photoID, title string) {
	t.Helper()
	ctx := context.Background()
	_, err := db.Pool.Exec(ctx,
		`INSERT INTO users (id, username, email, email_verified, registration_source)
		 VALUES ($1, $2, $3, true, 'test')
		 ON CONFLICT (id) DO NOTHING`,
		userID, "photo_"+strings.ReplaceAll(userID[len(userID)-4:], "-", ""), fmt.Sprintf("%s@example.invalid", userID[len(userID)-4:]),
	)
	if err != nil {
		t.Fatalf("insert user: %v", err)
	}
	_, err = db.Pool.Exec(ctx,
		`INSERT INTO activities (id, user_id, title, sport_type, started_at)
		 VALUES ($1, $2, $3, 'running', $4)
		 ON CONFLICT (id) DO NOTHING`,
		activityID, userID, title, time.Date(2026, 5, 10, 8, 0, 0, 0, time.UTC),
	)
	if err != nil {
		t.Fatalf("insert activity: %v", err)
	}
	_, err = db.Pool.Exec(ctx,
		`INSERT INTO activity_photos (id, user_id, activity_id, original_filename, content_type, file_size_bytes, taken_at, lat, lng, clip_embedding)
		 VALUES ($1, $2, $3, 'photo.jpg', 'image/jpeg', 123, $4, 47.1, 8.1, $5::vector)
		 ON CONFLICT (id) DO UPDATE SET clip_embedding = EXCLUDED.clip_embedding`,
		photoID, userID, activityID, time.Date(2026, 5, 10, 9, 0, 0, 0, time.UTC), photoSearchVectorLiteral(),
	)
	if err != nil {
		t.Fatalf("insert activity_photo: %v", err)
	}
}

func photoSearchVector() []float64 {
	v := make([]float64, 1152)
	v[0] = 1
	return v
}

func photoSearchVectorLiteral() string {
	parts := make([]string, 1152)
	parts[0] = "1"
	for i := 1; i < len(parts); i++ {
		parts[i] = "0"
	}
	return "[" + strings.Join(parts, ",") + "]"
}
