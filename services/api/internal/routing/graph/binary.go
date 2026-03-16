package graph

import (
	"encoding/binary"
	"fmt"
	"hash/crc32"
	"math"
	"os"
	"path/filepath"
	"time"
)

var le = binary.LittleEndian
var castagnoliTable = crc32.MakeTable(crc32.Castagnoli)

type sectionEntry struct {
	Type   uint16
	Offset uint64
	Length uint32
}

// Serialize writes the graph to a binary file with section directory and CRC32-C.
// Uses atomic write (tmp file + rename) to prevent corrupt files on crash.
func Serialize(g *Graph, path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", filepath.Dir(path), err)
	}
	tmpPath := path + ".tmp"
	f, err := os.Create(tmpPath)
	if err != nil {
		return fmt.Errorf("create %s: %w", tmpPath, err)
	}
	defer os.Remove(tmpPath) // clean up on error
	defer f.Close()

	type sectionData struct {
		typ  uint16
		data []byte
	}
	var sections []sectionData

	sections = append(sections, sectionData{SectionOffsets, uint32SliceToBytes(g.Offsets)})
	sections = append(sections, sectionData{SectionTargets, uint32SliceToBytes(g.Targets)})
	sections = append(sections, sectionData{SectionEdgeDist, uint32SliceToBytes(g.EdgeDist)})
	sections = append(sections, sectionData{SectionEdgeTags1, uint64SliceToBytes(g.EdgeTags1)})
	sections = append(sections, sectionData{SectionEdgeTags2, uint64SliceToBytes(g.EdgeTags2)})
	sections = append(sections, sectionData{SectionEdgeOSMIdx, uint32SliceToBytes(g.EdgeOSMIdx)})
	sections = append(sections, sectionData{SectionOSMIDPool, encodeOSMIDPool(g.OSMIDPool)})
	sections = append(sections, sectionData{SectionNodeLat, int32SliceToBytes(g.NodeLat)})
	sections = append(sections, sectionData{SectionNodeLon, int32SliceToBytes(g.NodeLon)})
	sections = append(sections, sectionData{SectionNodeTags, g.NodeTags})
	sections = append(sections, sectionData{SectionNodeEle, deltaEncodeElevation(g.NodeEle)})
	sections = append(sections, sectionData{SectionSpatialGrid, encodeSpatialGrid(g.Grid)})

	if len(g.EdgeGeomOff) > 0 {
		sections = append(sections, sectionData{SectionEdgeGeomOff, uint32SliceToBytes(g.EdgeGeomOff)})
	}
	if len(g.EdgeGeomLat) > 0 {
		sections = append(sections, sectionData{SectionEdgeGeomLat, int32SliceToBytes(g.EdgeGeomLat)})
	}
	if len(g.EdgeGeomLon) > 0 {
		sections = append(sections, sectionData{SectionEdgeGeomLon, int32SliceToBytes(g.EdgeGeomLon)})
	}
	if len(g.EdgeElevOff) > 0 {
		sections = append(sections, sectionData{SectionEdgeElevOff, uint32SliceToBytes(g.EdgeElevOff)})
	}
	if len(g.EdgeElevProf) > 0 {
		sections = append(sections, sectionData{SectionEdgeElevProf, int32SliceToBytes(g.EdgeElevProf)})
	}

	sectionCnt := uint16(len(sections))
	dirSize := int(sectionCnt) * 12

	dataStart := uint64(HeaderSize + dirSize)
	offset := dataStart
	entries := make([]sectionEntry, len(sections))
	for i, s := range sections {
		entries[i] = sectionEntry{
			Type:   s.typ,
			Offset: offset,
			Length: uint32(len(s.data)),
		}
		offset += uint64(len(s.data))
	}

	header := make([]byte, HeaderSize)
	copy(header[0:4], Magic)
	le.PutUint32(header[4:8], FormatVersion)
	le.PutUint32(header[8:12], g.NodeCount)
	le.PutUint32(header[12:16], g.EdgeCount)
	le.PutUint32(header[16:20], uint32(len(g.OSMIDPool)))
	le.PutUint64(header[20:28], math.Float64bits(g.BBox[0]))
	le.PutUint64(header[28:36], math.Float64bits(g.BBox[1]))
	le.PutUint64(header[36:44], math.Float64bits(g.BBox[2]))
	le.PutUint64(header[44:52], math.Float64bits(g.BBox[3]))
	bt := g.BuildTime
	if bt.IsZero() {
		bt = time.Now()
	}
	le.PutUint64(header[52:60], uint64(bt.Unix()))
	le.PutUint16(header[60:62], sectionCnt)

	crcHash := crc32.New(castagnoliTable)

	if _, err := f.Write(header); err != nil {
		return err
	}
	crcHash.Write(header)

	dirBuf := make([]byte, dirSize)
	for i, e := range entries {
		off := i * 12
		le.PutUint16(dirBuf[off:off+2], e.Type)
		dirBuf[off+2] = byte(e.Offset)
		dirBuf[off+3] = byte(e.Offset >> 8)
		dirBuf[off+4] = byte(e.Offset >> 16)
		dirBuf[off+5] = byte(e.Offset >> 24)
		dirBuf[off+6] = byte(e.Offset >> 32)
		dirBuf[off+7] = byte(e.Offset >> 40)
		le.PutUint32(dirBuf[off+8:off+12], e.Length)
	}
	if _, err := f.Write(dirBuf); err != nil {
		return err
	}
	crcHash.Write(dirBuf)

	for _, s := range sections {
		if _, err := f.Write(s.data); err != nil {
			return err
		}
		crcHash.Write(s.data)
	}

	var crcBuf [4]byte
	le.PutUint32(crcBuf[:], crcHash.Sum32())
	if _, err := f.Write(crcBuf[:]); err != nil {
		return err
	}

	if err := f.Close(); err != nil {
		return fmt.Errorf("close %s: %w", tmpPath, err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("rename %s -> %s: %w", tmpPath, path, err)
	}
	return nil
}

