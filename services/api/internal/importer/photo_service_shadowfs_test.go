package importer

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/database/testdb"
	"github.com/skadia-app/skadia/services/api/internal/model"
	"github.com/skadia-app/skadia/services/api/internal/shadowfs"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestProcessPhotoReplacementStoresNewOriginalAndArchivesOld(t *testing.T) {
	ctx := context.Background()
	svc, pool, userID, activityID := newPhotoServiceStorageFixture(t)
	setPhotoPhashThreshold(t, pool, userID, 64)

	small := jpegPhotoTestBytes(t, 20, 20, color.RGBA{R: 230, G: 20, B: 20, A: 255}, 90)
	created, err := svc.ProcessPhoto(ctx, userID, activityID, small, PhotoMetadata{
		OriginalFilename: "small.jpg",
		ContentType:      "image/jpeg",
	})
	if err != nil {
		t.Fatalf("ProcessPhoto created: %v", err)
	}
	if created.Status != "created" {
		t.Fatalf("first ProcessPhoto status = %q, want created", created.Status)
	}
	oldFileID := photoFileID(t, pool, created.PhotoID)

	large := jpegPhotoTestBytes(t, 40, 40, color.RGBA{R: 230, G: 20, B: 20, A: 255}, 90)
	replaced, err := svc.ProcessPhoto(ctx, userID, activityID, large, PhotoMetadata{
		OriginalFilename: "large.jpg",
		ContentType:      "image/jpeg",
	})
	if err != nil {
		t.Fatalf("ProcessPhoto replaced: %v", err)
	}
	if replaced.Status != "replaced" {
		t.Fatalf("second ProcessPhoto status = %q, want replaced", replaced.Status)
	}
	if replaced.PhotoID != created.PhotoID {
		t.Fatalf("replaced photo id = %q, want existing %q", replaced.PhotoID, created.PhotoID)
	}

	newFileID := photoFileID(t, pool, created.PhotoID)
	if newFileID == "" {
		t.Fatal("replacement left activity_photos.file_id empty")
	}
	if newFileID == oldFileID {
		t.Fatalf("replacement kept old file_id %q; want a new original file", oldFileID)
	}

	assertUserFileStatus(t, pool, userID, oldFileID, "archived", newFileID)
	assertUserFileStatus(t, pool, userID, newFileID, "active", "")
	if got := activeMediaCount(t, pool, userID); got != 1 {
		t.Fatalf("active media user_files after replacement = %d, want 1", got)
	}
	if got := fileHashForUserFile(t, pool, userID, newFileID); got != sha256Hex(large) {
		t.Fatalf("replacement active file hash = %q, want %q", got, sha256Hex(large))
	}
}

func TestProcessPhotoExactDuplicateDoesNotWriteAnotherOriginal(t *testing.T) {
	ctx := context.Background()
	svc, pool, userID, activityID := newPhotoServiceStorageFixture(t)

	body := jpegPhotoTestBytes(t, 24, 24, color.RGBA{R: 20, G: 90, B: 230, A: 255}, 90)
	created, err := svc.ProcessPhoto(ctx, userID, activityID, body, PhotoMetadata{
		OriginalFilename: "exact-a.jpg",
		ContentType:      "image/jpeg",
	})
	if err != nil {
		t.Fatalf("ProcessPhoto created: %v", err)
	}
	if created.Status != "created" {
		t.Fatalf("first ProcessPhoto status = %q, want created", created.Status)
	}
	beforeFileID := photoFileID(t, pool, created.PhotoID)

	duplicate, err := svc.ProcessPhoto(ctx, userID, activityID, body, PhotoMetadata{
		OriginalFilename: "exact-b.jpg",
		ContentType:      "image/jpeg",
	})
	if err != nil {
		t.Fatalf("ProcessPhoto duplicate: %v", err)
	}
	if duplicate.Status != "duplicate_exact" {
		t.Fatalf("duplicate status = %q, want duplicate_exact", duplicate.Status)
	}
	if got := activeMediaCount(t, pool, userID); got != 1 {
		t.Fatalf("active media user_files after exact duplicate = %d, want 1", got)
	}
	if got := photoFileID(t, pool, created.PhotoID); got != beforeFileID {
		t.Fatalf("exact duplicate changed file_id from %q to %q", beforeFileID, got)
	}
}

