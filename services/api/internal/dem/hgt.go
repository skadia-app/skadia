package dem

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"syscall"
	"unsafe"
)

// Known HGT file sizes and their dimensions.
var hgtSizes = map[int64][2]int{
	2884802:   {1201, 1201},   // 3" (~90m)
	25934402:  {3601, 3601},   // 1" (~30m)
	103694402: {7201, 7201},   // 0.5" (~15m)
}

// HGTGrid is a single mmap'd .hgt file.
type HGTGrid struct {
	data     []byte
	file     *os.File
	rows     int
	cols     int
	cellSize float64 // degrees per pixel
	north    float64 // north edge latitude
	west     float64 // west edge longitude
}

// OpenHGTGrid opens and mmap's an HGT file.
func OpenHGTGrid(path string) (*HGTGrid, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("dem: open %s: %w", path, err)
	}

	info, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, fmt.Errorf("dem: stat %s: %w", path, err)
	}

	dims, ok := hgtSizes[info.Size()]
	if !ok {
		f.Close()
		return nil, fmt.Errorf("dem: invalid HGT file size %d for %s (expected 1201², 3601², or 7201²×2)", info.Size(), path)
	}

	data, err := syscall.Mmap(int(f.Fd()), 0, int(info.Size()),
		syscall.PROT_READ, syscall.MAP_SHARED)
	if err != nil {
		f.Close()
		return nil, fmt.Errorf("dem: mmap %s: %w", path, err)
	}

	// Trigger async readahead for cold-cache performance.
	_ = madvise(data, syscall.MADV_WILLNEED)

	south, west, err := parseHGTName(path)
	if err != nil {
		syscall.Munmap(data)
		f.Close()
		return nil, err
	}

	return &HGTGrid{
		data:     data,
		file:     f,
		rows:     dims[0],
		cols:     dims[1],
		cellSize: 1.0 / float64(dims[0]-1),
		north:    south + 1.0,
		west:     west,
	}, nil
}

// ElevationAt returns elevation in decimeters for a WGS84 coordinate.
func (g *HGTGrid) ElevationAt(lat, lon float64) int32 {
	row := int((g.north - lat) / g.cellSize)
	col := int((lon - g.west) / g.cellSize)

	if row < 0 || row >= g.rows || col < 0 || col >= g.cols {
		return NoData
	}

	v := g.rawAt(row, col)
	if v == -32768 {
		return g.interpolateVoid(row, col)
	}
	return int32(v) * 10 // meters → decimeters
}

// ElevationBatch returns elevations for int32×1e7 encoded coordinates.
func (g *HGTGrid) ElevationBatch(lats, lons []int32) []int32 {
	out := make([]int32, len(lats))
	for i := range lats {
		out[i] = g.ElevationAt(float64(lats[i])/1e7, float64(lons[i])/1e7)
	}
	return out
}

// Close releases mmap resources.
func (g *HGTGrid) Close() {
	if g.data != nil {
		_ = madvise(g.data, syscall.MADV_DONTNEED)
		syscall.Munmap(g.data)
		g.data = nil
	}
	if g.file != nil {
		g.file.Close()
		g.file = nil
	}
}

// rawAt reads the big-endian int16 at (row, col).
func (g *HGTGrid) rawAt(row, col int) int16 {
	off := (row*g.cols + col) * 2
	return int16(g.data[off])<<8 | int16(g.data[off+1])
}

// interpolateVoid fills a void pixel using 8-direction spiral search.
func (g *HGTGrid) interpolateVoid(row, col int) int32 {
	const maxRadius = 5
	dx := [8]int{0, 1, 1, 1, 0, -1, -1, -1}
	dy := [8]int{-1, -1, 0, 1, 1, 1, 0, -1}
	dist := [8]float64{1, 1.41421, 1, 1.41421, 1, 1.41421, 1, 1.41421}

	var sumVal float64
	var sumWeight float64

	for radius := 1; radius <= maxRadius; radius++ {
		for d := range 8 {
			nr := row + dy[d]*radius
			nc := col + dx[d]*radius
			if nr < 0 || nr >= g.rows || nc < 0 || nc >= g.cols {
				continue
			}
			v := g.rawAt(nr, nc)
			if v == -32768 {
				continue
			}
			w := 1.0 / (dist[d] * float64(radius))
			sumVal += float64(v) * w
			sumWeight += w
		}
		if sumWeight > 0 && radius >= 2 {
			break
		}
	}

	if sumWeight == 0 {
		return NoData
	}
	return int32(math.Round(sumVal/sumWeight)) * 10
}

// parseHGTName extracts south latitude and west longitude from an HGT filename.
func parseHGTName(path string) (south, west float64, err error) {
	base := filepath.Base(path)
	if len(base) < 11 {
		return 0, 0, fmt.Errorf("dem: cannot parse HGT name %q", base)
	}

	latSign := 1.0
	if base[0] == 'S' || base[0] == 's' {
		latSign = -1.0
	} else if base[0] != 'N' && base[0] != 'n' {
		return 0, 0, fmt.Errorf("dem: invalid HGT lat prefix %q", base)
	}

	lonSign := 1.0
	if base[3] == 'W' || base[3] == 'w' {
		lonSign = -1.0
	} else if base[3] != 'E' && base[3] != 'e' {
		return 0, 0, fmt.Errorf("dem: invalid HGT lon prefix %q", base)
	}

	var latAbs, lonAbs int
	_, err = fmt.Sscanf(base[1:3], "%d", &latAbs)
	if err != nil {
		return 0, 0, fmt.Errorf("dem: parse HGT lat %q: %w", base, err)
	}
	_, err = fmt.Sscanf(base[4:7], "%d", &lonAbs)
	if err != nil {
		return 0, 0, fmt.Errorf("dem: parse HGT lon %q: %w", base, err)
	}

	return latSign * float64(latAbs), lonSign * float64(lonAbs), nil
}

// madvise wraps the Linux madvise syscall.
func madvise(data []byte, advice int) error {
	_, _, errno := syscall.Syscall(syscall.SYS_MADVISE,
		uintptr(unsafe.Pointer(&data[0])),
		uintptr(len(data)),
		uintptr(advice))
	if errno != 0 {
		return errno
	}
	return nil
}
