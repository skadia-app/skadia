//go:build go1.18

package garmin

import (
	"os"
	"testing"
)

func FuzzDecodeSleep(f *testing.F) {
	// Seed with synthetic sleep fixture when available.
	for _, path := range []string{
		"testdata/sample_type49_sleep.fit",
	} {
		if data, err := os.ReadFile(path); err == nil {
			f.Add(data)
		}
	}

	// Minimal fallback seeds to ensure the fuzzer always has a starting corpus.
	f.Add([]byte{})
	f.Add([]byte{0x0E, 0x10}) // truncated FIT header

	f.Fuzz(func(t *testing.T, raw []byte) {
		_, _ = decodeSleep(raw, "fuzz-user")
		// No panic = pass
	})
}
