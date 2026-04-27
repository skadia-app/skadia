package dem

// service_drift_test.go — tests for ScanTilesForDrift and related drift-
// detection logic.
//
// These tests stay in package dem (not dem_test) so they can access the
// internal rawHGTManifest and healthState types.
// They MUST NOT import routing/graph — that would create a circular import.
// Instead, they build tile binary blobs manually using the known format
// constants defined in drift_scan.go.

import (
	"encoding/binary"
	"fmt"
	"hash/crc32"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/zeebo/xxh3"
)

// makeMinimalTileWithManifest builds a minimal valid tile blob (.graph file)
// containing only a section-27 HGT manifest and the mandatory header/crc.
// The tile has no other sections (no routing data) — just enough for the drift
// scan to parse.
func makeMinimalTileWithManifest(t *testing.T, cells []hgtManifestEntry) []byte {
	t.Helper()

	// Encode the manifest payload.
	manifestData := encodeRawManifest(cells)

	// Build section directory (1 entry: section 27).
	const sectionEntrySize = 12
	const sectionCount = 1
	dirSize := sectionCount * sectionEntrySize
	headerSize := tileHeaderSize
	dataStart := uint64(headerSize + dirSize)

	// Section entry: type=27, offset=dataStart, length=len(manifestData).
	dirBuf := make([]byte, dirSize)
	binary.LittleEndian.PutUint16(dirBuf[0:2], sectionHGTManifest)
	dirBuf[2] = byte(dataStart)
	dirBuf[3] = byte(dataStart >> 8)
	dirBuf[4] = byte(dataStart >> 16)
	dirBuf[5] = byte(dataStart >> 24)
	dirBuf[6] = byte(dataStart >> 32)
	dirBuf[7] = byte(dataStart >> 40)
	binary.LittleEndian.PutUint32(dirBuf[8:12], uint32(len(manifestData)))

	// Build header (64 bytes).
	header := make([]byte, tileHeaderSize)
	copy(header[0:4], tileGraphMagic)
	// FormatVersion = 4 (must match graph.FormatVersion or Deserialize will reject).
	binary.LittleEndian.PutUint32(header[4:8], 4)
	// NodeCount=0, EdgeCount=0, poolSize=0 (no routing data).
	// BBox: all zeros. BuildTime: 0.
	binary.LittleEndian.PutUint16(header[60:62], sectionCount)

	// Concatenate header + dir + data.
	payload := append(header, dirBuf...)
	payload = append(payload, manifestData...)

	// CRC32-Castagnoli trailer.
	table := crc32.MakeTable(crc32.Castagnoli)
	crcVal := crc32.Checksum(payload, table)
	var crcBuf [4]byte
	binary.LittleEndian.PutUint32(crcBuf[:], crcVal)
	return append(payload, crcBuf[:]...)
}

// encodeRawManifest serialises []hgtManifestEntry using the same format as
// graph.EncodeHGTManifest (uvarint count, then per cell: uvarint nameLen +
// name + int64 fileSize + uint64 xxhash3).
func encodeRawManifest(cells []hgtManifestEntry) []byte {
	var buf []byte
	buf = appendUvarintBytes(buf, uint64(len(cells)))
	for _, c := range cells {
		nameBytes := []byte(c.cellName)
		buf = appendUvarintBytes(buf, uint64(len(nameBytes)))
		buf = append(buf, nameBytes...)
		buf = binary.LittleEndian.AppendUint64(buf, uint64(c.fileSize))
		buf = binary.LittleEndian.AppendUint64(buf, c.xxhash3)
	}
	return buf
}

func appendUvarintBytes(buf []byte, v uint64) []byte {
	for v >= 0x80 {
		buf = append(buf, byte(v)|0x80)
		v >>= 7
	}
	return append(buf, byte(v))
}

// xxh3Content returns the xxhash3 digest of a byte slice.
func xxh3Content(b []byte) uint64 {
	h := xxh3.New()
	_, _ = io.Writer(h).Write(b)
	return h.Sum64()
}

// xxh3ContentPartial returns the same partial xxhash3 digest that xxh3File
// computes at scan time: head + tail xxh3PartialWindowSize bytes (or full
// slice when smaller). Use this when building manifest entries in tests so
// that the stored hash matches what the drift scan will compute from disk.
func xxh3ContentPartial(b []byte) uint64 {
	if len(b) <= 2*xxh3PartialWindowSize {
		return xxh3Content(b)
	}
	h := xxh3.New()
	h.Write(b[:xxh3PartialWindowSize])          //nolint:errcheck
	h.Write(b[len(b)-xxh3PartialWindowSize:])   //nolint:errcheck
	return h.Sum64()
}

