package shadowfs

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	// LIMITATION: S3ShadowFS.Put buffers the entire upload in memory (up to maxUploadBytes)
	// because the AWS SDK PutObject requires a known Content-Length. For files approaching
	// this limit, the API container needs at least 500 MB free heap. A future multipart-upload
	// path would stream in 5 MB chunks. LocalShadowFS does not have this limitation.
	//
	// CONCURRENCY NOTE: There is no global limit on concurrent S3 uploads. Each concurrent
	// Put call buffers up to 500 MB. Under heavy bulk-import load (e.g. N concurrent goroutines)
	// memory usage can reach N * 500 MB. The import worker pool (max 2 concurrent per-user) provides
	// the primary concurrency bound. If unbounded parallelism is ever added, a semaphore should be
	// placed here.
	maxUploadBytes = 500 << 20 // 500 MB
	maxZipBytes    = 10 << 30  // 10 GB
	presignExpiry  = 15 * time.Minute
	deleteBatch    = 1000
)

// S3Config holds configuration for an S3-compatible backend.
type S3Config struct {
	Endpoint       string
	AccessKey      string
	SecretKey      string
	Bucket         string
	Region         string
	ForcePathStyle bool
	BucketActivity string
	BucketMedia    string
}

// S3ShadowFS implements ShadowFS using S3-compatible object storage.
type S3ShadowFS struct {
	pool           *pgxpool.Pool
	client         *s3.Client
	presigner      *s3.PresignClient
	bucket         string
	bucketActivity string
	bucketMedia    string
	keepVersions   int
	maxUserBytes   int64
}

// NewS3ShadowFS creates an S3ShadowFS backed by an S3-compatible service.
func NewS3ShadowFS(pool *pgxpool.Pool, cfg S3Config, keepVersions int, maxUserBytes int64) *S3ShadowFS {
	client := s3.New(s3.Options{
		BaseEndpoint: aws.String(cfg.Endpoint),
		Region:       cfg.Region,
		Credentials:  credentials.NewStaticCredentialsProvider(cfg.AccessKey, cfg.SecretKey, ""),
		UsePathStyle: cfg.ForcePathStyle,
	})
	return &S3ShadowFS{
		pool:           pool,
		client:         client,
		presigner:      s3.NewPresignClient(client),
		bucket:         cfg.Bucket,
		bucketActivity: cfg.BucketActivity,
		bucketMedia:    cfg.BucketMedia,
		keepVersions:   keepVersions,
		maxUserBytes:   maxUserBytes,
	}
}

// bucketFor returns the appropriate bucket for the given file type.
func (s *S3ShadowFS) bucketFor(fileType string) string {
	switch fileType {
	case "activity", "health":
		if s.bucketActivity != "" {
			return s.bucketActivity
		}
	case "media", "document":
		if s.bucketMedia != "" {
			return s.bucketMedia
		}
	}
	return s.bucket
}

