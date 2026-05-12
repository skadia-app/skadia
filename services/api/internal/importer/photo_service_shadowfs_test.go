package importer

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/database/testdb"
	"github.com/skadia-app/skadia/services/api/internal/event"
	"github.com/skadia-app/skadia/services/api/internal/model"
	"github.com/skadia-app/skadia/services/api/internal/shadowfs"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestProcessPhotoReplacementStoresNewOriginalAndArchivesOld(t *testing.T) {
	ctx := context.Background()
	svc, pool, userID, activityID := newPhotoServiceStorageFixture(t)
	setPhotoPhashThreshold(t, pool, userID, 64)

	small := encodeJPEGImage(t, photoPatternImage(300, 300), 90)
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

	large := encodeJPEGImage(t, photoPatternImage(600, 600), 90)
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

func TestProcessPhotoReplacementNetsOldOriginalForQuota(t *testing.T) {
	ctx := context.Background()
	pool := testdb.Open(t)
	db := &database.DB{Pool: pool}
	userID := insertImporterPhotoTestUser(t, pool)
	activityID := insertImporterPhotoTestActivity(t, db, userID)
	setPhotoPhashThreshold(t, pool, userID, 64)

	small := encodeJPEGImage(t, photoPatternImage(120, 120), 90)
	large := encodeJPEGImage(t, photoPatternImage(240, 240), 90)
	dir := t.TempDir()
	fs := shadowfs.NewUnifiedShadowFS(pool, shadowfs.NewLocalBackend(dir), nil, "local", dir, shadowfs.NewSchemaRenderer(), nil, 1, int64(len(large)+1), "", "", "")
	svc := NewService(db, fs, nil)

	created, err := svc.ProcessPhoto(ctx, userID, activityID, small, PhotoMetadata{
		OriginalFilename: "quota-small.jpg",
		ContentType:      "image/jpeg",
	})
	if err != nil {
		t.Fatalf("ProcessPhoto created: %v", err)
	}
	oldFileID := photoFileID(t, pool, created.PhotoID)

	replaced, err := svc.ProcessPhoto(ctx, userID, activityID, large, PhotoMetadata{
		OriginalFilename: "quota-large.jpg",
		ContentType:      "image/jpeg",
	})
	if err != nil {
		t.Fatalf("ProcessPhoto replacement should net old original before quota check: %v", err)
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
	if got := activeMediaCount(t, pool, userID); got != 1 {
		t.Fatalf("active media after quota-netted replacement = %d, want 1", got)
	}
}

func TestProcessPhotoReplacementRefreshesDerivativesAndContentMetadata(t *testing.T) {
	ctx := context.Background()
	svc, pool, userID, activityID := newPhotoServiceStorageFixture(t)
	publisher := &recordingPhotoEmbedPublisher{}
	svc.PhotoEmbedPublisher = publisher
	setPhotoPhashThreshold(t, pool, userID, 64)

	small := encodeJPEGImage(t, photoPatternImage(2600, 2600), 90)
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
	oldThumbID := activePhotoDerivedFileID(t, pool, userID, created.PhotoID, "thumb")
	oldPreviewID := activePhotoDerivedFileID(t, pool, userID, created.PhotoID, "preview")
	setPhotoClipEmbedding(t, pool, created.PhotoID)

	large := encodePNGImage(t, photoPatternImage(2800, 2800))
	replaced, err := svc.ProcessPhoto(ctx, userID, activityID, large, PhotoMetadata{
		OriginalFilename: "large.png",
		ContentType:      "image/png",
	})
	if err != nil {
		t.Fatalf("ProcessPhoto replaced: %v", err)
	}
	if replaced.Status != "replaced" {
		t.Fatalf("replacement status = %q, want replaced", replaced.Status)
	}

	var filename, contentType string
	var clipIsNull bool
	if err := pool.QueryRow(ctx, `
		SELECT original_filename, content_type, clip_embedding IS NULL
		FROM activity_photos
		WHERE id = $1`,
		created.PhotoID,
	).Scan(&filename, &contentType, &clipIsNull); err != nil {
		t.Fatalf("select replacement metadata: %v", err)
	}
	if filename != "large.png" {
		t.Fatalf("original_filename = %q, want large.png", filename)
	}
	if contentType != "image/png" {
		t.Fatalf("content_type = %q, want image/png", contentType)
	}
	if !clipIsNull {
		t.Fatal("clip_embedding was not cleared after replacing original bytes")
	}

	newThumbID := activePhotoDerivedFileID(t, pool, userID, created.PhotoID, "thumb")
	newPreviewID := activePhotoDerivedFileID(t, pool, userID, created.PhotoID, "preview")
	if newThumbID == "" || newPreviewID == "" {
		t.Fatalf("replacement missing derivatives: thumb=%q preview=%q", newThumbID, newPreviewID)
	}
	if newThumbID == oldThumbID {
		t.Fatalf("replacement kept old thumb %q", oldThumbID)
	}
	if newPreviewID == oldPreviewID {
		t.Fatalf("replacement kept old preview %q", oldPreviewID)
	}
	assertUserFileStatus(t, pool, userID, oldThumbID, "archived", newThumbID)
	assertUserFileStatus(t, pool, userID, oldPreviewID, "archived", newPreviewID)

	if len(publisher.events) != 2 {
		t.Fatalf("photo embed events = %d, want 2", len(publisher.events))
	}
	last := publisher.events[len(publisher.events)-1]
	if last.PhotoID != created.PhotoID || last.FileID != photoFileID(t, pool, created.PhotoID) || last.ContentType != "image/png" {
		t.Fatalf("replacement embed event = %#v", last)
	}
}

func TestReplaceActivityPhotoVideoOriginalUsesProbedMetadata(t *testing.T) {
	ctx := context.Background()
	svc, pool, userID, activityID := newPhotoServiceStorageFixture(t)
	setPhotoPhashThreshold(t, pool, userID, 64)

	small := encodeJPEGImage(t, photoPatternImage(2600, 2600), 90)
	created, err := svc.ProcessPhoto(ctx, userID, activityID, small, PhotoMetadata{
		OriginalFilename: "small.jpg",
		ContentType:      "image/jpeg",
	})
	if err != nil {
		t.Fatalf("ProcessPhoto created: %v", err)
	}
	if created.Status != "created" {
		t.Fatalf("first status = %q, want created", created.Status)
	}
	oldThumbID := activePhotoDerivedFileID(t, pool, userID, created.PhotoID, "thumb")
	oldPreviewID := activePhotoDerivedFileID(t, pool, userID, created.PhotoID, "preview")
	if _, err := pool.Exec(ctx,
		`UPDATE activity_photos SET width = 64, height = 64 WHERE id = $1 AND user_id = $2`,
		created.PhotoID, userID,
	); err != nil {
		t.Fatalf("shrink stored dimensions for replacement fixture: %v", err)
	}

	bogusDuration := 999.0
	bogusWidth, bogusHeight := 999, 999
	large := generateTestVideoOrSkip(t, 128, 128, 2)
	replaced, err := svc.replaceActivityPhotoOriginal(ctx, replaceActivityPhotoOriginalRequest{
		userID:      userID,
		activityID:  activityID,
		photoID:     created.PhotoID,
		photoBytes:  large,
		contentType: "video/mp4",
		fileHash:    sha256Hex(large),
		pHash:       created.PHash,
		width:       128,
		height:      128,
		meta: PhotoMetadata{
			OriginalFilename: "large.mp4",
			ContentType:      "video/mp4",
			DurationS:        &bogusDuration,
			VideoWidth:       &bogusWidth,
			VideoHeight:      &bogusHeight,
		},
	})
	if err != nil {
		t.Fatalf("replaceActivityPhotoOriginal video: %v", err)
	}
	if replaced.Status != "replaced" {
		t.Fatalf("video replacement status = %q, want replaced", replaced.Status)
	}

	var duration float64
	var videoWidth, videoHeight int
	if err := pool.QueryRow(ctx, `
		SELECT COALESCE(duration_s, 0), COALESCE(video_width, 0), COALESCE(video_height, 0)
		FROM activity_photos
		WHERE id = $1 AND user_id = $2`,
		created.PhotoID, userID,
	).Scan(&duration, &videoWidth, &videoHeight); err != nil {
		t.Fatalf("select video replacement metadata: %v", err)
	}
	if duration <= 0 || duration >= 10 {
		t.Fatalf("duration_s = %f, want probed short video duration", duration)
	}
	if videoWidth != 128 || videoHeight != 128 {
		t.Fatalf("video dimensions = %dx%d, want probed 128x128", videoWidth, videoHeight)
	}

	newThumbID := activePhotoDerivedFileID(t, pool, userID, created.PhotoID, "thumb")
	newPreviewID := activePhotoDerivedFileID(t, pool, userID, created.PhotoID, "preview")
	if newThumbID == "" || newPreviewID == "" {
		t.Fatalf("video replacement missing derivatives: thumb=%q preview=%q", newThumbID, newPreviewID)
	}
	if newThumbID == oldThumbID || newPreviewID == oldPreviewID {
		t.Fatalf("video replacement did not refresh derivatives: old thumb=%q new thumb=%q old preview=%q new preview=%q", oldThumbID, newThumbID, oldPreviewID, newPreviewID)
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

func TestProcessPhotoExactDuplicateIsIsolatedAcrossUsers(t *testing.T) {
	ctx := context.Background()
	svc, pool, userID, activityID := newPhotoServiceStorageFixture(t)
	otherUserID := insertImporterPhotoTestUser(t, pool)
	otherActivityID := insertImporterPhotoTestActivity(t, svc.DB, otherUserID)

	body := jpegPhotoTestBytes(t, 24, 24, color.RGBA{R: 15, G: 110, B: 210, A: 255}, 90)
	created, err := svc.ProcessPhoto(ctx, userID, activityID, body, PhotoMetadata{
		OriginalFilename: "user-a.jpg",
		ContentType:      "image/jpeg",
	})
	if err != nil {
		t.Fatalf("ProcessPhoto user A: %v", err)
	}
	other, err := svc.ProcessPhoto(ctx, otherUserID, otherActivityID, body, PhotoMetadata{
		OriginalFilename: "user-b.jpg",
		ContentType:      "image/jpeg",
	})
	if err != nil {
		t.Fatalf("ProcessPhoto user B: %v", err)
	}
	if created.Status != "created" || other.Status != "created" {
		t.Fatalf("statuses = %q/%q, want created/created", created.Status, other.Status)
	}
	if created.PhotoID == other.PhotoID {
		t.Fatalf("cross-user exact duplicate reused photo id %q", created.PhotoID)
	}
	if got := activeMediaCount(t, pool, userID); got != 1 {
		t.Fatalf("user A active media = %d, want 1", got)
	}
	if got := activeMediaCount(t, pool, otherUserID); got != 1 {
		t.Fatalf("user B active media = %d, want 1", got)
	}
}

func TestProcessPhotoDefaultDoesNotEnforceInteractiveLimit(t *testing.T) {
	ctx := context.Background()
	svc, _, userID, activityID := newPhotoServiceStorageFixture(t)
	svc.ActivityPhotoCreateGuard = func(context.Context, string, string) (func(), error) {
		return nil, &ActivityPhotoLimitError{ActivityID: activityID, Limit: 0, Current: 0}
	}

	body := jpegPhotoTestBytes(t, 24, 24, color.RGBA{R: 20, G: 80, B: 210, A: 255}, 90)
	created, err := svc.ProcessPhoto(ctx, userID, activityID, body, PhotoMetadata{
		OriginalFilename: "external-default.jpg",
		ContentType:      "image/jpeg",
	})
	if err != nil {
		t.Fatalf("ProcessPhoto default should not enforce interactive limit: %v", err)
	}
	if created.Status != "created" {
		t.Fatalf("ProcessPhoto default status = %q, want created", created.Status)
	}

	limitedSvc, _, limitedUserID, limitedActivityID := newPhotoServiceStorageFixture(t)
	limitedSvc.ActivityPhotoCreateGuard = func(context.Context, string, string) (func(), error) {
		return nil, &ActivityPhotoLimitError{ActivityID: limitedActivityID, Limit: 0, Current: 0}
	}
	_, err = limitedSvc.ProcessPhotoWithOptions(ctx, limitedUserID, limitedActivityID, jpegPhotoTestBytes(t, 25, 25, color.RGBA{R: 210, G: 80, B: 20, A: 255}, 90), PhotoMetadata{
		OriginalFilename: "interactive.jpg",
		ContentType:      "image/jpeg",
	}, PhotoProcessOptions{EnforceActivityPhotoLimit: true})
	if !errors.Is(err, ErrActivityPhotoLimitExceeded) {
		t.Fatalf("ProcessPhotoWithOptions err=%v, want ErrActivityPhotoLimitExceeded", err)
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

func TestProcessPhotoExactDuplicateReattachesLegacyNullActivityPhoto(t *testing.T) {
	ctx := context.Background()
	svc, pool, userID, activityID := newPhotoServiceStorageFixture(t)

	body := jpegPhotoTestBytes(t, 24, 24, color.RGBA{R: 40, G: 160, B: 90, A: 255}, 90)
	created, err := svc.ProcessPhoto(ctx, userID, activityID, body, PhotoMetadata{
		OriginalFilename: "legacy-exact.jpg",
		ContentType:      "image/jpeg",
	})
	if err != nil {
		t.Fatalf("ProcessPhoto created: %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE activity_photos SET activity_id = NULL WHERE id = $1`, created.PhotoID); err != nil {
		t.Fatalf("detach legacy exact photo: %v", err)
	}

	targetActivityID := insertImporterPhotoTestActivity(t, svc.DB, userID)
	duplicate, err := svc.ProcessPhoto(ctx, userID, targetActivityID, body, PhotoMetadata{
		OriginalFilename: "target-exact.jpg",
		ContentType:      "image/jpeg",
	})
	if err != nil {
		t.Fatalf("ProcessPhoto duplicate: %v", err)
	}
	if duplicate.Status != "duplicate_exact" {
		t.Fatalf("duplicate status = %q, want duplicate_exact", duplicate.Status)
	}
	if duplicate.ActivityID != targetActivityID {
		t.Fatalf("duplicate activity = %q, want %q", duplicate.ActivityID, targetActivityID)
	}
	var gotActivityID string
	if err := pool.QueryRow(ctx,
		`SELECT COALESCE(activity_id::text, '') FROM activity_photos WHERE id = $1`,
		created.PhotoID,
	).Scan(&gotActivityID); err != nil {
		t.Fatalf("select reattached activity: %v", err)
	}
	if gotActivityID != targetActivityID {
		t.Fatalf("stored activity_id = %q, want %q", gotActivityID, targetActivityID)
	}
}

func TestProcessPhotoExactDuplicateReattachHonorsInteractiveLimit(t *testing.T) {
	ctx := context.Background()
	svc, pool, userID, activityID := newPhotoServiceStorageFixture(t)

	body := jpegPhotoTestBytes(t, 24, 24, color.RGBA{R: 60, G: 120, B: 200, A: 255}, 90)
	created, err := svc.ProcessPhoto(ctx, userID, activityID, body, PhotoMetadata{
		OriginalFilename: "legacy-limit.jpg",
		ContentType:      "image/jpeg",
	})
	if err != nil {
		t.Fatalf("ProcessPhoto created: %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE activity_photos SET activity_id = NULL WHERE id = $1`, created.PhotoID); err != nil {
		t.Fatalf("detach legacy exact photo: %v", err)
	}

	targetActivityID := insertImporterPhotoTestActivity(t, svc.DB, userID)
	svc.ActivityPhotoCreateGuard = func(context.Context, string, string) (func(), error) {
		return nil, &ActivityPhotoLimitError{ActivityID: targetActivityID, Limit: 1, Current: 1}
	}
	_, err = svc.ProcessPhotoWithOptions(ctx, userID, targetActivityID, body, PhotoMetadata{
		OriginalFilename: "target-limit.jpg",
		ContentType:      "image/jpeg",
	}, PhotoProcessOptions{EnforceActivityPhotoLimit: true})
	if !errors.Is(err, ErrActivityPhotoLimitExceeded) {
		t.Fatalf("reattach err=%v, want ErrActivityPhotoLimitExceeded", err)
	}

	var gotActivityID string
	if err := pool.QueryRow(ctx,
		`SELECT COALESCE(activity_id::text, '') FROM activity_photos WHERE id = $1`,
		created.PhotoID,
	).Scan(&gotActivityID); err != nil {
		t.Fatalf("select activity_id: %v", err)
	}
	if gotActivityID != "" {
		t.Fatalf("activity_id after rejected reattach = %q, want empty", gotActivityID)
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

func TestProcessPhotoPhashDuplicateIsIsolatedAcrossUsers(t *testing.T) {
	ctx := context.Background()
	svc, pool, userID, activityID := newPhotoServiceStorageFixture(t)
	otherUserID := insertImporterPhotoTestUser(t, pool)
	otherActivityID := insertImporterPhotoTestActivity(t, svc.DB, otherUserID)
	setPhotoPhashThreshold(t, pool, userID, 64)
	setPhotoPhashThreshold(t, pool, otherUserID, 64)

	src := photoPatternImage(48, 48)
	first := encodeJPEGImage(t, src, 95)
	created, err := svc.ProcessPhoto(ctx, userID, activityID, first, PhotoMetadata{
		OriginalFilename: "user-a.jpg",
		ContentType:      "image/jpeg",
	})
	if err != nil {
		t.Fatalf("ProcessPhoto user A: %v", err)
	}

	second := encodePNGImage(t, src)
	other, err := svc.ProcessPhoto(ctx, otherUserID, otherActivityID, second, PhotoMetadata{
		OriginalFilename: "user-b.png",
		ContentType:      "image/png",
	})
	if err != nil {
		t.Fatalf("ProcessPhoto user B: %v", err)
	}
	if created.Status != "created" || other.Status != "created" {
		t.Fatalf("statuses = %q/%q, want created/created", created.Status, other.Status)
	}
	if created.PhotoID == other.PhotoID {
		t.Fatalf("cross-user phash duplicate reused photo id %q", created.PhotoID)
	}
	if got := activityPhotoRowCount(t, pool, userID); got != 1 {
		t.Fatalf("user A activity_photos = %d, want 1", got)
	}
	if got := activityPhotoRowCount(t, pool, otherUserID); got != 1 {
		t.Fatalf("user B activity_photos = %d, want 1", got)
	}
}

func TestProcessPhotoIgnoresLegacyNullActivityPhashMatches(t *testing.T) {
	ctx := context.Background()
	svc, pool, userID, activityID := newPhotoServiceStorageFixture(t)
	setPhotoPhashThreshold(t, pool, userID, 64)

	body := encodeJPEGImage(t, photoPatternImage(48, 48), 95)
	created, err := svc.ProcessPhoto(ctx, userID, activityID, body, PhotoMetadata{
		OriginalFilename: "legacy.jpg",
		ContentType:      "image/jpeg",
	})
	if err != nil {
		t.Fatalf("ProcessPhoto created: %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE activity_photos SET activity_id = NULL WHERE id = $1`, created.PhotoID); err != nil {
		t.Fatalf("detach legacy photo: %v", err)
	}

	targetActivityID := insertImporterPhotoTestActivity(t, svc.DB, userID)
	reupload := encodePNGImage(t, photoPatternImage(48, 48))
	result, err := svc.ProcessPhoto(ctx, userID, targetActivityID, reupload, PhotoMetadata{
		OriginalFilename: "target.png",
		ContentType:      "image/png",
	})
	if err != nil {
		t.Fatalf("ProcessPhoto reupload: %v", err)
	}
	if result.Status != "created" {
		t.Fatalf("reupload status = %q, want created", result.Status)
	}
	if result.ActivityID != targetActivityID {
		t.Fatalf("result activity = %q, want %q", result.ActivityID, targetActivityID)
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

func TestProcessPhotoPhashDuplicateAddsMissingLngAndLocationSource(t *testing.T) {
	ctx := context.Background()
	svc, pool, userID, activityID := newPhotoServiceStorageFixture(t)
	setPhotoPhashThreshold(t, pool, userID, 64)

	lat := 48.1111
	first := jpegPhotoTestBytes(t, 24, 24, color.RGBA{R: 110, G: 90, B: 180, A: 255}, 80)
	created, err := svc.ProcessPhoto(ctx, userID, activityID, first, PhotoMetadata{
		OriginalFilename: "lat-first.jpg",
		ContentType:      "image/jpeg",
		Lat:              &lat,
	})
	if err != nil {
		t.Fatalf("ProcessPhoto created: %v", err)
	}

	otherLat, lng := 49.9999, 12.2222
	second := jpegPhotoTestBytes(t, 24, 24, color.RGBA{R: 110, G: 90, B: 180, A: 255}, 95)
	duplicate, err := svc.ProcessPhoto(ctx, userID, activityID, second, PhotoMetadata{
		OriginalFilename: "lng-source-second.jpg",
		ContentType:      "image/jpeg",
		Lat:              &otherLat,
		Lng:              &lng,
		LocationSource:   "exif",
	})
	if err != nil {
		t.Fatalf("ProcessPhoto lng/source duplicate: %v", err)
	}
	if duplicate.Status != "duplicate_phash" {
		t.Fatalf("duplicate status = %q, want duplicate_phash", duplicate.Status)
	}
	if got := activeMediaCount(t, pool, userID); got != 1 {
		t.Fatalf("active media user_files after lng/source duplicate = %d, want 1", got)
	}

	var gotLat, gotLng float64
	var source string
	if err := pool.QueryRow(ctx, `
		SELECT lat, lng, COALESCE(location_source, '')
		FROM activity_photos
		WHERE id = $1 AND user_id = $2`,
		created.PhotoID, userID,
	).Scan(&gotLat, &gotLng, &source); err != nil {
		t.Fatalf("select enriched photo metadata: %v", err)
	}
	if gotLat != lat {
		t.Fatalf("lat was overwritten to %f, want original %f", gotLat, lat)
	}
	if gotLng != lng {
		t.Fatalf("lng = %f, want %f", gotLng, lng)
	}
	if source != "exif" {
		t.Fatalf("location_source = %q, want exif", source)
	}
}

func TestResolveQueuedPhotoPreservesQueuedExifMetadata(t *testing.T) {
	ctx := context.Background()
	svc, pool, userID, activityID := newPhotoServiceStorageFixture(t)

	takenAt := time.Now().UTC().Add(48 * time.Hour).Truncate(time.Second)
	lat, lng := 47.123, 11.456
	body := jpegPhotoTestBytes(t, 24, 24, color.RGBA{R: 20, G: 140, B: 90, A: 255}, 90)

	queued, err := svc.ProcessOrphanPhoto(ctx, userID, body, PhotoMetadata{
		OriginalFilename: "queued-exif.jpg",
		ContentType:      "image/jpeg",
		TakenAt:          &takenAt,
		Lat:              &lat,
		Lng:              &lng,
		LocationSource:   "exif",
	})
	if err != nil {
		t.Fatalf("ProcessOrphanPhoto queued: %v", err)
	}
	if queued.Status != "queued" {
		t.Fatalf("orphan status = %q, want queued", queued.Status)
	}

	item, err := svc.DB.GetImportQueueItem(ctx, queued.QueueEntryID, userID)
	if err != nil {
		t.Fatalf("get queued item: %v", err)
	}
	if item.StorageBackend == nil || *item.StorageBackend != "local" {
		t.Fatalf("storage_backend = %v, want local", item.StorageBackend)
	}
	if item.StoragePath != nil {
		t.Fatalf("storage_path = %q, want nil for ShadowFS-backed queue item", *item.StoragePath)
	}
	if item.Metadata["taken_at"] == "" || item.Metadata["lat"] == nil || item.Metadata["lng"] == nil || item.Metadata["location_source"] != "exif" {
		t.Fatalf("queue metadata missing EXIF fields: %#v", item.Metadata)
	}

	resolved, err := svc.ResolveQueuedPhoto(ctx, userID, queued.QueueEntryID, activityID)
	if err != nil {
		t.Fatalf("ResolveQueuedPhoto: %v", err)
	}
	if resolved.Status != "created" {
		t.Fatalf("resolve status = %q, want created", resolved.Status)
	}

	var gotTakenAt time.Time
	var gotLat, gotLng float64
	var source string
	if err := pool.QueryRow(ctx, `
		SELECT taken_at, lat, lng, COALESCE(location_source, '')
		FROM activity_photos
		WHERE id = $1`,
		resolved.PhotoID,
	).Scan(&gotTakenAt, &gotLat, &gotLng, &source); err != nil {
		t.Fatalf("select resolved photo metadata: %v", err)
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

	item, err = svc.DB.GetImportQueueItem(ctx, queued.QueueEntryID, userID)
	if err != nil {
		t.Fatalf("get resolved item: %v", err)
	}
	if item.Status != "resolved" {
		t.Fatalf("queue status = %q, want resolved", item.Status)
	}
	if activeQueuedMediaCount(t, pool, userID) != 0 {
		t.Fatalf("queued media files still active after resolve")
	}
}

func TestResolveQueuedPhotoCleansAssignedPhotoWhenQueueResolveFails(t *testing.T) {
	ctx := context.Background()
	svc, pool, userID, activityID := newPhotoServiceStorageFixture(t)

	body := jpegPhotoTestBytes(t, 24, 24, color.RGBA{R: 210, G: 120, B: 40, A: 255}, 90)
	queued, err := svc.ProcessOrphanPhoto(ctx, userID, body, PhotoMetadata{
		OriginalFilename: "resolve-fail.jpg",
		ContentType:      "image/jpeg",
	})
	if err != nil {
		t.Fatalf("ProcessOrphanPhoto queued: %v", err)
	}

	installFailingImportQueueResolveTrigger(t, pool, queued.QueueEntryID)
	_, err = svc.ResolveQueuedPhoto(ctx, userID, queued.QueueEntryID, activityID)
	if err == nil {
		t.Fatal("ResolveQueuedPhoto err = nil, want resolve failure")
	}

	item, itemErr := svc.DB.GetImportQueueItem(ctx, queued.QueueEntryID, userID)
	if itemErr != nil {
		t.Fatalf("get failed queue item: %v", itemErr)
	}
	if item.Status != "pending" {
		t.Fatalf("queue status after failed resolve = %q, want pending", item.Status)
	}
	if got := activeQueuedMediaCount(t, pool, userID); got != 1 {
		t.Fatalf("active queued media after failed resolve = %d, want 1", got)
	}
	if got := activeMediaCount(t, pool, userID); got != 1 {
		t.Fatalf("all active media after failed resolve = %d, want only queued original", got)
	}
	if got := activityPhotoRowCount(t, pool, userID); got != 0 {
		t.Fatalf("activity_photos rows after failed resolve = %d, want 0", got)
	}
}

func TestResolveQueuedPhotoRestoresExactDuplicateReattachWhenQueueResolveFails(t *testing.T) {
	ctx := context.Background()
	svc, pool, userID, sourceActivityID := newPhotoServiceStorageFixture(t)
	targetActivityID := insertImporterPhotoTestActivity(t, svc.DB, userID)

	body := jpegPhotoTestBytes(t, 24, 24, color.RGBA{R: 120, G: 160, B: 70, A: 255}, 90)
	created, err := svc.ProcessPhoto(ctx, userID, sourceActivityID, body, PhotoMetadata{
		OriginalFilename: "legacy-queued-exact.jpg",
		ContentType:      "image/jpeg",
	})
	if err != nil {
		t.Fatalf("ProcessPhoto legacy exact: %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE activity_photos SET activity_id = NULL WHERE id = $1`, created.PhotoID); err != nil {
		t.Fatalf("detach legacy exact photo: %v", err)
	}

	queued, err := svc.ProcessOrphanPhoto(ctx, userID, body, PhotoMetadata{
		OriginalFilename: "queued-exact-duplicate.jpg",
		ContentType:      "image/jpeg",
	})
	if err != nil {
		t.Fatalf("ProcessOrphanPhoto queued exact duplicate: %v", err)
	}
	installFailingImportQueueResolveTrigger(t, pool, queued.QueueEntryID)

	_, err = svc.ResolveQueuedPhoto(ctx, userID, queued.QueueEntryID, targetActivityID)
	if err == nil {
		t.Fatal("ResolveQueuedPhoto err = nil, want resolve failure")
	}

	var gotActivityID string
	if err := pool.QueryRow(ctx,
		`SELECT COALESCE(activity_id::text, '') FROM activity_photos WHERE id = $1`,
		created.PhotoID,
	).Scan(&gotActivityID); err != nil {
		t.Fatalf("select restored exact duplicate activity_id: %v", err)
	}
	if gotActivityID != "" {
		t.Fatalf("activity_id after failed exact duplicate resolve = %q, want empty", gotActivityID)
	}
	if got := activityPhotoRowCount(t, pool, userID); got != 1 {
		t.Fatalf("activity_photos rows after failed exact duplicate resolve = %d, want 1", got)
	}
	if got := activeMediaCount(t, pool, userID); got != 2 {
		t.Fatalf("active media after failed exact duplicate resolve = %d, want existing original + queued original", got)
	}
}

func TestResolveQueuedPhotoRestoresDuplicateEnrichmentWhenQueueResolveFails(t *testing.T) {
	ctx := context.Background()
	svc, pool, userID, activityID := newPhotoServiceStorageFixture(t)
	setPhotoPhashThreshold(t, pool, userID, 64)

	lat := 48.1111
	first := jpegPhotoTestBytes(t, 24, 24, color.RGBA{R: 120, G: 80, B: 180, A: 255}, 85)
	created, err := svc.ProcessPhoto(ctx, userID, activityID, first, PhotoMetadata{
		OriginalFilename: "existing.jpg",
		ContentType:      "image/jpeg",
		Lat:              &lat,
	})
	if err != nil {
		t.Fatalf("ProcessPhoto created: %v", err)
	}

	lng := 12.2222
	second := jpegPhotoTestBytes(t, 24, 24, color.RGBA{R: 120, G: 80, B: 180, A: 255}, 95)
	queued, err := svc.ProcessOrphanPhoto(ctx, userID, second, PhotoMetadata{
		OriginalFilename: "queued-duplicate.jpg",
		ContentType:      "image/jpeg",
		Lng:              &lng,
		LocationSource:   "exif",
	})
	if err != nil {
		t.Fatalf("ProcessOrphanPhoto queued duplicate: %v", err)
	}
	installFailingImportQueueResolveTrigger(t, pool, queued.QueueEntryID)

	_, err = svc.ResolveQueuedPhoto(ctx, userID, queued.QueueEntryID, activityID)
	if err == nil {
		t.Fatal("ResolveQueuedPhoto err = nil, want resolve failure")
	}

	var gotLat *float64
	var gotLng *float64
	var gotSource *string
	if err := pool.QueryRow(ctx, `
		SELECT lat, lng, location_source
		FROM activity_photos
		WHERE id = $1 AND user_id = $2`,
		created.PhotoID, userID,
	).Scan(&gotLat, &gotLng, &gotSource); err != nil {
		t.Fatalf("select restored duplicate metadata: %v", err)
	}
	if gotLat == nil || *gotLat != lat {
		t.Fatalf("lat = %v, want %f", gotLat, lat)
	}
	if gotLng != nil {
		t.Fatalf("lng survived failed queue resolve = %v, want nil", *gotLng)
	}
	if gotSource != nil {
		t.Fatalf("location_source survived failed queue resolve = %q, want nil", *gotSource)
	}
}

func TestResolveQueuedPhotoRestoresReplacementWhenQueueResolveFails(t *testing.T) {
	ctx := context.Background()
	svc, pool, userID, activityID := newPhotoServiceStorageFixture(t)
	setPhotoPhashThreshold(t, pool, userID, 64)

	small := encodeJPEGImage(t, photoPatternImage(120, 120), 90)
	created, err := svc.ProcessPhoto(ctx, userID, activityID, small, PhotoMetadata{
		OriginalFilename: "existing-small.jpg",
		ContentType:      "image/jpeg",
	})
	if err != nil {
		t.Fatalf("ProcessPhoto created: %v", err)
	}
	oldFileID := photoFileID(t, pool, created.PhotoID)

	large := encodeJPEGImage(t, photoPatternImage(240, 240), 90)
	queued, err := svc.ProcessOrphanPhoto(ctx, userID, large, PhotoMetadata{
		OriginalFilename: "queued-large.jpg",
		ContentType:      "image/jpeg",
	})
	if err != nil {
		t.Fatalf("ProcessOrphanPhoto queued replacement: %v", err)
	}
	installFailingImportQueueResolveTrigger(t, pool, queued.QueueEntryID)

	_, err = svc.ResolveQueuedPhoto(ctx, userID, queued.QueueEntryID, activityID)
	if err == nil {
		t.Fatal("ResolveQueuedPhoto err = nil, want resolve failure")
	}

	currentFileID := photoFileID(t, pool, created.PhotoID)
	if currentFileID != oldFileID {
		t.Fatalf("photo file_id after failed queue replacement = %q, want original %q", currentFileID, oldFileID)
	}
	assertUserFileStatus(t, pool, userID, oldFileID, "active", "")
	if got := activeMediaCount(t, pool, userID); got != 2 {
		t.Fatalf("active media after failed queue replacement = %d, want existing original + queued original", got)
	}
}

func TestResolveQueuedPhotoNetsQueuedFileForQuota(t *testing.T) {
	ctx := context.Background()
	pool := testdb.Open(t)
	db := &database.DB{Pool: pool}
	userID := insertImporterPhotoTestUser(t, pool)
	activityID := insertImporterPhotoTestActivity(t, db, userID)
	body := jpegPhotoTestBytes(t, 24, 24, color.RGBA{R: 90, G: 120, B: 210, A: 255}, 90)
	dir := t.TempDir()
	fs := shadowfs.NewUnifiedShadowFS(pool, shadowfs.NewLocalBackend(dir), nil, "local", dir, shadowfs.NewSchemaRenderer(), nil, 1, int64(len(body)+1), "", "", "")
	svc := NewService(db, fs, nil)

	queued, err := svc.ProcessOrphanPhoto(ctx, userID, body, PhotoMetadata{
		OriginalFilename: "quota-queued.jpg",
		ContentType:      "image/jpeg",
	})
	if err != nil {
		t.Fatalf("ProcessOrphanPhoto queued: %v", err)
	}

	resolved, err := svc.ResolveQueuedPhotoWithOptions(ctx, userID, queued.QueueEntryID, activityID, PhotoProcessOptions{
		EnforceActivityPhotoLimit: true,
	})
	if err != nil {
		t.Fatalf("ResolveQueuedPhoto should net queued file before quota check: %v", err)
	}
	if resolved.Status != "created" {
		t.Fatalf("resolve status = %q, want created", resolved.Status)
	}
	if got := activeQueuedMediaCount(t, pool, userID); got != 0 {
		t.Fatalf("queued media files still active after resolve = %d", got)
	}
	if got := activeMediaCount(t, pool, userID); got != 1 {
		t.Fatalf("active media after resolve = %d, want 1", got)
	}
}

func TestProcessOrphanPhotoMatchesActivityByExifTime(t *testing.T) {
	ctx := context.Background()
	svc, _, userID, activityID := newPhotoServiceStorageFixture(t)

	takenAt := time.Now().UTC().Truncate(time.Second)
	body := jpegPhotoTestBytes(t, 24, 24, color.RGBA{R: 90, G: 20, B: 180, A: 255}, 90)
	result, err := svc.ProcessOrphanPhoto(ctx, userID, body, PhotoMetadata{
		OriginalFilename: "matched.jpg",
		ContentType:      "image/jpeg",
		TakenAt:          &takenAt,
	})
	if err != nil {
		t.Fatalf("ProcessOrphanPhoto: %v", err)
	}
	if result.Status != "matched" {
		t.Fatalf("orphan status = %q, want matched", result.Status)
	}
	if result.ActivityID != activityID {
		t.Fatalf("matched activity = %q, want %q", result.ActivityID, activityID)
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

func generateTestVideoOrSkip(t *testing.T, width, height int, durationSeconds int) []byte {
	t.Helper()
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skipf("ffmpeg not available: %v", err)
	}
	dir := t.TempDir()
	out := filepath.Join(dir, "video.mp4")
	size := strconv.Itoa(width) + "x" + strconv.Itoa(height)
	cmd := exec.Command("ffmpeg",
		"-hide_banner", "-loglevel", "error", "-y",
		"-f", "lavfi",
		"-i", "testsrc=size="+size+":rate=10:duration="+strconv.Itoa(durationSeconds),
		"-pix_fmt", "yuv420p",
		out,
	)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Skipf("ffmpeg cannot create mp4 fixture: %v: %s", err, string(output))
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read video fixture: %v", err)
	}
	if len(data) == 0 {
		t.Skip("ffmpeg produced empty mp4 fixture")
	}
	return data
}

func installFailingImportQueueResolveTrigger(t *testing.T, pool *pgxpool.Pool, queueEntryID string) {
	t.Helper()
	triggerName := "trg_fail_import_queue_resolve_" + strings.ReplaceAll(uuid.NewString(), "-", "_")
	functionName := "fail_import_queue_resolve_" + strings.ReplaceAll(uuid.NewString(), "-", "_")
	if _, err := pool.Exec(context.Background(), `
		CREATE FUNCTION `+functionName+`() RETURNS trigger LANGUAGE plpgsql AS $$
		BEGIN
			IF NEW.id = '`+queueEntryID+`'::uuid AND NEW.status = 'resolved' THEN
				RAISE EXCEPTION 'forced queue resolve failure';
			END IF;
			RETURN NEW;
		END;
		$$;
		CREATE TRIGGER `+triggerName+`
			BEFORE UPDATE ON import_queue
			FOR EACH ROW EXECUTE FUNCTION `+functionName+`();
	`); err != nil {
		t.Fatalf("install failing import_queue trigger: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DROP TRIGGER IF EXISTS `+triggerName+` ON import_queue`)
		_, _ = pool.Exec(context.Background(), `DROP FUNCTION IF EXISTS `+functionName+`()`)
	})
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

func activePhotoDerivedFileID(t *testing.T, pool *pgxpool.Pool, userID, photoID, fileType string) string {
	t.Helper()
	var fileID string
	if err := pool.QueryRow(context.Background(), `
		SELECT COALESCE(id::text, '')
		FROM user_files
		WHERE user_id = $1 AND parent_type = 'photo' AND parent_id = $2
		  AND file_type = $3 AND status = 'active'
		ORDER BY created_at DESC
		LIMIT 1`,
		userID, photoID, fileType,
	).Scan(&fileID); err != nil {
		t.Fatalf("select active %s for photo: %v", fileType, err)
	}
	return fileID
}

func activeQueuedMediaCount(t *testing.T, pool *pgxpool.Pool, userID string) int {
	t.Helper()
	var count int
	if err := pool.QueryRow(context.Background(), `
		SELECT COUNT(*) FROM user_files
		WHERE user_id = $1 AND file_type = 'media' AND status = 'active'
		  AND parent_type IS NULL`,
		userID,
	).Scan(&count); err != nil {
		t.Fatalf("count queued media files: %v", err)
	}
	return count
}

func setPhotoClipEmbedding(t *testing.T, pool *pgxpool.Pool, photoID string) {
	t.Helper()
	zeros := "[" + strings.TrimRight(strings.Repeat("0,", 1152), ",") + "]"
	if _, err := pool.Exec(context.Background(),
		`UPDATE activity_photos SET clip_embedding = $1::vector WHERE id = $2`,
		zeros, photoID,
	); err != nil {
		t.Fatalf("set clip embedding: %v", err)
	}
}

type recordingPhotoEmbedPublisher struct {
	events []event.PhotoEmbedEvent
}

func (p *recordingPhotoEmbedPublisher) PublishPhotoEmbed(ev event.PhotoEmbedEvent) error {
	p.events = append(p.events, ev)
	return nil
}