// --- Tests ---

// TestDriftCheck_StaleHGT_Warns verifies that when a manifest entry has a
// different FileSize than the actual HGT file on disk, the tile is recorded
// in s.health.tileDriftCells.
func TestDriftCheck_StaleHGT_Warns(t *testing.T) {
	dir := t.TempDir()
	tileDir := t.TempDir()
	hgtDir := filepath.Join(dir, "dem")
	if err := os.MkdirAll(hgtDir, 0o755); err != nil {
		t.Fatal(err)
	}

	cellName := "N47E011"
	hgtPath := filepath.Join(hgtDir, cellName+".hgt")
	realContent := make([]byte, 2884802) // 3" resolution (1201×1201×2)
	for i := range realContent {
		realContent[i] = byte(i & 0xff)
	}
	if err := os.WriteFile(hgtPath, realContent, 0o644); err != nil {
		t.Fatal(err)
	}

	// Manifest claims a different file size (stale).
	cells := []hgtManifestEntry{{
		cellName: cellName,
		fileSize: int64(len(realContent)) + 1000, // wrong size
		xxhash3:  0xdeadbeef,
	}}
	tileBlob := makeMinimalTileWithManifest(t, cells)
	tilePath := filepath.Join(tileDir, "tile_00049331.graph")
	if err := os.WriteFile(tilePath, tileBlob, 0o644); err != nil {
		t.Fatal(err)
	}

	svc := NewService([]string{hgtDir})
	svc.ScanTilesForDrift(tileDir, 10)

	drift := svc.health.driftSnapshot()
	if len(drift) == 0 {
		t.Fatal("expected drift to be flagged, got no drift tiles")
	}
	tileName := filepath.Base(tilePath)
	cells2, ok := drift[tileName]
	if !ok {
		t.Fatalf("expected tile %q in drift map, got %v", tileName, drift)
	}
	found := false
	for _, c := range cells2 {
		if c == cellName {
			found = true
		}
	}
	if !found {
		t.Errorf("expected cell %q in drift cells, got %v", cellName, cells2)
	}
}

// TestDriftCheck_MtimePreservingCopy_DetectedByXxhash3 verifies that a file
// with the same size but modified content (e.g. cp --preserve=mtime,size)
// is detected by the xxhash3 comparison.
func TestDriftCheck_MtimePreservingCopy_DetectedByXxhash3(t *testing.T) {
	dir := t.TempDir()
	tileDir := t.TempDir()
	hgtDir := filepath.Join(dir, "dem")
	if err := os.MkdirAll(hgtDir, 0o755); err != nil {
		t.Fatal(err)
	}

	cellName := "N47E011"
	hgtPath := filepath.Join(hgtDir, cellName+".hgt")

	// Original content — what the manifest was built against.
	originalContent := make([]byte, 2884802)
	for i := range originalContent {
		originalContent[i] = byte(i & 0xff)
	}
	// Use the same partial hash that buildHGTManifest stores at tile-build time.
	originalHash := xxh3ContentPartial(originalContent)
	originalSize := int64(len(originalContent))

	// Write the "replaced" file (same size, different content).
	replacedContent := make([]byte, 2884802)
	for i := range replacedContent {
		replacedContent[i] = byte((i + 1) & 0xff) // shifted by 1
	}
	if err := os.WriteFile(hgtPath, replacedContent, 0o644); err != nil {
		t.Fatal(err)
	}
	// Simulate mtime-preserving copy: set the mtime to a fixed past time.
	// os.Chtimes is sufficient — the drift scan does not check mtime.

	// Manifest records the original hash (now mismatched with replaced file).
	cells := []hgtManifestEntry{{
		cellName: cellName,
		fileSize: originalSize,
		xxhash3:  originalHash,
	}}
	tileBlob := makeMinimalTileWithManifest(t, cells)
	tilePath := filepath.Join(tileDir, "tile_00049331.graph")
	if err := os.WriteFile(tilePath, tileBlob, 0o644); err != nil {
		t.Fatal(err)
	}

	svc := NewService([]string{hgtDir})
	svc.ScanTilesForDrift(tileDir, 10)

	drift := svc.health.driftSnapshot()
	if len(drift) == 0 {
		t.Fatal("expected xxhash3 drift to be flagged, got no drift tiles")
	}
	tileName := filepath.Base(tilePath)
	cells2, ok := drift[tileName]
	if !ok {
		t.Fatalf("expected tile %q in drift map, got %v", tileName, drift)
	}
	found := false
	for _, c := range cells2 {
		if c == cellName {
			found = true
		}
	}
	if !found {
		t.Errorf("expected cell %q in drift cells, got %v", cellName, cells2)
	}
}

