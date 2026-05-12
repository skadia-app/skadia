package shadowfs

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
)

const (
	s3MaxUploadBytes       = 500 << 20 // 500 MB
	s3StreamUploadPartSize = 64 << 20
	s3DeleteBatch          = 1000
)

// S3Backend implements StorageBackend for S3-compatible object storage.
type S3Backend struct {
	client        *s3.Client
	presigner     *s3.PresignClient
	defaultBucket string
}

// NewS3Backend creates an S3Backend.
func NewS3Backend(endpoint, accessKey, secretKey, bucket, region string, forcePathStyle bool) *S3Backend {
	client := s3.New(s3.Options{
		BaseEndpoint: aws.String(endpoint),
		Region:       region,
		Credentials:  credentials.NewStaticCredentialsProvider(accessKey, secretKey, ""),
		UsePathStyle: forcePathStyle,
	})
	return &S3Backend{
		client:        client,
		presigner:     s3.NewPresignClient(client),
		defaultBucket: bucket,
	}
}

// Client returns the raw S3 client (needed for presigning in Resolve).
func (b *S3Backend) Client() *s3.Client { return b.client }

// Presigner returns the presign client.
func (b *S3Backend) Presigner() *s3.PresignClient { return b.presigner }

func (b *S3Backend) bucket(opts *BackendOpts) string {
	if opts != nil && opts.Bucket != "" {
		return opts.Bucket
	}
	return b.defaultBucket
}

func (b *S3Backend) Read(ctx context.Context, path string, opts *BackendOpts) (io.ReadCloser, error) {
	out, err := b.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(b.bucket(opts)),
		Key:    aws.String(path),
	})
	if err != nil {
		return nil, fmt.Errorf("s3 get %s: %w", path, err)
	}
	return out.Body, nil
}

func (b *S3Backend) Write(ctx context.Context, path string, data io.Reader, contentType string, exclusive bool, opts *BackendOpts) (int64, string, error) {
	// exclusive flag is ignored for S3 — hash-based keys are inherently unique.
	// Buffer entire upload (S3 PutObject needs Content-Length).
	buf, err := io.ReadAll(io.LimitReader(data, s3MaxUploadBytes+1))
	if err != nil {
		return 0, "", fmt.Errorf("read upload data: %w", err)
	}
	if len(buf) > s3MaxUploadBytes {
		return 0, "", fmt.Errorf("upload exceeds %d bytes", s3MaxUploadBytes)
	}

	h := sha256.Sum256(buf)
	hash := hex.EncodeToString(h[:])

	ct := contentType
	if ct == "" {
		ct = "application/octet-stream"
	}

	_, err = b.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:        aws.String(b.bucket(opts)),
		Key:           aws.String(path),
		Body:          bytes.NewReader(buf),
		ContentType:   aws.String(ct),
		ContentLength: aws.Int64(int64(len(buf))),
	})
	if err != nil {
		return 0, "", fmt.Errorf("s3 put %s: %w", path, err)
	}
	return int64(len(buf)), hash, nil
}

func (b *S3Backend) WriteStream(ctx context.Context, path string, data io.Reader, contentType string, exclusive bool, expectedHash string, opts *BackendOpts) (int64, string, error) {
	// exclusive flag is ignored for S3 — hash-based keys are inherently unique.
	ct := contentType
	if ct == "" {
		ct = "application/octet-stream"
	}

	bucket := b.bucket(opts)
	h := sha256.New()
	buf := make([]byte, s3StreamUploadPartSize)
	var uploadID string
	var parts []s3types.CompletedPart
	var partNumber int32
	var written int64

	abort := func() {
		if uploadID == "" {
			return
		}
		_, _ = b.client.AbortMultipartUpload(context.Background(), &s3.AbortMultipartUploadInput{
			Bucket:   aws.String(bucket),
			Key:      aws.String(path),
			UploadId: aws.String(uploadID),
		})
	}
	ensureUpload := func() error {
		if uploadID != "" {
			return nil
		}
		out, err := b.client.CreateMultipartUpload(ctx, &s3.CreateMultipartUploadInput{
			Bucket:      aws.String(bucket),
			Key:         aws.String(path),
			ContentType: aws.String(ct),
		})
		if err != nil {
			return fmt.Errorf("s3 create multipart %s: %w", path, err)
		}
		uploadID = aws.ToString(out.UploadId)
		return nil
	}
	uploadPart := func(part []byte) error {
		if len(part) == 0 {
			return nil
		}
		if err := ensureUpload(); err != nil {
			return err
		}
		partNumber++
		body := append([]byte(nil), part...)
		out, err := b.client.UploadPart(ctx, &s3.UploadPartInput{
			Bucket:        aws.String(bucket),
			Key:           aws.String(path),
			UploadId:      aws.String(uploadID),
			PartNumber:    aws.Int32(partNumber),
			Body:          bytes.NewReader(body),
			ContentLength: aws.Int64(int64(len(body))),
		})
		if err != nil {
			return fmt.Errorf("s3 upload part %s part %d: %w", path, partNumber, err)
		}
		parts = append(parts, s3types.CompletedPart{
			ETag:       out.ETag,
			PartNumber: aws.Int32(partNumber),
		})
		return nil
	}

	for {
		n, readErr := io.ReadFull(data, buf)
		if n > 0 {
			if _, err := h.Write(buf[:n]); err != nil {
				abort()
				return 0, "", err
			}
			written += int64(n)
			if err := uploadPart(buf[:n]); err != nil {
				abort()
				return 0, "", err
			}
		}
		if readErr == nil {
			continue
		}
		if errors.Is(readErr, io.EOF) || errors.Is(readErr, io.ErrUnexpectedEOF) {
			break
		}
		abort()
		return 0, "", fmt.Errorf("read upload data: %w", readErr)
	}

	hash := hex.EncodeToString(h.Sum(nil))
	if expectedHash != "" && !strings.EqualFold(expectedHash, hash) {
		abort()
		return 0, "", fmt.Errorf("sha256 mismatch: expected %s got %s", expectedHash, hash)
	}

	if uploadID == "" {
		_, err := b.client.PutObject(ctx, &s3.PutObjectInput{
			Bucket:        aws.String(bucket),
			Key:           aws.String(path),
			Body:          bytes.NewReader(nil),
			ContentType:   aws.String(ct),
			ContentLength: aws.Int64(0),
		})
		if err != nil {
			return 0, "", fmt.Errorf("s3 put %s: %w", path, err)
		}
		return written, hash, nil
	}
	_, err := b.client.CompleteMultipartUpload(ctx, &s3.CompleteMultipartUploadInput{
		Bucket:   aws.String(bucket),
		Key:      aws.String(path),
		UploadId: aws.String(uploadID),
		MultipartUpload: &s3types.CompletedMultipartUpload{
			Parts: parts,
		},
	})
	if err != nil {
		abort()
		return 0, "", fmt.Errorf("s3 complete multipart %s: %w", path, err)
	}
	return written, hash, nil
}

