package dem

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMultiHGTReader_TwoGrids(t *testing.T) {
	dir := t.TempDir()
	createTestHGT(t, dir, "N47E008.hgt", func(r, c int) int16 { return 500 })
	createTestHGT(t, dir, "N47E009.hgt", func(r, c int) int16 { return 800 })

	g1, _ := OpenHGTGrid(filepath.Join(dir, "N47E008.hgt"))
	g2, _ := OpenHGTGrid(filepath.Join(dir, "N47E009.hgt"))

	mr := &MultiHGTReader{grids: []*HGTGrid{g1, g2}}
	defer mr.Close()

	ele1 := mr.ElevationAt(47.5, 8.5)
	if ele1 != 5000 {
		t.Errorf("tile 1 elevation = %d, want 5000", ele1)
	}
	ele2 := mr.ElevationAt(47.5, 9.5)
	if ele2 != 8000 {
		t.Errorf("tile 2 elevation = %d, want 8000", ele2)
	}
	ele3 := mr.ElevationAt(47.5, 10.5)
	if ele3 != NoData {
		t.Errorf("outside elevation = %d, want NoData", ele3)
	}
}

func TestHGTCellsForBBox(t *testing.T) {
	// BBox spanning a tile corner: 46.98-48.02, 7.98-9.02
	bbox := [4]float64{46.98, 7.98, 48.02, 9.02}
	cells := HGTCellsForBBox(bbox)

	// floor(46.98)=46..floor(48.02)=48, floor(7.98)=7..floor(9.02)=9
	// = 3 lat × 3 lon = 9 cells
	if len(cells) != 9 {
		t.Errorf("expected 9 cells, got %d: %v", len(cells), cells)
	}

	found := false
	for _, c := range cells {
		if c.Lat == 47 && c.Lon == 8 {
			found = true
		}
	}
	if !found {
		t.Error("N47E008 not in cells list")
	}
}

func TestNewReaderForTile_Cascade(t *testing.T) {
	dir := t.TempDir()
	highDir := filepath.Join(dir, "sonny-05")
	lowDir := filepath.Join(dir, "glo30")
	os.MkdirAll(highDir, 0o755)
	os.MkdirAll(lowDir, 0o755)

	// Only low-res available for N47E008
	createTestHGT(t, lowDir, "N47E008.hgt", func(r, c int) int16 { return 300 })

	tileID := uint32((47+90)*360 + (8 + 180))
	reader, err := NewReaderForTile([]string{highDir, lowDir}, tileID)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()

	ele := reader.ElevationAt(47.5, 8.5)
	if ele != 3000 {
		t.Errorf("cascade elevation = %d, want 3000", ele)
	}
}

func TestNewReaderForTile_NoDEM(t *testing.T) {
	dir := t.TempDir()
	tileID := uint32((47+90)*360 + (8 + 180))
	reader, err := NewReaderForTile([]string{dir}, tileID)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()

	ele := reader.ElevationAt(47.5, 8.5)
	if ele != NoData {
		t.Errorf("no DEM: elevation = %d, want NoData", ele)
	}
}

func TestHGTCell_FileName(t *testing.T) {
	tests := []struct {
		lat, lon int
		want     string
	}{
		{47, 8, "N47E008.hgt"},
		{-33, -71, "S33W071.hgt"},
		{0, 0, "N00E000.hgt"},
		{47, -1, "N47W001.hgt"},
	}
	for _, tt := range tests {
		got := HGTCell{Lat: tt.lat, Lon: tt.lon}.FileName()
		if got != tt.want {
			t.Errorf("HGTCell{%d,%d}.FileName() = %q, want %q", tt.lat, tt.lon, got, tt.want)
		}
	}
}

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
