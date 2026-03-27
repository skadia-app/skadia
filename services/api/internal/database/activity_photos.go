package database

import (
	"context"
	"fmt"
	"regexp"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/model"
)

// clipVecLiteralRe validates pgvector literal format: '[float,float,...]'
var clipVecLiteralRe = regexp.MustCompile(`^\[-?[0-9]+(\.[0-9]+)?(e[+-]?[0-9]+)?(,-?[0-9]+(\.[0-9]+)?(e[+-]?[0-9]+)?)*\]$`)

func (db *DB) InsertActivityPhoto(ctx context.Context, p model.ActivityPhoto) (model.ActivityPhoto, error) {
	var out model.ActivityPhoto
	err := db.Pool.QueryRow(ctx, `
		INSERT INTO activity_photos
			(user_id, activity_id, file_id,
			 original_filename, content_type, file_size_bytes, file_hash,
			 taken_at, lat, lng, location_source,
			 is_video, duration_s, video_width, video_height,
			 phash, width, height)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18)
		RETURNING id, user_id, activity_id, file_id,
		          original_filename, content_type, file_size_bytes, file_hash,
		          taken_at, lat, lng, location_source, created_at,
		          is_video, duration_s, video_width, video_height,
		          phash, width, height
	`, p.UserID, p.ActivityID, p.FileID,
		p.OriginalFilename, p.ContentType, p.FileSizeBytes, p.FileHash,
		p.TakenAt, p.Lat, p.Lng, p.LocationSource,
		p.IsVideo, p.DurationS, p.VideoWidth, p.VideoHeight,
		p.PHash, p.Width, p.Height,
	).Scan(&out.ID, &out.UserID, &out.ActivityID, &out.FileID,
		&out.OriginalFilename, &out.ContentType, &out.FileSizeBytes, &out.FileHash,
		&out.TakenAt, &out.Lat, &out.Lng, &out.LocationSource, &out.CreatedAt,
		&out.IsVideo, &out.DurationS, &out.VideoWidth, &out.VideoHeight,
		&out.PHash, &out.Width, &out.Height)
	return out, err
}

