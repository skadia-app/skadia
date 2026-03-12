package phash

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"math/bits"
	"testing"
)

func TestHammingDistance(t *testing.T) {
	tests := []struct {
		name string
		a, b uint64
		want int
	}{
		{"identical", 0xFFFFFFFF, 0xFFFFFFFF, 0},
		{"one bit", 0x0, 0x1, 1},
		{"all different", 0x0, 0xFFFFFFFFFFFFFFFF, 64},
		{"three bits", 0b111, 0b000, 3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := HammingDistance(tt.a, tt.b)
			if got != tt.want {
				t.Errorf("HammingDistance(%x, %x) = %d, want %d", tt.a, tt.b, got, tt.want)
			}
		})
	}
	_ = bits.OnesCount64(0) // ensure import used
}

func TestIsDuplicate(t *testing.T) {
	if !IsDuplicate(0xFF, 0xFE, 3) {
		t.Error("expected duplicate for distance 1")
	}
	if IsDuplicate(0xFF, 0xF0, 3) {
		t.Error("expected not duplicate for distance 4")
	}
}

func TestComputeFromBytes_JPEG(t *testing.T) {
	// Create a checkerboard pattern (avoids solid-color edge case)
	img := image.NewRGBA(image.Rect(0, 0, 64, 64))
	for y := 0; y < 64; y++ {
		for x := 0; x < 64; x++ {
			if (x/8+y/8)%2 == 0 {
				img.Set(x, y, color.White)
			} else {
				img.Set(x, y, color.Black)
			}
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, nil); err != nil {
		t.Fatal(err)
	}
	hash, err := ComputeFromBytes(buf.Bytes())
	if err != nil {
		t.Fatalf("ComputeFromBytes: %v", err)
	}
	if hash == 0 {
		t.Error("expected non-zero hash for checkerboard image")
	}
}
