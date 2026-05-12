package gdprartifacts

import (
	"bytes"
	"context"
	"fmt"
	"io"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

type s3RangeReaderAt struct {
	client      s3GetObjectAPI
	bucket      string
	key         string
	size        int64
	blockBytes  int64
	cacheBlocks int
	cache       map[int64][]byte
	order       []int64
}

func newS3RangeReaderAt(client s3GetObjectAPI, bucket, key string, size, blockBytes int64, cacheBlocks int) *s3RangeReaderAt {
	if blockBytes <= 0 {
		blockBytes = 1 << 20
	}
	if cacheBlocks <= 0 {
		cacheBlocks = 1
	}
	return &s3RangeReaderAt{
		client:      client,
		bucket:      bucket,
		key:         key,
		size:        size,
		blockBytes:  blockBytes,
		cacheBlocks: cacheBlocks,
		cache:       make(map[int64][]byte),
	}
}

func (r *s3RangeReaderAt) Size() int64 { return r.size }

func (r *s3RangeReaderAt) Close() error {
	r.cache = nil
	r.order = nil
	return nil
}

func (r *s3RangeReaderAt) ReadAt(p []byte, off int64) (int, error) {
	if off < 0 {
		return 0, fmt.Errorf("negative offset")
	}
	if off >= r.size {
		return 0, io.EOF
	}
	total := 0
	for total < len(p) && off+int64(total) < r.size {
		pos := off + int64(total)
		blockStart := (pos / r.blockBytes) * r.blockBytes
		block, err := r.readBlock(blockStart)
		if err != nil {
			return total, err
		}
		within := pos - blockStart
		n := copy(p[total:], block[within:])
		total += n
	}
	if total < len(p) {
		return total, io.EOF
	}
	return total, nil
}

func (r *s3RangeReaderAt) readBlock(start int64) ([]byte, error) {
	if block, ok := r.cache[start]; ok {
		return block, nil
	}
	end := start + r.blockBytes - 1
	if end >= r.size {
		end = r.size - 1
	}
	rangeHeader := fmt.Sprintf("bytes=%d-%d", start, end)
	out, err := r.client.GetObject(context.Background(), &s3.GetObjectInput{
		Bucket: aws.String(r.bucket),
		Key:    aws.String(r.key),
		Range:  aws.String(rangeHeader),
	})
	if err != nil {
		return nil, err
	}
	defer out.Body.Close()
	body, err := io.ReadAll(out.Body)
	if err != nil {
		return nil, err
	}
	r.cache[start] = body
	r.order = append(r.order, start)
	for len(r.order) > r.cacheBlocks {
		delete(r.cache, r.order[0])
		r.order = r.order[1:]
	}
	return body, nil
}

type memoryObjectReader struct {
	*bytes.Reader
	closeFn     func() error
	size        int64
	contentType string
}

func (r *memoryObjectReader) Close() error {
	if r.closeFn != nil {
		return r.closeFn()
	}
	return nil
}

func (r *memoryObjectReader) Size() int64 { return r.size }

func (r *memoryObjectReader) ContentType() string { return r.contentType }
