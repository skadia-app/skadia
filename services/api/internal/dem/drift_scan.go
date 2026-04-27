package dem

// drift_scan.go — boot-time HGT drift detection.
//
// ScanTilesForDrift samples a small number of tile files, reads the embedded
// HGT manifest (section 27), and compares each manifest entry against the
// current state of the corresponding HGT file on disk (size + xxhash3).
//
// Design constraint: the routing/graph package imports dem, so dem MUST NOT
// import routing/graph.  This file therefore contains a minimal self-contained
// binary reader that can locate section 27 in a tile file without depending on
// graph.Deserialize.

import (
	"encoding/binary"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sort"

	"github.com/zeebo/xxh3"
)

// xxh3PartialWindowSize is the number of bytes read from each end of an HGT
// file when computing the drift-detection hash. Reading the full file
// (~25 MB SRTM-1" / ~100 MB Cop-30) for every cell of every sampled tile
// causes several GB of unnecessary I/O at boot. A 64 KB head+tail window
// detects truncation, header/metadata changes, and random content corruption
// with high probability.
//
// This is a drift-detection heuristic, not a cryptographic integrity hash.
// A deliberate "cp --preserve=mtime,size" that also preserves the first and
// last 64 KB of the file would evade detection — that threat model is
// considered out of scope for an advisory boot check.
const xxh3PartialWindowSize = 64 * 1024 // 64 KB

// Xxh3Partial computes the xxhash3 digest of the first and last
// xxh3PartialWindowSize bytes of the file at path (or the full file when
// smaller). It is exported so that routing/graph.buildHGTManifest can store
// the same partial hash at tile-build time, keeping the comparison consistent.
//
// This is a drift-detection heuristic, not a cryptographic integrity hash.
func Xxh3Partial(path string) (uint64, error) {
	return xxh3Partial(path)
}

// xxh3Partial is the unexported implementation called by both Xxh3Partial and
// xxh3File.
func xxh3Partial(path string) (uint64, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return 0, err
	}
	size := info.Size()

	h := xxh3.New()

	if size <= 2*xxh3PartialWindowSize {
		// File fits in two windows — hash it entirely.
		if _, err := io.Copy(h, f); err != nil {
			return 0, err
		}
		return h.Sum64(), nil
	}

	// Head window.
	head := make([]byte, xxh3PartialWindowSize)
	if _, err := io.ReadFull(f, head); err != nil {
		return 0, fmt.Errorf("xxh3 partial head read: %w", err)
	}
	h.Write(head) //nolint:errcheck

	// Tail window.
	tail := make([]byte, xxh3PartialWindowSize)
	if _, err := f.ReadAt(tail, size-xxh3PartialWindowSize); err != nil {
		return 0, fmt.Errorf("xxh3 partial tail read: %w", err)
	}
	h.Write(tail) //nolint:errcheck

	return h.Sum64(), nil
}

// tileGraphMagic is the magic bytes expected at offset 0 of every tile file.
// Duplicated from routing/graph.Magic to avoid the circular import.
const tileGraphMagic = "FRGR"

// sectionHGTManifest is the section type ID for the HGT manifest.
// Must match graph.SectionHGTManifest = 27.
const sectionHGTManifest = uint16(27)

// tileHeaderSize matches graph.HeaderSize = 64.
const tileHeaderSize = 64

// ScanTilesForDrift samples up to sampleN tile files from tileDir and checks
// each tile's embedded HGT manifest (section 27) against the current state of
// the HGT files on disk.  Mismatches are recorded in s.health.tileDriftCells
// and logged as warnings.
//
// This is a best-effort operation: errors per tile are logged but do not stop
// the scan and do not cause a boot failure.  The function is intended to be
// called asynchronously (go demSvc.ScanTilesForDrift(...)) so that boot is
// non-blocked.
func (s *Service) ScanTilesForDrift(tileDir string, sampleN int) {
	entries, err := os.ReadDir(tileDir)
	if err != nil {
		slog.Warn("dem drift scan: cannot read tile directory",
			"tile_dir", tileDir, "error", err)
		return
	}

	// Collect .graph files and sort for deterministic sampling.
	var paths []string
	for _, e := range entries {
		if !e.IsDir() && filepath.Ext(e.Name()) == ".graph" {
			paths = append(paths, filepath.Join(tileDir, e.Name()))
		}
	}
	sort.Strings(paths)

	if sampleN > 0 && len(paths) > sampleN {
		paths = paths[:sampleN]
	}

	slog.Info("dem drift scan: starting", "tile_count", len(paths), "tile_dir", tileDir)

	for _, tilePath := range paths {
		if err := s.scanTile(tilePath); err != nil {
			slog.Warn("dem drift scan: tile scan error", "path", tilePath, "error", err)
		}
	}

	slog.Info("dem drift scan: complete",
		"scanned", len(paths),
		"drift_tiles", len(s.health.driftSnapshot()))
}

