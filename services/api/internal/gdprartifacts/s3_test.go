package gdprartifacts

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
)

type fakeS3Client struct {
	objects     map[string][]byte
	contentType map[string]string
	multipart   map[string]*fakeMultipartUpload
	nextUpload  int

	failUploadPart bool
	failComplete   bool
	failHead       error
	failPutBuckets map[string]bool
	failDeleteKeys map[string]bool

	putObjectCalls        int
	createMultipartCalls  int
	uploadPartCalls       int
	completeCalls         int
	abortCalls            int
	abortIncompleteCalls  int
	getRanges             []string
	listMultipartPrefixes []string
	deletePrefixes        []string
}

type fakeMultipartUpload struct {
	bucket string
	key    string
	parts  map[int32][]byte
}

func newFakeS3Client() *fakeS3Client {
	return &fakeS3Client{
		objects:     make(map[string][]byte),
		contentType: make(map[string]string),
		multipart:   make(map[string]*fakeMultipartUpload),
	}
}

func (f *fakeS3Client) objectKey(bucket, key string) string { return bucket + "/" + key }

func (f *fakeS3Client) PutObject(ctx context.Context, in *s3.PutObjectInput, _ ...func(*s3.Options)) (*s3.PutObjectOutput, error) {
	f.putObjectCalls++
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if f.failPutBuckets[aws.ToString(in.Bucket)] {
		return nil, fmt.Errorf("bucket not found")
	}
	body, err := io.ReadAll(in.Body)
	if err != nil {
		return nil, err
	}
	key := f.objectKey(aws.ToString(in.Bucket), aws.ToString(in.Key))
	f.objects[key] = body
	f.contentType[key] = aws.ToString(in.ContentType)
	return &s3.PutObjectOutput{}, nil
}

func (f *fakeS3Client) CreateMultipartUpload(ctx context.Context, in *s3.CreateMultipartUploadInput, _ ...func(*s3.Options)) (*s3.CreateMultipartUploadOutput, error) {
	f.createMultipartCalls++
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	f.nextUpload++
	uploadID := "upload-" + string(rune('0'+f.nextUpload))
	f.multipart[uploadID] = &fakeMultipartUpload{
		bucket: aws.ToString(in.Bucket),
		key:    aws.ToString(in.Key),
		parts:  make(map[int32][]byte),
	}
	return &s3.CreateMultipartUploadOutput{UploadId: aws.String(uploadID)}, nil
}

func (f *fakeS3Client) UploadPart(ctx context.Context, in *s3.UploadPartInput, _ ...func(*s3.Options)) (*s3.UploadPartOutput, error) {
	f.uploadPartCalls++
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if f.failUploadPart {
		return nil, errors.New("upload part failed")
	}
	body, err := io.ReadAll(in.Body)
	if err != nil {
		return nil, err
	}
	f.multipart[aws.ToString(in.UploadId)].parts[aws.ToInt32(in.PartNumber)] = body
	return &s3.UploadPartOutput{ETag: aws.String("etag")}, nil
}

func (f *fakeS3Client) CompleteMultipartUpload(_ context.Context, in *s3.CompleteMultipartUploadInput, _ ...func(*s3.Options)) (*s3.CompleteMultipartUploadOutput, error) {
	f.completeCalls++
	if f.failComplete {
		return nil, errors.New("complete failed")
	}
	upload := f.multipart[aws.ToString(in.UploadId)]
	var out []byte
	for _, part := range in.MultipartUpload.Parts {
		out = append(out, upload.parts[*part.PartNumber]...)
	}
	f.objects[f.objectKey(upload.bucket, upload.key)] = out
	delete(f.multipart, aws.ToString(in.UploadId))
	return &s3.CompleteMultipartUploadOutput{}, nil
}

func (f *fakeS3Client) AbortMultipartUpload(_ context.Context, in *s3.AbortMultipartUploadInput, _ ...func(*s3.Options)) (*s3.AbortMultipartUploadOutput, error) {
	f.abortCalls++
	delete(f.multipart, aws.ToString(in.UploadId))
	return &s3.AbortMultipartUploadOutput{}, nil
}

