//go:build go1.18

package garmin

import (
	"os"
	"testing"
)

func FuzzDecodeSegment(f *testing.F) {
	// Seed with segment fixtures when available.
	for _, path := range []string{
		"testdata/sample_type34_segment.fit",
		"testdata/sample_type34_segment_large.fit",
	} {
		if data, err := os.ReadFile(path); err == nil {
			f.Add(data)
		}
	}

	// Minimal fallback seeds to ensure the fuzzer always has a starting corpus.
	f.Add([]byte{})
	f.Add([]byte{0x0E, 0x10}) // truncated FIT header

	f.Fuzz(func(t *testing.T, raw []byte) {
		_, _ = decodeSegment(raw)
		// No panic = pass
	})
}
