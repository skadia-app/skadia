package gdprartifacts

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/google/uuid"
)

const (
	minS3MultipartPartSize    = 5 << 20
	defaultS3ArtifactPartSize = 64 << 20
)

type S3Config struct {
	Endpoint        string
	AccessKey       string
	SecretKey       string
	Region          string
	ForcePathStyle  bool
	ExportBucket    string
	ImportBucket    string
	ExportPrefix    string
	ImportPrefix    string
	PartSizeBytes   int64
	ReadBlockBytes  int64
	ReadCacheBlocks int
}

type s3CoreAPI interface {
	s3GetObjectAPI
	PutObject(context.Context, *s3.PutObjectInput, ...func(*s3.Options)) (*s3.PutObjectOutput, error)
	CreateMultipartUpload(context.Context, *s3.CreateMultipartUploadInput, ...func(*s3.Options)) (*s3.CreateMultipartUploadOutput, error)
	UploadPart(context.Context, *s3.UploadPartInput, ...func(*s3.Options)) (*s3.UploadPartOutput, error)
	CompleteMultipartUpload(context.Context, *s3.CompleteMultipartUploadInput, ...func(*s3.Options)) (*s3.CompleteMultipartUploadOutput, error)
	AbortMultipartUpload(context.Context, *s3.AbortMultipartUploadInput, ...func(*s3.Options)) (*s3.AbortMultipartUploadOutput, error)
	HeadObject(context.Context, *s3.HeadObjectInput, ...func(*s3.Options)) (*s3.HeadObjectOutput, error)
	DeleteObject(context.Context, *s3.DeleteObjectInput, ...func(*s3.Options)) (*s3.DeleteObjectOutput, error)
	ListObjectsV2(context.Context, *s3.ListObjectsV2Input, ...func(*s3.Options)) (*s3.ListObjectsV2Output, error)
	DeleteObjects(context.Context, *s3.DeleteObjectsInput, ...func(*s3.Options)) (*s3.DeleteObjectsOutput, error)
	ListMultipartUploads(context.Context, *s3.ListMultipartUploadsInput, ...func(*s3.Options)) (*s3.ListMultipartUploadsOutput, error)
}

type s3GetObjectAPI interface {
	GetObject(context.Context, *s3.GetObjectInput, ...func(*s3.Options)) (*s3.GetObjectOutput, error)
}

type S3Store struct {
	client s3CoreAPI
	cfg    S3Config
}

func NewS3Store(cfg S3Config) (*S3Store, error) {
	if strings.TrimSpace(cfg.Endpoint) == "" {
		return nil, fmt.Errorf("s3 endpoint is required")
	}
	if strings.TrimSpace(cfg.AccessKey) == "" || strings.TrimSpace(cfg.SecretKey) == "" {
		return nil, fmt.Errorf("s3 credentials are required")
	}
	if cfg.Region == "" {
		cfg.Region = "auto"
	}
	client := s3.New(s3.Options{
		BaseEndpoint: aws.String(cfg.Endpoint),
		Region:       cfg.Region,
		Credentials:  credentials.NewStaticCredentialsProvider(cfg.AccessKey, cfg.SecretKey, ""),
		UsePathStyle: cfg.ForcePathStyle,
	})
	return newS3StoreForTest(client, cfg), nil
}

func newS3StoreForTest(client s3CoreAPI, cfg S3Config) *S3Store {
	cfg.ExportPrefix = cleanS3Prefix(cfg.ExportPrefix)
	cfg.ImportPrefix = cleanS3Prefix(cfg.ImportPrefix)
	if cfg.PartSizeBytes <= 0 {
		cfg.PartSizeBytes = defaultS3ArtifactPartSize
	} else if cfg.PartSizeBytes < minS3MultipartPartSize {
		cfg.PartSizeBytes = minS3MultipartPartSize
	}
	if cfg.ReadBlockBytes <= 0 {
		cfg.ReadBlockBytes = 1 << 20
	}
	if cfg.ReadCacheBlocks <= 0 {
		cfg.ReadCacheBlocks = 4
	}
	return &S3Store{client: client, cfg: cfg}
}

func (s *S3Store) CreateWriter(_ context.Context, spec ObjectSpec) (ObjectWriter, error) {
	bucket, prefix, err := s.bucketAndPrefix(spec.Kind)
	if err != nil {
		return nil, err
	}
	key, err := objectKey(prefix, spec.UserID, spec.JobID, spec.Role, spec.Name)
	if err != nil {
		return nil, err
	}
	return &s3ObjectWriter{
		store:       s,
		spec:        spec,
		bucket:      bucket,
		key:         key,
		contentType: defaultContentType(spec.ContentType),
		partSize:    s.cfg.PartSizeBytes,
		hash:        sha256.New(),
	}, nil
}