func (f *fakeS3Client) HeadObject(_ context.Context, in *s3.HeadObjectInput, _ ...func(*s3.Options)) (*s3.HeadObjectOutput, error) {
	if f.failHead != nil {
		return nil, f.failHead
	}
	key := f.objectKey(aws.ToString(in.Bucket), aws.ToString(in.Key))
	body, ok := f.objects[key]
	if !ok {
		return nil, os.ErrNotExist
	}
	return &s3.HeadObjectOutput{
		ContentLength: aws.Int64(int64(len(body))),
		ContentType:   aws.String(f.contentType[key]),
	}, nil
}

func (f *fakeS3Client) GetObject(_ context.Context, in *s3.GetObjectInput, _ ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
	key := f.objectKey(aws.ToString(in.Bucket), aws.ToString(in.Key))
	body, ok := f.objects[key]
	if !ok {
		return nil, os.ErrNotExist
	}
	if in.Range != nil {
		f.getRanges = append(f.getRanges, aws.ToString(in.Range))
		var start, end int64
		if _, err := fmt.Sscanf(aws.ToString(in.Range), "bytes=%d-%d", &start, &end); err != nil {
			return nil, err
		}
		if start > int64(len(body)) {
			return &s3.GetObjectOutput{Body: io.NopCloser(bytes.NewReader(nil)), ContentLength: aws.Int64(0)}, nil
		}
		if end >= int64(len(body)) {
			end = int64(len(body)) - 1
		}
		body = body[start : end+1]
	}
	return &s3.GetObjectOutput{
		Body:          io.NopCloser(bytes.NewReader(body)),
		ContentLength: aws.Int64(int64(len(body))),
		ContentType:   aws.String(f.contentType[key]),
	}, nil
}

func (f *fakeS3Client) DeleteObject(_ context.Context, in *s3.DeleteObjectInput, _ ...func(*s3.Options)) (*s3.DeleteObjectOutput, error) {
	delete(f.objects, f.objectKey(aws.ToString(in.Bucket), aws.ToString(in.Key)))
	return &s3.DeleteObjectOutput{}, nil
}

func (f *fakeS3Client) ListObjectsV2(_ context.Context, in *s3.ListObjectsV2Input, _ ...func(*s3.Options)) (*s3.ListObjectsV2Output, error) {
	prefix := aws.ToString(in.Prefix)
	f.deletePrefixes = append(f.deletePrefixes, prefix)
	var contents []s3types.Object
	for key := range f.objects {
		trimmed := strings.TrimPrefix(key, aws.ToString(in.Bucket)+"/")
		if strings.HasPrefix(trimmed, prefix) {
			k := trimmed
			contents = append(contents, s3types.Object{Key: &k})
		}
	}
	return &s3.ListObjectsV2Output{Contents: contents}, nil
}

func (f *fakeS3Client) DeleteObjects(_ context.Context, in *s3.DeleteObjectsInput, _ ...func(*s3.Options)) (*s3.DeleteObjectsOutput, error) {
	var failures []s3types.Error
	for _, obj := range in.Delete.Objects {
		key := aws.ToString(obj.Key)
		if f.failDeleteKeys[key] {
			failures = append(failures, s3types.Error{Key: aws.String(key), Code: aws.String("AccessDenied"), Message: aws.String("denied")})
			continue
		}
		delete(f.objects, f.objectKey(aws.ToString(in.Bucket), key))
	}
	return &s3.DeleteObjectsOutput{Errors: failures}, nil
}

func (f *fakeS3Client) ListMultipartUploads(_ context.Context, in *s3.ListMultipartUploadsInput, _ ...func(*s3.Options)) (*s3.ListMultipartUploadsOutput, error) {
	prefix := aws.ToString(in.Prefix)
	f.listMultipartPrefixes = append(f.listMultipartPrefixes, prefix)
	var uploads []s3types.MultipartUpload
	for uploadID, upload := range f.multipart {
		if upload.bucket == aws.ToString(in.Bucket) && strings.HasPrefix(upload.key, prefix) {
			id := uploadID
			key := upload.key
			uploads = append(uploads, s3types.MultipartUpload{Key: &key, UploadId: &id})
		}
	}
	return &s3.ListMultipartUploadsOutput{Uploads: uploads}, nil
}

