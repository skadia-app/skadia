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
func (s *Service) scanTile(tilePath string) error {
	data, err := os.ReadFile(tilePath)
	if err != nil {
		return fmt.Errorf("read tile: %w", err)
	}

	manifest, err := extractHGTManifest(data)
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

// xxh3File computes the xxhash3 digest of the file at path.
func xxh3File(path string) (uint64, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	h := xxh3.New()
	if _, err := io.Copy(h, f); err != nil {
		return 0, err
	}
	return h.Sum64(), nil
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

// extractHGTManifest reads section 27 from raw tile bytes and decodes it.
// Returns (nil, nil) when the section is absent (pre-manifest tile).
// Returns an error on malformed data.
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
