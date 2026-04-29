package fitimport

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// Trust-boundary fuzz tests for the FIT parsing entry points.
//
// Every importer that ingests user-controlled FIT bytes funnels through
// either ParseMultiSessionFIT or ParseFIT in this package:
//   - handler/upload.go        (web uploader)
//   - handler/hotfolder.go     (filesystem drop)
//   - handler/import_url.go    (URL-based imports)
//   - handler/import_zip.go    (GDPR-archive ZIP, FIT bytes per entry)
//   - handler/recompute.go     (re-parse existing activities)
//   - garmin/importer_activities.go
//   - strava/importer_activities.go
//   - komoot/importer_activities.go
//   - cmd/restore/main.go      (admin restore tool)
//
// Fuzzing both functions here covers the activity-FIT branch of all callers.
// Garmin daily-data parsers (parser_monitoring_b, parser_sleep, parser_spo2,
// parser_hrv_*) live in internal/garmin/ and call muktihari/fit directly;
// they need separate fuzz targets, blocked on extracting their pure-decoder
// helper from the surrounding context.Context + database.DB plumbing.
//
// Seed-corpus limitation: the available fixtures in ../garmin/testdata/ are
// Garmin daily-data files (type 32/44/49/58/68 — monitoring, HRV, sleep,
// SpO2). They do not exercise the session/lap/record code paths that
// matter most for activity-FIT bugs. Real activity FIT files exist locally
// in `data/hotfolder/` but are real user data (GPS = PII) and cannot be
// committed to a public repo. The mutator still finds general decoder
// bugs from these seeds, but session/lap-specific coverage is weaker than
// it could be. Synthesising a minimal activity-shaped FIT fixture for the
// `testdata/fuzz/` corpus is a follow-up task tracked separately from this
// session.
//
// Oracle for both fuzz targets: the parser must never panic. Any error
// return is acceptable — malformed FIT input legitimately produces errors.
// Only crashes are bugs.
//
// Run modes (services/api/ has its own go.mod — run go test from there,
// not from the repo root):
//
//   cd services/api
//   go test ./internal/fitimport/                                        # regression mode (seed-only, fast)
//   go test -run='^$' -fuzz=FuzzParseMultiSessionFIT -fuzztime=10m \     # active fuzzing
//       ./internal/fitimport/
//
// Crash inputs land under testdata/fuzz/<FuzzName>/ and become permanent
// regression seeds — commit them after triaging the underlying bug.

// fuzzMaxInputBytes is a deliberate trade-off, not the production ceiling.
// Real FIT files top out around 5 MB; the production upload limit
// (`limits.max_upload_bytes`, default 50 MB, raisable to 256 MB+) accepts
// much larger inputs. We cap fuzz inputs at 16 MB because mutator output
// at 50–256 MB makes each iteration so slow that the campaign produces
// fewer total executions and therefore catches fewer bugs. OOM/quadratic-
// blowup bugs in the 16–256 MB band need a separate large-input regression
// suite (TODO — not in this file).
const fuzzMaxInputBytes = 16 * 1024 * 1024

// fitSeedCorpusDir resolves the seed-corpus directory relative to THIS
// source file, not the test runner's current working directory. Standard
// `go test` runs each package in its own dir (so `../garmin/testdata/`
// works), but IDE test-explorers and some CI harnesses chdir to the
// module root before running, which would silently miss the corpus and
// cause the fuzz target to skip. Anchoring via runtime.Caller fixes both.
func fitSeedCorpusDir(tb testing.TB) string {
	tb.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		tb.Fatal("runtime.Caller(0) failed — cannot resolve seed corpus path")
	}
	pkgDir := filepath.Dir(thisFile)
	return filepath.Join(pkgDir, "..", "garmin", "testdata")
}

// loadFITSeedCorpus returns the bytes of every .fit fixture that exists in
// the garmin testdata directory. Returns nil if the directory is empty.
func loadFITSeedCorpus(tb testing.TB) [][]byte {
	tb.Helper()
	dir := fitSeedCorpusDir(tb)
	seeds, err := filepath.Glob(filepath.Join(dir, "*.fit"))
	if err != nil {
		tb.Fatalf("glob seed corpus at %s: %v", dir, err)
	}
	var out [][]byte
	for _, seed := range seeds {
		data, err := os.ReadFile(seed)
		if err != nil {
			tb.Logf("skip seed %s: %v", seed, err)
			continue
		}
		out = append(out, data)
	}
	return out
}

// FuzzParseMultiSessionFIT covers the multi-session aware activity parser.
// Used by every activity importer (upload, hotfolder, GDPR via import_url
// and import_zip, Garmin, Strava).
func FuzzParseMultiSessionFIT(f *testing.F) {
	corpus := loadFITSeedCorpus(f)
	if len(corpus) == 0 {
		dir := fitSeedCorpusDir(f)
		f.Skipf("no seed FIT files at %s — fuzz target needs at least one valid input", dir)
	}
	for _, data := range corpus {
		f.Add(data)
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > fuzzMaxInputBytes {
			t.Skip("oversized fuzz input")
		}
		// Filename hardcoded so only the byte stream is mutated; the parser
		// uses filename for error-message context, not for parsing.
		_, _ = ParseMultiSessionFIT(data, "fuzz.fit")
	})
}

// FuzzParseFIT covers the single-session legacy parser used as fallback by
// every importer when the multi-session decoder rejects the input, plus by
// recompute and the admin restore tool. Different code path (different
// session-handling logic) so it gets its own fuzz target.
func FuzzParseFIT(f *testing.F) {
	corpus := loadFITSeedCorpus(f)
	if len(corpus) == 0 {
		dir := fitSeedCorpusDir(f)
		f.Skipf("no seed FIT files at %s — fuzz target needs at least one valid input", dir)
	}
	for _, data := range corpus {
		f.Add(data)
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > fuzzMaxInputBytes {
			t.Skip("oversized fuzz input")
		}
		_, _ = ParseFIT(bytes.NewReader(data), "fuzz.fit")
	})
}