// PutBytes uploads pre-buffered data to S3, avoiding double-buffering when the
// caller has already read the entire stream. The caller supplies the pre-computed
// SHA-256 hex hash so PutBytes does not re-hash the data.
func (b *S3Backend) PutBytes(ctx context.Context, path string, data []byte, contentType, precomputedHash string, opts *BackendOpts) (int64, error) {
	if len(data) > s3MaxUploadBytes {
		return 0, fmt.Errorf("upload exceeds %d bytes", s3MaxUploadBytes)
	}

	ct := contentType
	if ct == "" {
		ct = "application/octet-stream"
	}

	_, err := b.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:        aws.String(b.bucket(opts)),
		Key:           aws.String(path),
		Body:          bytes.NewReader(data),
		ContentType:   aws.String(ct),
		ContentLength: aws.Int64(int64(len(data))),
	})
	if err != nil {
		return 0, fmt.Errorf("s3 put %s: %w", path, err)
	}
	return int64(len(data)), nil
}

func (b *S3Backend) Delete(ctx context.Context, path string, opts *BackendOpts) error {
	_, err := b.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(b.bucket(opts)),
		Key:    aws.String(path),
	})
	return err
}

func (b *S3Backend) Exists(ctx context.Context, path string, opts *BackendOpts) (bool, error) {
	_, err := b.client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(b.bucket(opts)),
		Key:    aws.String(path),
	})
	if err != nil {
		var nsk *s3types.NoSuchKey
		if errors.As(err, &nsk) {
			return false, nil
		}
		var nfe *s3types.NotFound
		if errors.As(err, &nfe) {
			return false, nil
		}
		return false, fmt.Errorf("s3 head %s: %w", path, err)
	}
	return true, nil
}

func (b *S3Backend) DeletePrefix(ctx context.Context, prefix string, opts *BackendOpts) error {
	bkt := b.bucket(opts)
	paginator := s3.NewListObjectsV2Paginator(b.client, &s3.ListObjectsV2Input{
		Bucket: aws.String(bkt),
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
		for i := 0; i < len(page.Contents); i += s3DeleteBatch {
			end := min(i+s3DeleteBatch, len(page.Contents))
			objects := make([]s3types.ObjectIdentifier, 0, end-i)
			for _, obj := range page.Contents[i:end] {
				objects = append(objects, s3types.ObjectIdentifier{Key: obj.Key})
			}
			out, err := b.client.DeleteObjects(ctx, &s3.DeleteObjectsInput{
				Bucket: aws.String(bkt),
				Delete: &s3types.Delete{Objects: objects, Quiet: aws.Bool(true)},
			})
			if err != nil {
				errs = append(errs, fmt.Errorf("batch delete: %w", err))
				continue // try remaining batches
			}
			for _, e := range out.Errors {
				errs = append(errs, fmt.Errorf("delete %s: %s", aws.ToString(e.Key), aws.ToString(e.Message)))
			}
		}
	}
	return errors.Join(errs...)
}

func (b *S3Backend) ListFunc(ctx context.Context, prefix string, opts *BackendOpts, fn func(string) error) error {
	bkt := b.bucket(opts)
	paginator := s3.NewListObjectsV2Paginator(b.client, &s3.ListObjectsV2Input{
		Bucket: aws.String(bkt),
		Prefix: aws.String(prefix),
	})
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return fmt.Errorf("list objects: %w", err)
		}
		for _, obj := range page.Contents {
			if err := fn(aws.ToString(obj.Key)); err != nil {
				return err
			}
		}
	}
	return nil
}

// Ping checks S3 connectivity by issuing a HeadBucket request against the default bucket.
// Returns nil if the bucket is reachable, an error otherwise.
func (b *S3Backend) Ping(ctx context.Context) error {
	return b.PingBucket(ctx, b.defaultBucket)
}

// PingBucket checks S3 connectivity for a specific bucket via HeadBucket.
// Returns nil if the bucket is reachable, an error otherwise.
func (b *S3Backend) PingBucket(ctx context.Context, bucket string) error {
	_, err := b.client.HeadBucket(ctx, &s3.HeadBucketInput{
		Bucket: aws.String(bucket),
	})
	if err != nil {
		return fmt.Errorf("s3 ping (HeadBucket %q): %w", bucket, err)
	}
	return nil
}

var _ StorageBackend = (*S3Backend)(nil)
var _ StreamingBackend = (*S3Backend)(nil)