func TestProcessPhotoExactDuplicateIsUserWide(t *testing.T) {
	ctx := context.Background()
	svc, pool, userID, activityA := newPhotoServiceStorageFixture(t)
	activityB := insertImporterPhotoTestActivity(t, svc.DB, userID)

	body := jpegPhotoTestBytes(t, 24, 24, color.RGBA{R: 80, G: 140, B: 220, A: 255}, 90)
	created, err := svc.ProcessPhoto(ctx, userID, activityA, body, PhotoMetadata{
		OriginalFilename: "activity-a.jpg",
		ContentType:      "image/jpeg",
	})
	if err != nil {
		t.Fatalf("ProcessPhoto created: %v", err)
	}
	if created.Status != "created" {
		t.Fatalf("first ProcessPhoto status = %q, want created", created.Status)
	}

	duplicate, err := svc.ProcessPhoto(ctx, userID, activityB, body, PhotoMetadata{
		OriginalFilename: "activity-b.jpg",
		ContentType:      "image/jpeg",
	})
	if err != nil {
		t.Fatalf("ProcessPhoto duplicate: %v", err)
	}
	if duplicate.Status != "duplicate_exact" {
		t.Fatalf("duplicate status = %q, want duplicate_exact", duplicate.Status)
	}
	if got := activeMediaCount(t, pool, userID); got != 1 {
		t.Fatalf("active media user_files after user-wide exact duplicate = %d, want 1", got)
	}
	if got := activityPhotoRowCount(t, pool, userID); got != 1 {
		t.Fatalf("activity_photos rows after user-wide exact duplicate = %d, want 1", got)
	}
}

func TestProcessPhotoPhashDuplicateIsUserWideAcrossJPEGAndPNG(t *testing.T) {
	ctx := context.Background()
	svc, pool, userID, activityA := newPhotoServiceStorageFixture(t)
	activityB := insertImporterPhotoTestActivity(t, svc.DB, userID)
	setPhotoPhashThreshold(t, pool, userID, 8)

	src := photoPatternImage(48, 48)
	first := encodeJPEGImage(t, src, 95)
	created, err := svc.ProcessPhoto(ctx, userID, activityA, first, PhotoMetadata{
		OriginalFilename: "activity-a.jpg",
		ContentType:      "image/jpeg",
	})
	if err != nil {
		t.Fatalf("ProcessPhoto created: %v", err)
	}
	if created.Status != "created" {
		t.Fatalf("first ProcessPhoto status = %q, want created", created.Status)
	}

	second := encodePNGImage(t, src)
	duplicate, err := svc.ProcessPhoto(ctx, userID, activityB, second, PhotoMetadata{
		OriginalFilename: "activity-b.png",
		ContentType:      "image/png",
	})
	if err != nil {
		t.Fatalf("ProcessPhoto png duplicate: %v", err)
	}
	if duplicate.Status != "duplicate_phash" {
		t.Fatalf("png duplicate status = %q, want duplicate_phash", duplicate.Status)
	}
	if got := activeMediaCount(t, pool, userID); got != 1 {
		t.Fatalf("active media user_files after user-wide phash duplicate = %d, want 1", got)
	}
	if got := activityPhotoRowCount(t, pool, userID); got != 1 {
		t.Fatalf("activity_photos rows after user-wide phash duplicate = %d, want 1", got)
	}
}

func TestProcessPhotoHigherResolutionFFmpegStillDuplicateReplacesJPEGOriginal(t *testing.T) {
	for _, tt := range []struct {
		name        string
		ext         string
		contentType string
	}{
		{name: "jxl", ext: ".jxl", contentType: "image/jxl"},
		{name: "avif", ext: ".avif", contentType: "image/avif"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			svc, pool, userID, activityID := newPhotoServiceStorageFixture(t)
			setPhotoPhashThreshold(t, pool, userID, 64)

			small := encodeJPEGImage(t, photoPatternImage(24, 24), 95)
			created, err := svc.ProcessPhoto(ctx, userID, activityID, small, PhotoMetadata{
				OriginalFilename: "small.jpg",
				ContentType:      "image/jpeg",
			})
			if err != nil {
				t.Fatalf("ProcessPhoto created: %v", err)
			}
			if created.Status != "created" {
				t.Fatalf("first ProcessPhoto status = %q, want created", created.Status)
			}
			oldFileID := photoFileID(t, pool, created.PhotoID)

			largePNG := encodePNGImage(t, photoPatternImage(48, 48))
			large := encodeImageWithFFmpegOrSkip(t, largePNG, ".png", tt.ext)
			replaced, err := svc.ProcessPhoto(ctx, userID, activityID, large, PhotoMetadata{
				OriginalFilename: "large" + tt.ext,
				ContentType:      tt.contentType,
			})
			if err != nil {
				t.Fatalf("ProcessPhoto replacement: %v", err)
			}
			if replaced.Status != "replaced" {
				t.Fatalf("replacement status = %q, want replaced", replaced.Status)
			}

			newFileID := photoFileID(t, pool, created.PhotoID)
			if newFileID == "" || newFileID == oldFileID {
				t.Fatalf("replacement file_id = %q, old %q; want new file", newFileID, oldFileID)
			}
			assertUserFileStatus(t, pool, userID, oldFileID, "archived", newFileID)
			assertUserFileStatus(t, pool, userID, newFileID, "active", "")
			if got := fileHashForUserFile(t, pool, userID, newFileID); got != sha256Hex(large) {
				t.Fatalf("replacement active file hash = %q, want %q", got, sha256Hex(large))
			}

			var width, height int
			if err := pool.QueryRow(ctx,
				`SELECT COALESCE(width, 0), COALESCE(height, 0) FROM activity_photos WHERE id = $1`,
				created.PhotoID,
			).Scan(&width, &height); err != nil {
				t.Fatalf("select replacement dimensions: %v", err)
			}
			if width != 48 || height != 48 {
				t.Fatalf("replacement dimensions = %dx%d, want 48x48", width, height)
			}
		})
	}
}