func (db *DB) ListActivityPhotos(ctx context.Context, activityID, userID string) ([]model.ActivityPhoto, error) {
	rows, err := db.Pool.Query(ctx, `
		SELECT id, user_id, activity_id,
		       original_filename, content_type, file_size_bytes, file_hash,
		       taken_at, lat, lng, location_source, created_at,
		       is_video, duration_s, video_width, video_height
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
		if err := rows.Scan(&p.ID, &p.UserID, &p.ActivityID,
			&p.OriginalFilename, &p.ContentType, &p.FileSizeBytes, &p.FileHash,
			&p.TakenAt, &p.Lat, &p.Lng, &p.LocationSource, &p.CreatedAt,
			&p.IsVideo, &p.DurationS, &p.VideoWidth, &p.VideoHeight); err != nil {
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
	var fileID string
	err := db.Pool.QueryRow(ctx, `
		SELECT id, user_id, activity_id, COALESCE(file_id::text, ''),
		       original_filename, content_type, file_size_bytes, file_hash,
		       taken_at, lat, lng, location_source, created_at,
		       is_video, duration_s, video_width, video_height
		FROM activity_photos WHERE id = $1 AND user_id = $2
	`, id, userID).Scan(&p.ID, &p.UserID, &p.ActivityID, &fileID,
		&p.OriginalFilename, &p.ContentType, &p.FileSizeBytes, &p.FileHash,
		&p.TakenAt, &p.Lat, &p.Lng, &p.LocationSource, &p.CreatedAt,
		&p.IsVideo, &p.DurationS, &p.VideoWidth, &p.VideoHeight)
	if fileID != "" {
		p.FileID = &fileID
	}
	return p, err
}

func (db *DB) UpdateActivityPhotoLocation(ctx context.Context, id, userID string, lat, lng *float64, takenAt *time.Time, locationSource string) (model.ActivityPhoto, error) {
	var p model.ActivityPhoto
	err := db.Pool.QueryRow(ctx, `
		UPDATE activity_photos
		SET lat = $3, lng = $4, taken_at = COALESCE($5, taken_at), location_source = $6
		WHERE id = $1 AND user_id = $2
		RETURNING id, user_id, activity_id,
		          original_filename, content_type, file_size_bytes, file_hash,
		          taken_at, lat, lng, location_source, created_at,
		          is_video, duration_s, video_width, video_height
	`, id, userID, lat, lng, takenAt, locationSource,
	).Scan(&p.ID, &p.UserID, &p.ActivityID,
		&p.OriginalFilename, &p.ContentType, &p.FileSizeBytes, &p.FileHash,
		&p.TakenAt, &p.Lat, &p.Lng, &p.LocationSource, &p.CreatedAt,
		&p.IsVideo, &p.DurationS, &p.VideoWidth, &p.VideoHeight)
	return p, err
}

func (db *DB) DeleteActivityPhoto(ctx context.Context, id, userID string) (fileID string, err error) {
	err = db.Pool.QueryRow(ctx,
		`DELETE FROM activity_photos WHERE id = $1 AND user_id = $2
		 RETURNING COALESCE(file_id::text, '')`,
		id, userID,
	).Scan(&fileID)
	return
}

// ListAllUserPhotos returns all photos for a user (used by recompute-previews job).
func (db *DB) ListAllUserPhotos(ctx context.Context, userID string) ([]model.ActivityPhoto, error) {
	rows, err := db.Pool.Query(ctx, `
		SELECT id, user_id, activity_id, file_id,
		       original_filename, content_type, file_size_bytes, file_hash,
		       taken_at, lat, lng, location_source, created_at,
		       is_video, duration_s, video_width, video_height
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
		if err := rows.Scan(&p.ID, &p.UserID, &p.ActivityID, &p.FileID,
			&p.OriginalFilename, &p.ContentType, &p.FileSizeBytes, &p.FileHash,
			&p.TakenAt, &p.Lat, &p.Lng, &p.LocationSource, &p.CreatedAt,
			&p.IsVideo, &p.DurationS, &p.VideoWidth, &p.VideoHeight); err != nil {
			return nil, err
		}
		photos = append(photos, p)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return photos, nil
}

// GetPhotosWithoutEmbedding returns id and file_id for all photos of a user
// that have no CLIP embedding yet. The ML service uses file_id to locate the photo.
func (db *DB) GetPhotosWithoutEmbedding(ctx context.Context, userID string) ([]model.ActivityPhoto, error) {
	rows, err := db.Pool.Query(ctx, `
		SELECT id, COALESCE(file_id::text, '') AS file_id
		FROM activity_photos
		WHERE user_id = $1 AND clip_embedding IS NULL
		ORDER BY created_at
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	photos := make([]model.ActivityPhoto, 0)
	for rows.Next() {
		var p model.ActivityPhoto
		var fileID string
		if err := rows.Scan(&p.ID, &fileID); err != nil {
			return nil, err
		}
		if fileID != "" {
			p.FileID = &fileID
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

// ActivityPhotoExistsByHashAndActivity checks whether the same photo (by hash) already
// exists for a specific activity. This allows the same photo in different activities.
func (db *DB) ActivityPhotoExistsByHashAndActivity(ctx context.Context, userID, hash, activityID string) (bool, error) {
	var exists bool
	err := db.Pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM activity_photos WHERE user_id = $1 AND file_hash = $2 AND activity_id = $3)`,
		userID, hash, activityID,
	).Scan(&exists)
	return exists, err
}

// PhotoFeedRow ist ein internes Scan-Ziel für ListPhotoFeed.
type PhotoFeedRow struct {
	Photo             model.ActivityPhoto
	ActID             *string
	ActTitle          *string
	ActSportType      *string
	ActStartedAt      *time.Time
	ActDistanceM      *float64
	ActMovingTimeS    *int
	ActElevationGainM *float64
	ActTrackGeoJSON   *string
	ActLat            *float64
	ActLng            *float64
}

// ListPhotoFeed gibt paginierte Fotos zurück mit zugehörigen Aktivitätsdaten.
// sort: "date" = nach Aktivitätsdatum DESC, "added" = nach created_at DESC.
func (db *DB) ListPhotoFeed(ctx context.Context, userID, sort string, limit, offset int) ([]PhotoFeedRow, int, error) {
	var orderExpr string
	if sort == "added" {
		orderExpr = "ap.created_at DESC"
	} else {
		// Aktivitäts-Fotos (activity_id NOT NULL) zuerst, danach nach Datum DESC.
		// Innerhalb einer Aktivität: nach taken_at sortiert (chronologisch).
		// Verlinkte Fotos ohne Aktivitäts-Match fallen ans Ende (NULLS LAST).
		orderExpr = `
			CASE WHEN ap.activity_id IS NOT NULL AND a.id IS NOT NULL THEN 0 ELSE 1 END,
			COALESCE(a.started_at, ap.taken_at, ap.created_at) DESC,
			ap.activity_id NULLS LAST,
			COALESCE(ap.taken_at, ap.created_at)`
	}

	q := `
		SELECT
			ap.id, ap.activity_id, ap.taken_at, ap.created_at,
			ap.lat, ap.lng, ap.location_source,
			ap.content_type, ap.is_video, ap.duration_s,
			a.id              AS act_id,
			a.title           AS act_title,
			a.sport_type      AS act_sport_type,
			a.started_at      AS act_started_at,
			a.distance_m      AS act_distance_m,
			a.moving_time_s   AS act_moving_time_s,
			a.elevation_gain_m AS act_elevation_gain_m,
			ST_AsGeoJSON(at.track_simplified) AS act_track_geojson,
			a.location_lat    AS act_lat,
			a.location_lng    AS act_lng
		FROM activity_photos ap
		LEFT JOIN activities a ON ap.activity_id = a.id AND a.user_id = ap.user_id
		LEFT JOIN activity_tracks at ON at.activity_id = a.id
		WHERE ap.user_id = $1
		ORDER BY ` + orderExpr + `
		LIMIT $2 OFFSET $3
	`

	rows, err := db.Pool.Query(ctx, q, userID, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var result []PhotoFeedRow
	for rows.Next() {
		var r PhotoFeedRow
		if err := rows.Scan(
			&r.Photo.ID, &r.Photo.ActivityID, &r.Photo.TakenAt, &r.Photo.CreatedAt,
			&r.Photo.Lat, &r.Photo.Lng, &r.Photo.LocationSource,
			&r.Photo.ContentType, &r.Photo.IsVideo, &r.Photo.DurationS,
			&r.ActID, &r.ActTitle, &r.ActSportType,
			&r.ActStartedAt, &r.ActDistanceM, &r.ActMovingTimeS, &r.ActElevationGainM,
			&r.ActTrackGeoJSON, &r.ActLat, &r.ActLng,
		); err != nil {
			return nil, 0, err
		}
		result = append(result, r)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}

	var total int
	if err := db.Pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM activity_photos WHERE user_id = $1`, userID,
	).Scan(&total); err != nil {
		return nil, 0, err
	}

	return result, total, nil
}

// SearchPhotos führt kombinierte CLIP-Vektor- und Titelsuche durch.
// clipVecLiteral: L2-normierter 1152-dim Vektor als pgvector literal string (z.B. "'[0.1,...]'::vector"), leer = nur Titelsuche.
// titleQuery: Suchbegriff für ILIKE, leer = nur CLIP.
func (db *DB) SearchPhotos(ctx context.Context, userID string, clipVecLiteral, titleQuery string, limit int) ([]model.PhotoSearchResult, error) {
	var results []model.PhotoSearchResult
	seen := map[string]bool{}

	if clipVecLiteral != "" {
		if !clipVecLiteralRe.MatchString(clipVecLiteral) {
			return nil, fmt.Errorf("SearchPhotos: invalid clip_embedding literal format")
		}
		rows, err := db.Pool.Query(ctx, `
			SELECT
				ap.id, ap.activity_id, ap.taken_at, ap.created_at,
				ap.lat, ap.lng, ap.location_source,
				ap.content_type, ap.is_video, ap.duration_s,
				1 - (ap.clip_embedding <=> `+clipVecLiteral+`::vector) AS similarity,
				a.id, a.title, a.sport_type, a.started_at,
				a.distance_m, a.moving_time_s, a.elevation_gain_m,
				NULL::text AS act_track_geojson,
				a.location_lat, a.location_lng
			FROM activity_photos ap
			LEFT JOIN activities a ON ap.activity_id = a.id AND a.user_id = ap.user_id
			WHERE ap.user_id = $1 AND ap.clip_embedding IS NOT NULL
			ORDER BY ap.clip_embedding <=> `+clipVecLiteral+`::vector
			LIMIT $2
		`, userID, limit)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		for rows.Next() {
			var r model.PhotoSearchResult
			var actID, actTitle, actSportType, actTrackGeoJSON *string
			var actStartedAt *time.Time
			var actDistM, actElevM, actLat, actLng *float64
			var actMovS *int
			if err := rows.Scan(
				&r.Photo.ID, &r.Photo.ActivityID, &r.Photo.TakenAt, &r.Photo.CreatedAt,
				&r.Photo.Lat, &r.Photo.Lng, &r.Photo.LocationSource,
				&r.Photo.ContentType, &r.Photo.IsVideo, &r.Photo.DurationS,
				&r.Similarity,
				&actID, &actTitle, &actSportType, &actStartedAt,
				&actDistM, &actMovS, &actElevM, &actTrackGeoJSON, &actLat, &actLng,
			); err != nil {
				return nil, err
			}
			r.MatchType = "clip"
			r.Activity = buildPhotoActivitySummary(actID, actTitle, actSportType, actStartedAt, actDistM, actMovS, actElevM, actTrackGeoJSON, actLat, actLng)
			results = append(results, r)
			seen[r.Photo.ID] = true
		}
		if err := rows.Err(); err != nil {
			return nil, err
		}
	}

	if titleQuery != "" {
		rows, err := db.Pool.Query(ctx, `
			SELECT
				ap.id, ap.activity_id, ap.taken_at, ap.created_at,
				ap.lat, ap.lng, ap.location_source,
				ap.content_type, ap.is_video, ap.duration_s,
				a.id, a.title, a.sport_type, a.started_at,
				a.distance_m, a.moving_time_s, a.elevation_gain_m,
				NULL::text AS act_track_geojson,
				a.location_lat, a.location_lng
			FROM activity_photos ap
			JOIN activities a ON ap.activity_id = a.id AND a.user_id = ap.user_id
			WHERE ap.user_id = $1 AND a.title ILIKE '%' || $2 || '%'
			LIMIT $3
		`, userID, titleQuery, limit)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		for rows.Next() {
			var r model.PhotoSearchResult
			var actID, actTitle, actSportType, actTrackGeoJSON *string
			var actStartedAt *time.Time
			var actDistM, actElevM, actLat, actLng *float64
			var actMovS *int
			if err := rows.Scan(
				&r.Photo.ID, &r.Photo.ActivityID, &r.Photo.TakenAt, &r.Photo.CreatedAt,
				&r.Photo.Lat, &r.Photo.Lng, &r.Photo.LocationSource,
				&r.Photo.ContentType, &r.Photo.IsVideo, &r.Photo.DurationS,
				&actID, &actTitle, &actSportType, &actStartedAt,
				&actDistM, &actMovS, &actElevM, &actTrackGeoJSON, &actLat, &actLng,
			); err != nil {
				return nil, err
			}
			if seen[r.Photo.ID] {
				continue
			}
			r.MatchType = "title"
			r.Similarity = 1.0
			r.Activity = buildPhotoActivitySummary(actID, actTitle, actSportType, actStartedAt, actDistM, actMovS, actElevM, actTrackGeoJSON, actLat, actLng)
			results = append(results, r)
		}
		if err := rows.Err(); err != nil {
			return nil, err
		}
	}

	return results, nil
}

func buildPhotoActivitySummary(id, title, sportType *string, startedAt *time.Time, distM *float64, movS *int, elevM *float64, trackGeoJSON *string, lat, lng *float64) *model.PhotoFeedActivitySummary {
	if id == nil {
		return nil
	}
	s := &model.PhotoFeedActivitySummary{
		ID:             *id,
		TrackGeoJSON:   trackGeoJSON,
		Lat:            lat,
		Lng:            lng,
		DistanceM:      distM,
		MovingTimeS:    movS,
		ElevationGainM: elevM,
	}
	if title != nil {
		s.Title = *title
	}
	if sportType != nil {
		s.SportType = *sportType
	}
	if startedAt != nil {
		t := startedAt.Format(time.RFC3339)
		s.StartedAt = &t
	}
	return s
}

// PhotoHashRow holds pHash comparison data for a single photo.
type PhotoHashRow struct {
	ID     string
	PHash  *int64
	Width  *int
	Height *int
}

// GetActivityPhotoHashes returns photo IDs and pHashes for an activity (for pHash comparison).
func (db *DB) GetActivityPhotoHashes(ctx context.Context, activityID string) ([]PhotoHashRow, error) {
	rows, err := db.Pool.Query(ctx,
		`SELECT id, phash, width, height
		 FROM activity_photos
		 WHERE activity_id = $1 AND phash IS NOT NULL`,
		activityID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []PhotoHashRow
	for rows.Next() {
		var r PhotoHashRow
		if err := rows.Scan(&r.ID, &r.PHash, &r.Width, &r.Height); err != nil {
			return nil, err
		}
		result = append(result, r)
	}
	return result, rows.Err()
}

// UpdateActivityPhotoPHash updates the pHash for a photo.
func (db *DB) UpdateActivityPhotoPHash(ctx context.Context, photoID string, phash int64) error {
	_, err := db.Pool.Exec(ctx,
		`UPDATE activity_photos SET phash = $1 WHERE id = $2`,
		phash, photoID)
	return err
}

// UpdateActivityPhotoDimensions updates width and height.
func (db *DB) UpdateActivityPhotoDimensions(ctx context.Context, photoID string, width, height int) error {
	_, err := db.Pool.Exec(ctx,
		`UPDATE activity_photos SET width = $1, height = $2 WHERE id = $3`,
		width, height, photoID)
	return err
}

// ThumbnailBackfillRow holds data for thumbnail backfill processing.
type ThumbnailBackfillRow struct {
	FileID      string // user_files ID of the original media file
	ParentID    string // photo ID (parent_id in user_files)
	UserID      string
	ContentType string
	FileName    string
}

// PhotosWithoutThumb returns user_files media entries with parent_type='photo'
// that have no corresponding thumb entry in user_files, across ALL users.
// Used by the admin thumbnail backfill endpoint.
func (db *DB) PhotosWithoutThumb(ctx context.Context) ([]ThumbnailBackfillRow, error) {
	rows, err := db.Pool.Query(ctx, `
		SELECT uf.id, uf.parent_id, uf.user_id, uf.content_type, uf.file_name
		FROM user_files uf
		WHERE uf.file_type = 'media'
		  AND uf.parent_type = 'photo'
		  AND uf.status = 'active'
		  AND NOT EXISTS (
			SELECT 1 FROM user_files t
			WHERE t.parent_type = uf.parent_type
			  AND t.parent_id = uf.parent_id
			  AND t.file_type = 'thumb'
			  AND t.status = 'active'
		  )
		ORDER BY uf.created_at
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []ThumbnailBackfillRow
	for rows.Next() {
		var r ThumbnailBackfillRow
		if err := rows.Scan(&r.FileID, &r.ParentID, &r.UserID, &r.ContentType, &r.FileName); err != nil {
			return nil, err
		}
		result = append(result, r)
	}
	return result, rows.Err()
}

// BackfillPhotoRow holds data for pHash backfill processing.
type BackfillPhotoRow struct {
	ID        string
	IsVideo   bool
	DurationS *float64
	Width     *int
	Height    *int
	FileID    string // user_files registry ID (ShadowFS)
}

// PhotosWithoutPHash returns all photo IDs that need pHash or dimension backfill.
func (db *DB) PhotosWithoutPHash(ctx context.Context, userID string) ([]BackfillPhotoRow, error) {
	rows, err := db.Pool.Query(ctx,
		`SELECT id, is_video, duration_s, width, height, COALESCE(file_id::text, '')
		 FROM activity_photos
		 WHERE user_id = $1 AND (phash IS NULL OR width IS NULL)
		 ORDER BY created_at`,
		userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []BackfillPhotoRow
	for rows.Next() {
		var r BackfillPhotoRow
		if err := rows.Scan(&r.ID, &r.IsVideo, &r.DurationS, &r.Width, &r.Height, &r.FileID); err != nil {
			return nil, err
		}
		result = append(result, r)
	}
	return result, rows.Err()
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
