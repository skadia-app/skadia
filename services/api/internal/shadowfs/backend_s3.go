package shadowfs

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
)

const (
	s3MaxUploadBytes = 500 << 20 // 500 MB
	s3DeleteBatch    = 1000
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

// PutBytes uploads pre-buffered data to S3, avoiding double-buffering when the
// caller has already read the entire stream (e.g. for local-hash computation).
func (b *S3Backend) PutBytes(ctx context.Context, path string, data []byte, contentType string, opts *BackendOpts) (int64, string, error) {
	if len(data) > s3MaxUploadBytes {
		return 0, "", fmt.Errorf("upload exceeds %d bytes", s3MaxUploadBytes)
	}
	h := sha256.Sum256(data)
	hash := hex.EncodeToString(h[:])

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
		return 0, "", fmt.Errorf("s3 put %s: %w", path, err)
	}
	return int64(len(data)), hash, nil
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
		var nfe *s3types.NotFound
		if errors.As(err, &nfe) {
			return false, nil
		}
		// Also handle NoSuchKey (some S3-compatible stores return this instead)
		var nsk *s3types.NoSuchKey
		if errors.As(err, &nsk) {
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

var _ StorageBackend = (*S3Backend)(nil)
