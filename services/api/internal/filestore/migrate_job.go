package filestore

import (
	"context"
	"log/slog"
	"os"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

// MigrateJob populates user_files from existing domain tables.
type MigrateJob struct {
	pool         *pgxpool.Pool
	shadowFSBase string // legacy ShadowFS base path
	s3Bucket     string // legacy S3 bucket name
}

// NewMigrateJob creates a new migration job.
func NewMigrateJob(pool *pgxpool.Pool, shadowFSBase, s3Bucket string) *MigrateJob {
	return &MigrateJob{pool: pool, shadowFSBase: shadowFSBase, s3Bucket: s3Bucket}
}

// Run executes the full registry population. It is idempotent: each row is
// skipped when a journal entry with status='done' already exists.
func (m *MigrateJob) Run(ctx context.Context, jobID string) error {
	slog.Info("filestore migration: starting registry population", "job_id", jobID)

	var totalMigrated int

	type step struct {
		name string
		fn   func(context.Context) (int, error)
	}
	steps := []step{
		{"activities", m.migrateActivities},
		{"activity_photos", m.migrateActivityPhotos},
		{"gear_documents", m.migrateGearDocuments},
		{"route_photos", m.migrateRoutePhotos},
		{"tags", m.migrateTags},
		{"custom_map_markers", m.migrateCustomMapMarkers},
	}

	for _, s := range steps {
		n, err := s.fn(ctx)
		totalMigrated += n
		if err != nil {
			slog.Error("filestore migration: step failed", "step", s.name, "error", err)
		} else {
			slog.Info("filestore migration: step done", "step", s.name, "migrated", n)
		}
		// Update progress in background_jobs
		m.pool.Exec(ctx, //nolint:errcheck
			`UPDATE background_jobs SET progress_current = progress_current + $1 WHERE id = $2`,
			int64(n), jobID,
		)
	}

	slog.Info("filestore migration: complete", "total_migrated", totalMigrated, "job_id", jobID)
	return nil
}

// ── activities ───────────────────────────────────────────────────────────────

// migrateActivities registers local_file_path and/or original_file_key for
// activities that do not yet have a file_id.
func (m *MigrateJob) migrateActivities(ctx context.Context) (int, error) {
	const pageSize = 500
	var migrated int

	for {
		type row struct {
			id              string
			userID          string
			localFilePath   *string
			originalFileKey *string
		}

		rows, err := m.pool.Query(ctx, `
			SELECT a.id, a.user_id, a.local_file_path, a.original_file_key
			FROM activities a
			WHERE a.file_id IS NULL
			  AND (a.local_file_path IS NOT NULL OR a.original_file_key IS NOT NULL)
			  AND NOT EXISTS (
			    SELECT 1 FROM migration_filestore_journal j
			    WHERE j.source_table = 'activities' AND j.source_id = a.id AND j.status = 'done'
			  )
			ORDER BY a.id
			LIMIT $1`, pageSize)
		if err != nil {
			return migrated, err
		}

		var batch []row
		for rows.Next() {
			var r row
			if err := rows.Scan(&r.id, &r.userID, &r.localFilePath, &r.originalFileKey); err != nil {
				rows.Close()
				return migrated, err
			}
			batch = append(batch, r)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return migrated, err
		}
		if len(batch) == 0 {
			break
		}

		for _, r := range batch {
			// Prefer local file if it physically exists.
			if r.localFilePath != nil && *r.localFilePath != "" {
				absPath := *r.localFilePath
				if m.shadowFSBase != "" && !strings.HasPrefix(absPath, "/") {
					absPath = m.shadowFSBase + "/" + absPath
				}
				if _, statErr := os.Stat(absPath); statErr == nil {
					if err := m.registerActivityLocal(ctx, r.id, r.userID, *r.localFilePath); err != nil {
						slog.Warn("filestore migration: activities local failed", "id", r.id, "error", err)
					} else {
						migrated++
					}
					continue
				}
			}
			// Fall back to S3 key.
			if r.originalFileKey != nil && *r.originalFileKey != "" {
				if err := m.registerActivityS3(ctx, r.id, r.userID, *r.originalFileKey); err != nil {
					slog.Warn("filestore migration: activities s3 failed", "id", r.id, "error", err)
				} else {
					migrated++
				}
			}
		}

		if len(batch) < pageSize {
			break
		}
	}

	return migrated, nil
}

func (m *MigrateJob) registerActivityLocal(ctx context.Context, id, userID, localPath string) error {
	tx, err := m.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	var fileID string
	err = tx.QueryRow(ctx, `
		INSERT INTO user_files (user_id, file_type, storage_backend, storage_path,
		                        content_type, file_name, parent_type, parent_id)
		VALUES ($1, 'activity', 'local', $2, 'application/octet-stream',
		        COALESCE(NULLIF(REGEXP_REPLACE($2, '^.*/', ''), ''), 'activity.fit'),
		        'activity', $3::uuid)
		ON CONFLICT (storage_backend, storage_path) DO UPDATE SET updated_at = NOW()
		RETURNING id`,
		userID, localPath, id,
	).Scan(&fileID)
	if err != nil {
		return err
	}

	if _, err = tx.Exec(ctx,
		`UPDATE activities SET file_id = $1 WHERE id = $2 AND file_id IS NULL`,
		fileID, id,
	); err != nil {
		return err
	}

	if _, err = tx.Exec(ctx, `
		INSERT INTO migration_filestore_journal (source_table, source_id, status, user_id, file_id, old_path)
		VALUES ('activities', $1, 'done', $2, $3, $4)
		ON CONFLICT (source_table, source_id, phase) DO UPDATE
		    SET status = 'done', file_id = EXCLUDED.file_id`,
		id, userID, fileID, localPath,
	); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

func (m *MigrateJob) registerActivityS3(ctx context.Context, id, userID, s3Key string) error {
	tx, err := m.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	storagePath := m.s3Path(s3Key)

	var fileID string
	err = tx.QueryRow(ctx, `
		INSERT INTO user_files (user_id, file_type, storage_backend, storage_path,
		                        content_type, file_name, parent_type, parent_id)
		VALUES ($1, 'activity', 's3', $2, 'application/octet-stream',
		        COALESCE(NULLIF(REGEXP_REPLACE($3, '^.*/', ''), ''), 'activity.fit'),
		        'activity', $4::uuid)
		ON CONFLICT (storage_backend, storage_path) DO UPDATE SET updated_at = NOW()
		RETURNING id`,
		userID, storagePath, s3Key, id,
	).Scan(&fileID)
	if err != nil {
		return err
	}

	if _, err = tx.Exec(ctx,
		`UPDATE activities SET file_id = $1 WHERE id = $2 AND file_id IS NULL`,
		fileID, id,
	); err != nil {
		return err
	}

	if _, err = tx.Exec(ctx, `
		INSERT INTO migration_filestore_journal (source_table, source_id, status, user_id, file_id, old_path)
		VALUES ('activities', $1, 'done', $2, $3, $4)
		ON CONFLICT (source_table, source_id, phase) DO UPDATE
		    SET status = 'done', file_id = EXCLUDED.file_id`,
		id, userID, fileID, s3Key,
	); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

// ── activity_photos ───────────────────────────────────────────────────────────

func (m *MigrateJob) migrateActivityPhotos(ctx context.Context) (int, error) {
	const pageSize = 500
	var migrated int

	for {
		type row struct {
			id       string
			userID   string
			s3Bucket string
			s3Key    string
		}

		rows, err := m.pool.Query(ctx, `
			SELECT p.id, p.user_id, p.s3_bucket, p.s3_key
			FROM activity_photos p
			WHERE p.file_id IS NULL
			  AND p.s3_key != ''
			  AND NOT EXISTS (
			    SELECT 1 FROM migration_filestore_journal j
			    WHERE j.source_table = 'activity_photos' AND j.source_id = p.id AND j.status = 'done'
			  )
			ORDER BY p.id
			LIMIT $1`, pageSize)
		if err != nil {
			return migrated, err
		}

		var batch []row
		for rows.Next() {
			var r row
			if err := rows.Scan(&r.id, &r.userID, &r.s3Bucket, &r.s3Key); err != nil {
				rows.Close()
				return migrated, err
			}
			batch = append(batch, r)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return migrated, err
		}
		if len(batch) == 0 {
			break
		}

		for _, r := range batch {
			if err := m.registerS3File(ctx, "activity_photos", r.id, r.userID,
				r.s3Key, "media", "file_id", "image/jpeg"); err != nil {
				slog.Warn("filestore migration: activity_photos failed", "id", r.id, "error", err)
			} else {
				migrated++
			}
		}

		if len(batch) < pageSize {
			break
		}
	}

	return migrated, nil
}

// ── gear_documents ────────────────────────────────────────────────────────────

func (m *MigrateJob) migrateGearDocuments(ctx context.Context) (int, error) {
	const pageSize = 500
	var migrated int

	for {
		type row struct {
			id       string
			userID   string
			s3Bucket string
			s3Key    string
		}

		rows, err := m.pool.Query(ctx, `
			SELECT d.id, d.user_id, d.s3_bucket, d.s3_key
			FROM gear_documents d
			WHERE d.file_id IS NULL
			  AND d.s3_key != ''
			  AND NOT EXISTS (
			    SELECT 1 FROM migration_filestore_journal j
			    WHERE j.source_table = 'gear_documents' AND j.source_id = d.id AND j.status = 'done'
			  )
			ORDER BY d.id
			LIMIT $1`, pageSize)
		if err != nil {
			return migrated, err
		}

		var batch []row
		for rows.Next() {
			var r row
			if err := rows.Scan(&r.id, &r.userID, &r.s3Bucket, &r.s3Key); err != nil {
				rows.Close()
				return migrated, err
			}
			batch = append(batch, r)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return migrated, err
		}
		if len(batch) == 0 {
			break
		}

		for _, r := range batch {
			if err := m.registerS3File(ctx, "gear_documents", r.id, r.userID,
				r.s3Key, "document", "file_id", "application/octet-stream"); err != nil {
				slog.Warn("filestore migration: gear_documents failed", "id", r.id, "error", err)
			} else {
				migrated++
			}
		}

		if len(batch) < pageSize {
			break
		}
	}

	return migrated, nil
}

// ── route_photos ──────────────────────────────────────────────────────────────

func (m *MigrateJob) migrateRoutePhotos(ctx context.Context) (int, error) {
	const pageSize = 500
	var migrated int

	for {
		type row struct {
			id     string
			userID string
			s3Key  string
		}

		rows, err := m.pool.Query(ctx, `
			SELECT p.id, p.user_id, p.s3_key
			FROM route_photos p
			WHERE p.file_id IS NULL
			  AND p.s3_key != ''
			  AND NOT EXISTS (
			    SELECT 1 FROM migration_filestore_journal j
			    WHERE j.source_table = 'route_photos' AND j.source_id = p.id AND j.status = 'done'
			  )
			ORDER BY p.id
			LIMIT $1`, pageSize)
		if err != nil {
			return migrated, err
		}

		var batch []row
		for rows.Next() {
			var r row
			if err := rows.Scan(&r.id, &r.userID, &r.s3Key); err != nil {
				rows.Close()
				return migrated, err
			}
			batch = append(batch, r)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return migrated, err
		}
		if len(batch) == 0 {
			break
		}

		for _, r := range batch {
			if err := m.registerS3File(ctx, "route_photos", r.id, r.userID,
				r.s3Key, "media", "file_id", "image/jpeg"); err != nil {
				slog.Warn("filestore migration: route_photos failed", "id", r.id, "error", err)
			} else {
				migrated++
			}
		}

		if len(batch) < pageSize {
			break
		}
	}

	return migrated, nil
}

// ── tags ──────────────────────────────────────────────────────────────────────

// migrateTags registers cover images for tags where cover_image_url points at
// the configured S3 bucket.
func (m *MigrateJob) migrateTags(ctx context.Context) (int, error) {
	const pageSize = 500
	var migrated int

	for {
		type row struct {
			id            string
			userID        string
			coverImageURL *string
		}

		rows, err := m.pool.Query(ctx, `
			SELECT t.id, t.user_id, t.cover_image_url
			FROM tags t
			WHERE t.cover_file_id IS NULL
			  AND t.cover_image_url IS NOT NULL
			  AND t.cover_image_url != ''
			  AND NOT EXISTS (
			    SELECT 1 FROM migration_filestore_journal j
			    WHERE j.source_table = 'tags' AND j.source_id = t.id AND j.status = 'done'
			  )
			ORDER BY t.id
			LIMIT $1`, pageSize)
		if err != nil {
			return migrated, err
		}

		var batch []row
		for rows.Next() {
			var r row
			if err := rows.Scan(&r.id, &r.userID, &r.coverImageURL); err != nil {
				rows.Close()
				return migrated, err
			}
			batch = append(batch, r)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return migrated, err
		}
		if len(batch) == 0 {
			break
		}

		for _, r := range batch {
			if r.coverImageURL == nil || *r.coverImageURL == "" {
				continue
			}
			s3Key := m.extractS3Key(*r.coverImageURL)
			if s3Key == "" {
				continue
			}
			if err := m.registerS3FileCol(ctx, "tags", r.id, r.userID,
				s3Key, "media", "cover_file_id", "image/jpeg"); err != nil {
				slog.Warn("filestore migration: tags cover failed", "id", r.id, "error", err)
			} else {
				migrated++
			}
		}

		if len(batch) < pageSize {
			break
		}
	}

	return migrated, nil
}

// ── custom_map_markers ────────────────────────────────────────────────────────

func (m *MigrateJob) migrateCustomMapMarkers(ctx context.Context) (int, error) {
	const pageSize = 500
	var migrated int

	for {
		type row struct {
			id       string
			userID   string
			imageURL *string
		}

		rows, err := m.pool.Query(ctx, `
			SELECT mk.id, mk.user_id, mk.image_url
			FROM custom_map_markers mk
			WHERE mk.image_file_id IS NULL
			  AND mk.image_url IS NOT NULL
			  AND mk.image_url != ''
			  AND NOT EXISTS (
			    SELECT 1 FROM migration_filestore_journal j
			    WHERE j.source_table = 'custom_map_markers' AND j.source_id = mk.id AND j.status = 'done'
			  )
			ORDER BY mk.id
			LIMIT $1`, pageSize)
		if err != nil {
			return migrated, err
		}

		var batch []row
		for rows.Next() {
			var r row
			if err := rows.Scan(&r.id, &r.userID, &r.imageURL); err != nil {
				rows.Close()
				return migrated, err
			}
			batch = append(batch, r)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return migrated, err
		}
		if len(batch) == 0 {
			break
		}

		for _, r := range batch {
			if r.imageURL == nil || *r.imageURL == "" {
				continue
			}
			s3Key := m.extractS3Key(*r.imageURL)
			if s3Key == "" {
				continue
			}
			if err := m.registerS3FileCol(ctx, "custom_map_markers", r.id, r.userID,
				s3Key, "media", "image_file_id", "image/jpeg"); err != nil {
				slog.Warn("filestore migration: custom_map_markers failed", "id", r.id, "error", err)
			} else {
				migrated++
			}
		}

		if len(batch) < pageSize {
			break
		}
	}

	return migrated, nil
}

// ── shared helpers ────────────────────────────────────────────────────────────

// registerS3File inserts a user_files row backed by S3 and links it via
// file_id on the domain table. Uses "file_id" as the column name.
func (m *MigrateJob) registerS3File(
	ctx context.Context,
	table, id, userID, s3Key, fileType, fileIDCol, contentType string,
) error {
	return m.registerS3FileCol(ctx, table, id, userID, s3Key, fileType, fileIDCol, contentType)
}

// registerS3FileCol handles the generic case for any domain table / column.
func (m *MigrateJob) registerS3FileCol(
	ctx context.Context,
	table, id, userID, s3Key, fileType, fileIDCol, contentType string,
) error {
	tx, err := m.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	storagePath := m.s3Path(s3Key)
	fileName := s3Key
	if idx := strings.LastIndex(s3Key, "/"); idx >= 0 {
		fileName = s3Key[idx+1:]
	}
	if fileName == "" {
		fileName = "file"
	}

	var fileID string
	err = tx.QueryRow(ctx, `
		INSERT INTO user_files (user_id, file_type, storage_backend, storage_path,
		                        content_type, file_name)
		VALUES ($1, $2, 's3', $3, $4, $5)
		ON CONFLICT (storage_backend, storage_path) DO UPDATE SET updated_at = NOW()
		RETURNING id`,
		userID, fileType, storagePath, contentType, fileName,
	).Scan(&fileID)
	if err != nil {
		return err
	}

	// Dynamic column name is safe: fileIDCol is always a compile-time constant
	// passed from internal code, never from user input.
	if _, err = tx.Exec(ctx,
		`UPDATE `+table+` SET `+fileIDCol+` = $1 WHERE id = $2 AND `+fileIDCol+` IS NULL`,
		fileID, id,
	); err != nil {
		return err
	}

	if _, err = tx.Exec(ctx, `
		INSERT INTO migration_filestore_journal (source_table, source_id, status, user_id, file_id, old_path)
		VALUES ($1, $2, 'done', $3, $4, $5)
		ON CONFLICT (source_table, source_id, phase) DO UPDATE
		    SET status = 'done', file_id = EXCLUDED.file_id`,
		table, id, userID, fileID, s3Key,
	); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

// s3Path returns the full storage_path stored in user_files for an S3 key.
// Format: "<bucket>/<key>" — keeps it unambiguous and queryable.
func (m *MigrateJob) s3Path(s3Key string) string {
	if m.s3Bucket == "" {
		return s3Key
	}
	return m.s3Bucket + "/" + s3Key
}

// extractS3Key extracts an S3 key from a URL that contains the bucket prefix.
// Returns "" if the URL does not appear to be an S3 object in our bucket.
func (m *MigrateJob) extractS3Key(url string) string {
	if url == "" {
		return ""
	}
	// Accept: "https://garage.example.com/skadia/media/abc.jpg"
	//         "/skadia/media/abc.jpg"
	//         "media/abc.jpg"
	// Strategy: look for the bucket name as a path segment and take everything after it.
	if m.s3Bucket != "" {
		needle := "/" + m.s3Bucket + "/"
		if idx := strings.Index(url, needle); idx >= 0 {
			return url[idx+len(needle):]
		}
	}
	// Bare key (no bucket prefix in URL): accept as-is if not a full HTTP URL.
	if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
		return strings.TrimPrefix(url, "/")
	}
	return ""
}
