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

// LandmarkDists returns precomputed distances from a local node to all 16 landmarks.
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