// Deserialize loads a graph from a binary file.
func Deserialize(path string) (*Graph, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}

	if len(data) < HeaderSize+4 {
		return nil, fmt.Errorf("file too small: %d bytes", len(data))
	}

	payload := data[:len(data)-4]
	storedCRC := le.Uint32(data[len(data)-4:])
	computedCRC := crc32.Checksum(payload, castagnoliTable)
	if storedCRC != computedCRC {
		return nil, fmt.Errorf("CRC mismatch: stored %08x, computed %08x", storedCRC, computedCRC)
	}

	if string(data[0:4]) != Magic {
		return nil, fmt.Errorf("invalid magic: %q", data[0:4])
	}
	version := le.Uint32(data[4:8])
	if version != FormatVersion {
		return nil, fmt.Errorf("unsupported version: %d (expected %d)", version, FormatVersion)
	}

	g := &Graph{
		NodeCount: le.Uint32(data[8:12]),
		EdgeCount: le.Uint32(data[12:16]),
		Version:   version,
	}
	poolSize := le.Uint32(data[16:20])
	g.BBox[0] = math.Float64frombits(le.Uint64(data[20:28]))
	g.BBox[1] = math.Float64frombits(le.Uint64(data[28:36]))
	g.BBox[2] = math.Float64frombits(le.Uint64(data[36:44]))
	g.BBox[3] = math.Float64frombits(le.Uint64(data[44:52]))
	g.BuildTime = time.Unix(int64(le.Uint64(data[52:60])), 0)
	sectionCnt := le.Uint16(data[60:62])

	dirStart := HeaderSize
	entries := make([]sectionEntry, sectionCnt)
	for i := 0; i < int(sectionCnt); i++ {
		off := dirStart + i*12
		entries[i].Type = le.Uint16(data[off : off+2])
		entries[i].Offset = uint64(data[off+2]) |
			uint64(data[off+3])<<8 |
			uint64(data[off+4])<<16 |
			uint64(data[off+5])<<24 |
			uint64(data[off+6])<<32 |
			uint64(data[off+7])<<40
		entries[i].Length = le.Uint32(data[off+8 : off+12])
	}

	findSection := func(typ uint16) ([]byte, bool) {
		for _, e := range entries {
			if e.Type == typ {
				start := int(e.Offset)
				end := start + int(e.Length)
				if start < 0 || start > len(payload) || end < start || end > len(payload) {
					return nil, false
				}
				return data[start:end], true
			}
		}
		return nil, false
	}

	if d, ok := findSection(SectionOffsets); ok {
		g.Offsets = bytesToUint32Slice(d)
	}
	if d, ok := findSection(SectionTargets); ok {
		g.Targets = bytesToUint32Slice(d)
	}
	if d, ok := findSection(SectionEdgeDist); ok {
		g.EdgeDist = bytesToUint32Slice(d)
	}
	if d, ok := findSection(SectionEdgeTags1); ok {
		g.EdgeTags1 = bytesToUint64Slice(d)
	}
	if d, ok := findSection(SectionEdgeTags2); ok {
		g.EdgeTags2 = bytesToUint64Slice(d)
	}
	if d, ok := findSection(SectionEdgeOSMIdx); ok {
		g.EdgeOSMIdx = bytesToUint32Slice(d)
	}
	if d, ok := findSection(SectionOSMIDPool); ok {
		g.OSMIDPool = decodeOSMIDPool(d, poolSize)
	}
	if d, ok := findSection(SectionNodeLat); ok {
		g.NodeLat = bytesToInt32Slice(d)
	}
	if d, ok := findSection(SectionNodeLon); ok {
		g.NodeLon = bytesToInt32Slice(d)
	}
	if d, ok := findSection(SectionNodeTags); ok {
		g.NodeTags = make([]uint8, len(d))
		copy(g.NodeTags, d)
	}
	if d, ok := findSection(SectionNodeEle); ok {
		g.NodeEle = deltaDecodeElevation(d, int(g.NodeCount))
	}
	if d, ok := findSection(SectionSpatialGrid); ok {
		g.Grid = decodeSpatialGrid(d)
	}
	if d, ok := findSection(SectionEdgeGeomOff); ok {
		g.EdgeGeomOff = bytesToUint32Slice(d)
	}
	if d, ok := findSection(SectionEdgeGeomLat); ok {
		g.EdgeGeomLat = bytesToInt32Slice(d)
	}
	if d, ok := findSection(SectionEdgeGeomLon); ok {
		g.EdgeGeomLon = bytesToInt32Slice(d)
	}
	if d, ok := findSection(SectionEdgeElevOff); ok {
		g.EdgeElevOff = bytesToUint32Slice(d)
	}
	if d, ok := findSection(SectionEdgeElevProf); ok {
		g.EdgeElevProf = bytesToInt32Slice(d)
	}

	return g, nil
}

