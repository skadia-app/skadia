package gdprartifacts

import (
	"bytes"
	"context"
	"io"
	"testing"
)

func TestS3RangeReaderAtReadsCentralDirectoryWithoutFullObjectFetch(t *testing.T) {
	fake := newFakeS3Client()
	body := bytes.Repeat([]byte("z"), 1024)
	key := fake.objectKey("skadia-exports", "gdpr/exports/user/job/export.zip")
	fake.objects[key] = body
	fake.contentType[key] = "application/zip"

	reader := newS3RangeReaderAt(fake, "skadia-exports", "gdpr/exports/user/job/export.zip", int64(len(body)), 64, 2)
	buf := make([]byte, 22)
	if _, err := reader.ReadAt(buf, int64(len(body)-22)); err != nil && err != io.EOF {
		t.Fatalf("ReadAt: %v", err)
	}
	if len(fake.getRanges) != 1 {
		t.Fatalf("range requests = %#v", fake.getRanges)
	}
	if fake.getRanges[0] != "bytes=960-1023" {
		t.Fatalf("range = %q, want final block only", fake.getRanges[0])
	}
	if len(buf) != 22 {
		t.Fatalf("buf length changed")
	}
	if err := reader.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	_ = context.Background()
}