// TestDriftCheck_ManifestMatch_NoFlag verifies that when the manifest and
// the actual HGT file are identical, no drift flag is set.
func TestDriftCheck_ManifestMatch_NoFlag(t *testing.T) {
	dir := t.TempDir()
	tileDir := t.TempDir()
	hgtDir := filepath.Join(dir, "dem")
	if err := os.MkdirAll(hgtDir, 0o755); err != nil {
		t.Fatal(err)
	}

	cellName := "N47E011"
	hgtPath := filepath.Join(hgtDir, cellName+".hgt")
	content := make([]byte, 2884802)
	for i := range content {
		content[i] = byte(i & 0xff)
	}
	if err := os.WriteFile(hgtPath, content, 0o644); err != nil {
		t.Fatal(err)
	}

	// Manifest matches the real file using the same partial hash that the drift
	// scan computes at runtime (head + tail xxh3PartialWindowSize bytes).
	hash := xxh3ContentPartial(content)
	cells := []hgtManifestEntry{{
		cellName: cellName,
		fileSize: int64(len(content)),
		xxhash3:  hash,
	}}
	tileBlob := makeMinimalTileWithManifest(t, cells)
	tilePath := filepath.Join(tileDir, "tile_00049331.graph")
	if err := os.WriteFile(tilePath, tileBlob, 0o644); err != nil {
		t.Fatal(err)
	}

	svc := NewService([]string{hgtDir})
	svc.ScanTilesForDrift(tileDir, 10)

	drift := svc.health.driftSnapshot()
	if len(drift) != 0 {
		t.Errorf("expected no drift, got %v", drift)
	}
}

// TestDriftCheck_PreManifestTile_NoFlag verifies that a tile without section
// 27 (pre-manifest) does not cause any health flags.
func TestDriftCheck_PreManifestTile_NoFlag(t *testing.T) {
	tileDir := t.TempDir()

	// Build a minimal tile blob without a manifest section (no section 27).
	// Header with sectionCnt=0 and valid CRC.
	header := make([]byte, tileHeaderSize)
	copy(header[0:4], tileGraphMagic)
	binary.LittleEndian.PutUint32(header[4:8], 4)
	binary.LittleEndian.PutUint16(header[60:62], 0) // 0 sections
	table := crc32.MakeTable(crc32.Castagnoli)
	crcVal := crc32.Checksum(header, table)
	var crcBuf [4]byte
	binary.LittleEndian.PutUint32(crcBuf[:], crcVal)
	tileBlob := append(header, crcBuf[:]...)

	tilePath := filepath.Join(tileDir, "old_tile.graph")
	if err := os.WriteFile(tilePath, tileBlob, 0o644); err != nil {
		t.Fatal(err)
	}

	svc := NewService([]string{})
	svc.ScanTilesForDrift(tileDir, 10)

	drift := svc.health.driftSnapshot()
	if len(drift) != 0 {
		t.Errorf("expected no drift for pre-manifest tile, got %v", drift)
	}
}

// TestHealthStruct_TileDriftCells verifies that the Health() snapshot includes
// drift cells that were marked by the drift scan.
func TestHealthStruct_TileDriftCells(t *testing.T) {
	svc := NewService(nil)
	svc.health.markTileDrift("tile_001.graph", []string{"N47E011", "N47E012"})

	h := svc.Health()
	if len(h.TileDriftCells) == 0 {
		t.Fatal("expected TileDriftCells in Health snapshot")
	}
	cells, ok := h.TileDriftCells["tile_001.graph"]
	if !ok {
		t.Fatalf("tile_001.graph not in TileDriftCells: %v", h.TileDriftCells)
	}
	if fmt.Sprintf("%v", cells) != "[N47E011 N47E012]" {
		t.Errorf("unexpected cells: %v", cells)
	}
}