// scanTile reads section 27 of a single tile file, then compares each cell
// entry against the corresponding HGT file on disk.  Any mismatch is flagged.
//
// Only the tile header, section directory, and section-27 payload are read —
// the rest of the tile (routing graph data, which can be 100 MB–1 GB) is
// never loaded into memory.
func (s *Service) scanTile(tilePath string) error {
	manifest, err := extractHGTManifestFromFile(tilePath)
	if err != nil {
		// Section absent (pre-manifest tile) or corrupt — skip silently.
		slog.Debug("dem drift scan: no manifest in tile (pre-manifest or corrupt)",
			"path", tilePath, "error", err)
		return nil
	}
	if manifest == nil || len(manifest.cells) == 0 {
		return nil // pre-manifest tile; nothing to check
	}

	tileID := filepath.Base(tilePath)

	var driftCells []string
	for _, ref := range manifest.cells {
		cellFile := s.findHGTFile(ref.cellName)
		if cellFile == "" {
			slog.Debug("dem drift scan: HGT file not found for manifest cell",
				"tile", tileID, "cell", ref.cellName)
			continue
		}

		info, err := os.Stat(cellFile)
		if err != nil {
			slog.Warn("dem drift scan: cannot stat HGT file",
				"tile", tileID, "cell", ref.cellName, "path", cellFile, "error", err)
			continue
		}

		if info.Size() != ref.fileSize {
			slog.Warn("dem drift scan: HGT size mismatch — possible drift",
				"tile", tileID, "cell", ref.cellName,
				"manifest_size", ref.fileSize, "disk_size", info.Size())
			driftCells = append(driftCells, ref.cellName)
			continue
		}

		// Size matches — verify content with xxhash3 (detects cp --preserve=mtime,size).
		diskHash, err := xxh3File(cellFile)
		if err != nil {
			slog.Warn("dem drift scan: cannot hash HGT file",
				"tile", tileID, "cell", ref.cellName, "error", err)
			continue
		}
		if diskHash != ref.xxhash3 {
			slog.Warn("dem drift scan: HGT content mismatch — drift detected",
				"tile", tileID, "cell", ref.cellName,
				"manifest_hash", fmt.Sprintf("%016x", ref.xxhash3),
				"disk_hash", fmt.Sprintf("%016x", diskHash))
			driftCells = append(driftCells, ref.cellName)
		}
	}

	if len(driftCells) > 0 {
		s.health.markTileDrift(tileID, driftCells)
	}
	return nil
}

// findHGTFile returns the path of the HGT file for cellName (without extension)
// by searching hgtDirs in priority order.  Returns "" if not found.
func (s *Service) findHGTFile(cellName string) string {
	filename := cellName + ".hgt"
	for _, dir := range s.hgtDirs {
		path := filepath.Join(dir, filename)
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}
	return ""
}

// xxh3File computes a partial xxhash3 digest of the HGT file at path.
// It hashes the first and last xxh3PartialWindowSize bytes (or the full file
// when smaller) — see Xxh3Partial for rationale.
func xxh3File(path string) (uint64, error) {
	return xxh3Partial(path)
}

// --- Minimal tile binary reader (section 27 only) ---

type hgtManifestEntry struct {
	cellName string
	fileSize int64
	xxhash3  uint64
}

type rawHGTManifest struct {
	cells []hgtManifestEntry
}

// extractHGTManifestFromFile opens the tile at path, reads only the header
// (64 bytes), section directory (sectionCnt × 12 bytes), and the section-27
// payload.  The bulk of the tile file (routing graph data) is never read into
// memory, avoiding the multi-GB heap spike that os.ReadFile caused on large
// tiles.
//
// Returns (nil, nil) when section 27 is absent (pre-manifest tile).
func extractHGTManifestFromFile(path string) (*rawHGTManifest, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open tile: %w", err)
	}
	defer f.Close()

	// Read the fixed-size header.
	header := make([]byte, tileHeaderSize)
	if _, err := io.ReadFull(f, header); err != nil {
		return nil, fmt.Errorf("tile header read: %w", err)
	}
	if string(header[0:4]) != tileGraphMagic {
		return nil, fmt.Errorf("invalid magic %q", header[0:4])
	}

	sectionCnt := int(binary.LittleEndian.Uint16(header[60:62]))
	if sectionCnt == 0 {
		return nil, nil // pre-manifest tile with no sections
	}

	// Read only the section directory.
	const sectionEntrySize = 12
	dirBuf := make([]byte, sectionCnt*sectionEntrySize)
	if _, err := io.ReadFull(f, dirBuf); err != nil {
		return nil, fmt.Errorf("tile section directory read: %w", err)
	}

	// Locate section 27 within the directory.
	var sectionOffset int64
	var sectionLength int
	found := false
	for i := 0; i < sectionCnt; i++ {
		off := i * sectionEntrySize
		typ := binary.LittleEndian.Uint16(dirBuf[off : off+2])
		rawOff := uint64(dirBuf[off+2]) |
			uint64(dirBuf[off+3])<<8 |
			uint64(dirBuf[off+4])<<16 |
			uint64(dirBuf[off+5])<<24 |
			uint64(dirBuf[off+6])<<32 |
			uint64(dirBuf[off+7])<<40
		length := binary.LittleEndian.Uint32(dirBuf[off+8 : off+12])
		if typ == sectionHGTManifest {
			sectionOffset = int64(rawOff)
			sectionLength = int(length)
			found = true
			break
		}
	}
	if !found {
		return nil, nil // pre-manifest tile
	}

	// Read only the section-27 payload using ReadAt to avoid a seek+read
	// sequence that would require the file position to be advanced past
	// potentially gigabytes of routing data.
	sectionData := make([]byte, sectionLength)
	if _, err := f.ReadAt(sectionData, sectionOffset); err != nil {
		return nil, fmt.Errorf("section 27 read (offset=%d length=%d): %w",
			sectionOffset, sectionLength, err)
	}

	return decodeRawHGTManifest(sectionData)
}

