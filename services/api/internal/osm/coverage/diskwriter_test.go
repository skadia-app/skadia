package coverage

import (
	"path/filepath"
	"testing"

	"github.com/paulmach/orb"
)

func TestCropGridToCore(t *testing.T) {
	// Grid built with 0.1° expansion: 46.9–48.1 lat, 7.9–9.1 lon
	expanded := orb.Bound{Min: orb.Point{7.9, 46.9}, Max: orb.Point{9.1, 48.1}}
	g := NewGrid(expanded, 100)

	// Fill entire grid with row index as value
	for row := range g.Rows {
		for col := range g.Cols {
			g.Data[row*g.Cols+col] = uint8(row % 256)
		}
	}

	// Crop to core 47–48 lat, 8–9 lon
	core := orb.Bound{Min: orb.Point{8.0, 47.0}, Max: orb.Point{9.0, 48.0}}
	cropped := CropGridToCore(g, core)

	if cropped.Bound != core {
		t.Errorf("cropped bound = %v, want %v", cropped.Bound, core)
	}
	if cropped.Rows*cropped.Cols != len(cropped.Data) {
		t.Errorf("data size mismatch: Rows=%d Cols=%d len=%d", cropped.Rows, cropped.Cols, len(cropped.Data))
	}
	// Core center should have the value from the corresponding expanded row
	centerVal := cropped.SamplePoint(orb.Point{8.5, 47.5})
	if centerVal == 0 || centerVal == OutOfBounds {
		t.Error("expected non-zero value at core center")
	}
}

func TestWriteTileGrids(t *testing.T) {
	dir := t.TempDir()

	// Create minimal TileGrids with 2 layers
	bound := orb.Bound{Min: orb.Point{7.9, 46.9}, Max: orb.Point{9.1, 48.1}}
	tg := NewTileGrids()
	forest := NewGrid(bound, 50)
	forest.Set(forest.Rows/2, forest.Cols/2, 1)
	tg.Set(LayerForest, forest)

	noise := NewGrid(bound, 100)
	noise.Set(noise.Rows/2, noise.Cols/2, 128)
	tg.Set(LayerNoise, noise)

	coreBound := orb.Bound{Min: orb.Point{8.0, 47.0}, Max: orb.Point{9.0, 48.0}}
	err := WriteTileGrids(tg, nil, dir, coreBound, "")
	if err != nil {
		t.Fatal(err)
	}

	// Verify forest file exists
	forestPath := filepath.Join(dir, "forest", "47_8.bin")
	h, data, err := ReadFCOV(forestPath)
	if err != nil {
		t.Fatal(err)
	}
	if h.ResM != 50 {
		t.Errorf("forest ResM = %d, want 50", h.ResM)
	}
	if len(data) != int(h.Rows)*int(h.Cols) {
		t.Error("data size mismatch")
	}

	// Verify noise file
	noisePath := filepath.Join(dir, "noise", "47_8.bin")
	h2, _, err := ReadFCOV(noisePath)
	if err != nil {
		t.Fatal(err)
	}
	if h2.ResM != 100 {
		t.Errorf("noise ResM = %d, want 100", h2.ResM)
	}
}
