//go:build largeinput

package fitimport

// Large-input regression suite for ParseMultiSessionFIT / ParseFIT.
//
// These tests do NOT run under `go test ./...` — invoke explicitly:
//
//	cd services/api
//	go test -tags=largeinput -timeout=10m ./internal/fitimport/
//
// The fuzz cap (fuzzMaxInputBytes = 16 MiB) is a campaign-throughput trade-off.
// Production accepts 50–256 MiB activity uploads and up to 4 GiB GDPR-archive
// imports. Panics or runaway allocations in that band would not be found by the
// fuzzer; these tests provide a floor-level regression check.
//
// Oracle: no panic. Any error return from the parser is acceptable.
// Memory delta is logged per case; the test only fails on panic (recovered via
// defer). Strict numeric bounds are omitted — they are too environment-dependent
// to be reliable across machines.

import (
	"bytes"
	"crypto/rand"
	"fmt"
	"runtime"
	"testing"
)

// largeFITTestCase describes one large-input scenario.
type largeFITTestCase struct {
	name  string
	build func(t *testing.T) []byte
}

func largeFITCases(t *testing.T) []largeFITTestCase {
	t.Helper()
	return []largeFITTestCase{
		{
			name: "50MiB_all_zeros",
			build: func(t *testing.T) []byte {
				return make([]byte, 50*1024*1024)
			},
		},
		{
			name: "50MiB_valid_activity_then_50MiB_zeros",
			build: func(t *testing.T) []byte {
				activity, err := synthesizeMinimalActivityFIT()
				if err != nil {
					t.Fatalf("synthesize: %v", err)
				}
				tail := make([]byte, 50*1024*1024)
				return append(activity, tail...)
			},
		},
		{
			name: "256MiB_random_bytes",
			build: func(t *testing.T) []byte {
				buf := make([]byte, 256*1024*1024)
				if _, err := rand.Read(buf); err != nil {
					t.Fatalf("rand.Read: %v", err)
				}
				return buf
			},
		},
		{
			name: "256MiB_valid_header_garbage_body",
			build: func(t *testing.T) []byte {
				// Real 14-byte FIT file header: size=14, protocol=0x10, profile=0x6407,
				// data_size=(256MiB - 16 bytes, little-endian), data_type=".FIT", CRC=0x0000.
				// The parser will read a well-formed header but encounter garbage afterwards.
				const totalSize = 256 * 1024 * 1024
				const headerSize = 14
				buf := make([]byte, totalSize)
				// header_size
				buf[0] = headerSize
				// protocol_version
				buf[1] = 0x10
				// profile_version (little-endian, 2070 = 0x0816)
				buf[2] = 0x16
				buf[3] = 0x08
				// data_size: totalSize - 16 (header 14 + trailing CRC 2)
				dataSize := uint32(totalSize - 16)
				buf[4] = byte(dataSize)
				buf[5] = byte(dataSize >> 8)
				buf[6] = byte(dataSize >> 16)
				buf[7] = byte(dataSize >> 24)
				// data_type ".FIT"
				buf[8] = '.'
				buf[9] = 'F'
				buf[10] = 'I'
				buf[11] = 'T'
				// header CRC (bytes 12-13): leave as 0x0000 — parser uses WithIgnoreChecksum
				// body (bytes 14 onwards): left as zero, which is not a valid FIT record stream
				if _, err := rand.Read(buf[headerSize : totalSize-2]); err != nil {
					t.Fatalf("rand.Read: %v", err)
				}
				return buf
			},
		},
	}
}

func TestLargeInputParseMultiSessionFIT(t *testing.T) {
	for _, tc := range largeFITCases(t) {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			data := tc.build(t)
			runLargeInputCase(t, fmt.Sprintf("%d MiB", len(data)/1024/1024), func() {
				_, _ = ParseMultiSessionFIT(data, "largeinput.fit")
			})
		})
	}
}

func TestLargeInputParseFIT(t *testing.T) {
	for _, tc := range largeFITCases(t) {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			data := tc.build(t)
			runLargeInputCase(t, fmt.Sprintf("%d MiB", len(data)/1024/1024), func() {
				_, _ = ParseFIT(bytes.NewReader(data), "largeinput.fit")
			})
		})
	}
}

// runLargeInputCase calls fn, catches any panic, logs heap allocation delta.
// It only marks the test as failed when a panic is recovered.
func runLargeInputCase(t *testing.T, label string, fn func()) {
	t.Helper()

	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)

	panicked := true
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("panic in parser (%s): %v", label, r)
		} else if panicked {
			// panic from a goroutine outside defer — treat as failure
			t.Errorf("unrecovered panic in parser (%s)", label)
		}
	}()

	fn()
	panicked = false

	runtime.ReadMemStats(&after)
	allocDelta := int64(after.TotalAlloc) - int64(before.TotalAlloc)
	t.Logf("%s: heap alloc delta = %d MiB", label, allocDelta/1024/1024)
}