func (s *S3Store) Open(ctx context.Context, ref ObjectRef, rangeSpec *RangeSpec) (ObjectReader, error) {
	rangeHeader := ""
	if rangeSpec != nil && (rangeSpec.Start != nil || rangeSpec.End != nil) {
		start := int64(0)
		if rangeSpec.Start != nil {
			start = *rangeSpec.Start
		}
		if rangeSpec.End != nil {
			rangeHeader = fmt.Sprintf("bytes=%d-%d", start, *rangeSpec.End)
		} else {
			rangeHeader = fmt.Sprintf("bytes=%d-", start)
		}
	}
	input := &s3.GetObjectInput{Bucket: aws.String(ref.Bucket), Key: aws.String(ref.Key)}
	if rangeHeader != "" {
		input.Range = aws.String(rangeHeader)
	}
	out, err := s.client.GetObject(ctx, input)
	if err != nil {
		return nil, err
	}
	return &s3ObjectReader{
		ReadCloser:  out.Body,
		size:        aws.ToInt64(out.ContentLength),
		contentType: aws.ToString(out.ContentType),
	}, nil
}

func (s *S3Store) OpenReaderAt(ctx context.Context, ref ObjectRef) (SizedReaderAt, error) {
	stat, err := s.Stat(ctx, ref)
	if err != nil {
		return nil, err
	}
	if stat.State != ObjectPresent {
		return nil, fmt.Errorf("object not present: %s", stat.State)
	}
	return newS3RangeReaderAt(s.client, ref.Bucket, ref.Key, stat.SizeBytes, s.cfg.ReadBlockBytes, s.cfg.ReadCacheBlocks), nil
}

func (s *S3Store) Stat(ctx context.Context, ref ObjectRef) (ObjectStat, error) {
	out, err := s.client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(ref.Bucket),
		Key:    aws.String(ref.Key),
	})
	if err != nil {
		if isS3NotFound(err) {
			return ObjectStat{State: ObjectMissing}, nil
		}
		return ObjectStat{State: ObjectUnknown, Err: err}, nil
	}
	return ObjectStat{State: ObjectPresent, SizeBytes: aws.ToInt64(out.ContentLength), SHA256: ref.SHA256}, nil
}

func (s *S3Store) Delete(ctx context.Context, ref ObjectRef) error {
	_, err := s.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(ref.Bucket),
		Key:    aws.String(ref.Key),
	})
	return err
}

func (s *S3Store) DeletePrefix(ctx context.Context, prefix ArtifactPrefix) error {
	bucket, s3Prefix, err := s.bucketAndPrefix(prefix.Kind)
	if err != nil {
		return err
	}
	if prefix.UserID == "" || prefix.JobID == "" {
		return fmt.Errorf("refusing to delete unscoped artifact prefix")
	}
	scoped, err := scopedPrefix(s3Prefix, prefix)
	if err != nil {
		return err
	}
	var token *string
	for {
		out, err := s.client.ListObjectsV2(ctx, &s3.ListObjectsV2Input{
			Bucket:            aws.String(bucket),
			Prefix:            aws.String(scoped),
			ContinuationToken: token,
		})
		if err != nil {
			return err
		}
		if len(out.Contents) > 0 {
			objects := make([]s3types.ObjectIdentifier, 0, len(out.Contents))
			for _, obj := range out.Contents {
				objects = append(objects, s3types.ObjectIdentifier{Key: obj.Key})
			}
			if _, err := s.client.DeleteObjects(ctx, &s3.DeleteObjectsInput{
				Bucket: aws.String(bucket),
				Delete: &s3types.Delete{Objects: objects, Quiet: aws.Bool(true)},
			}); err != nil {
				return err
			}
		}
		if !aws.ToBool(out.IsTruncated) {
			return nil
		}
		token = out.NextContinuationToken
	}
}

func (s *S3Store) AbortIncompleteUploads(ctx context.Context, prefix ArtifactPrefix) error {
	bucket, s3Prefix, err := s.bucketAndPrefix(prefix.Kind)
	if err != nil {
		return err
	}
	scoped, err := scopedPrefix(s3Prefix, prefix)
	if err != nil {
		return err
	}
	var keyMarker *string
	var uploadMarker *string
	for {
		out, err := s.client.ListMultipartUploads(ctx, &s3.ListMultipartUploadsInput{
			Bucket:         aws.String(bucket),
			Prefix:         aws.String(scoped),
			KeyMarker:      keyMarker,
			UploadIdMarker: uploadMarker,
		})
		if err != nil {
			return err
		}
		for _, upload := range out.Uploads {
			if _, err := s.client.AbortMultipartUpload(ctx, &s3.AbortMultipartUploadInput{
				Bucket:   aws.String(bucket),
				Key:      upload.Key,
				UploadId: upload.UploadId,
			}); err != nil {
				return err
			}
		}
		if !aws.ToBool(out.IsTruncated) {
			return nil
		}
		keyMarker = out.NextKeyMarker
		uploadMarker = out.NextUploadIdMarker
	}
}