// --- Delta encoding for elevation ---

func deltaEncodeElevation(elevations []int32) []byte {
	n := len(elevations)
	if n == 0 {
		buf := make([]byte, 8)
		le.PutUint32(buf[0:4], EleChunkSize)
		le.PutUint32(buf[4:8], 0)
		return buf
	}

	est := 8 + (n/int(EleChunkSize)+2)*(4+4+int(EleChunkSize)*2)
	buf := make([]byte, 0, est)

	buf = le.AppendUint32(buf, EleChunkSize)
	chunkCountPos := len(buf)
	buf = le.AppendUint32(buf, 0)

	chunkCount := uint32(0)
	i := 0
	for i < n {
		chunkStart := len(buf)
		buf = le.AppendUint32(buf, 0)

		buf = appendInt32(buf, elevations[i])
		prev := elevations[i]
		i++
		written := uint32(1)

		for i < n && written < EleChunkSize {
			delta := elevations[i] - prev
			if delta < math.MinInt16 || delta > math.MaxInt16 {
				break
			}
			buf = appendInt16(buf, int16(delta))
			prev = elevations[i]
			i++
			written++
		}

		le.PutUint32(buf[chunkStart:chunkStart+4], written)
		chunkCount++
	}

	le.PutUint32(buf[chunkCountPos:chunkCountPos+4], chunkCount)

	return buf
}

func deltaDecodeElevation(data []byte, nodeCount int) []int32 {
	if len(data) < 8 {
		return make([]int32, nodeCount)
	}

	fileChunkSize := le.Uint32(data[0:4])
	if fileChunkSize != EleChunkSize {
		// Format mismatch: file was written with a different chunk size.
		// Return zeros rather than silently producing wrong elevations.
		return make([]int32, nodeCount)
	}
	chunkCount := int(le.Uint32(data[4:8]))
	if chunkCount == 0 {
		return make([]int32, nodeCount)
	}

	result := make([]int32, nodeCount)
	pos := 8
	outIdx := 0

	for c := 0; c < chunkCount && outIdx < nodeCount; c++ {
		if pos+4 > len(data) {
			break
		}
		chunkNodes := int(le.Uint32(data[pos : pos+4]))
		pos += 4

		if pos+4 > len(data) {
			break
		}
		anchor := int32(le.Uint32(data[pos : pos+4]))
		pos += 4
		result[outIdx] = anchor
		outIdx++

		for d := 1; d < chunkNodes && outIdx < nodeCount; d++ {
			if pos+2 > len(data) {
				break
			}
			delta := int16(le.Uint16(data[pos : pos+2]))
			pos += 2
			result[outIdx] = result[outIdx-1] + int32(delta)
			outIdx++
		}
	}

	return result
}

// --- Spatial grid encoding ---

