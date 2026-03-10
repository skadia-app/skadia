package database

import (
	"context"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/model"
)

func (db *DB) InsertActivityPhoto(ctx context.Context, p model.ActivityPhoto) (model.ActivityPhoto, error) {
	var out model.ActivityPhoto
	err := db.Pool.QueryRow(ctx, `
		INSERT INTO activity_photos
			(user_id, activity_id, s3_bucket, s3_key, s3_key_thumb, s3_key_preview,
			 original_filename, content_type, file_size_bytes, file_hash,
			 taken_at, lat, lng, location_source)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)
		RETURNING id, user_id, activity_id, s3_bucket, s3_key, s3_key_thumb, s3_key_preview,
		          original_filename, content_type, file_size_bytes, file_hash,
		          taken_at, lat, lng, location_source, created_at
	`, p.UserID, p.ActivityID, p.S3Bucket, p.S3Key, p.S3KeyThumb, p.S3KeyPreview,
		p.OriginalFilename, p.ContentType, p.FileSizeBytes, p.FileHash,
		p.TakenAt, p.Lat, p.Lng, p.LocationSource,
	).Scan(&out.ID, &out.UserID, &out.ActivityID, &out.S3Bucket, &out.S3Key, &out.S3KeyThumb, &out.S3KeyPreview,
		&out.OriginalFilename, &out.ContentType, &out.FileSizeBytes, &out.FileHash,
		&out.TakenAt, &out.Lat, &out.Lng, &out.LocationSource, &out.CreatedAt)
	return out, err
}

