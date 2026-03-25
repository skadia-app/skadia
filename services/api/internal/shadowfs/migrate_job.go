package shadowfs

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/jackc/pgx/v5/pgxpool"
)

// MigrateJob populates user_files from existing domain tables (Phase 1)
// and physically moves files to the Library structure (Phase 2).
type MigrateJob struct {
	pool         *pgxpool.Pool
	shadowFSBase string           // legacy ShadowFS base path
	s3Bucket     string           // legacy S3 bucket name
	basePath     string           // Library base path for file moves
	renderer     *SchemaRenderer  // schema renderer for target paths
}

// NewMigrateJob creates a new migration job.
func NewMigrateJob(pool *pgxpool.Pool, shadowFSBase, s3Bucket string) *MigrateJob {
	return &MigrateJob{pool: pool, shadowFSBase: shadowFSBase, s3Bucket: s3Bucket}
}

// NewMigrateJobWithLibrary creates a migration job configured for file moves.
func NewMigrateJobWithLibrary(pool *pgxpool.Pool, basePath, shadowFSBase, s3Bucket string, renderer *SchemaRenderer) *MigrateJob {
	return &MigrateJob{
		pool:         pool,
		shadowFSBase: shadowFSBase,
		s3Bucket:     s3Bucket,
		basePath:     basePath,
		renderer:     renderer,
	}
}

