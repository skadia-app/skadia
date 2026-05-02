//go:build go1.18

package garmin

import (
	"os"
	"testing"
)

func FuzzDecodeMonitoringB(f *testing.F) {
	// Seed with real monitoring_b fixtures when available.
	for _, path := range []string{
		"testdata/sample_type32_monitoring_b.fit",
		"testdata/sample_type32_monitoring_b_large.fit",
	} {
		if data, err := os.ReadFile(path); err == nil {
			f.Add(data)
		}
	}

	// Minimal fallback seeds to ensure the fuzzer always has a starting corpus.
	f.Add([]byte{})
	f.Add([]byte{0x0E, 0x10}) // truncated FIT header

	f.Fuzz(func(t *testing.T, raw []byte) {
		_, _ = decodeMonitoringB(raw, "fuzz-user")
		// No panic = pass
	})
}
