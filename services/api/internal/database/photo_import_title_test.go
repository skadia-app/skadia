package database

import (
	"context"
	"testing"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/database/testdb"
	"github.com/skadia-app/skadia/services/api/internal/model"
	"github.com/google/uuid"
)

func photoImportTitleTestDB(t *testing.T) (*DB, string) {
	t.Helper()

	ctx := context.Background()
	pool := testdb.Open(t)
	db := &DB{Pool: pool}

	var userID string
	username := "photo-title-" + uuid.NewString()
	if err := pool.QueryRow(ctx, `
		INSERT INTO users (username, email, registration_source)
		VALUES ($1, $2, 'test')
		RETURNING id`, username, username+"@example.test").Scan(&userID); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, userID)
	})

	return db, userID
}

func insertTitleActivity(t *testing.T, db *DB, userID, sourceTitle, autoTitle string, manual bool, startedAt time.Time) string {
	t.Helper()

	var activityID string
	endedAt := startedAt.Add(time.Hour)
	if err := db.Pool.QueryRow(context.Background(), `
		INSERT INTO activities (user_id, sport_type, title, title_auto, title_is_manual, started_at, ended_at)
		VALUES ($1, 'hiking', $2, $3, $4, $5, $6)
		RETURNING id`, userID, sourceTitle, autoTitle, manual, startedAt, endedAt).Scan(&activityID); err != nil {
		t.Fatalf("insert activity: %v", err)
	}
	return activityID
}

func insertTitlePhoto(t *testing.T, db *DB, userID, activityID string, lat, lng *float64) string {
	t.Helper()

	hash := uuid.NewString()
	photo, err := db.InsertActivityPhoto(context.Background(), model.ActivityPhoto{
		UserID:           userID,
		ActivityID:       &activityID,
		OriginalFilename: "photo-" + uuid.NewString() + ".jpg",
		ContentType:      "image/jpeg",
		FileSizeBytes:    1234,
		FileHash:         &hash,
		Lat:              lat,
		Lng:              lng,
	})
	if err != nil {
		t.Fatalf("insert photo: %v", err)
	}
	return photo.ID
}

func TestPhotoSurfacesUseGeneratedTitleUnlessManual(t *testing.T) {
	ctx := context.Background()
	db, userID := photoImportTitleTestDB(t)
	startedAt := time.Now().UTC().Truncate(time.Second)
	lat, lng := 47.1, 11.2
	generatedID := insertTitleActivity(t, db, userID, "Imported source", "Generated ridge walk", false, startedAt)
	manualID := insertTitleActivity(t, db, userID, "Manual photo label", "Generated ignored", true, startedAt.Add(-time.Hour))
	insertTitlePhoto(t, db, userID, generatedID, &lat, &lng)
	insertTitlePhoto(t, db, userID, manualID, nil, nil)

	feed, _, err := db.ListPhotoFeed(ctx, userID, "date", 10, 0)
	if err != nil {
		t.Fatalf("ListPhotoFeed: %v", err)
	}
	titles := map[string]string{}
	for _, row := range feed {
		if row.ActID != nil && row.ActTitle != nil {
			titles[*row.ActID] = *row.ActTitle
		}
	}
	if titles[generatedID] != "Generated ridge walk" {
		t.Fatalf("feed generated title = %q, want Generated ridge walk", titles[generatedID])
	}
	if titles[manualID] != "Manual photo label" {
		t.Fatalf("feed manual title = %q, want Manual photo label", titles[manualID])
	}

	search, err := db.SearchPhotos(ctx, userID, "", "ridge", 10)
	if err != nil {
		t.Fatalf("SearchPhotos: %v", err)
	}
	if len(search) != 1 {
		t.Fatalf("SearchPhotos len = %d, want 1", len(search))
	}
	if search[0].Activity == nil || search[0].Activity.Title != "Generated ridge walk" {
		t.Fatalf("search title = %#v, want Generated ridge walk", search[0].Activity)
	}

	mapPhotos, err := db.ListMapPhotos(ctx, userID)
	if err != nil {
		t.Fatalf("ListMapPhotos: %v", err)
	}
	if len(mapPhotos) != 1 {
		t.Fatalf("map photos len = %d, want 1", len(mapPhotos))
	}
	if mapPhotos[0].ActivityTitle == nil || *mapPhotos[0].ActivityTitle != "Generated ridge walk" {
		t.Fatalf("map title = %#v, want Generated ridge walk", mapPhotos[0].ActivityTitle)
	}
}

func TestFindActivitiesCoveringTimeUsesGeneratedTitleUnlessManual(t *testing.T) {
	ctx := context.Background()
	db, userID := photoImportTitleTestDB(t)
	startedAt := time.Now().UTC().Truncate(time.Second)
	insertTitleActivity(t, db, userID, "Imported source", "Generated picker title", false, startedAt)
	insertTitleActivity(t, db, userID, "Manual picker title", "Generated ignored", true, startedAt.Add(-5*time.Minute))

	matches, err := db.FindActivitiesCoveringTime(ctx, userID, startedAt.Add(10*time.Minute))
	if err != nil {
		t.Fatalf("FindActivitiesCoveringTime: %v", err)
	}
	if len(matches) != 2 {
		t.Fatalf("matches len = %d, want 2", len(matches))
	}
	if matches[0].Title == nil || *matches[0].Title != "Generated picker title" {
		t.Fatalf("first title = %#v, want generated title", matches[0].Title)
	}
	if matches[1].Title == nil || *matches[1].Title != "Manual picker title" {
		t.Fatalf("second title = %#v, want manual title", matches[1].Title)
	}
}