func (s *S3Store) EnsureLifecycle(context.Context) error {
	return fmt.Errorf("S3 lifecycle setup is not managed automatically for this backend; app cleanup remains authoritative")
}

func (s *S3Store) bucketAndPrefix(kind string) (string, string, error) {
	switch kind {
	case "export":
		if s.cfg.ExportBucket == "" {
			return "", "", fmt.Errorf("export bucket is required")
		}
		return s.cfg.ExportBucket, s.cfg.ExportPrefix, nil
	case "import":
		if s.cfg.ImportBucket == "" {
			return "", "", fmt.Errorf("import bucket is required")
		}
		return s.cfg.ImportBucket, s.cfg.ImportPrefix, nil
	default:
		return "", "", fmt.Errorf("unsupported artifact kind %q", kind)
	}
}

type s3ObjectWriter struct {
	store       *S3Store
	spec        ObjectSpec
	bucket      string
	key         string
	contentType string
	partSize    int64
	hash        hashWriter
	buffer      bytes.Buffer
	written     int64
	uploadID    string
	parts       []s3types.CompletedPart
	partNumber  int32
	aborted     bool
	committed   bool
}

type hashWriter interface {
	io.Writer
	Sum([]byte) []byte
}

func (w *s3ObjectWriter) Write(p []byte) (int, error) {
	if w.aborted || w.committed {
		return 0, fmt.Errorf("writer is closed")
	}
	total := len(p)
	if _, err := w.hash.Write(p); err != nil {
		return 0, err
	}
	w.written += int64(len(p))
	for len(p) > 0 {
		space := int(w.partSize) - w.buffer.Len()
		if space > len(p) {
			space = len(p)
		}
		if _, err := w.buffer.Write(p[:space]); err != nil {
			return 0, err
		}
		p = p[space:]
		if int64(w.buffer.Len()) >= w.partSize {
			if err := w.ensureMultipartAndUpload(context.Background(), false); err != nil {
				_ = w.Abort(context.Background())
				return 0, err
			}
		}
	}
	return total, nil
}

func (w *s3ObjectWriter) Commit(ctx context.Context) (ObjectRef, error) {
	if w.aborted {
		return ObjectRef{}, fmt.Errorf("writer aborted")
	}
	if w.committed {
		return ObjectRef{}, fmt.Errorf("writer already committed")
	}
	if w.uploadID == "" {
		_, err := w.store.client.PutObject(ctx, &s3.PutObjectInput{
			Bucket:        aws.String(w.bucket),
			Key:           aws.String(w.key),
			Body:          bytes.NewReader(w.buffer.Bytes()),
			ContentType:   aws.String(w.contentType),
			ContentLength: aws.Int64(w.written),
		})
		if err != nil {
			return ObjectRef{}, err
		}
	} else {
		if w.buffer.Len() > 0 {
			if err := w.uploadBufferedPart(ctx); err != nil {
				_ = w.Abort(ctx)
				return ObjectRef{}, err
			}
		}
		_, err := w.store.client.CompleteMultipartUpload(ctx, &s3.CompleteMultipartUploadInput{
			Bucket:   aws.String(w.bucket),
			Key:      aws.String(w.key),
			UploadId: aws.String(w.uploadID),
			MultipartUpload: &s3types.CompletedMultipartUpload{
				Parts: w.parts,
			},
		})
		if err != nil {
			_ = w.Abort(ctx)
			return ObjectRef{}, err
		}
	}
	w.committed = true
	now := time.Now().UTC()
	return ObjectRef{
		Backend:     "s3",
		Kind:        w.spec.Kind,
		UserID:      w.spec.UserID,
		JobID:       w.spec.JobID,
		Role:        w.spec.Role,
		Name:        w.spec.Name,
		Bucket:      w.bucket,
		Key:         w.key,
		SHA256:      hex.EncodeToString(w.hash.Sum(nil)),
		SizeBytes:   w.written,
		ContentType: w.contentType,
		CreatedAt:   now,
		ExpiresAt:   w.spec.ExpiresAt,
	}, nil
}

