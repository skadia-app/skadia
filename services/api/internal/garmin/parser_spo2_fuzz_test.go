//go:build go1.18

package garmin

import (
	"os"
	"testing"
)

func FuzzDecodeSpO2(f *testing.F) {
	// Seed with real SpO2 fixture when available.
	for _, path := range []string{
		"testdata/sample_type58_spo2.fit",
	} {
		if data, err := os.ReadFile(path); err == nil {
			f.Add(data)
		}
	}

	// Minimal fallback seeds to ensure the fuzzer always has a starting corpus.
	f.Add([]byte{})
	f.Add([]byte{0x0E, 0x10}) // truncated FIT header

	f.Fuzz(func(t *testing.T, raw []byte) {
		_, _ = decodeSpO2(raw, "fuzz-user")
		// No panic = pass
	})
}
