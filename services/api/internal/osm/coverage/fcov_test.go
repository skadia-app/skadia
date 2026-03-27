package coverage

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFCOVRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.bin")

	header := FCOVHeader{
		Magic:   [4]byte{'F', 'C', 'O', 'V'},
		Version: 1,
		ResM:    50,
		Rows:    100,
		Cols:    80,
		MinLat:  47.0,
		MinLon:  8.0,
		MaxLat:  48.0,
		MaxLon:  9.0,
	}
	data := make([]uint8, 100*80)
	data[0] = 1
	data[99*80+79] = 255

	if err := WriteFCOV(path, header, data); err != nil {
		t.Fatal(err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() != 32+100*80 {
		t.Errorf("expected %d bytes, got %d", 32+100*80, info.Size())
	}

	h2, d2, err := ReadFCOV(path)
	if err != nil {
		t.Fatal(err)
	}
	if h2 != header {
		t.Errorf("header mismatch: got %+v", h2)
	}
	if d2[0] != 1 || d2[99*80+79] != 255 {
		t.Error("data mismatch")
	}
}

func TestFCOVValidation(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.bin")

	os.WriteFile(path, []byte("not a coverage file"), 0644)
	_, _, err := ReadFCOV(path)
	if err == nil {
		t.Error("expected error for invalid magic")
	}
}

func TestFCOVMmap(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "mmap.bin")

	header := FCOVHeader{
		Magic:   [4]byte{'F', 'C', 'O', 'V'},
		Version: 1,
		ResM:    100,
		Rows:    50,
		Cols:    40,
		MinLat:  47.0,
		MinLon:  8.0,
		MaxLat:  48.0,
		MaxLon:  9.0,
	}
	data := make([]uint8, 50*40)
	data[25*40+20] = 42

	if err := WriteFCOV(path, header, data); err != nil {
		t.Fatal(err)
	}

	h, mapped, cleanup, err := MmapFCOV(path)
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()

	if h.Rows != 50 || h.Cols != 40 {
		t.Errorf("header mismatch: %+v", h)
	}
	if mapped[25*40+20] != 42 {
		t.Error("mmap data mismatch")
	}
}