func TestS3StoreMultipartWriterUsesBoundedPartsAndCommits(t *testing.T) {
	fake := newFakeS3Client()
	store := newS3StoreForTest(fake, S3Config{
		ExportBucket:  "skadia-exports",
		ImportBucket:  "skadia-imports",
		ExportPrefix:  "gdpr/exports",
		ImportPrefix:  "gdpr/imports",
		PartSizeBytes: 5 << 20,
	})
	writer, err := store.CreateWriter(context.Background(), ObjectSpec{
		Kind: "export", UserID: "11111111-1111-1111-1111-111111111111", JobID: "22222222-2222-2222-2222-222222222222",
		Role: RoleSingleZIP, Name: "export.zip", ContentType: "application/zip", ExpiresAt: time.Now().Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("CreateWriter: %v", err)
	}
	payload := bytes.Repeat([]byte("x"), 17<<20)
	if _, err := writer.Write(payload); err != nil {
		t.Fatalf("Write: %v", err)
	}
	ref, err := writer.Commit(context.Background())
	if err != nil {
		t.Fatalf("Commit: %v", err)
	}
	if fake.putObjectCalls != 0 {
		t.Fatalf("PutObject calls = %d, want 0 for multipart input", fake.putObjectCalls)
	}
	if fake.createMultipartCalls != 1 || fake.uploadPartCalls != 4 || fake.completeCalls != 1 {
		t.Fatalf("multipart calls create/upload/complete = %d/%d/%d", fake.createMultipartCalls, fake.uploadPartCalls, fake.completeCalls)
	}
	if got := fake.objects[fake.objectKey(ref.Bucket, ref.Key)]; !bytes.Equal(got, payload) {
		t.Fatalf("stored payload mismatch: got %d bytes", len(got))
	}
}

func TestS3StoreClampsMultipartPartSizeToS3Minimum(t *testing.T) {
	fake := newFakeS3Client()
	store := newS3StoreForTest(fake, S3Config{
		ExportBucket:  "skadia-exports",
		ExportPrefix:  "gdpr/exports",
		PartSizeBytes: 1024,
	})
	if store.cfg.PartSizeBytes != minS3MultipartPartSize {
		t.Fatalf("PartSizeBytes = %d, want %d", store.cfg.PartSizeBytes, minS3MultipartPartSize)
	}
}

func TestS3StoreEnsureLifecycleProbesExportAndImportBuckets(t *testing.T) {
	fake := newFakeS3Client()
	store := newS3StoreForTest(fake, S3Config{
		ExportBucket: "skadia-exports",
		ImportBucket: "skadia-imports",
		ExportPrefix: "gdpr/exports",
		ImportPrefix: "gdpr/imports",
	})
	if err := store.EnsureLifecycle(context.Background()); err != nil {
		t.Fatalf("EnsureLifecycle: %v", err)
	}
	if fake.putObjectCalls != 2 {
		t.Fatalf("PutObject calls = %d, want export+import probes", fake.putObjectCalls)
	}
	for key := range fake.objects {
		if strings.Contains(key, ".skadia-artifact-probe-") {
			t.Fatalf("probe object was not deleted: %s", key)
		}
	}
}

func TestS3StoreEnsureLifecycleFailsWhenArtifactBucketMissing(t *testing.T) {
	fake := newFakeS3Client()
	fake.failPutBuckets = map[string]bool{"skadia-exports": true}
	store := newS3StoreForTest(fake, S3Config{
		ExportBucket: "skadia-exports",
		ImportBucket: "skadia-imports",
		ExportPrefix: "gdpr/exports",
		ImportPrefix: "gdpr/imports",
	})
	err := store.EnsureLifecycle(context.Background())
	if err == nil {
		t.Fatal("EnsureLifecycle succeeded with missing export bucket")
	}
	if !strings.Contains(err.Error(), `export artifact bucket "skadia-exports" is not writable`) {
		t.Fatalf("EnsureLifecycle error = %v", err)
	}
}

func TestS3StoreMultipartWriteUsesCreateWriterContext(t *testing.T) {
	fake := newFakeS3Client()
	store := newS3StoreForTest(fake, S3Config{
		ExportBucket:  "skadia-exports",
		ExportPrefix:  "gdpr/exports",
		PartSizeBytes: minS3MultipartPartSize,
	})
	ctx, cancel := context.WithCancel(context.Background())
	writer, err := store.CreateWriter(ctx, ObjectSpec{
		Kind:        "export",
		UserID:      "11111111-1111-1111-1111-111111111111",
		JobID:       "22222222-2222-2222-2222-222222222222",
		Role:        RoleSingleZIP,
		Name:        "export.zip",
		ContentType: "application/zip",
	})
	if err != nil {
		t.Fatalf("CreateWriter: %v", err)
	}
	cancel()
	if _, err := writer.Write(bytes.Repeat([]byte("x"), minS3MultipartPartSize)); !errors.Is(err, context.Canceled) {
		t.Fatalf("Write error = %v, want context.Canceled", err)
	}
	if fake.createMultipartCalls != 1 {
		t.Fatalf("create multipart calls = %d, want 1", fake.createMultipartCalls)
	}
}

func TestS3StoreRejectsRefsOutsideConfiguredScope(t *testing.T) {
	fake := newFakeS3Client()
	fake.objects[fake.objectKey("skadia-exports", "gdpr/exports/attacker/job/export.zip")] = []byte("private")
	store := newS3StoreForTest(fake, S3Config{ExportBucket: "skadia-exports", ExportPrefix: "gdpr/exports"})
	ref := ObjectRef{
		Backend: "s3",
		Kind:    "export",
		UserID:  "11111111-1111-1111-1111-111111111111",
		JobID:   "22222222-2222-2222-2222-222222222222",
		Role:    RoleSingleZIP,
		Name:    "export.zip",
		Bucket:  "skadia-exports",
		Key:     "gdpr/exports/attacker/job/export.zip",
	}
	if _, err := store.Open(context.Background(), ref, nil); err == nil {
		t.Fatal("Open accepted S3 ref outside configured user/job scope")
	}
	if stat, err := store.Stat(context.Background(), ref); err == nil || stat.State != ObjectUnknown {
		t.Fatalf("Stat outside scope = stat=%#v err=%v, want hard validation error", stat, err)
	}
	if err := store.Delete(context.Background(), ref); err == nil {
		t.Fatal("Delete accepted S3 ref outside configured user/job scope")
	}
	if _, ok := fake.objects[fake.objectKey("skadia-exports", "gdpr/exports/attacker/job/export.zip")]; !ok {
		t.Fatal("Delete removed object outside configured user/job scope")
	}
}

func TestS3StoreDeletePrefixFailsOnPartialDeleteErrors(t *testing.T) {
	fake := newFakeS3Client()
	key := "gdpr/exports/11111111-1111-1111-1111-111111111111/22222222-2222-2222-2222-222222222222/export.zip"
	fake.objects[fake.objectKey("skadia-exports", key)] = []byte("payload")
	fake.failDeleteKeys = map[string]bool{key: true}
	store := newS3StoreForTest(fake, S3Config{ExportBucket: "skadia-exports", ExportPrefix: "gdpr/exports"})
	err := store.DeletePrefix(context.Background(), ArtifactPrefix{Kind: "export", UserID: "11111111-1111-1111-1111-111111111111", JobID: "22222222-2222-2222-2222-222222222222"})
	if err == nil {
		t.Fatal("DeletePrefix succeeded despite per-object S3 delete error")
	}
	if _, ok := fake.objects[fake.objectKey("skadia-exports", key)]; !ok {
		t.Fatal("DeletePrefix removed object despite fake partial delete failure")
	}
}

func TestS3StoreAbortOnWriteOrCommitFailure(t *testing.T) {
	fake := newFakeS3Client()
	fake.failUploadPart = true
	store := newS3StoreForTest(fake, S3Config{ExportBucket: "skadia-exports", ExportPrefix: "gdpr/exports", PartSizeBytes: 5 << 20})
	writer, err := store.CreateWriter(context.Background(), ObjectSpec{
		Kind: "export", UserID: "11111111-1111-1111-1111-111111111111", JobID: "22222222-2222-2222-2222-222222222222",
		Role: RoleSingleZIP, Name: "export.zip", ContentType: "application/zip",
	})
	if err != nil {
		t.Fatalf("CreateWriter: %v", err)
	}
	if _, err := writer.Write(bytes.Repeat([]byte("x"), 6<<20)); err == nil {
		t.Fatal("Write succeeded despite UploadPart failure")
	}
	if fake.abortCalls != 1 {
		t.Fatalf("abort calls = %d, want 1", fake.abortCalls)
	}

	fake = newFakeS3Client()
	fake.failComplete = true
	store = newS3StoreForTest(fake, S3Config{ExportBucket: "skadia-exports", ExportPrefix: "gdpr/exports", PartSizeBytes: 5 << 20})
	writer, _ = store.CreateWriter(context.Background(), ObjectSpec{Kind: "export", UserID: "11111111-1111-1111-1111-111111111111", JobID: "22222222-2222-2222-2222-222222222222", Role: RoleSingleZIP, Name: "export.zip", ContentType: "application/zip"})
	if _, err := writer.Write(bytes.Repeat([]byte("x"), 6<<20)); err != nil {
		t.Fatalf("Write before complete failure: %v", err)
	}
	if _, err := writer.Commit(context.Background()); err == nil {
		t.Fatal("Commit succeeded despite CompleteMultipartUpload failure")
	}
	if fake.abortCalls != 1 {
		t.Fatalf("abort calls after complete failure = %d, want 1", fake.abortCalls)
	}
}

func TestS3StoreStatDistinguishesMissingAndUnknown(t *testing.T) {
	fake := newFakeS3Client()
	store := newS3StoreForTest(fake, S3Config{ExportBucket: "skadia-exports", ExportPrefix: "gdpr/exports"})
	ref := ObjectRef{
		Backend: "s3",
		Kind:    "export",
		UserID:  "11111111-1111-1111-1111-111111111111",
		JobID:   "22222222-2222-2222-2222-222222222222",
		Role:    RoleSingleZIP,
		Name:    "export.zip",
		Bucket:  "skadia-exports",
		Key:     "gdpr/exports/11111111-1111-1111-1111-111111111111/22222222-2222-2222-2222-222222222222/export.zip",
	}
	stat, err := store.Stat(context.Background(), ref)
	if err != nil {
		t.Fatalf("Stat missing: %v", err)
	}
	if stat.State != ObjectMissing {
		t.Fatalf("missing state = %q", stat.State)
	}
	fake.failHead = errors.New("network down")
	stat, err = store.Stat(context.Background(), ref)
	if err != nil {
		t.Fatalf("Stat unknown should not return hard error: %v", err)
	}
	if stat.State != ObjectUnknown || stat.Err == nil {
		t.Fatalf("unknown state = %#v", stat)
	}
}

func TestS3StoreOpenUsesRangeHeader(t *testing.T) {
	fake := newFakeS3Client()
	key := "gdpr/exports/11111111-1111-1111-1111-111111111111/22222222-2222-2222-2222-222222222222/export.zip"
	fake.objects[fake.objectKey("skadia-exports", key)] = []byte("0123456789")
	fake.contentType[fake.objectKey("skadia-exports", key)] = "application/zip"
	store := newS3StoreForTest(fake, S3Config{ExportBucket: "skadia-exports", ExportPrefix: "gdpr/exports"})
	start, end := int64(2), int64(5)
	reader, err := store.Open(context.Background(), ObjectRef{
		Backend: "s3",
		Kind:    "export",
		UserID:  "11111111-1111-1111-1111-111111111111",
		JobID:   "22222222-2222-2222-2222-222222222222",
		Role:    RoleSingleZIP,
		Name:    "export.zip",
		Bucket:  "skadia-exports",
		Key:     key,
	}, &RangeSpec{Start: &start, End: &end})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	body, _ := io.ReadAll(reader)
	_ = reader.Close()
	if string(body) != "2345" {
		t.Fatalf("range body = %q", body)
	}
	if len(fake.getRanges) != 1 || fake.getRanges[0] != "bytes=2-5" {
		t.Fatalf("ranges = %#v", fake.getRanges)
	}
}

func TestS3StoreDeletePrefixAndAbortIncompleteUploadsUseConfiguredPrefix(t *testing.T) {
	fake := newFakeS3Client()
	fake.multipart["upload-1"] = &fakeMultipartUpload{bucket: "skadia-exports", key: "gdpr/exports/11111111-1111-1111-1111-111111111111/22222222-2222-2222-2222-222222222222/export.zip", parts: make(map[int32][]byte)}
	store := newS3StoreForTest(fake, S3Config{ExportBucket: "skadia-exports", ImportBucket: "skadia-imports", ExportPrefix: "gdpr/exports", ImportPrefix: "gdpr/imports"})

	prefix := ArtifactPrefix{Kind: "export", UserID: "11111111-1111-1111-1111-111111111111", JobID: "22222222-2222-2222-2222-222222222222"}
	if err := store.DeletePrefix(context.Background(), prefix); err != nil {
		t.Fatalf("DeletePrefix: %v", err)
	}
	if err := store.AbortIncompleteUploads(context.Background(), prefix); err != nil {
		t.Fatalf("AbortIncompleteUploads: %v", err)
	}
	want := "gdpr/exports/11111111-1111-1111-1111-111111111111/22222222-2222-2222-2222-222222222222/"
	if fake.deletePrefixes[0] != want || fake.listMultipartPrefixes[0] != want {
		t.Fatalf("prefixes delete/list = %#v/%#v, want %q", fake.deletePrefixes, fake.listMultipartPrefixes, want)
	}
	if fake.abortCalls != 1 {
		t.Fatalf("abort calls = %d, want 1", fake.abortCalls)
	}
}

func TestS3StoreCountIncompleteUploadsUsesConfiguredPrefixWithoutAbort(t *testing.T) {
	fake := newFakeS3Client()
	fake.multipart["upload-1"] = &fakeMultipartUpload{bucket: "skadia-exports", key: "gdpr/exports/11111111-1111-1111-1111-111111111111/22222222-2222-2222-2222-222222222222/export.zip", parts: make(map[int32][]byte)}
	fake.multipart["upload-2"] = &fakeMultipartUpload{bucket: "skadia-exports", key: "gdpr/exports/other-user/other-job/export.zip", parts: make(map[int32][]byte)}
	store := newS3StoreForTest(fake, S3Config{ExportBucket: "skadia-exports", ImportBucket: "skadia-imports", ExportPrefix: "gdpr/exports", ImportPrefix: "gdpr/imports"})

	prefix := ArtifactPrefix{Kind: "export", UserID: "11111111-1111-1111-1111-111111111111", JobID: "22222222-2222-2222-2222-222222222222"}
	got, err := store.CountIncompleteUploads(context.Background(), prefix)
	if err != nil {
		t.Fatalf("CountIncompleteUploads: %v", err)
	}
	if got != 1 {
		t.Fatalf("incomplete uploads = %d, want 1", got)
	}
	want := "gdpr/exports/11111111-1111-1111-1111-111111111111/22222222-2222-2222-2222-222222222222/"
	if fake.listMultipartPrefixes[len(fake.listMultipartPrefixes)-1] != want {
		t.Fatalf("list prefix = %#v, want %q", fake.listMultipartPrefixes, want)
	}
	if fake.abortCalls != 0 {
		t.Fatalf("abort calls = %d, want 0", fake.abortCalls)
	}
}

func TestS3StoreCountIncompleteUploadsAllowsDedicatedBucketWithoutPrefix(t *testing.T) {
	fake := newFakeS3Client()
	fake.multipart["upload-1"] = &fakeMultipartUpload{bucket: "skadia-exports", key: "11111111-1111-1111-1111-111111111111/22222222-2222-2222-2222-222222222222/export.zip", parts: make(map[int32][]byte)}
	store := newS3StoreForTest(fake, S3Config{ExportBucket: "skadia-exports", ImportBucket: "skadia-imports"})

	got, err := store.CountIncompleteUploads(context.Background(), ArtifactPrefix{Kind: "export"})
	if err != nil {
		t.Fatalf("CountIncompleteUploads: %v", err)
	}
	if got != 1 {
		t.Fatalf("incomplete uploads = %d, want 1", got)
	}
	if fake.listMultipartPrefixes[len(fake.listMultipartPrefixes)-1] != "" {
		t.Fatalf("list prefix = %#v, want empty dedicated-bucket prefix", fake.listMultipartPrefixes)
	}
	if fake.abortCalls != 0 {
		t.Fatalf("abort calls = %d, want 0", fake.abortCalls)
	}
}
