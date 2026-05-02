//go:build go1.18

package garmin

import (
	"os"
	"testing"
)

func FuzzDecodeWorkout(f *testing.F) {
	// Seed with workout fixtures when available.
	for _, path := range []string{
		"testdata/sample_workout.fit",
		"testdata/sample_workout_steps.fit",
	} {
		if data, err := os.ReadFile(path); err == nil {
			f.Add(data)
		}
	}

	// Minimal fallback seeds to ensure the fuzzer always has a starting corpus.
	f.Add([]byte{})
	f.Add([]byte{0x0E, 0x10}) // truncated FIT header

	f.Fuzz(func(t *testing.T, raw []byte) {
		_, _ = decodeWorkout(raw)
		// No panic = pass
	})
}