// Put stores a file in S3 and registers it in user_files.
// NOTE: S3ShadowFS buffers the entire file in memory (up to 500 MB). For streaming uploads, consider multipart upload in a future iteration. LocalShadowFS streams to disk with minimal memory.
func (s *S3ShadowFS) Put(ctx context.Context, entry FileEntry, data io.Reader) (FileRecord, error) {
	// Validate file type.
	if _, ok := fileTypeDirName[entry.FileType]; !ok {
		return FileRecord{}, fmt.Errorf("unknown file type %q", entry.FileType)
	}

	// Quota check.
	// NOTE: Quota is a soft limit (TOCTOU race possible with concurrent uploads). Hard enforcement via Reconciliation job. This is by design — see Spec §15.2.
	// DIVERGENCE vs LocalShadowFS: LocalShadowFS skips the pre-check when entry.SizeBytes == 0
	// (streaming uploads with unknown size). S3ShadowFS always runs the pre-check because the full
	// buffer is read before this point and the actual size is known. Both backends run the post-write
	// check after the upload to catch unconditional overages.
	used, err := diskUsage(ctx, s.pool, entry.UserID)
	if err != nil {
		return FileRecord{}, fmt.Errorf("quota check: %w", err)
	}
	if s.maxUserBytes > 0 && used+entry.SizeBytes > s.maxUserBytes {
		return FileRecord{}, fmt.Errorf("quota exceeded: %d + %d > %d", used, entry.SizeBytes, s.maxUserBytes)
	}

	// Read all data into buffer (capped at 500 MB).
	// +1 so exactly maxUploadBytes is accepted, maxUploadBytes+1 is rejected
	buf, err := io.ReadAll(io.LimitReader(data, maxUploadBytes+1))
	if err != nil {
		return FileRecord{}, fmt.Errorf("reading upload data: %w", err)
	}
	if len(buf) > maxUploadBytes {
		return FileRecord{}, fmt.Errorf("upload exceeds maximum size of %d bytes", maxUploadBytes)
	}

	// SHA256 hash — always use computed hash for DB storage, never trust caller-supplied entry.FileHash.
	h := sha256.Sum256(buf)
	hash := hex.EncodeToString(h[:])

	// Edit sidecars require a ParentID.
	if entry.FileType == "edit" && entry.ParentID == nil {
		return FileRecord{}, fmt.Errorf("edit sidecar requires ParentID")
	}

	// Validate ParentID as UUID if set.
	if entry.ParentID != nil && *entry.ParentID != "" {
		if _, err := uuid.Parse(*entry.ParentID); err != nil {
			return FileRecord{}, fmt.Errorf("invalid parent_id: %w", err)
		}
	}

	// Build S3 key.
	ext := sanitizeExt(filepath.Ext(entry.FileName))
	var s3Key string
	if entry.FileType == "edit" {
		// Edit sidecars: {userID}/activities/_edits/{parentID}.json
		parentID := ""
		if entry.ParentID != nil {
			parentID = *entry.ParentID
		}
		s3Key = fmt.Sprintf("%s/activities/_edits/%s.json", entry.UserID, parentID)
	} else {
		dir := DirForType(entry.FileType)
		s3Key = fmt.Sprintf("%s/%s/%s%s", entry.UserID, dir, hash, ext)
	}

	// Detect content type.
	first512 := buf
	if len(first512) > 512 {
		first512 = first512[:512]
	}
	contentType := DetectContentType(entry.FileName, entry.ContentType, first512)

	// Upload to S3.
	bucket := s.bucketFor(entry.FileType)
	_, err = s.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(bucket),
		Key:         aws.String(s3Key),
		Body:        bytes.NewReader(buf),
		ContentType: aws.String(contentType),
	})
	if err != nil {
		return FileRecord{}, fmt.Errorf("s3 put: %w", err)
	}

	// Insert into user_files.
	fileID := uuid.New().String()
	var rec FileRecord

	if entry.FileType == "edit" {
		// Upsert for edit sidecars (one per parent).
		err = s.pool.QueryRow(ctx,
			`INSERT INTO user_files (id, user_id, file_type, storage_backend, storage_path,
				content_type, file_name, size_bytes, file_hash, parent_type, parent_id,
				has_sidecar, status)
			VALUES ($1, $2, $3, 's3', $4, $5, $6, $7, $8, $9, $10, $11, 'active')
			ON CONFLICT (user_id, file_type, parent_id)
				WHERE file_type = 'edit'
			DO UPDATE SET storage_path = EXCLUDED.storage_path,
				content_type = EXCLUDED.content_type,
				file_name = EXCLUDED.file_name,
				size_bytes = EXCLUDED.size_bytes,
				file_hash = EXCLUDED.file_hash,
				has_sidecar = EXCLUDED.has_sidecar,
				updated_at = NOW()
			RETURNING id, user_id, file_type, storage_backend, storage_path, content_type,
				file_name, size_bytes, file_hash, parent_type, parent_id, has_sidecar,
				is_renaming, status, replaced_by, created_at, updated_at`,
			fileID, entry.UserID, entry.FileType, s3Key, contentType,
			entry.FileName, int64(len(buf)), hash, entry.ParentType, entry.ParentID,
			entry.HasSidecar,
		).Scan(&rec.ID, &rec.UserID, &rec.FileType, &rec.StorageBackend, &rec.StoragePath,
			&rec.ContentType, &rec.FileName, &rec.SizeBytes, &rec.FileHash, &rec.ParentType,
			&rec.ParentID, &rec.HasSidecar, &rec.IsRenaming, &rec.Status, &rec.ReplacedBy,
			&rec.CreatedAt, &rec.UpdatedAt)
	} else {
		// ON CONFLICT note: for non-edit files, S3 keys are hash-based ({userID}/{dir}/{sha256}{ext}),
		// so a conflict means the same file content was uploaded twice. We update metadata but keep
		// the existing record ID. LocalShadowFS uses a plain INSERT for non-edit files (no ON CONFLICT)
		// because it relies on O_EXCL + collision-suffix resolution instead. This behavioural difference
		// is intentional: S3 keys are content-addressed; local paths are not.
		err = s.pool.QueryRow(ctx,
			`INSERT INTO user_files (id, user_id, file_type, storage_backend, storage_path,
				content_type, file_name, size_bytes, file_hash, parent_type, parent_id,
				has_sidecar, status)
			VALUES ($1, $2, $3, 's3', $4, $5, $6, $7, $8, $9, $10, $11, 'active')
			ON CONFLICT (storage_backend, storage_path) DO UPDATE SET
				size_bytes = EXCLUDED.size_bytes, file_hash = EXCLUDED.file_hash,
				content_type = EXCLUDED.content_type, file_name = EXCLUDED.file_name,
				parent_type = EXCLUDED.parent_type, parent_id = EXCLUDED.parent_id,
				has_sidecar = EXCLUDED.has_sidecar, updated_at = NOW()
			RETURNING id, user_id, file_type, storage_backend, storage_path, content_type,
				file_name, size_bytes, file_hash, parent_type, parent_id, has_sidecar,
				is_renaming, status, replaced_by, created_at, updated_at`,
			fileID, entry.UserID, entry.FileType, s3Key, contentType,
			entry.FileName, int64(len(buf)), hash, entry.ParentType, entry.ParentID,
			entry.HasSidecar,
		).Scan(&rec.ID, &rec.UserID, &rec.FileType, &rec.StorageBackend, &rec.StoragePath,
			&rec.ContentType, &rec.FileName, &rec.SizeBytes, &rec.FileHash, &rec.ParentType,
			&rec.ParentID, &rec.HasSidecar, &rec.IsRenaming, &rec.Status, &rec.ReplacedBy,
			&rec.CreatedAt, &rec.UpdatedAt)
	}
	if err != nil {
		// DB insert failed — attempt S3 cleanup, but only if we own the key.
		// CAVEAT: For non-edit files the key is hash-based. If another user_files row already
		// referenced the same key (via ON CONFLICT DO UPDATE), the key now points to that row
		// and deleting it here would orphan that record. In practice this is rare (same file
		// content from two different users hitting the same S3 key is an edge case), and the
		// periodic reconciler handles resulting orphans. Accepting this trade-off to avoid
		// dangling S3 objects in the common failure case.
		slog.Error("user_files insert failed, cleaning up S3 object",
			"error", err, "key", s3Key, "bucket", bucket)
		_, _ = s.client.DeleteObject(ctx, &s3.DeleteObjectInput{
			Bucket: aws.String(bucket),
			Key:    aws.String(s3Key),
		})
		return FileRecord{}, fmt.Errorf("db insert: %w", err)
	}

	// Post-write quota check: catches cases where SizeBytes was 0 on entry.
	if s.maxUserBytes > 0 && int64(len(buf)) > 0 {
		usage, qErr := diskUsage(ctx, s.pool, entry.UserID)
		if qErr != nil {
			slog.Warn("shadowfs: post-write quota check failed", "error", qErr)
		}
		if usage > s.maxUserBytes {
			// Over quota — clean up S3 object and DB record.
			_, _ = s.client.DeleteObject(ctx, &s3.DeleteObjectInput{
				Bucket: aws.String(bucket),
				Key:    aws.String(s3Key),
			})
			_, _ = s.pool.Exec(ctx, `DELETE FROM user_files WHERE id = $1`, rec.ID)
			return FileRecord{}, fmt.Errorf("shadowfs: storage quota exceeded (post-write check)")
		}
	}

	return rec, nil
}