func encodeSpatialGrid(grid *SpatialGrid) []byte {
	if grid == nil {
		return nil
	}
	size := 32 + (grid.Rows*grid.Cols+1)*4 + len(grid.NodeOrder)*4
	buf := make([]byte, 0, size)
	buf = appendFloat64(buf, grid.CellSize)
	buf = appendFloat64(buf, grid.MinLat)
	buf = appendFloat64(buf, grid.MinLon)
	buf = le.AppendUint32(buf, uint32(grid.Cols))
	buf = le.AppendUint32(buf, uint32(grid.Rows))
	for _, v := range grid.CellStart {
		buf = le.AppendUint32(buf, v)
	}
	for _, v := range grid.NodeOrder {
		buf = le.AppendUint32(buf, v)
	}
	return buf
}

func decodeSpatialGrid(data []byte) *SpatialGrid {
	if len(data) < 32 {
		return nil
	}
	grid := &SpatialGrid{
		CellSize: math.Float64frombits(le.Uint64(data[0:8])),
		MinLat:   math.Float64frombits(le.Uint64(data[8:16])),
		MinLon:   math.Float64frombits(le.Uint64(data[16:24])),
		Cols:     int(le.Uint32(data[24:28])),
		Rows:     int(le.Uint32(data[28:32])),
	}
	pos := 32
	cellCount := grid.Rows*grid.Cols + 1
	grid.CellStart = make([]uint32, cellCount)
	for i := 0; i < cellCount && pos+4 <= len(data); i++ {
		grid.CellStart[i] = le.Uint32(data[pos : pos+4])
		pos += 4
	}
	remaining := (len(data) - pos) / 4
	grid.NodeOrder = make([]uint32, remaining)
	for i := range remaining {
		grid.NodeOrder[i] = le.Uint32(data[pos : pos+4])
		pos += 4
	}
	return grid
}

// --- OSM ID Pool encoding ---

func encodeOSMIDPool(pool []int64) []byte {
	buf := make([]byte, 0, 4+len(pool)*8)
	buf = le.AppendUint32(buf, uint32(len(pool)))
	for _, id := range pool {
		buf = appendInt64(buf, id)
	}
	return buf
}

func decodeOSMIDPool(data []byte, size uint32) []int64 {
	if len(data) < 4 {
		return nil
	}
	pool := make([]int64, size)
	pos := 4
	for i := uint32(0); i < size && pos+8 <= len(data); i++ {
		pool[i] = int64(le.Uint64(data[pos : pos+8]))
		pos += 8
	}
	return pool
}

// --- Byte conversion helpers ---

func uint32SliceToBytes(s []uint32) []byte {
	buf := make([]byte, len(s)*4)
	for i, v := range s {
		le.PutUint32(buf[i*4:], v)
	}
	return buf
}

func uint16SliceToBytes(s []uint16) []byte {
	buf := make([]byte, len(s)*2)
	for i, v := range s {
		le.PutUint16(buf[i*2:], v)
	}
	return buf
}

func uint64SliceToBytes(s []uint64) []byte {
	buf := make([]byte, len(s)*8)
	for i, v := range s {
		le.PutUint64(buf[i*8:], v)
	}
	return buf
}

func int32SliceToBytes(s []int32) []byte {
	buf := make([]byte, len(s)*4)
	for i, v := range s {
		le.PutUint32(buf[i*4:], uint32(v))
	}
	return buf
}

func bytesToUint32Slice(data []byte) []uint32 {
	n := len(data) / 4
	s := make([]uint32, n)
	for i := range n {
		s[i] = le.Uint32(data[i*4:])
	}
	return s
}

func bytesToUint16Slice(data []byte) []uint16 {
	n := len(data) / 2
	s := make([]uint16, n)
	for i := range n {
		s[i] = le.Uint16(data[i*2:])
	}
	return s
}

func bytesToUint64Slice(data []byte) []uint64 {
	n := len(data) / 8
	s := make([]uint64, n)
	for i := range n {
		s[i] = le.Uint64(data[i*8:])
	}
	return s
}

func bytesToInt32Slice(data []byte) []int32 {
	n := len(data) / 4
	s := make([]int32, n)
	for i := range n {
		s[i] = int32(le.Uint32(data[i*4:]))
	}
	return s
}

func appendInt32(buf []byte, v int32) []byte {
	return le.AppendUint32(buf, uint32(v))
}

func appendInt16(buf []byte, v int16) []byte {
	return le.AppendUint16(buf, uint16(v))
}

func appendInt64(buf []byte, v int64) []byte {
	return le.AppendUint64(buf, uint64(v))
}

func appendFloat64(buf []byte, v float64) []byte {
	return le.AppendUint64(buf, math.Float64bits(v))
}