func TestProcessPhotoPhashDuplicateAddsMissingExifWithoutWritingOriginal(t *testing.T) {
	ctx := context.Background()
	svc, pool, userID, activityID := newPhotoServiceStorageFixture(t)
	setPhotoPhashThreshold(t, pool, userID, 64)

	takenAt := time.Date(2026, 5, 12, 10, 30, 0, 0, time.UTC)
	first := jpegPhotoTestBytes(t, 24, 24, color.RGBA{R: 20, G: 180, B: 90, A: 255}, 80)
	created, err := svc.ProcessPhoto(ctx, userID, activityID, first, PhotoMetadata{
		OriginalFilename: "timestamp.jpg",
		ContentType:      "image/jpeg",
		TakenAt:          &takenAt,
	})
	if err != nil {
		t.Fatalf("ProcessPhoto created: %v", err)
	}
	if created.Status != "created" {
		t.Fatalf("first ProcessPhoto status = %q, want created", created.Status)
	}
	beforeFileID := photoFileID(t, pool, created.PhotoID)

	lat, lng := 47.1234, 11.5678
	second := jpegPhotoTestBytes(t, 24, 24, color.RGBA{R: 20, G: 180, B: 90, A: 255}, 95)
	duplicate, err := svc.ProcessPhoto(ctx, userID, activityID, second, PhotoMetadata{
		OriginalFilename: "gps.jpg",
		ContentType:      "image/jpeg",
		Lat:              &lat,
		Lng:              &lng,
		LocationSource:   "exif",
	})
	if err != nil {
		t.Fatalf("ProcessPhoto gps duplicate: %v", err)
	}
	if duplicate.Status != "duplicate_phash" {
		t.Fatalf("gps duplicate status = %q, want duplicate_phash", duplicate.Status)
	}
	if got := activeMediaCount(t, pool, userID); got != 1 {
		t.Fatalf("active media user_files after EXIF duplicate = %d, want 1", got)
	}
	if got := photoFileID(t, pool, created.PhotoID); got != beforeFileID {
		t.Fatalf("EXIF duplicate changed file_id from %q to %q", beforeFileID, got)
	}

	var gotTakenAt time.Time
	var gotLat, gotLng float64
	var source string
	if err := pool.QueryRow(ctx, `
		SELECT taken_at, lat, lng, COALESCE(location_source, '')
		FROM activity_photos WHERE id = $1 AND user_id = $2`,
		created.PhotoID, userID,
	).Scan(&gotTakenAt, &gotLat, &gotLng, &source); err != nil {
		t.Fatalf("select enriched photo metadata: %v", err)
	}
	if !gotTakenAt.Equal(takenAt) {
		t.Fatalf("taken_at = %s, want %s", gotTakenAt, takenAt)
	}
	if gotLat != lat || gotLng != lng {
		t.Fatalf("gps = (%f,%f), want (%f,%f)", gotLat, gotLng, lat, lng)
	}
	if source != "exif" {
		t.Fatalf("location_source = %q, want exif", source)
	}
}