// Get retrieves a file's contents from S3.
func (s *S3ShadowFS) Get(ctx context.Context, userID, fileID string) (io.ReadCloser, error) {
	var storagePath, fileType, status string
	err := s.pool.QueryRow(ctx,
		`SELECT storage_path, file_type, status FROM user_files WHERE id = $1 AND user_id = $2`, fileID, userID,
	).Scan(&storagePath, &fileType, &status)
	if err != nil {
		return nil, fmt.Errorf("file not found: %w", err)
	}
	if status == "purging" {
		return nil, fmt.Errorf("file is being purged")
	}

	bucket := s.bucketFor(fileType)
	out, err := s.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(storagePath),
	})
	if err != nil {
		return nil, fmt.Errorf("s3 get: %w", err)
	}
	return out.Body, nil
}

// Delete removes a file from S3 and unregisters it.
func (s *S3ShadowFS) Delete(ctx context.Context, userID, fileID string) error {
	var storagePath, fileType, fileName string
	var hasSidecar bool
	err := s.pool.QueryRow(ctx,
		`SELECT storage_path, file_type, file_name, has_sidecar FROM user_files WHERE id = $1 AND user_id = $2`, fileID, userID,
	).Scan(&storagePath, &fileType, &fileName, &hasSidecar)
	if err != nil {
		return fmt.Errorf("file not found: %w", err)
	}

	bucket := s.bucketFor(fileType)

	// Delete DB record first — an orphaned S3 object is safe (reconciler cleans it),
	// but a dangling DB record pointing to a deleted S3 object causes user-visible errors.
	_, err = s.pool.Exec(ctx,
		`DELETE FROM user_files WHERE id = $1 AND user_id = $2`, fileID, userID)
	if err != nil {
		return fmt.Errorf("db delete: %w", err)
	}

	// Delete main S3 object.
	if _, err := s.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(storagePath),
	}); err != nil {
		slog.Warn("s3 delete failed (orphaned object, reconciler will clean)",
			"error", err, "key", storagePath, "bucket", bucket)
	}

	// Delete sidecar if present.
	if hasSidecar {
		sidecarKey := sidecarKeyFor(storagePath, fileType)
		if sidecarKey != "" {
			_, _ = s.client.DeleteObject(ctx, &s3.DeleteObjectInput{
				Bucket: aws.String(bucket),
				Key:    aws.String(sidecarKey),
			})
		}
	}

	return nil
}

