package graph

import (
	"testing"
)

func TestEncodeDecodeHGTManifest_RoundTrip(t *testing.T) {
	orig := &HGTManifest{
		Cells: []HGTCellRef{
			{CellName: "N47E011", FileSize: 25934402, Xxhash3: 0xdeadbeefcafebabe},
			{CellName: "N47E012", FileSize: 103694402, Xxhash3: 0x0102030405060708},
			{CellName: "S01W179", FileSize: 2884802, Xxhash3: 0},
		},
	}

	encoded := EncodeHGTManifest(orig)
	if len(encoded) == 0 {
		t.Fatal("encoded manifest is empty")
	}

	decoded, err := DecodeHGTManifest(encoded)
	if err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if len(decoded.Cells) != len(orig.Cells) {
		t.Fatalf("cell count mismatch: got %d, want %d", len(decoded.Cells), len(orig.Cells))
	}
	for i, want := range orig.Cells {
		got := decoded.Cells[i]
		if got.CellName != want.CellName {
			t.Errorf("cell[%d].CellName: got %q, want %q", i, got.CellName, want.CellName)
		}
		if got.FileSize != want.FileSize {
			t.Errorf("cell[%d].FileSize: got %d, want %d", i, got.FileSize, want.FileSize)
		}
		if got.Xxhash3 != want.Xxhash3 {
			t.Errorf("cell[%d].Xxhash3: got %016x, want %016x", i, got.Xxhash3, want.Xxhash3)
		}
	}
}

func TestEncodeDecodeHGTManifest_Empty(t *testing.T) {
	empty := &HGTManifest{}
	encoded := EncodeHGTManifest(empty)
	decoded, err := DecodeHGTManifest(encoded)
	if err != nil {
		t.Fatalf("decode empty error: %v", err)
	}
	if len(decoded.Cells) != 0 {
		t.Errorf("expected 0 cells, got %d", len(decoded.Cells))
	}
}

func TestDecodeHGTManifest_TruncatedBuf_Error(t *testing.T) {
	orig := &HGTManifest{
		Cells: []HGTCellRef{
			{CellName: "N47E011", FileSize: 25934402, Xxhash3: 0xabcd1234},
		},
	}
	encoded := EncodeHGTManifest(orig)

	// Truncate just before the hash fields — remove last 8 bytes.
	truncated := encoded[:len(encoded)-8]
	_, err := DecodeHGTManifest(truncated)
	if err == nil {
		t.Fatal("expected error for truncated buffer, got nil")
	}
}