func (w *s3ObjectWriter) Abort(ctx context.Context) error {
	if w.aborted || w.committed {
		return nil
	}
	w.aborted = true
	w.buffer.Reset()
	if w.uploadID == "" {
		return nil
	}
	_, err := w.store.client.AbortMultipartUpload(ctx, &s3.AbortMultipartUploadInput{
		Bucket:   aws.String(w.bucket),
		Key:      aws.String(w.key),
		UploadId: aws.String(w.uploadID),
	})
	return err
}

func (w *s3ObjectWriter) ensureMultipartAndUpload(ctx context.Context, final bool) error {
	if w.uploadID == "" {
		out, err := w.store.client.CreateMultipartUpload(ctx, &s3.CreateMultipartUploadInput{
			Bucket:      aws.String(w.bucket),
			Key:         aws.String(w.key),
			ContentType: aws.String(w.contentType),
		})
		if err != nil {
			return err
		}
		w.uploadID = aws.ToString(out.UploadId)
	}
	if final || int64(w.buffer.Len()) >= w.partSize {
		return w.uploadBufferedPart(ctx)
	}
	return nil
}

func (w *s3ObjectWriter) uploadBufferedPart(ctx context.Context) error {
	if w.buffer.Len() == 0 {
		return nil
	}
	w.partNumber++
	body := append([]byte(nil), w.buffer.Bytes()...)
	w.buffer.Reset()
	out, err := w.store.client.UploadPart(ctx, &s3.UploadPartInput{
		Bucket:        aws.String(w.bucket),
		Key:           aws.String(w.key),
		UploadId:      aws.String(w.uploadID),
		PartNumber:    aws.Int32(w.partNumber),
		Body:          bytes.NewReader(body),
		ContentLength: aws.Int64(int64(len(body))),
	})
	if err != nil {
		return err
	}
	w.parts = append(w.parts, s3types.CompletedPart{
		ETag:       out.ETag,
		PartNumber: aws.Int32(w.partNumber),
	})
	return nil
}

type s3ObjectReader struct {
	io.ReadCloser
	size        int64
	contentType string
}

func (r *s3ObjectReader) Size() int64 { return r.size }

func (r *s3ObjectReader) ContentType() string { return r.contentType }

func objectKey(prefix, userID, jobID string, role ObjectRole, name string) (string, error) {
	if _, err := uuid.Parse(userID); err != nil {
		return "", fmt.Errorf("invalid user_id: %w", err)
	}
	if _, err := uuid.Parse(jobID); err != nil {
		return "", fmt.Errorf("invalid job_id: %w", err)
	}
	if name == "" || strings.Contains(name, "/") || strings.Contains(name, "\\") || path.Clean(name) != name {
		return "", fmt.Errorf("invalid object name %q", name)
	}
	segment := ""
	switch role {
	case RoleSingleZIP:
		segment = name
	case RoleManifest, RoleMetadataZIP, RoleFilesZIP:
		segment = path.Join("multipart", name)
	default:
		return "", fmt.Errorf("unsupported object role %q", role)
	}
	return path.Join(prefix, userID, jobID, segment), nil
}

func scopedPrefix(prefix string, artifactPrefix ArtifactPrefix) (string, error) {
	if artifactPrefix.UserID == "" && artifactPrefix.JobID == "" {
		if prefix == "" {
			return "", fmt.Errorf("refusing empty artifact prefix")
		}
		return path.Clean(prefix) + "/", nil
	}
	if _, err := uuid.Parse(artifactPrefix.UserID); err != nil {
		return "", fmt.Errorf("invalid user_id: %w", err)
	}
	if _, err := uuid.Parse(artifactPrefix.JobID); err != nil {
		return "", fmt.Errorf("invalid job_id: %w", err)
	}
	return path.Join(prefix, artifactPrefix.UserID, artifactPrefix.JobID) + "/", nil
}

func cleanS3Prefix(prefix string) string {
	return strings.Trim(strings.TrimSpace(prefix), "/")
}

func defaultContentType(contentType string) string {
	if contentType == "" {
		return "application/octet-stream"
	}
	return contentType
}

func isS3NotFound(err error) bool {
	if errors.Is(err, os.ErrNotExist) {
		return true
	}
	var noSuchKey *s3types.NoSuchKey
	if errors.As(err, &noSuchKey) {
		return true
	}
	var notFound *s3types.NotFound
	return errors.As(err, &notFound)
}

var _ Store = (*S3Store)(nil)
var _ IncompleteMultipartAborter = (*S3Store)(nil)
var _ LifecycleEnsurer = (*S3Store)(nil)
