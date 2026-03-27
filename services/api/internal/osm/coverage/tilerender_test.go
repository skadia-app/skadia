package coverage

import (
	"bytes"
	"image/png"
	"testing"

	"github.com/paulmach/orb"
)

func TestRenderSingleLayerTile(t *testing.T) {
	core := orb.Bound{Min: orb.Point{8.0, 47.0}, Max: orb.Point{9.0, 48.0}}
	forest := NewGrid(core, 50)
	for row := range forest.Rows / 2 {
		for col := range forest.Cols {
			forest.Data[row*forest.Cols+col] = 1
		}
	}

	mg := &MappedGrid{
		header: FCOVHeader{
			MinLat: 47.0,
			MinLon: 8.0,
			MaxLat: 48.0,
			MaxLon: 9.0,
			Rows:   uint32(forest.Rows),
			Cols:   uint32(forest.Cols),
		},
		data:    append([]byte{}, forest.Data...),
		rows:    forest.Rows,
		cols:    forest.Cols,
		resM:    forest.ResM,
		bound:   forest.Bound,
		latStep: forest.LatStep,
		lonStep: forest.LonStep,
	}

	img := RenderSingleLayerTile([]*MappedGrid{mg}, paletteForest, 256, 256, core)
	if img == nil {
		t.Fatal("expected non-nil image")
	}
	if img.Bounds().Dx() != 256 || img.Bounds().Dy() != 256 {
		t.Errorf("image size = %v, want 256x256", img.Bounds())
	}

	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	if buf.Len() < 100 {
		t.Error("PNG suspiciously small")
	}
}

func TestTransparentPNG(t *testing.T) {
	data := TransparentPNG()
	if len(data) < 50 {
		t.Error("transparent PNG too small")
	}
	_, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("transparent PNG not decodable: %v", err)
	}
}

func TestPNGLRUCache(t *testing.T) {
	cache := NewPNGCache(10, "test-ts")
	key := PNGCacheKey{Layer: "forest", Z: 10, X: 5, Y: 3}

	if _, ok := cache.Get(key); ok {
		t.Error("expected cache miss")
	}

	data := []byte("fake-png-data")
	cache.Put(key, data)
	got, ok := cache.Get(key)
	if !ok {
		t.Error("expected cache hit")
	}
	if !bytes.Equal(got, data) {
		t.Error("cache data mismatch")
	}

	cache.InvalidateIfStale("new-ts")
	if _, ok := cache.Get(key); ok {
		t.Error("expected cache miss after timestamp invalidation")
	}
}
