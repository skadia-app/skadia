package enrichment

import (
	"context"
	"errors"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// The tests below exercise the TTL-retry cache that replaces the previous
// sync.Once implementation (findings #2, #3, #5). The probe function is
// injected so the tests don't need a live pgxpool.

func TestCheckOsmBoundariesExists_FirstProbeErrorAllowsRetry(t *testing.T) {
	// Regression for #3 / #5:
	// Previously: first caller saw err -> sync.Once ran, flag stayed false,
	// every subsequent call returned false for the rest of the process.
	// Now: error must not pin the cache — the next caller must re-probe.
	resetOsmBoundariesCache()
	t.Cleanup(resetOsmBoundariesCache)

	var probeCalls atomic.Int32

	// First call: probe fails (simulating context cancellation mid-startup).
	got := checkOsmBoundariesExists(context.Background(), func(ctx context.Context) (bool, error) {
		probeCalls.Add(1)
		return false, errors.New("simulated cancel")
	})
	if got {
		t.Fatalf("first call on error: got %v, want false", got)
	}
	if probeCalls.Load() != 1 {
		t.Fatalf("first call: probeCalls = %d, want 1", probeCalls.Load())
	}

	// Second call: probe succeeds (the simulated outage is over).
	got = checkOsmBoundariesExists(context.Background(), func(ctx context.Context) (bool, error) {
		probeCalls.Add(1)
		return true, nil
	})
	if !got {
		t.Fatalf("second call after recovery: got false, want true")
	}
	if probeCalls.Load() != 2 {
		t.Fatalf("second call: probeCalls = %d, want 2 (retry expected after error)", probeCalls.Load())
	}
}

func TestCheckOsmBoundariesExists_PositiveResultCached(t *testing.T) {
	// A successful probe caches the answer; subsequent calls within the TTL
	// must NOT invoke the probe (expensive information_schema query).
	resetOsmBoundariesCache()
	t.Cleanup(resetOsmBoundariesCache)

	var probeCalls atomic.Int32
	probe := func(ctx context.Context) (bool, error) {
		probeCalls.Add(1)
		return true, nil
	}

	for i := 0; i < 5; i++ {
		if !checkOsmBoundariesExists(context.Background(), probe) {
			t.Fatalf("iteration %d: got false, want true", i)
		}
	}
	if probeCalls.Load() != 1 {
		t.Fatalf("probeCalls = %d, want 1 (TTL cache should suppress re-probes)", probeCalls.Load())
	}
}

func TestCheckOsmBoundariesExists_TTLExpiryForcesReprobe(t *testing.T) {
	// Regression for #2:
	// After a fresh OSM import, a previously-false answer must turn true
	// without restarting the process. We simulate TTL expiry by resetting
	// lastChecked to a time older than osmBoundariesTTL.
	resetOsmBoundariesCache()
	t.Cleanup(resetOsmBoundariesCache)

	var probeCalls atomic.Int32
	var exists atomic.Bool // controls what the probe returns

	probe := func(ctx context.Context) (bool, error) {
		probeCalls.Add(1)
		return exists.Load(), nil
	}

	// First probe: table not yet imported.
	if checkOsmBoundariesExists(context.Background(), probe) {
		t.Fatalf("pre-import probe: got true, want false")
	}
	if probeCalls.Load() != 1 {
		t.Fatalf("first probe: calls=%d want 1", probeCalls.Load())
	}

	// OSM import runs in the background; table now exists.
	exists.Store(true)

	// Within TTL, the cached answer (false) is still returned.
	if checkOsmBoundariesExists(context.Background(), probe) {
		t.Fatalf("within TTL: got true, want cached false")
	}
	if probeCalls.Load() != 1 {
		t.Fatalf("within TTL: calls=%d want 1", probeCalls.Load())
	}

	// Force TTL expiry.
	osmBoundariesCheck.mu.Lock()
	osmBoundariesCheck.lastChecked = time.Now().Add(-2 * osmBoundariesTTL)
	osmBoundariesCheck.mu.Unlock()

	// Next call must re-probe and learn that the table now exists.
	if !checkOsmBoundariesExists(context.Background(), probe) {
		t.Fatalf("after TTL expiry: got false, want true")
	}
	if probeCalls.Load() != 2 {
		t.Fatalf("after TTL expiry: calls=%d want 2", probeCalls.Load())
	}
}

func TestCheckOsmBoundariesExists_ConcurrentCallersDoNotStampede(t *testing.T) {
	// Under concurrency, only one probe should fire per TTL window.
	// sync.Once had this property too; the replacement must preserve it.
	resetOsmBoundariesCache()
	t.Cleanup(resetOsmBoundariesCache)

	var probeCalls atomic.Int32
	probe := func(ctx context.Context) (bool, error) {
		probeCalls.Add(1)
		// Simulate a slow information_schema query.
		time.Sleep(5 * time.Millisecond)
		return true, nil
	}

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if !checkOsmBoundariesExists(context.Background(), probe) {
				t.Errorf("concurrent call: got false, want true")
			}
		}()
	}
	wg.Wait()

	if probeCalls.Load() != 1 {
		t.Fatalf("concurrent probeCalls = %d, want 1 (stampede guard failed)", probeCalls.Load())
	}
}

// TestBorderAdminLevels_IncludesGreekRegions verifies the SQL constant in
// enrichGeoTagsBorders includes admin_level 5 (Greek Περιφέρεια), finding #61.
//
// We don't execute the query against a real DB (the package tests run without
// PostGIS). Instead we pin the literal so a regression in geo_enrich.go
// (e.g. someone reverting to `IN (2, 4)`) trips this test.
func TestBorderAdminLevels_IncludesGreekRegions(t *testing.T) {
	// The enrichGeoTagsBorders query must include admin_level 5 for Greek
	// regions. Expected substring in the SQL source:
	expected := "b.admin_level IN (2, 4, 5)"

	src, err := os.ReadFile("geo_enrich.go")
	if err != nil {
		t.Fatalf("read source: %v", err)
	}
	if !strings.Contains(string(src), expected) {
		t.Fatalf("enrichGeoTagsBorders: missing %q — Greek regions (admin_level 5) would be skipped", expected)
	}
}