func TestProcessPhotoPhashDuplicateAddsMissingTimestampWithoutOverwritingGps(t *testing.T) {
	ctx := context.Background()
	svc, pool, userID, activityID := newPhotoServiceStorageFixture(t)
	setPhotoPhashThreshold(t, pool, userID, 64)

	lat, lng := 48.1111, 12.2222
	first := jpegPhotoTestBytes(t, 24, 24, color.RGBA{R: 190, G: 90, B: 30, A: 255}, 80)
	created, err := svc.ProcessPhoto(ctx, userID, activityID, first, PhotoMetadata{
		OriginalFilename: "gps-first.jpg",
		ContentType:      "image/jpeg",
		Lat:              &lat,
		Lng:              &lng,
		LocationSource:   "exif",
	})
	if err != nil {
		t.Fatalf("ProcessPhoto created: %v", err)
	}
	if created.Status != "created" {
		t.Fatalf("first ProcessPhoto status = %q, want created", created.Status)
	}

	takenAt := time.Date(2026, 5, 12, 12, 15, 0, 0, time.UTC)
	otherLat, otherLng := 49.3333, 13.4444
	second := jpegPhotoTestBytes(t, 24, 24, color.RGBA{R: 190, G: 90, B: 30, A: 255}, 95)
	duplicate, err := svc.ProcessPhoto(ctx, userID, activityID, second, PhotoMetadata{
		OriginalFilename: "timestamp-second.jpg",
		ContentType:      "image/jpeg",
		TakenAt:          &takenAt,
		Lat:              &otherLat,
		Lng:              &otherLng,
		LocationSource:   "manual",
	})
	if err != nil {
		t.Fatalf("ProcessPhoto timestamp duplicate: %v", err)
	}
	if duplicate.Status != "duplicate_phash" {
		t.Fatalf("timestamp duplicate status = %q, want duplicate_phash", duplicate.Status)
	}
	if got := activeMediaCount(t, pool, userID); got != 1 {
		t.Fatalf("active media user_files after timestamp duplicate = %d, want 1", got)
	}

	var gotTakenAt time.Time
	var gotLat, gotLng float64
	var source string
	if err := pool.QueryRow(ctx, `
		SELECT taken_at, lat, lng, COALESCE(location_source, '')
		FROM activity_photos WHERE id = $1 AND user_id = $2`,
		created.PhotoID, userID,
	).Scan(&gotTakenAt, &gotLat, &gotLng, &source); err != nil {
		t.Fatalf("select enriched photo metadata: %v", err)
	}
	if !gotTakenAt.Equal(takenAt) {
		t.Fatalf("taken_at = %s, want %s", gotTakenAt, takenAt)
	}
	if gotLat != lat || gotLng != lng {
		t.Fatalf("gps was overwritten to (%f,%f), want original (%f,%f)", gotLat, gotLng, lat, lng)
	}
	if source != "exif" {
		t.Fatalf("location_source = %q, want original exif", source)
	}
}

func newPhotoServiceStorageFixture(t *testing.T) (*Service, *pgxpool.Pool, string, string) {
	t.Helper()
	pool := testdb.Open(t)
	db := &database.DB{Pool: pool}
	userID := insertImporterPhotoTestUser(t, pool)
	activityID := insertImporterPhotoTestActivity(t, db, userID)
	dir := t.TempDir()
	fs := shadowfs.NewUnifiedShadowFS(pool, shadowfs.NewLocalBackend(dir), nil, "local", dir, shadowfs.NewSchemaRenderer(), nil, 1, 0, "", "", "")
	return NewService(db, fs, nil), pool, userID, activityID
}

func insertImporterPhotoTestUser(t *testing.T, pool *pgxpool.Pool) string {
	t.Helper()
	var userID string
	err := pool.QueryRow(context.Background(), `
		INSERT INTO users (id, username, email, email_verified, registration_source)
		VALUES (gen_random_uuid(), $1, $2, true, 'test')
		RETURNING id`,
		"photo-test-"+uuid.NewString(), "photo-test-"+uuid.NewString()+"@example.com",
	).Scan(&userID)
	if err != nil {
		t.Fatalf("insert user: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, userID)
	})
	return userID
}

func insertImporterPhotoTestActivity(t *testing.T, db *database.DB, userID string) string {
	t.Helper()
	now := time.Now().UTC().Truncate(time.Second)
	act, err := db.InsertActivity(context.Background(), userID, &model.ActivityData{
		SportType: "cycling",
		StartedAt: now,
		EndedAt:   now.Add(30 * time.Minute),
		FileHash:  uuid.NewString(),
	})
	if err != nil {
		t.Fatalf("insert activity: %v", err)
	}
	return act.ID
}

func setPhotoPhashThreshold(t *testing.T, pool *pgxpool.Pool, userID string, threshold int) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), `
		INSERT INTO user_enrichment_settings (user_id, phash_threshold_bits)
		VALUES ($1, $2)
		ON CONFLICT (user_id) DO UPDATE SET phash_threshold_bits = EXCLUDED.phash_threshold_bits`,
		userID, threshold,
	); err != nil {
		t.Fatalf("set phash threshold: %v", err)
	}
}

