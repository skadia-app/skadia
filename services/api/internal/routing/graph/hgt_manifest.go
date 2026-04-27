package graph

import (
	"encoding/binary"
	"fmt"
)

// HGTCellRef records the identity of an HGT file used during tile build.
// FileSize and Xxhash3 together detect silent file replacement (e.g.
// cp --preserve=mtime,size with modified content).
type HGTCellRef struct {
	CellName string // canonical HGT cell name without extension, e.g. "N47E011"
	FileSize int64
	Xxhash3  uint64
}

// HGTManifest is the in-memory representation of SectionHGTManifest (section 27).
// It lists every HGT cell that was read during tile construction.
type HGTManifest struct {
	Cells []HGTCellRef
}

// EncodeHGTManifest serialises a manifest to a compact binary format:
//
//	varint count
//	per cell: varint nameLen | name bytes | int64 fileSize | uint64 xxhash3
func EncodeHGTManifest(m *HGTManifest) []byte {
	if m == nil || len(m.Cells) == 0 {
		return appendUvarint(nil, 0)
	}
	buf := appendUvarint(nil, uint64(len(m.Cells)))
	for _, c := range m.Cells {
		nameBytes := []byte(c.CellName)
		buf = appendUvarint(buf, uint64(len(nameBytes)))
		buf = append(buf, nameBytes...)
		buf = binary.LittleEndian.AppendUint64(buf, uint64(c.FileSize))
		buf = binary.LittleEndian.AppendUint64(buf, c.Xxhash3)
	}
	return buf
}

// DecodeHGTManifest deserialises a manifest produced by EncodeHGTManifest.
// Returns an error on truncated or malformed input.
func DecodeHGTManifest(buf []byte) (*HGTManifest, error) {
	if len(buf) == 0 {
		return &HGTManifest{}, nil
	}
	count, n := binary.Uvarint(buf)
	if n <= 0 {
		return nil, fmt.Errorf("hgt manifest: malformed count varint")
	}
	pos := n
	if count == 0 {
		return &HGTManifest{}, nil
	}

	cells := make([]HGTCellRef, 0, count)
	for i := uint64(0); i < count; i++ {
		nameLen, n2 := binary.Uvarint(buf[pos:])
		if n2 <= 0 {
			return nil, fmt.Errorf("hgt manifest: truncated name length at cell %d", i)
		}
		pos += n2
		if pos+int(nameLen) > len(buf) {
			return nil, fmt.Errorf("hgt manifest: truncated cell name at cell %d", i)
		}
		name := string(buf[pos : pos+int(nameLen)])
		pos += int(nameLen)
		// int64 fileSize (8 bytes) + uint64 xxhash3 (8 bytes)
		if pos+16 > len(buf) {
			return nil, fmt.Errorf("hgt manifest: truncated hash fields at cell %d", i)
		}
		fileSize := int64(binary.LittleEndian.Uint64(buf[pos:]))
		pos += 8
		xxh := binary.LittleEndian.Uint64(buf[pos:])
		pos += 8
		cells = append(cells, HGTCellRef{
			CellName: name,
			FileSize: fileSize,
			Xxhash3:  xxh,
		})
	}
	return &HGTManifest{Cells: cells}, nil
}

// appendUvarint appends a variable-length uint64 to buf (encoding/binary.AppendUvarint
// is available in Go 1.19+; this avoids a dependency on the standard library varint
// encoder from a local helper for clarity).
func appendUvarint(buf []byte, v uint64) []byte {
	for v >= 0x80 {
		buf = append(buf, byte(v)|0x80)
		v >>= 7
	}
	return append(buf, byte(v))
}
