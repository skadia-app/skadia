package dem

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNoOpReader(t *testing.T) {
	r := &NoOpReader{}
	defer r.Close()

	if got := r.ElevationAt(47.0, 8.0); got != NoData {
		t.Errorf("ElevationAt = %d, want %d", got, NoData)
	}

	batch := r.ElevationBatch([]int32{470000000}, []int32{80000000})
	if len(batch) != 1 || batch[0] != NoData {
		t.Errorf("ElevationBatch = %v, want [%d]", batch, NoData)
	}
}

// createTestHGT writes a minimal 1°×1° HGT file (3" resolution = 1201×1201).
func createTestHGT(t *testing.T, dir, name string, elevFn func(row, col int) int16) string {
	t.Helper()
	const rows, cols = 1201, 1201
	path := filepath.Join(dir, name)
	data := make([]byte, rows*cols*2)
	for r := range rows {
		for c := range cols {
			v := elevFn(r, c)
			off := (r*cols + c) * 2
			data[off] = byte(v >> 8)
			data[off+1] = byte(v & 0xFF)
		}
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestHGTGrid_BasicLookup(t *testing.T) {
	dir := t.TempDir()
	createTestHGT(t, dir, "N47E008.hgt", func(r, c int) int16 { return 500 })

	g, err := OpenHGTGrid(filepath.Join(dir, "N47E008.hgt"))
	if err != nil {
		t.Fatal(err)
	}
	defer g.Close()

	ele := g.ElevationAt(47.5, 8.5)
	if ele != 5000 {
		t.Errorf("center elevation = %d dm, want 5000", ele)
	}

	ele = g.ElevationAt(46.5, 8.5)
	if ele != NoData {
		t.Errorf("outside elevation = %d, want NoData", ele)
	}
}

func TestHGTGrid_GradientLookup(t *testing.T) {
	dir := t.TempDir()
	createTestHGT(t, dir, "N47E008.hgt", func(r, c int) int16 {
		return int16(1200 - r)
	})

	g, err := OpenHGTGrid(filepath.Join(dir, "N47E008.hgt"))
	if err != nil {
		t.Fatal(err)
	}
	defer g.Close()

	south := g.ElevationAt(47.01, 8.5)
	north := g.ElevationAt(47.99, 8.5)

	if north <= south {
		t.Errorf("north (%d dm) should be > south (%d dm)", north, south)
	}
	if north < 10000 {
		t.Errorf("north too low: %d dm", north)
	}
}

func TestHGTGrid_VoidInterpolation(t *testing.T) {
	dir := t.TempDir()
	createTestHGT(t, dir, "N47E008.hgt", func(r, c int) int16 {
		if r == 600 && c == 600 {
			return -32768
		}
		return 1000
	})

	g, err := OpenHGTGrid(filepath.Join(dir, "N47E008.hgt"))
	if err != nil {
		t.Fatal(err)
	}
	defer g.Close()

	cellSize := 1.0 / 1200.0
	voidLat := 48.0 - float64(600)*cellSize
	voidLon := 8.0 + float64(600)*cellSize
	ele := g.ElevationAt(voidLat, voidLon)

	if ele == NoData {
		t.Error("void should have been interpolated, got NoData")
	}
	if ele < 9500 || ele > 10500 {
		t.Errorf("interpolated void = %d dm, want ~10000", ele)
	}
}

func TestHGTGrid_InvalidFileSize(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "N47E008.hgt")
	os.WriteFile(path, make([]byte, 1000), 0o644)

	_, err := OpenHGTGrid(path)
	if err == nil {
		t.Error("expected error for invalid file size")
	}
}

func TestHGTGrid_Batch(t *testing.T) {
	dir := t.TempDir()
	createTestHGT(t, dir, "N47E008.hgt", func(r, c int) int16 { return 500 })

	g, err := OpenHGTGrid(filepath.Join(dir, "N47E008.hgt"))
	if err != nil {
		t.Fatal(err)
	}
	defer g.Close()

	lats := []int32{475000000, 465000000} // inside, outside
	lons := []int32{85000000, 85000000}
	batch := g.ElevationBatch(lats, lons)
	if batch[0] != 5000 {
		t.Errorf("batch[0] = %d, want 5000", batch[0])
	}
	if batch[1] != NoData {
		t.Errorf("batch[1] = %d, want NoData", batch[1])
	}
}
