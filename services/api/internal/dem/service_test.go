package dem

import (
	"math"
	"sync"
	"sync/atomic"
	"testing"
)

// TestService_LazyOpen_ConcurrentSameCell verifies that exactly one mmap-open
// occurs for a given cell even when 50 goroutines race to look up the same
// coordinate simultaneously.
func TestService_LazyOpen_ConcurrentSameCell(t *testing.T) {
	dir := t.TempDir()
	createTestHGT(t, dir, "N47E008.hgt", func(r, c int) int16 { return 500 })

	var openCount atomic.Int64
	svc := newServiceWithOpener([]string{dir}, func(hgtDirs []string, cellName string) (*HGTGrid, error) {
		openCount.Add(1)
		return openGridFor(hgtDirs, cellName)
	})
	defer svc.Close()

	const goroutines = 50
	var wg sync.WaitGroup
	wg.Add(goroutines)
	start := make(chan struct{})
	for range goroutines {
		go func() {
			defer wg.Done()
			<-start
			ele, _ := svc.ElevationAt(47.5, 8.5)
			if ele == NoData {
				t.Errorf("unexpected NoData for valid cell")
			}
		}()
	}
	close(start)
	wg.Wait()

	if got := openCount.Load(); got != 1 {
		t.Errorf("open count = %d, want exactly 1", got)
	}
}

// TestService_NoCoverage_ReturnsNoData verifies that a lookup outside all
// configured directories returns NoData (not an error).
func TestService_NoCoverage_ReturnsNoData(t *testing.T) {
	dir := t.TempDir() // empty — no .hgt files

	svc := NewService([]string{dir})
	defer svc.Close()

	ele, err := svc.ElevationAt(47.5, 8.5)
	if err != nil {
		t.Fatalf("ElevationAt no-coverage: unexpected error: %v", err)
	}
	if ele != NoData {
		t.Errorf("ElevationAt no-coverage = %d, want NoData (%d)", ele, NoData)
	}
}

// TestService_CorruptHGT_LogsErrorAndSetsHealthFlag verifies that an opener
// returning a non-ErrNoCoverage error is recorded in the health's CorruptCells
// slice.
func TestService_CorruptHGT_LogsErrorAndSetsHealthFlag(t *testing.T) {
	corruptErr := &corruptTestErr{}
	svc := newServiceWithOpener(nil, func(_ []string, cellName string) (*HGTGrid, error) {
		return nil, corruptErr
	})
	defer svc.Close()

	ele, err := svc.ElevationAt(47.5, 8.5)
	// The error should propagate from gridFor.
	if err == nil {
		t.Error("expected error for corrupt cell, got nil")
	}
	if ele != NoData {
		t.Errorf("ele = %d, want NoData", ele)
	}

	h := svc.Health()
	if len(h.CorruptCells) == 0 {
		t.Error("expected CorruptCells to be non-empty after corrupt open")
	}
}

type corruptTestErr struct{}

func (e *corruptTestErr) Error() string { return "corrupt HGT" }

// TestService_LazyOpen_ConcurrentDifferentCells verifies that two different
// cells are both opened (one open per cell) when accessed concurrently.
func TestService_LazyOpen_ConcurrentDifferentCells(t *testing.T) {
	dir := t.TempDir()
	createTestHGT(t, dir, "N47E008.hgt", func(r, c int) int16 { return 500 })
	createTestHGT(t, dir, "N47E009.hgt", func(r, c int) int16 { return 800 })

	var openCount atomic.Int64
	svc := newServiceWithOpener([]string{dir}, func(hgtDirs []string, cellName string) (*HGTGrid, error) {
		openCount.Add(1)
		return openGridFor(hgtDirs, cellName)
	})
	defer svc.Close()

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		svc.ElevationAt(47.5, 8.5) //nolint:errcheck
	}()
	go func() {
		defer wg.Done()
		svc.ElevationAt(47.5, 9.5) //nolint:errcheck
	}()
	wg.Wait()

	if got := openCount.Load(); got != 2 {
		t.Errorf("open count = %d, want 2 (one per cell)", got)
	}
}

// TestService_ElevationBatch_MixedCells verifies batch lookups across two
// cells in a single call.
func TestService_ElevationBatch_MixedCells(t *testing.T) {
	dir := t.TempDir()
	// N47E008: all values 500 (decimeters = 5000 in raw; stored as int16)
	createTestHGT(t, dir, "N47E008.hgt", func(r, c int) int16 { return 500 })
	// N47E009: all values 800
	createTestHGT(t, dir, "N47E009.hgt", func(r, c int) int16 { return 800 })

	svc := NewService([]string{dir})
	defer svc.Close()

	// lat=47.5, lon=8.5 → N47E008; lat=47.5, lon=9.5 → N47E009
	lats := []int32{int32(math.Round(47.5 * 1e7)), int32(math.Round(47.5 * 1e7))}
	lons := []int32{int32(math.Round(8.5 * 1e7)), int32(math.Round(9.5 * 1e7))}

	eles, err := svc.ElevationBatch(lats, lons)
	if err != nil {
		t.Fatalf("ElevationBatch: %v", err)
	}
	if len(eles) != 2 {
		t.Fatalf("len(eles) = %d, want 2", len(eles))
	}
	// 500 raw int16 × 10 = 5000 decimeters
	if eles[0] != 5000 {
		t.Errorf("eles[0] = %d, want 5000", eles[0])
	}
	// 800 raw int16 × 10 = 8000 decimeters
	if eles[1] != 8000 {
		t.Errorf("eles[1] = %d, want 8000", eles[1])
	}
}

// TestService_CloseIdempotent verifies that calling Close twice does not panic.
func TestService_CloseIdempotent(t *testing.T) {
	svc := NewService(nil)
	svc.Close()
	svc.Close() // must not panic
}