// DeleteAllForUser purges all files for a user across all configured buckets.
// Acquires a per-user advisory lock to prevent races with concurrent uploads.
func (s *S3ShadowFS) DeleteAllForUser(ctx context.Context, userID string) error {
	// Advisory lock prevents concurrent uploads from creating files while we purge.
	// Must use a pinned connection — advisory locks are session-level and would
	// silently release if the pooled connection is returned between calls.
	conn, err := s.pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("acquire conn: %w", err)
	}
	defer conn.Release()

	// L56: Use Go-side FNV-1a 64-bit instead of PostgreSQL's hashtext (32-bit, collision-prone).
	lockKey := advisoryLockKey("shadowfs:" + userID)
	if _, err := conn.Exec(ctx,
		`SELECT pg_advisory_lock($1)`, lockKey); err != nil {
		return fmt.Errorf("advisory lock: %w", err)
	}
	defer func() {
		_, _ = conn.Exec(context.Background(), `SELECT pg_advisory_unlock($1)`, lockKey)
	}()

	// Mark all files as purging.
	_, err = conn.Exec(ctx,
		`UPDATE user_files SET status = 'purging' WHERE user_id = $1`, userID)
	if err != nil {
		return fmt.Errorf("mark purging: %w", err)
	}

	// Collect all buckets (deduplicated).
	buckets := s.allBuckets()
	prefixes := []string{
		userID + "/",
		"_cache/" + userID + "/",
		"_temp/" + userID + "/",
		// Legacy S3 prefixes from pre-ShadowFS era.
		"activity-photos/" + userID + "/",
		"gear/" + userID + "/",
		"route-photos/" + userID + "/",
		"marker-images/" + userID + "/",
		"tag-covers/" + userID + "/",
		// M36: Edit sidecars stored by the activity editor (edits/{userID}/).
		"edits/" + userID + "/",
	}

	for _, bucket := range buckets {
		for _, prefix := range prefixes {
			if err := s.deletePrefix(ctx, bucket, prefix); err != nil {
				slog.Error("deletePrefix failed",
					"bucket", bucket, "prefix", prefix, "error", err)
				// Continue with other buckets/prefixes.
			}
		}
	}

	// Clean journal entries (parity with LocalShadowFS.DeleteAllForUser).
	_, _ = conn.Exec(ctx,
		`DELETE FROM migration_shadowfs_journal WHERE user_id = $1`, userID)

	// Remove from DB.
	_, err = conn.Exec(ctx,
		`DELETE FROM user_files WHERE user_id = $1`, userID)
	if err != nil {
		return fmt.Errorf("db purge: %w", err)
	}
	return nil
}

