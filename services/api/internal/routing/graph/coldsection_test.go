package graph

import (
	"encoding/binary"
	"math"
	"os"
	"testing"
)

func TestReadColdFloat32Section(t *testing.T) {
	f, err := os.CreateTemp("", "cold_test_*.bin")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(f.Name())
	defer f.Close()

	values := make([]float32, 32)
	for i := range 32 {
		values[i] = float32(i) * 100.0
	}
	buf := make([]byte, 32*4)
	for i, v := range values {
		binary.LittleEndian.PutUint32(buf[i*4:], math.Float32bits(v))
	}
	f.Write(buf)
	f.Sync()

	entry := sectionEntry{Offset: 0, Length: uint32(len(buf))}

	dists, err := ReadColdFloat32Section(f, entry, 0, 16)
	if err != nil {
		t.Fatal(err)
	}
	if len(dists) != 16 {
		t.Fatalf("expected 16, got %d", len(dists))
	}
	if dists[0] != 0.0 || dists[15] != 1500.0 {
		t.Errorf("node 0: dists[0]=%f, dists[15]=%f", dists[0], dists[15])
	}

	dists, err = ReadColdFloat32Section(f, entry, 16, 16)
	if err != nil {
		t.Fatal(err)
	}
	if dists[0] != 1600.0 || dists[15] != 3100.0 {
		t.Errorf("node 1: dists[0]=%f, dists[15]=%f", dists[0], dists[15])
	}
}

func TestGraphLandmarkDistsNil(t *testing.T) {
	g := &Graph{}
	dists := g.LandmarkDists(LandmarkSetA, 0)
	if dists != nil {
		t.Error("expected nil for graph without landmarks")
	}
}
