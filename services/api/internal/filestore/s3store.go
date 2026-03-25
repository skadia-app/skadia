package filestore

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

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
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

// S3FileStore implements FileStore using S3-compatible object storage.
type S3FileStore struct {
	pool           *pgxpool.Pool
	client         *s3.Client
	presigner      *s3.PresignClient
	bucket         string
	bucketActivity string
	bucketMedia    string
	keepVersions   int
	maxUserBytes   int64
}

// NewS3FileStore creates an S3FileStore backed by an S3-compatible service.
func NewS3FileStore(pool *pgxpool.Pool, cfg S3Config, keepVersions int, maxUserBytes int64) *S3FileStore {
	client := s3.New(s3.Options{
		BaseEndpoint: aws.String(cfg.Endpoint),
		Region:       cfg.Region,
		Credentials:  credentials.NewStaticCredentialsProvider(cfg.AccessKey, cfg.SecretKey, ""),
		UsePathStyle: cfg.ForcePathStyle,
	})
	return &S3FileStore{
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
func (s *S3FileStore) bucketFor(fileType string) string {
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
func (s *S3FileStore) Put(ctx context.Context, entry FileEntry, data io.Reader) (FileRecord, error) {
	// Validate file type.
	if _, ok := fileTypeDirName[entry.FileType]; !ok {
		return FileRecord{}, fmt.Errorf("unknown file type %q", entry.FileType)
	}

	// Quota check.
	used, err := diskUsage(ctx, s.pool, entry.UserID)
	if err != nil {
		return FileRecord{}, fmt.Errorf("quota check: %w", err)
	}
	if s.maxUserBytes > 0 && used+entry.SizeBytes > s.maxUserBytes {
		return FileRecord{}, fmt.Errorf("quota exceeded: %d + %d > %d", used, entry.SizeBytes, s.maxUserBytes)
	}

	// Read all data into buffer (capped at 500 MB).
	buf, err := io.ReadAll(io.LimitReader(data, maxUploadBytes+1))
	if err != nil {
		return FileRecord{}, fmt.Errorf("reading upload data: %w", err)
	}
	if len(buf) > maxUploadBytes {
		return FileRecord{}, fmt.Errorf("upload exceeds maximum size of %d bytes", maxUploadBytes)
	}

	// SHA256 hash.
	h := sha256.Sum256(buf)
	hash := hex.EncodeToString(h[:])
	if entry.FileHash == "" {
		entry.FileHash = hash
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
			entry.FileName, len(buf), entry.FileHash, entry.ParentType, entry.ParentID,
			entry.HasSidecar,
		).Scan(&rec.ID, &rec.UserID, &rec.FileType, &rec.StorageBackend, &rec.StoragePath,
			&rec.ContentType, &rec.FileName, &rec.SizeBytes, &rec.FileHash, &rec.ParentType,
			&rec.ParentID, &rec.HasSidecar, &rec.IsRenaming, &rec.Status, &rec.ReplacedBy,
			&rec.CreatedAt, &rec.UpdatedAt)
	} else {
		err = s.pool.QueryRow(ctx,
			`INSERT INTO user_files (id, user_id, file_type, storage_backend, storage_path,
				content_type, file_name, size_bytes, file_hash, parent_type, parent_id,
				has_sidecar, status)
			VALUES ($1, $2, $3, 's3', $4, $5, $6, $7, $8, $9, $10, $11, 'active')
			ON CONFLICT (storage_backend, storage_path) DO UPDATE SET
				size_bytes = EXCLUDED.size_bytes, file_hash = EXCLUDED.file_hash,
				content_type = EXCLUDED.content_type, updated_at = NOW()
			RETURNING id, user_id, file_type, storage_backend, storage_path, content_type,
				file_name, size_bytes, file_hash, parent_type, parent_id, has_sidecar,
				is_renaming, status, replaced_by, created_at, updated_at`,
			fileID, entry.UserID, entry.FileType, s3Key, contentType,
			entry.FileName, len(buf), entry.FileHash, entry.ParentType, entry.ParentID,
			entry.HasSidecar,
		).Scan(&rec.ID, &rec.UserID, &rec.FileType, &rec.StorageBackend, &rec.StoragePath,
			&rec.ContentType, &rec.FileName, &rec.SizeBytes, &rec.FileHash, &rec.ParentType,
			&rec.ParentID, &rec.HasSidecar, &rec.IsRenaming, &rec.Status, &rec.ReplacedBy,
			&rec.CreatedAt, &rec.UpdatedAt)
	}
	if err != nil {
		// DB insert failed — attempt S3 cleanup, but only if we own the key.
		slog.Error("user_files insert failed, cleaning up S3 object",
			"error", err, "key", s3Key, "bucket", bucket)
		_, _ = s.client.DeleteObject(ctx, &s3.DeleteObjectInput{
			Bucket: aws.String(bucket),
			Key:    aws.String(s3Key),
		})
		return FileRecord{}, fmt.Errorf("db insert: %w", err)
	}

	return rec, nil
}

// Get retrieves a file's contents from S3.
func (s *S3FileStore) Get(ctx context.Context, userID, fileID string) (io.ReadCloser, error) {
	var storagePath, fileType, status string
	err := s.pool.QueryRow(ctx,
		`SELECT storage_path, file_type, status FROM user_files WHERE id = $1 AND user_id = $2`, fileID, userID,
	).Scan(&storagePath, &fileType, &status)
	if err != nil {
		return nil, fmt.Errorf("file not found: %w", err)
	}
	if status != "active" {
		return nil, fmt.Errorf("file is %s", status)
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
func (s *S3FileStore) Delete(ctx context.Context, userID, fileID string) error {
	var storagePath, fileType, fileName string
	var hasSidecar bool
	err := s.pool.QueryRow(ctx,
		`SELECT storage_path, file_type, file_name, has_sidecar FROM user_files WHERE id = $1 AND user_id = $2`, fileID, userID,
	).Scan(&storagePath, &fileType, &fileName, &hasSidecar)
	if err != nil {
		return fmt.Errorf("file not found: %w", err)
	}

	bucket := s.bucketFor(fileType)

	// Delete main object.
	_, err = s.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(storagePath),
	})
	if err != nil {
		slog.Warn("s3 delete failed", "error", err, "key", storagePath)
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

	// Remove from DB.
	_, err = s.pool.Exec(ctx,
		`DELETE FROM user_files WHERE id = $1 AND user_id = $2`, fileID, userID)
	if err != nil {
		return fmt.Errorf("db delete: %w", err)
	}
	return nil
}

// DeleteAllForUser purges all files for a user across all configured buckets.
func (s *S3FileStore) DeleteAllForUser(ctx context.Context, userID string) error {
	// Mark all files as purging.
	_, err := s.pool.Exec(ctx,
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

	// Remove from DB.
	_, err = s.pool.Exec(ctx,
		`DELETE FROM user_files WHERE user_id = $1`, userID)
	if err != nil {
		return fmt.Errorf("db purge: %w", err)
	}
	return nil
}

// List returns all file records for a user, optionally filtered by type.
func (s *S3FileStore) List(ctx context.Context, userID string, fileType *string) ([]FileRecord, error) {
	return listRecords(ctx, s.pool, userID, fileType)
}

// DiskUsage returns the total bytes stored for a user.
func (s *S3FileStore) DiskUsage(ctx context.Context, userID string) (int64, error) {
	return diskUsage(ctx, s.pool, userID)
}

// StreamToZip writes all active user files into a zip archive.
func (s *S3FileStore) StreamToZip(ctx context.Context, userID string, zw *zip.Writer) error {
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

		// ZIP Slip prevention.
		cleanPath := filepath.Clean(rec.StoragePath)
		if strings.HasPrefix(cleanPath, "..") || strings.HasPrefix(cleanPath, "/") {
			slog.Warn("skipping suspicious path in zip", "path", rec.StoragePath)
			continue
		}

		// 10 GB zip limit.
		totalBytes += rec.SizeBytes
		if totalBytes > maxZipBytes {
			return fmt.Errorf("zip export exceeds %d byte limit", maxZipBytes)
		}

		// Determine zip entry path.
		zipPath := rec.StoragePath
		if rec.Status == "archived" {
			zipPath = "_archive/" + zipPath
		}
		// Strip userID prefix from path for the zip.
		if idx := strings.Index(zipPath, "/"); idx >= 0 {
			candidate := zipPath[idx+1:]
			if candidate != "" {
				zipPath = candidate
			}
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
func (s *S3FileStore) Resolve(ctx context.Context, userID, fileID string) (ResolvedFile, error) {
	var rec FileRecord
	err := s.pool.QueryRow(ctx,
		`SELECT id, user_id, file_type, storage_backend, storage_path, content_type,
			file_name, size_bytes, file_hash, parent_type, parent_id, has_sidecar,
			is_renaming, status, replaced_by, created_at, updated_at
		FROM user_files WHERE id = $1 AND user_id = $2`, fileID, userID,
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
func (s *S3FileStore) Archive(ctx context.Context, userID, fileID, replacedByID string) error {
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
func (s *S3FileStore) allBuckets() []string {
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
func (s *S3FileStore) deletePrefix(ctx context.Context, bucket, prefix string) error {
	paginator := s3.NewListObjectsV2Paginator(s.client, &s3.ListObjectsV2Input{
		Bucket: aws.String(bucket),
		Prefix: aws.String(prefix),
	})

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

			_, err := s.client.DeleteObjects(ctx, &s3.DeleteObjectsInput{
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
				// Continue with remaining batches.
			}
		}
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

var _ FileStore = (*S3FileStore)(nil)

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
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		} else if r >= 'A' && r <= 'Z' {
			b.WriteRune(r + 32) // toLower
		}
	}
	if b.Len() == 1 {
		return "" // Only had a dot, no valid chars.
	}
	return b.String()
}
