package dem

import (
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
)

// Servicer is the interface implemented by Service. It allows handler tests to
// inject a mock without depending on the concrete *Service type.
type Servicer interface {
	// ElevationBatch returns decimeter-scale elevations for int32×1e7 encoded
	// coordinates. NoData (-32768) is returned for coordinates without coverage.
	ElevationBatch(lats, lons []int32) ([]int32, error)
	// Health returns the current service health snapshot.
	Health() Health
	// Close releases all mmap'd resources. Idempotent.
	Close()
}

// cellEntry holds the lazily-opened HGT grid for a single 1°×1° cell.
// sync.Once guarantees exactly one mmap-open even under high concurrency.
type cellEntry struct {
	once sync.Once
	grid *HGTGrid // nil when err != nil
	err  error
}

// openerFn is the function type used to open an HGT grid by cell name.
// The production value is the package-level openGridFor; tests may inject
// a fake via newServiceWithOpener.
type openerFn func(hgtDirs []string, cellName string) (*HGTGrid, error)

// Service provides lazy, concurrent-safe elevation lookups across all
// configured HGT directories. Each 1°×1° cell is opened at most once
// (sync.Once per cell) and kept mmap'd until Close is called.
//
// Multi-user safety: a single Service instance is shared across all
// concurrent HTTP requests. The per-cell sync.Once ensures that even
// 100k+ simultaneous lookups on the same cell cause exactly one mmap-open.
type Service struct {
	hgtDirs []string
	opener  openerFn // production: openGridFor; tests: custom fake

	mu      sync.Mutex
	cells   map[string]*cellEntry // keyed by cellNameFor(lat,lon)
	corrupt []string              // cell names that failed to open

	closed    atomic.Bool
	closeOnce sync.Once
}

// NewService creates a Service that searches hgtDirs in priority order
// (highest resolution first). The directories are not validated at
// construction time — missing dirs simply result in ErrNoCoverage.
func NewService(hgtDirs []string) *Service {
	return &Service{
		hgtDirs: hgtDirs,
		opener:  openGridFor,
		cells:   make(map[string]*cellEntry),
	}
}

// newServiceWithOpener creates a Service with a custom opener function.
// Intended for tests within the dem package only — production code uses NewService.
func newServiceWithOpener(hgtDirs []string, opener openerFn) *Service {
	return &Service{
		hgtDirs: hgtDirs,
		opener:  opener,
		cells:   make(map[string]*cellEntry),
	}
}

// NewServiceWithOpenerForTest is the exported equivalent of newServiceWithOpener
// for tests in other packages (e.g. handler). Not for production use.
func NewServiceWithOpenerForTest(hgtDirs []string, opener func([]string, string) (*HGTGrid, error)) *Service {
	return newServiceWithOpener(hgtDirs, opener)
}

// ElevationAt returns the elevation in decimeters for a WGS84 coordinate.
// Returns NoData when the cell has no coverage or when the grid returned
// NoData (void that could not be interpolated).
func (s *Service) ElevationAt(lat, lon float64) (int32, error) {
	if s.closed.Load() {
		return NoData, fmt.Errorf("dem: service is closed")
	}
	g, err := s.gridFor(lat, lon)
	if err != nil {
		if err == ErrNoCoverage {
			return NoData, nil
		}
		return NoData, err
	}
	return g.ElevationAt(lat, lon), nil
}

// ElevationBatch returns elevations for a slice of int32×1e7 encoded
// coordinates. NoData is returned for any coordinate without coverage.
// The returned slice has the same length as lats/lons.
func (s *Service) ElevationBatch(lats, lons []int32) ([]int32, error) {
	if s.closed.Load() {
		return nil, fmt.Errorf("dem: service is closed")
	}
	out := make([]int32, len(lats))
	for i := range lats {
		lat := float64(lats[i]) / 1e7
		lon := float64(lons[i]) / 1e7
		g, err := s.gridFor(lat, lon)
		if err != nil {
			// No coverage or open error — emit NoData, continue.
			out[i] = NoData
			continue
		}
		out[i] = g.ElevationAt(lat, lon)
	}
	return out, nil
}

// Health returns a snapshot of the service health. corrupt lists the cell
// names that failed to open (e.g. truncated or unreadable HGT files).
type Health struct {
	CorruptCells []string `json:"corrupt_cells,omitempty"`
	Closed       bool     `json:"closed"`
}

// Health returns the current health snapshot. Safe to call concurrently.
func (s *Service) Health() Health {
	s.mu.Lock()
	corrupt := make([]string, len(s.corrupt))
	copy(corrupt, s.corrupt)
	s.mu.Unlock()
	return Health{
		CorruptCells: corrupt,
		Closed:       s.closed.Load(),
	}
}

// Close releases all mmap'd HGT grids. Idempotent — safe to call multiple
// times or concurrently. After Close, ElevationAt/ElevationBatch return errors.
func (s *Service) Close() {
	s.closeOnce.Do(func() {
		// Mark closed first so new callers fast-return before we take the lock.
		s.closed.Store(true)
		s.mu.Lock()
		defer s.mu.Unlock()
		// Only grids whose once.Do has already completed will have a non-nil
		// grid pointer set. Grids whose once.Do has not yet run have no mmap
		// to release (openGridFor has not been called for them yet), so we
		// skip them — they will see s.closed == true and return an error.
		for _, e := range s.cells {
			if e.grid != nil {
				e.grid.Close()
				e.grid = nil
			}
		}
		s.cells = make(map[string]*cellEntry)
	})
}

// gridFor returns the HGTGrid for the cell covering (lat, lon), opening it
// lazily on first access. Concurrent callers for the same cell block on
// sync.Once until the first caller completes — only one mmap-open occurs.
func (s *Service) gridFor(lat, lon float64) (*HGTGrid, error) {
	cellName := cellNameFor(lat, lon)

	s.mu.Lock()
	e, ok := s.cells[cellName]
	if !ok {
		e = &cellEntry{}
		s.cells[cellName] = e
	}
	s.mu.Unlock()

	e.once.Do(func() {
		g, err := s.opener(s.hgtDirs, cellName)
		if err != nil && err != ErrNoCoverage {
			slog.Error("dem: failed to open HGT cell",
				"cell", cellName,
				"error", err,
			)
			s.mu.Lock()
			s.corrupt = append(s.corrupt, cellName)
			s.mu.Unlock()
		}
		e.grid = g
		e.err = err
	})

	if e.err != nil {
		return nil, e.err
	}
	return e.grid, nil
}