// Run executes the full registry population. It is idempotent: each row is
// skipped when a journal entry with status='done' already exists.
func (m *MigrateJob) Run(ctx context.Context, jobID string) error {
	slog.Info("shadowfs migration: starting registry population", "job_id", jobID)

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

	var stepErrors []error
	for _, s := range steps {
		n, err := s.fn(ctx)
		totalMigrated += n
		if err != nil {
			slog.Error("shadowfs migration: step failed", "step", s.name, "error", err)
			stepErrors = append(stepErrors, fmt.Errorf("step %s: %w", s.name, err))
		} else {
			slog.Info("shadowfs migration: step done", "step", s.name, "migrated", n)
		}
		// Update progress in background_jobs
		m.pool.Exec(ctx, //nolint:errcheck
			`UPDATE background_jobs SET progress_current = progress_current + $1 WHERE id = $2`,
			int64(n), jobID,
		)
	}

	slog.Info("shadowfs migration: complete", "total_migrated", totalMigrated, "job_id", jobID)
	return errors.Join(stepErrors...)
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
			    SELECT 1 FROM migration_shadowfs_journal j
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
						slog.Warn("shadowfs migration: activities local failed", "id", r.id, "error", err)
					} else {
						migrated++
					}
					continue
				}
			}
			// Fall back to S3 key.
			if r.originalFileKey != nil && *r.originalFileKey != "" {
				if err := m.registerActivityS3(ctx, r.id, r.userID, *r.originalFileKey); err != nil {
					slog.Warn("shadowfs migration: activities s3 failed", "id", r.id, "error", err)
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
	var xmaxLocal uint32
	err = tx.QueryRow(ctx, `
		INSERT INTO user_files (user_id, file_type, storage_backend, storage_path,
		                        content_type, file_name, parent_type, parent_id)
		VALUES ($1, 'activity', 'local', $2, 'application/octet-stream',
		        COALESCE(NULLIF(REGEXP_REPLACE($2, '^.*/', ''), ''), 'activity.fit'),
		        'activity', $3::uuid)
		ON CONFLICT (storage_backend, storage_path) DO UPDATE SET updated_at = NOW()
		RETURNING id, xmax`,
		userID, localPath, id,
	).Scan(&fileID, &xmaxLocal)
	if err != nil {
		return err
	}
	if xmaxLocal != 0 {
		slog.Info("shadowfs migration: ON CONFLICT update (existing registry entry)",
			"table", "activities", "source_id", id, "file_id", fileID, "path", localPath)
	}

	if _, err = tx.Exec(ctx,
		`UPDATE activities SET file_id = $1 WHERE id = $2 AND file_id IS NULL`,
		fileID, id,
	); err != nil {
		return err
	}

	if _, err = tx.Exec(ctx, `
		INSERT INTO migration_shadowfs_journal (source_table, source_id, status, user_id, file_id, old_path)
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
	var xmaxS3 uint32
	err = tx.QueryRow(ctx, `
		INSERT INTO user_files (user_id, file_type, storage_backend, storage_path,
		                        content_type, file_name, parent_type, parent_id)
		VALUES ($1, 'activity', 's3', $2, 'application/octet-stream',
		        COALESCE(NULLIF(REGEXP_REPLACE($3, '^.*/', ''), ''), 'activity.fit'),
		        'activity', $4::uuid)
		ON CONFLICT (storage_backend, storage_path) DO UPDATE SET updated_at = NOW()
		RETURNING id, xmax`,
		userID, storagePath, s3Key, id,
	).Scan(&fileID, &xmaxS3)
	if err != nil {
		return err
	}
	if xmaxS3 != 0 {
		slog.Info("shadowfs migration: ON CONFLICT update (existing registry entry)",
			"table", "activities", "source_id", id, "file_id", fileID, "path", storagePath)
	}

	if _, err = tx.Exec(ctx,
		`UPDATE activities SET file_id = $1 WHERE id = $2 AND file_id IS NULL`,
		fileID, id,
	); err != nil {
		return err
	}

	if _, err = tx.Exec(ctx, `
		INSERT INTO migration_shadowfs_journal (source_table, source_id, status, user_id, file_id, old_path)
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
			    SELECT 1 FROM migration_shadowfs_journal j
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
				slog.Warn("shadowfs migration: activity_photos failed", "id", r.id, "error", err)
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
			    SELECT 1 FROM migration_shadowfs_journal j
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
				slog.Warn("shadowfs migration: gear_documents failed", "id", r.id, "error", err)
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
			    SELECT 1 FROM migration_shadowfs_journal j
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
				slog.Warn("shadowfs migration: route_photos failed", "id", r.id, "error", err)
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
			    SELECT 1 FROM migration_shadowfs_journal j
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
				slog.Warn("shadowfs migration: tags cover failed", "id", r.id, "error", err)
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
			    SELECT 1 FROM migration_shadowfs_journal j
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
				slog.Warn("shadowfs migration: custom_map_markers failed", "id", r.id, "error", err)
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
	var xmaxGen uint32
	err = tx.QueryRow(ctx, `
		INSERT INTO user_files (user_id, file_type, storage_backend, storage_path,
		                        content_type, file_name)
		VALUES ($1, $2, 's3', $3, $4, $5)
		ON CONFLICT (storage_backend, storage_path) DO UPDATE SET updated_at = NOW()
		RETURNING id, xmax`,
		userID, fileType, storagePath, contentType, fileName,
	).Scan(&fileID, &xmaxGen)
	if err != nil {
		return err
	}
	if xmaxGen != 0 {
		slog.Info("shadowfs migration: ON CONFLICT update (existing registry entry)",
			"table", table, "source_id", id, "file_id", fileID, "path", storagePath)
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
		INSERT INTO migration_shadowfs_journal (source_table, source_id, status, user_id, file_id, old_path)
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

// ── Phase 2: File Move ───────────────────────────────────────────────────────

// moveEntry represents a user_files row that needs its physical file relocated.
type moveEntry struct {
	fileID      string
	userID      string
	fileType    string
	storagePath string // current relative path
	fileName    string
	hasSidecar  bool
	contentType string
}

// RunFileMove physically moves local files from legacy ShadowFS locations
// to the new Library directory structure. It is idempotent: journal entries
// with phase='move' and status='done' are skipped on restart.
func (m *MigrateJob) RunFileMove(ctx context.Context, jobID string) error {
	if m.basePath == "" {
		return fmt.Errorf("shadowfs migrate: basePath not configured")
	}
	if m.renderer == nil {
		return fmt.Errorf("shadowfs migrate: renderer not configured")
	}

	slog.Info("shadowfs migrate: starting file move (Phase 2)", "job_id", jobID)

	// H5: Recover any pending journal entries from a previous crashed run.
	if err := m.recoverPendingMoves(ctx); err != nil {
		slog.Error("shadowfs migrate: recover pending moves failed", "error", err)
		// Non-fatal: continue with the main migration.
	}

	// Count total files to move for progress tracking.
	var total int64
	err := m.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM user_files
		WHERE storage_backend = 'local' AND status = 'active'
		  AND NOT EXISTS (
		    SELECT 1 FROM migration_shadowfs_journal j
		    WHERE j.file_id = user_files.id AND j.phase = 'move' AND j.status = 'done'
		  )`).Scan(&total)
	if err != nil {
		return fmt.Errorf("shadowfs migrate: count pending: %w", err)
	}

	m.pool.Exec(ctx, //nolint:errcheck
		`UPDATE background_jobs SET progress_total = $1 WHERE id = $2`, total, jobID)

	slog.Info("shadowfs migrate: files to move", "total", total, "job_id", jobID)

	const pageSize = 500
	var moved, skipped, failed int

	for {
		rows, err := m.pool.Query(ctx, `
			SELECT uf.id, uf.user_id, uf.file_type, uf.storage_path, uf.file_name,
			       uf.has_sidecar, uf.content_type
			FROM user_files uf
			WHERE uf.storage_backend = 'local' AND uf.status = 'active'
			  AND NOT EXISTS (
			    SELECT 1 FROM migration_shadowfs_journal j
			    WHERE j.file_id = uf.id AND j.phase = 'move' AND j.status = 'done'
			  )
			ORDER BY uf.user_id, uf.id
			LIMIT $1`, pageSize)
		if err != nil {
			return fmt.Errorf("shadowfs migrate: query batch: %w", err)
		}

		var batch []moveEntry
		for rows.Next() {
			var e moveEntry
			if err := rows.Scan(&e.fileID, &e.userID, &e.fileType, &e.storagePath,
				&e.fileName, &e.hasSidecar, &e.contentType); err != nil {
				rows.Close()
				return fmt.Errorf("shadowfs migrate: scan: %w", err)
			}
			batch = append(batch, e)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return fmt.Errorf("shadowfs migrate: rows: %w", err)
		}
		if len(batch) == 0 {
			break
		}

		for _, e := range batch {
			if ctx.Err() != nil {
				return ctx.Err()
			}

			if err := m.moveOneFile(ctx, e); err != nil {
				failed++
				slog.Warn("shadowfs migrate: move failed",
					"file_id", e.fileID, "path", e.storagePath, "error", err)
				// Record failure in journal for debugging; continue with next file.
				m.pool.Exec(ctx, //nolint:errcheck
					`INSERT INTO migration_shadowfs_journal (source_table, source_id, phase, status, user_id, file_id, old_path, error_msg)
					 VALUES ('user_files', $1::uuid, 'move', 'failed', $2, $3, $4, $5)
					 ON CONFLICT (source_table, source_id, phase) DO UPDATE
					     SET status = 'failed', error_msg = EXCLUDED.error_msg`,
					e.fileID, e.userID, e.fileID, e.storagePath, err.Error())
			} else {
				moved++
			}

			// Update progress every file (cheap single-row UPDATE).
			m.pool.Exec(ctx, //nolint:errcheck
				`UPDATE background_jobs SET progress_current = $1 WHERE id = $2`,
				int64(moved+skipped+failed), jobID)

			if (moved+failed)%100 == 0 && (moved+failed) > 0 {
				slog.Info("shadowfs migrate: progress",
					"moved", moved, "failed", failed, "job_id", jobID)
			}
		}

		if len(batch) < pageSize {
			break
		}
	}

	slog.Info("shadowfs migrate: file move complete",
		"moved", moved, "skipped", skipped, "failed", failed, "job_id", jobID)
	return nil
}

// moveOneFile moves a single local file to its Library path and updates the registry.
func (m *MigrateJob) moveOneFile(ctx context.Context, e moveEntry) error {
	// Compute the target Library path.
	userDir, err := m.getUserDir(ctx, e.userID)
	if err != nil {
		return fmt.Errorf("get user dir: %w", err)
	}

	ext := filepath.Ext(e.fileName)
	typeDir := DirForType(e.fileType)

	// Get user schema for this file type.
	schema := m.renderer.DefaultSchema(e.fileType)
	var userSchema string
	schemaErr := m.pool.QueryRow(ctx,
		`SELECT schema_template FROM user_file_schemas WHERE user_id = $1 AND file_type = $2`,
		e.userID, e.fileType).Scan(&userSchema)
	if schemaErr == nil && userSchema != "" {
		schema = userSchema
	}

	// Build tags from existing metadata for schema rendering.
	tags := m.buildTagsFromPath(e.storagePath, e.fileType)

	rendered := m.renderer.RenderPath(e.fileType, schema, tags, ext)
	newRelPath := filepath.Join(userDir, typeDir, rendered)

	// If the path is already correct, mark as done and skip.
	if filepath.Clean(e.storagePath) == filepath.Clean(newRelPath) {
		return m.journalMoveDone(ctx, e, e.storagePath, newRelPath)
	}

	// Resolve collisions in the target path.
	newRelPath = ResolveCollision(m.basePath, newRelPath)

	oldFull := filepath.Join(m.basePath, e.storagePath)
	newFull := filepath.Join(m.basePath, newRelPath)

	// H5: Write a pending journal entry BEFORE the rename so that a crash between
	// rename and DB update can be recovered on restart.
	// Verify old file exists first to avoid pointless journal writes.
	if _, err := os.Stat(oldFull); err != nil {
		if os.IsNotExist(err) {
			// File is missing on disk — mark done (registry-only migration).
			return m.journalMoveDone(ctx, e, e.storagePath, newRelPath)
		}
		return fmt.Errorf("stat old: %w", err)
	}

	// Write pending journal entry before making any filesystem changes.
	if _, err := m.pool.Exec(ctx, `
		INSERT INTO migration_shadowfs_journal (source_table, source_id, phase, status, user_id, file_id, old_path, new_path)
		VALUES ('user_files', $1::uuid, 'move', 'pending', $2, $3, $4, $5)
		ON CONFLICT (source_table, source_id, phase) DO UPDATE
		    SET status = 'pending', new_path = EXCLUDED.new_path, old_path = EXCLUDED.old_path`,
		e.fileID, e.userID, e.fileID, e.storagePath, newRelPath); err != nil {
		return fmt.Errorf("journal pending: %w", err)
	}

	// Create target directories.
	if err := os.MkdirAll(filepath.Dir(newFull), 0o755); err != nil {
		return fmt.Errorf("mkdir: %w", err)
	}

	// Attempt atomic rename first.
	if err := os.Rename(oldFull, newFull); err != nil {
		// Cross-device: fall back to copy+delete.
		if !errors.Is(err, os.ErrInvalid) && !isCrossDevice(err) {
			return fmt.Errorf("rename: %w", err)
		}
		if err := copyFile(oldFull, newFull); err != nil {
			return fmt.Errorf("copy: %w", err)
		}
		if err := os.Remove(oldFull); err != nil && !os.IsNotExist(err) {
			slog.Warn("shadowfs migrate: remove old after copy",
				"path", oldFull, "error", err)
		}
	}

	// Move sidecar files if present.
	if e.hasSidecar {
		m.moveSidecar(oldFull, newFull, e.fileType)
	}

	// Update storage_path in the registry.
	if _, err := m.pool.Exec(ctx,
		`UPDATE user_files SET storage_path = $1, updated_at = NOW() WHERE id = $2`,
		newRelPath, e.fileID); err != nil {
		return fmt.Errorf("update storage_path: %w", err)
	}

	// Clean empty directories left behind.
	cleanEmptyDirs(filepath.Dir(oldFull), m.basePath)

	return m.journalMoveDone(ctx, e, e.storagePath, newRelPath)
}

// journalMoveDone records a completed move in the journal.
func (m *MigrateJob) journalMoveDone(ctx context.Context, e moveEntry, oldPath, newPath string) error {
	_, err := m.pool.Exec(ctx, `
		INSERT INTO migration_shadowfs_journal (source_table, source_id, phase, status, user_id, file_id, old_path, new_path)
		VALUES ('user_files', $1::uuid, 'move', 'done', $2, $3, $4, $5)
		ON CONFLICT (source_table, source_id, phase) DO UPDATE
		    SET status = 'done', new_path = EXCLUDED.new_path`,
		e.fileID, e.userID, e.fileID, oldPath, newPath)
	return err
}

// moveSidecar moves sidecar files that accompany the main file.
func (m *MigrateJob) moveSidecar(oldFull, newFull, fileType string) {
	type sidecar struct{ old, new string }
	var sidecars []sidecar

	switch fileType {
	case "activity":
		sidecars = append(sidecars, sidecar{
			old: oldFull + ".meta",
			new: newFull + ".meta",
		})
	case "media":
		oldExt := filepath.Ext(oldFull)
		oldStem := strings.TrimSuffix(oldFull, oldExt)
		newExt := filepath.Ext(newFull)
		newStem := strings.TrimSuffix(newFull, newExt)
		sidecars = append(sidecars, sidecar{
			old: oldStem + ".meta.json",
			new: newStem + ".meta.json",
		})
	}

	for _, sc := range sidecars {
		if _, err := os.Stat(sc.old); err != nil {
			continue // sidecar doesn't exist
		}
		if err := os.MkdirAll(filepath.Dir(sc.new), 0o755); err != nil {
			slog.Warn("shadowfs migrate: mkdir sidecar", "path", sc.new, "error", err)
			continue
		}
		if err := os.Rename(sc.old, sc.new); err != nil {
			if isCrossDevice(err) || errors.Is(err, os.ErrInvalid) {
				if cpErr := copyFile(sc.old, sc.new); cpErr != nil {
					slog.Warn("shadowfs migrate: copy sidecar", "from", sc.old, "error", cpErr)
					continue
				}
				os.Remove(sc.old) //nolint:errcheck
			} else {
				slog.Warn("shadowfs migrate: rename sidecar", "from", sc.old, "error", err)
			}
		}
	}
}

// buildTagsFromPath extracts schema variables from an existing storage path.
// This is a best-effort extraction used to render the target path for files
// that were registered without explicit tags.
func (m *MigrateJob) buildTagsFromPath(storagePath, fileType string) map[string]string {
	tags := make(map[string]string)

	// Extract date components from paths like "username/activities/2025/03/15_14-30_running.fit"
	parts := strings.Split(filepath.Clean(storagePath), string(filepath.Separator))

	// Walk parts looking for 4-digit year.
	for i, p := range parts {
		if len(p) == 4 && p >= "2000" && p <= "2099" {
			tags["year"] = p
			if i+1 < len(parts) && len(parts[i+1]) == 2 {
				tags["month"] = parts[i+1]
			}
		}
	}

	// Extract sport/title from filename (last segment before extension).
	if len(parts) > 0 {
		base := parts[len(parts)-1]
		ext := filepath.Ext(base)
		stem := strings.TrimSuffix(base, ext)
		// For activity files: "15_14-30_running" → day=15, time=14-30, sport=running
		segments := strings.SplitN(stem, "_", 4)
		if fileType == "activity" && len(segments) >= 3 {
			tags["day"] = segments[0]
			tags["time"] = segments[1]
			tags["sport"] = segments[2]
		} else {
			tags["title"] = stem
		}
	}

	return tags
}

// getUserDir returns the relative user directory name.
func (m *MigrateJob) getUserDir(ctx context.Context, userID string) (string, error) {
	var dirName *string
	err := m.pool.QueryRow(ctx,
		`SELECT library_dir_name FROM users WHERE id = $1`, userID).Scan(&dirName)
	if err != nil {
		return "", fmt.Errorf("query user: %w", err)
	}
	name := userID
	if dirName != nil && *dirName != "" {
		name = *dirName
	}
	if strings.Contains(name, "..") || strings.Contains(name, "/") || strings.Contains(name, "\\") {
		return "", fmt.Errorf("invalid library_dir_name: %q", name)
	}
	return name, nil
}

// ── Rollback ─────────────────────────────────────────────────────────────────

// recoverPendingMoves re-applies any journal entries from a previous run that
// were left in 'pending' state (e.g. due to a crash mid-migration). For each
// such entry it retries the rename and on success marks the entry as 'done'.
// Non-fatal errors are logged and skipped so the main migration can proceed.
func (m *MigrateJob) recoverPendingMoves(ctx context.Context) error {
	rows, err := m.pool.Query(ctx, `
		SELECT file_id, user_id, old_path, new_path
		FROM migration_shadowfs_journal
		WHERE phase = 'move' AND status = 'pending'`)
	if err != nil {
		return fmt.Errorf("query pending moves: %w", err)
	}
	defer rows.Close()

	type pendingMove struct {
		fileID, userID, oldPath, newPath string
	}
	var items []pendingMove
	for rows.Next() {
		var p pendingMove
		if err := rows.Scan(&p.fileID, &p.userID, &p.oldPath, &p.newPath); err != nil {
			return fmt.Errorf("scan pending move: %w", err)
		}
		items = append(items, p)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate pending moves: %w", err)
	}

	if len(items) == 0 {
		return nil
	}
	slog.Info("shadowfs migrate: recovering pending moves", "count", len(items))

	for _, p := range items {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		oldFull := filepath.Join(m.basePath, p.oldPath)
		newFull := filepath.Join(m.basePath, p.newPath)

		// If new path already exists, the rename succeeded — just mark done.
		if _, statErr := os.Stat(newFull); statErr == nil {
			if _, execErr := m.pool.Exec(ctx, `
				UPDATE migration_shadowfs_journal SET status = 'done'
				WHERE file_id = $1 AND phase = 'move'`, p.fileID); execErr != nil {
				slog.Error("shadowfs migrate: recover: mark done failed", "file_id", p.fileID, "error", execErr)
			}
			continue
		}

		if mkErr := os.MkdirAll(filepath.Dir(newFull), 0o755); mkErr != nil {
			slog.Error("shadowfs migrate: recover: mkdir failed", "file_id", p.fileID, "error", mkErr)
			continue
		}
		if renErr := os.Rename(oldFull, newFull); renErr != nil {
			slog.Error("shadowfs migrate: recover: rename failed",
				"file_id", p.fileID, "from", oldFull, "to", newFull, "error", renErr)
			continue
		}
		if _, execErr := m.pool.Exec(ctx, `
			UPDATE migration_shadowfs_journal SET status = 'done'
			WHERE file_id = $1 AND phase = 'move'`, p.fileID); execErr != nil {
			slog.Error("shadowfs migrate: recover: mark done failed", "file_id", p.fileID, "error", execErr)
		}
		slog.Info("shadowfs migrate: recovered pending move",
			"file_id", p.fileID, "from", p.oldPath, "to", p.newPath)
	}
	return nil
}

// RunRollback reverses Phase 2 file moves by reading journal entries with
// phase='move' and status='done', and moving files back to their old paths.
func (m *MigrateJob) RunRollback(ctx context.Context, jobID string) error {
	if m.basePath == "" {
		return fmt.Errorf("shadowfs migrate: basePath not configured")
	}

	slog.Info("shadowfs migrate: starting rollback", "job_id", jobID)

	var total int64
	err := m.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM migration_shadowfs_journal
		WHERE phase = 'move' AND status = 'done'
		  AND old_path IS NOT NULL AND new_path IS NOT NULL
		  AND old_path != new_path`).Scan(&total)
	if err != nil {
		return fmt.Errorf("shadowfs migrate rollback: count: %w", err)
	}

	m.pool.Exec(ctx, //nolint:errcheck
		`UPDATE background_jobs SET progress_total = $1 WHERE id = $2`, total, jobID)

	slog.Info("shadowfs migrate: entries to rollback", "total", total, "job_id", jobID)

	const pageSize = 500
	var rolled, failed int

	for {
		type jEntry struct {
			sourceID string
			fileID   string
			userID   string
			oldPath  string
			newPath  string
		}

		rows, err := m.pool.Query(ctx, `
			SELECT source_id, file_id, user_id, old_path, new_path
			FROM migration_shadowfs_journal
			WHERE phase = 'move' AND status = 'done'
			  AND old_path IS NOT NULL AND new_path IS NOT NULL
			  AND old_path != new_path
			ORDER BY source_id
			LIMIT $1`, pageSize)
		if err != nil {
			return fmt.Errorf("shadowfs migrate rollback: query: %w", err)
		}

		var batch []jEntry
		for rows.Next() {
			var j jEntry
			if err := rows.Scan(&j.sourceID, &j.fileID, &j.userID, &j.oldPath, &j.newPath); err != nil {
				rows.Close()
				return fmt.Errorf("shadowfs migrate rollback: scan: %w", err)
			}
			batch = append(batch, j)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return fmt.Errorf("shadowfs migrate rollback: rows: %w", err)
		}
		if len(batch) == 0 {
			break
		}

		for _, j := range batch {
			if ctx.Err() != nil {
				return ctx.Err()
			}

			newFull := filepath.Join(m.basePath, j.newPath)
			oldFull := filepath.Join(m.basePath, j.oldPath)

			moveErr := func() error {
				// Only move if the file exists at the new location.
				if _, err := os.Stat(newFull); err != nil {
					if os.IsNotExist(err) {
						return nil // nothing to move back
					}
					return err
				}

				if err := os.MkdirAll(filepath.Dir(oldFull), 0o755); err != nil {
					return fmt.Errorf("mkdir: %w", err)
				}

				if err := os.Rename(newFull, oldFull); err != nil {
					if !isCrossDevice(err) && !errors.Is(err, os.ErrInvalid) {
						return err
					}
					if err := copyFile(newFull, oldFull); err != nil {
						return err
					}
					os.Remove(newFull) //nolint:errcheck
				}

				// Clean empty directories at the new location.
				cleanEmptyDirs(filepath.Dir(newFull), m.basePath)
				return nil
			}()

			if moveErr != nil {
				failed++
				slog.Warn("shadowfs migrate rollback: move failed",
					"file_id", j.fileID, "error", moveErr)
				continue
			}

			// Restore storage_path in user_files.
			m.pool.Exec(ctx, //nolint:errcheck
				`UPDATE user_files SET storage_path = $1, updated_at = NOW() WHERE id = $2`,
				j.oldPath, j.fileID)

			// Remove journal entry so it can be re-migrated.
			m.pool.Exec(ctx, //nolint:errcheck
				`DELETE FROM migration_shadowfs_journal
				 WHERE source_table = 'user_files' AND source_id = $1::uuid AND phase = 'move'`,
				j.sourceID)

			rolled++

			m.pool.Exec(ctx, //nolint:errcheck
				`UPDATE background_jobs SET progress_current = $1 WHERE id = $2`,
				int64(rolled+failed), jobID)

			if (rolled+failed)%100 == 0 && (rolled+failed) > 0 {
				slog.Info("shadowfs migrate rollback: progress",
					"rolled", rolled, "failed", failed, "job_id", jobID)
			}
		}

		if len(batch) < pageSize {
			break
		}
	}

	slog.Info("shadowfs migrate: rollback complete",
		"rolled", rolled, "failed", failed, "job_id", jobID)
	return nil
}

// ── cross-device helpers ─────────────────────────────────────────────────────

// isCrossDevice checks if an error is EXDEV (cross-device link).
func isCrossDevice(err error) bool {
	// os.Rename returns *os.LinkError wrapping syscall.EXDEV on Linux.
	return errors.Is(err, syscall.EXDEV)
}

// copyFile copies src to dst preserving permissions. dst is created with O_EXCL.
func copyFile(src, dst string) (err error) {
	sf, err := os.Open(src)
	if err != nil {
		return err
	}
	defer sf.Close()

	info, err := sf.Stat()
	if err != nil {
		return err
	}

	df, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, info.Mode().Perm())
	if err != nil {
		return err
	}
	defer func() {
		if cerr := df.Close(); err == nil {
			err = cerr
		}
	}()

	if _, err = io.Copy(df, sf); err != nil {
		os.Remove(dst) //nolint:errcheck
		return err
	}
	return nil
}