// extractHGTManifest reads section 27 from raw tile bytes and decodes it.
// Returns (nil, nil) when the section is absent (pre-manifest tile).
// Returns an error on malformed data.
//
// Kept for use in tests that build in-memory tile blobs.
func extractHGTManifest(data []byte) (*rawHGTManifest, error) {
	if len(data) < tileHeaderSize+4 {
		return nil, fmt.Errorf("tile too small (%d bytes)", len(data))
	}
	if string(data[0:4]) != tileGraphMagic {
		return nil, fmt.Errorf("invalid magic %q", data[0:4])
	}

	sectionCnt := int(binary.LittleEndian.Uint16(data[60:62]))
	dirStart := tileHeaderSize
	requiredDir := dirStart + sectionCnt*12
	if requiredDir > len(data) {
		return nil, fmt.Errorf("section directory overflows file")
	}

	// Locate section 27.
	var sectionOffset int
	var sectionLength int
	found := false
	for i := 0; i < sectionCnt; i++ {
		off := dirStart + i*12
		typ := binary.LittleEndian.Uint16(data[off : off+2])
		offset := uint64(data[off+2]) |
			uint64(data[off+3])<<8 |
			uint64(data[off+4])<<16 |
			uint64(data[off+5])<<24 |
			uint64(data[off+6])<<32 |
			uint64(data[off+7])<<40
		length := binary.LittleEndian.Uint32(data[off+8 : off+12])
		if typ == sectionHGTManifest {
			sectionOffset = int(offset)
			sectionLength = int(length)
			found = true
			break
		}
	}
	if !found {
		return nil, nil // pre-manifest tile
	}

	end := sectionOffset + sectionLength
	if sectionOffset < 0 || end > len(data) {
		return nil, fmt.Errorf("section 27 out of bounds (offset=%d length=%d filesize=%d)",
			sectionOffset, sectionLength, len(data))
	}
	sectionData := data[sectionOffset:end]

	return decodeRawHGTManifest(sectionData)
}

// decodeRawHGTManifest decodes the section 27 payload (same format as
// graph.DecodeHGTManifest but without importing the graph package).
func decodeRawHGTManifest(buf []byte) (*rawHGTManifest, error) {
	if len(buf) == 0 {
		return &rawHGTManifest{}, nil
	}
	count, n := binary.Uvarint(buf)
	if n <= 0 {
		return nil, fmt.Errorf("malformed count varint")
	}
	pos := n
	if count == 0 {
		return &rawHGTManifest{}, nil
	}

	cells := make([]hgtManifestEntry, 0, count)
	for i := uint64(0); i < count; i++ {
		nameLen, n2 := binary.Uvarint(buf[pos:])
		if n2 <= 0 {
			return nil, fmt.Errorf("truncated name length at cell %d", i)
		}
		pos += n2
		if pos+int(nameLen) > len(buf) {
			return nil, fmt.Errorf("truncated cell name at cell %d", i)
		}
		name := string(buf[pos : pos+int(nameLen)])
		pos += int(nameLen)
		if pos+16 > len(buf) {
			return nil, fmt.Errorf("truncated hash fields at cell %d", i)
		}
		fileSize := int64(binary.LittleEndian.Uint64(buf[pos:]))
		pos += 8
		xxh := binary.LittleEndian.Uint64(buf[pos:])
		pos += 8
		cells = append(cells, hgtManifestEntry{
			cellName: name,
			fileSize: fileSize,
			xxhash3:  xxh,
		})
	}
	return &rawHGTManifest{cells: cells}, nil
}