func jpegPhotoTestBytes(t *testing.T, width, height int, c color.RGBA, quality int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	draw.Draw(img, img.Bounds(), &image.Uniform{C: c}, image.Point{}, draw.Src)
	return encodeJPEGImage(t, img, quality)
}

func photoPatternImage(width, height int) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			r := uint8(40 + (x * 150 / max(width-1, 1)))
			g := uint8(60 + (y * 130 / max(height-1, 1)))
			b := uint8(180 - ((x + y) * 90 / max(width+height-2, 1)))
			if x > width/3 && x < 2*width/3 && y > height/3 && y < 2*height/3 {
				r, g, b = 230, 80, 40
			}
			img.Set(x, y, color.RGBA{R: r, G: g, B: b, A: 255})
		}
	}
	return img
}

func encodeJPEGImage(t *testing.T, img image.Image, quality int) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: quality}); err != nil {
		t.Fatalf("jpeg encode: %v", err)
	}
	return buf.Bytes()
}

func encodePNGImage(t *testing.T, img image.Image) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("png encode: %v", err)
	}
	return buf.Bytes()
}

func encodeImageWithFFmpegOrSkip(t *testing.T, input []byte, inputExt, outputExt string) []byte {
	t.Helper()
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skipf("ffmpeg not available: %v", err)
	}
	dir := t.TempDir()
	in := filepath.Join(dir, "input"+inputExt)
	out := filepath.Join(dir, "output"+outputExt)
	if err := os.WriteFile(in, input, 0o600); err != nil {
		t.Fatalf("write ffmpeg input: %v", err)
	}
	cmd := exec.Command("ffmpeg", "-hide_banner", "-loglevel", "error", "-y", "-i", in, "-frames:v", "1", out)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Skipf("ffmpeg cannot encode %s: %v: %s", outputExt, err, string(output))
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read ffmpeg output: %v", err)
	}
	if len(data) == 0 {
		t.Skipf("ffmpeg produced empty %s fixture", outputExt)
	}
	return data
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func photoFileID(t *testing.T, pool *pgxpool.Pool, photoID string) string {
	t.Helper()
	var fileID string
	if err := pool.QueryRow(context.Background(),
		`SELECT COALESCE(file_id::text, '') FROM activity_photos WHERE id = $1`,
		photoID,
	).Scan(&fileID); err != nil {
		t.Fatalf("select photo file_id: %v", err)
	}
	return fileID
}

func activeMediaCount(t *testing.T, pool *pgxpool.Pool, userID string) int {
	t.Helper()
	var count int
	if err := pool.QueryRow(context.Background(), `
		SELECT COUNT(*) FROM user_files
		WHERE user_id = $1 AND file_type = 'media' AND status = 'active'`,
		userID,
	).Scan(&count); err != nil {
		t.Fatalf("count active media: %v", err)
	}
	return count
}

func activityPhotoRowCount(t *testing.T, pool *pgxpool.Pool, userID string) int {
	t.Helper()
	var count int
	if err := pool.QueryRow(context.Background(), `
		SELECT COUNT(*) FROM activity_photos
		WHERE user_id = $1`,
		userID,
	).Scan(&count); err != nil {
		t.Fatalf("count activity_photos: %v", err)
	}
	return count
}

func assertUserFileStatus(t *testing.T, pool *pgxpool.Pool, userID, fileID, wantStatus, wantReplacedBy string) {
	t.Helper()
	var status, replacedBy string
	if err := pool.QueryRow(context.Background(), `
		SELECT status, COALESCE(replaced_by::text, '')
		FROM user_files
		WHERE id = $1 AND user_id = $2`,
		fileID, userID,
	).Scan(&status, &replacedBy); err != nil {
		t.Fatalf("select user_file %s: %v", fileID, err)
	}
	if status != wantStatus {
		t.Fatalf("user_file %s status = %q, want %q", fileID, status, wantStatus)
	}
	if replacedBy != wantReplacedBy {
		t.Fatalf("user_file %s replaced_by = %q, want %q", fileID, replacedBy, wantReplacedBy)
	}
}

func fileHashForUserFile(t *testing.T, pool *pgxpool.Pool, userID, fileID string) string {
	t.Helper()
	var hash string
	if err := pool.QueryRow(context.Background(), `
		SELECT COALESCE(file_hash, '')
		FROM user_files
		WHERE id = $1 AND user_id = $2`,
		fileID, userID,
	).Scan(&hash); err != nil {
		t.Fatalf("select user_file hash: %v", err)
	}
	return hash
}