// List returns all file records for a user, optionally filtered by type.
func (s *S3ShadowFS) List(ctx context.Context, userID string, fileType *string) ([]FileRecord, error) {
	return listRecords(ctx, s.pool, userID, fileType)
}

// DiskUsage returns the total bytes stored for a user.
func (s *S3ShadowFS) DiskUsage(ctx context.Context, userID string) (int64, error) {
	return diskUsage(ctx, s.pool, userID)
}

// StreamToZip writes all active user files into a zip archive.
func (s *S3ShadowFS) StreamToZip(ctx context.Context, userID string, zw *zip.Writer) error {
	records, err := listRecords(ctx, s.pool, userID, nil)
	if err != nil {
		return fmt.Errorf("list records: %w", err)
	}

	var totalBytes int64
	for _, rec := range records {
		// Skip cache paths.
		if strings.HasPrefix(rec.StoragePath, "_cache/") {
			continue
		}

		// 10 GB zip limit.
		totalBytes += rec.SizeBytes
		if totalBytes > maxZipBytes {
			return fmt.Errorf("zip export exceeds %d byte limit", maxZipBytes)
		}

		// Determine zip entry path: strip userID prefix FIRST, then prepend _archive/.
		zipPath := rec.StoragePath
		if idx := strings.Index(zipPath, "/"); idx >= 0 {
			candidate := zipPath[idx+1:]
			if candidate != "" {
				zipPath = candidate
			}
		}
		if rec.Status == "archived" {
			zipPath = "_archive/" + zipPath
		}

		// ZIP Slip prevention: Clean the path after building it.
		zipPath = filepath.Clean(zipPath)
		if strings.Contains(zipPath, "..") || strings.HasPrefix(zipPath, "/") {
			slog.Warn("skipping suspicious path in zip", "path", rec.StoragePath)
			continue
		}

		bucket := s.bucketFor(rec.FileType)
		out, err := s.client.GetObject(ctx, &s3.GetObjectInput{
			Bucket: aws.String(bucket),
			Key:    aws.String(rec.StoragePath),
		})
		if err != nil {
			slog.Warn("skipping file in zip export",
				"file_id", rec.ID, "key", rec.StoragePath, "error", err)
			continue
		}

		w, err := zw.CreateHeader(&zip.FileHeader{
			Name:     zipPath,
			Method:   zip.Deflate,
			Modified: rec.CreatedAt,
		})
		if err != nil {
			out.Body.Close()
			return fmt.Errorf("zip create header: %w", err)
		}
		_, err = io.Copy(w, out.Body)
		out.Body.Close()
		if err != nil {
			return fmt.Errorf("zip copy: %w", err)
		}
	}
	return nil
}

// Resolve returns a pre-signed URL for direct access to the file.
// NOTE: Presigned URLs remain valid for up to 15 minutes after file/user deletion.
// For sensitive types, use server-side proxy (ServeFile handler). This is industry-standard behavior.
func (s *S3ShadowFS) Resolve(ctx context.Context, userID, fileID string) (ResolvedFile, error) {
	var rec FileRecord
	err := s.pool.QueryRow(ctx,
		`SELECT id, user_id, file_type, storage_backend, storage_path, content_type,
			file_name, size_bytes, file_hash, parent_type, parent_id, has_sidecar,
			is_renaming, status, replaced_by, created_at, updated_at
		FROM user_files WHERE id = $1 AND user_id = $2 AND status != 'purging'`, fileID, userID,
	).Scan(&rec.ID, &rec.UserID, &rec.FileType, &rec.StorageBackend, &rec.StoragePath,
		&rec.ContentType, &rec.FileName, &rec.SizeBytes, &rec.FileHash, &rec.ParentType,
		&rec.ParentID, &rec.HasSidecar, &rec.IsRenaming, &rec.Status, &rec.ReplacedBy,
		&rec.CreatedAt, &rec.UpdatedAt)
	if err != nil {
		return ResolvedFile{}, fmt.Errorf("file not found: %w", err)
	}

	bucket := s.bucketFor(rec.FileType)
	presigned, err := s.presigner.PresignGetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(rec.StoragePath),
	}, s3.WithPresignExpires(presignExpiry))
	if err != nil {
		return ResolvedFile{}, fmt.Errorf("presign: %w", err)
	}

	expiresAt := time.Now().Add(presignExpiry)
	return ResolvedFile{
		Type:        "s3",
		URL:         presigned.URL,
		ExpiresAt:   expiresAt,
		ContentType: rec.ContentType,
		FileName:    rec.FileName,
		SizeBytes:   rec.SizeBytes,
	}, nil
}

