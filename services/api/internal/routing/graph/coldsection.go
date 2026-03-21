package graph

import (
	"encoding/binary"
	"fmt"
	"math"
	"os"
)

// ReadColdFloat32Section reads a float32 slice from a cold (mmap-backed) section.
func ReadColdFloat32Section(f *os.File, entry sectionEntry, offset, count uint32) ([]float32, error) {
	if f == nil {
		return nil, fmt.Errorf("no backing file for cold section")
	}
	byteOff := int64(entry.Offset) + int64(offset)*4
	byteLen := int(count) * 4
	buf := make([]byte, byteLen)
	n, err := f.ReadAt(buf, byteOff)
	if err != nil || n != byteLen {
		return nil, fmt.Errorf("cold section read: got %d/%d bytes: %w", n, byteLen, err)
	}
	result := make([]float32, count)
	for i := range count {
		result[i] = math.Float32frombits(binary.LittleEndian.Uint32(buf[i*4:]))
	}
	return result, nil
}

// ReadColdFloat32SectionInto reads into a pre-allocated dst slice, avoiding allocation.
// dst must have length >= count. buf must have length >= count*4 (grown automatically if not).
// Returns dst[:count].
func ReadColdFloat32SectionInto(f *os.File, entry sectionEntry, offset, count uint32, dst []float32, buf []byte) ([]float32, error) {
	if f == nil {
		return nil, fmt.Errorf("no backing file for cold section")
	}
	byteOff := int64(entry.Offset) + int64(offset)*4
	byteLen := int(count) * 4
	if len(buf) < byteLen {
		buf = make([]byte, byteLen)
	}
	n, err := f.ReadAt(buf[:byteLen], byteOff)
	if err != nil || n != byteLen {
		return nil, fmt.Errorf("cold section read: got %d/%d bytes: %w", n, byteLen, err)
	}
	if len(dst) < int(count) {
		dst = make([]float32, count)
	}
	for i := range count {
		dst[i] = math.Float32frombits(binary.LittleEndian.Uint32(buf[i*4:]))
	}
	return dst[:count], nil
}

// LandmarkDists returns precomputed distances from a local node to all 16 landmarks.
// NOTE: Allocates on every call. For hot-path usage, consider LandmarkDistsInto
// with pre-allocated buffers.
func (g *Graph) LandmarkDists(setID LandmarkSetID, localNodeID uint32) []float32 {
	sectionID := SectionLandmarkDistA + uint16(setID)
	entry, ok := g.coldSections[sectionID]
	if !ok || g.backingFile == nil {
		return nil
	}
	offset := uint32(localNodeID) * LandmarksPerSet
	dists, err := ReadColdFloat32Section(g.backingFile, entry, offset, LandmarksPerSet)
	if err != nil {
		return nil
	}
	return dists
}

// LandmarkDistsInto reads landmark distances into pre-allocated buffers, avoiding allocation.
// dst must have length >= LandmarksPerSet; byteBuf must have length >= LandmarksPerSet*4.
// Both buffers are grown automatically if undersized.
// Returns the filled slice or nil if no data is available.
func (g *Graph) LandmarkDistsInto(setID LandmarkSetID, localNodeID uint32, dst []float32, byteBuf []byte) []float32 {
	sectionID := SectionLandmarkDistA + uint16(setID)
	entry, ok := g.coldSections[sectionID]
	if !ok || g.backingFile == nil {
		return nil
	}
	offset := uint32(localNodeID) * LandmarksPerSet
	dists, err := ReadColdFloat32SectionInto(g.backingFile, entry, offset, LandmarksPerSet, dst, byteBuf)
	if err != nil {
		return nil
	}
	return dists
}