func (db *DB) ListActivityPhotos(ctx context.Context, activityID, userID string) ([]model.ActivityPhoto, error) {
	rows, err := db.Pool.Query(ctx, `
		SELECT id, user_id, activity_id, s3_bucket, s3_key, s3_key_thumb, s3_key_preview,
		       original_filename, content_type, file_size_bytes, file_hash,
		       taken_at, lat, lng, location_source, created_at
		FROM activity_photos
		WHERE activity_id = $1 AND user_id = $2
		ORDER BY COALESCE(taken_at, created_at)
	`, activityID, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	photos := make([]model.ActivityPhoto, 0)
	for rows.Next() {
		var p model.ActivityPhoto
		if err := rows.Scan(&p.ID, &p.UserID, &p.ActivityID, &p.S3Bucket, &p.S3Key, &p.S3KeyThumb, &p.S3KeyPreview,
			&p.OriginalFilename, &p.ContentType, &p.FileSizeBytes, &p.FileHash,
			&p.TakenAt, &p.Lat, &p.Lng, &p.LocationSource, &p.CreatedAt); err != nil {
			return nil, err
		}
		photos = append(photos, p)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return photos, nil
}

func (db *DB) GetActivityPhoto(ctx context.Context, id, userID string) (model.ActivityPhoto, error) {
	var p model.ActivityPhoto
	err := db.Pool.QueryRow(ctx, `
		SELECT id, user_id, activity_id, s3_bucket, s3_key, s3_key_thumb, s3_key_preview,
		       original_filename, content_type, file_size_bytes, file_hash,
		       taken_at, lat, lng, location_source, created_at
		FROM activity_photos WHERE id = $1 AND user_id = $2
	`, id, userID).Scan(&p.ID, &p.UserID, &p.ActivityID, &p.S3Bucket, &p.S3Key, &p.S3KeyThumb, &p.S3KeyPreview,
		&p.OriginalFilename, &p.ContentType, &p.FileSizeBytes, &p.FileHash,
		&p.TakenAt, &p.Lat, &p.Lng, &p.LocationSource, &p.CreatedAt)
	return p, err
}

func (db *DB) UpdateActivityPhotoLocation(ctx context.Context, id, userID string, lat, lng *float64, takenAt *time.Time, locationSource string) (model.ActivityPhoto, error) {
	var p model.ActivityPhoto
	err := db.Pool.QueryRow(ctx, `
		UPDATE activity_photos
		SET lat = $3, lng = $4, taken_at = COALESCE($5, taken_at), location_source = $6
		WHERE id = $1 AND user_id = $2
		RETURNING id, user_id, activity_id, s3_bucket, s3_key, s3_key_thumb, s3_key_preview,
		          original_filename, content_type, file_size_bytes, file_hash,
		          taken_at, lat, lng, location_source, created_at
	`, id, userID, lat, lng, takenAt, locationSource,
	).Scan(&p.ID, &p.UserID, &p.ActivityID, &p.S3Bucket, &p.S3Key, &p.S3KeyThumb, &p.S3KeyPreview,
		&p.OriginalFilename, &p.ContentType, &p.FileSizeBytes, &p.FileHash,
		&p.TakenAt, &p.Lat, &p.Lng, &p.LocationSource, &p.CreatedAt)
	return p, err
}

func (db *DB) DeleteActivityPhoto(ctx context.Context, id, userID string) (bucket, key, keyThumb, keyPreview string, err error) {
	err = db.Pool.QueryRow(ctx,
		`DELETE FROM activity_photos WHERE id = $1 AND user_id = $2 RETURNING s3_bucket, s3_key, s3_key_thumb, COALESCE(s3_key_preview, '')`,
		id, userID,
	).Scan(&bucket, &key, &keyThumb, &keyPreview)
	return
}

// UpdateActivityPhotoPreviewKey updates only s3_key_preview for a single photo.
func (db *DB) UpdateActivityPhotoPreviewKey(ctx context.Context, id, userID, previewKey string) error {
	_, err := db.Pool.Exec(ctx,
		`UPDATE activity_photos SET s3_key_preview = $3 WHERE id = $1 AND user_id = $2`,
		id, userID, previewKey,
	)
	return err
}

// ListAllUserPhotos returns all photos for a user (used by recompute-previews job).
func (db *DB) ListAllUserPhotos(ctx context.Context, userID string) ([]model.ActivityPhoto, error) {
	rows, err := db.Pool.Query(ctx, `
		SELECT id, user_id, activity_id, s3_bucket, s3_key, s3_key_thumb, s3_key_preview,
		       original_filename, content_type, file_size_bytes, file_hash,
		       taken_at, lat, lng, location_source, created_at
		FROM activity_photos WHERE user_id = $1
		ORDER BY created_at
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	photos := make([]model.ActivityPhoto, 0)
	for rows.Next() {
		var p model.ActivityPhoto
		if err := rows.Scan(&p.ID, &p.UserID, &p.ActivityID, &p.S3Bucket, &p.S3Key, &p.S3KeyThumb, &p.S3KeyPreview,
			&p.OriginalFilename, &p.ContentType, &p.FileSizeBytes, &p.FileHash,
			&p.TakenAt, &p.Lat, &p.Lng, &p.LocationSource, &p.CreatedAt); err != nil {
			return nil, err
		}
		photos = append(photos, p)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return photos, nil
}

func (db *DB) ActivityPhotoExistsByHash(ctx context.Context, userID, hash string) (bool, error) {
	var exists bool
	err := db.Pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM activity_photos WHERE user_id = $1 AND file_hash = $2)`,
		userID, hash,
	).Scan(&exists)
	return exists, err
}

func (db *DB) ListMapPhotos(ctx context.Context, userID string) ([]model.MapPhotoEntry, error) {
	rows, err := db.Pool.Query(ctx, `
		SELECT ap.id, ap.activity_id, a.title, ap.lat, ap.lng, ap.taken_at
		FROM activity_photos ap
		LEFT JOIN activities a ON a.id = ap.activity_id
		WHERE ap.user_id = $1 AND ap.lat IS NOT NULL AND ap.lng IS NOT NULL
		ORDER BY COALESCE(ap.taken_at, ap.created_at) DESC
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	entries := make([]model.MapPhotoEntry, 0)
	for rows.Next() {
		var e model.MapPhotoEntry
		if err := rows.Scan(&e.ID, &e.ActivityID, &e.ActivityTitle, &e.Lat, &e.Lng, &e.TakenAt); err != nil {
			return nil, err
		}
		entries = append(entries, e)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return entries, nil
}