// Archive marks a file as archived. For S3, this is metadata-only (no CopyObject).
func (s *S3ShadowFS) Archive(ctx context.Context, userID, fileID, replacedByID string) error {
	if s.keepVersions == 0 {
		return s.Delete(ctx, userID, fileID)
	}

	result, err := s.pool.Exec(ctx,
		`UPDATE user_files SET status = 'archived', replaced_by = $1, updated_at = NOW()
		WHERE id = $2 AND user_id = $3 AND status = 'active'`,
		replacedByID, fileID, userID)
	if err != nil {
		return fmt.Errorf("archive: %w", err)
	}
	if result.RowsAffected() == 0 {
		return fmt.Errorf("file not found or already archived")
	}
	return nil
}

// --- helpers ---

// allBuckets returns a deduplicated list of all configured buckets.
func (s *S3ShadowFS) allBuckets() []string {
	seen := map[string]bool{s.bucket: true}
	buckets := []string{s.bucket}
	for _, b := range []string{s.bucketActivity, s.bucketMedia} {
		if b != "" && !seen[b] {
			seen[b] = true
			buckets = append(buckets, b)
		}
	}
	return buckets
}

// deletePrefix deletes all objects under a prefix using batched DeleteObjects.
// M32: Returns accumulated errors from partial batch failures instead of swallowing them.
func (s *S3ShadowFS) deletePrefix(ctx context.Context, bucket, prefix string) error {
	paginator := s3.NewListObjectsV2Paginator(s.client, &s3.ListObjectsV2Input{
		Bucket: aws.String(bucket),
		Prefix: aws.String(prefix),
	})

	var errs []error
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return fmt.Errorf("list objects: %w", err)
		}
		if len(page.Contents) == 0 {
			continue
		}

		// Batch into groups of 1000 (S3 limit).
		for i := 0; i < len(page.Contents); i += deleteBatch {
			end := min(i+deleteBatch, len(page.Contents))
			objects := make([]s3types.ObjectIdentifier, 0, end-i)
			for _, obj := range page.Contents[i:end] {
				objects = append(objects, s3types.ObjectIdentifier{
					Key: obj.Key,
				})
			}

			out, err := s.client.DeleteObjects(ctx, &s3.DeleteObjectsInput{
				Bucket: aws.String(bucket),
				Delete: &s3types.Delete{
					Objects: objects,
					Quiet:   aws.Bool(true),
				},
			})
			if err != nil {
				slog.Error("batch delete failed",
					"bucket", bucket, "prefix", prefix,
					"batch_size", len(objects), "error", err)
				errs = append(errs, err)
				// Continue with remaining batches.
			} else if out != nil && len(out.Errors) > 0 {
				for _, e := range out.Errors {
					slog.Error("s3 delete object error",
						"bucket", bucket, "key", aws.ToString(e.Key),
						"code", aws.ToString(e.Code), "message", aws.ToString(e.Message))
					errs = append(errs, fmt.Errorf("s3 delete %s: %s", aws.ToString(e.Key), aws.ToString(e.Message)))
				}
			}
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("deletePrefix %s/%s: %d batch errors (first: %w)", bucket, prefix, len(errs), errs[0])
	}
	return nil
}

// sidecarKeyFor returns the sidecar S3 key for a given storage path.
// Media files use .meta.json, others use .meta.
func sidecarKeyFor(storagePath, fileType string) string {
	if storagePath == "" {
		return ""
	}
	ext := filepath.Ext(storagePath)
	stem := strings.TrimSuffix(storagePath, ext)
	if fileType == "media" {
		return stem + ".meta.json"
	}
	return stem + ".meta"
}

var _ ShadowFS = (*S3ShadowFS)(nil)

// sanitizeExt cleans a file extension, allowing only alphanumeric and dot.
func sanitizeExt(ext string) string {
	if ext == "" {
		return ""
	}
	if !strings.HasPrefix(ext, ".") {
		ext = "." + ext
	}
	// Keep only the dot and lowercase alphanumeric chars.
	var b strings.Builder
	b.WriteByte('.')
	for _, r := range ext[1:] {
		lower := unicode.ToLower(r)
		if (lower >= 'a' && lower <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(lower)
		}
	}
	if b.Len() == 1 {
		return "" // Only had a dot, no valid chars.
	}
	return b.String()
}
