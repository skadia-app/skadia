package handler

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"strings"
)

// decompressGzip decompresses gzip data with bomb protection.
// maxDecompressed limits output size to prevent decompression bombs.
func decompressGzip(data []byte, maxDecompressed int64) ([]byte, error) {
	r, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("gzip: open reader: %w", err)
	}
	defer r.Close()
	lr := &io.LimitedReader{R: r, N: maxDecompressed + 1}
	out, err := io.ReadAll(lr)
	if err != nil {
		return nil, fmt.Errorf("gzip: decompress: %w", err)
	}
	if lr.N == 0 {
		return nil, fmt.Errorf("gzip: decompressed file exceeds %d bytes limit", maxDecompressed)
	}
	return out, nil
}

// detectFormatAndDecompress detects format from filename/content,
// decompresses .gz if needed, returns (decompressedBytes, format, error).
// format is "fit", "gpx", or "tcx".
func detectFormatAndDecompress(fileBytes []byte, filename string, maxDecompressed int64) ([]byte, string, error) {
	lower := strings.ToLower(filename)

	if strings.HasSuffix(lower, ".gz") {
		// Calculate bomb protection limit: min(10 * compressed, 500 MB)
		limit := int64(len(fileBytes)) * 10
		const maxAbsolute = 500 << 20 // 500 MB
		if limit > maxAbsolute {
			limit = maxAbsolute
		}
		if maxDecompressed > 0 && limit > maxDecompressed {
			limit = maxDecompressed
		}
		decompressed, err := decompressGzip(fileBytes, limit)
		if err != nil {
			return nil, "", fmt.Errorf("decompress .gz: %w", err)
		}
		// Strip .gz and detect inner format from decompressed bytes + inner filename
		innerName := strings.TrimSuffix(lower, ".gz")
		format := detectFormat(decompressed, innerName)
		if format == "" {
			return nil, "", fmt.Errorf("unsupported format after decompression: %s", filename)
		}
		return decompressed, format, nil
	}

	format := detectFormat(fileBytes, filename)
	if format == "" {
		return nil, "", fmt.Errorf("unsupported format: %s", filename)
	}
	return fileBytes, format, nil
}

// isValidActivityFilename returns true if the filename has a valid activity extension.
func isValidActivityFilename(name string) bool {
	lower := strings.ToLower(name)
	for _, ext := range []string{".fit", ".gpx", ".tcx"} {
		if strings.HasSuffix(lower, ext) || strings.HasSuffix(lower, ext+".gz") {
			return true
		}
	}
	return false
}
