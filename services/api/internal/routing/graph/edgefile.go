package graph

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"io"
	"os"
)

const (
	edgeFileMagic          = "FRTM"
	edgeFileVersion uint32 = 1
	edgeFileBufSize        = 4 * 1024 * 1024
)

// WaySegment represents one directed segment of an OSM way between two graph nodes.
type WaySegment struct {
	FromID  uint32
	ToID    uint32
	Dist    uint32
	GeomLat []int32
	GeomLon []int32
}

// EdgeFileWriter writes per-way edge records to a binary temp file.
// Header WayCount and NodeCount are patched on CloseWithNodeCount.
type EdgeFileWriter struct {
	f    *os.File
	w    *bufio.Writer
	buf  []byte
	ways uint32
}

// EdgeFileReader reads per-way edge records from a binary temp file.
type EdgeFileReader struct {
	f         *os.File
	r         *bufio.Reader
	buf       []byte
	wayCount  uint32
	nodeCount uint32
}

// NewEdgeFileWriter creates the file at path and writes a placeholder 16-byte header.
func NewEdgeFileWriter(path string) (*EdgeFileWriter, error) {
	f, err := os.Create(path)
	if err != nil {
		return nil, fmt.Errorf("edgefile: create %s: %w", path, err)
	}
	ew := &EdgeFileWriter{
		f:   f,
		w:   bufio.NewWriterSize(f, edgeFileBufSize),
		buf: make([]byte, 28), // largest fixed header used in WriteWay (way header)
	}
	// Write placeholder header (16 bytes): magic(4) + version(4) + wayCount(4) + nodeCount(4)
	copy(ew.buf[:4], edgeFileMagic)
	binary.LittleEndian.PutUint32(ew.buf[4:8], edgeFileVersion)
	binary.LittleEndian.PutUint32(ew.buf[8:12], 0)  // wayCount — patched on close
	binary.LittleEndian.PutUint32(ew.buf[12:16], 0) // nodeCount — patched on close
	if _, err := ew.w.Write(ew.buf[:16]); err != nil {
		f.Close()
		return nil, fmt.Errorf("edgefile: write header: %w", err)
	}
	return ew, nil
}

// WriteWay encodes one way record and appends it to the file.
//
// Way header (28 bytes):
//
//	OSMID     int64   (8)
//	Tags1     uint64  (8)
//	Tags2     uint64  (8)
//	Flags     uint8   (1) — bit 0: isOneway
//	_pad1     byte    (1)
//	SegCount  uint16  (2)
//
// Per segment (16 bytes + geometry):
//
//	FromID    uint32  (4)
//	ToID      uint32  (4)
//	Dist      uint32  (4)
//	GeomCount uint16  (2)
//	_pad2     [2]byte (2)
//	Lat[GeomCount] int32
//	Lon[GeomCount] int32
func (ew *EdgeFileWriter) WriteWay(osmID int64, tags1, tags2 uint64, isOneway bool, segments []WaySegment) error {
	// Encode way header into buf (28 bytes).
	binary.LittleEndian.PutUint64(ew.buf[0:8], uint64(osmID))
	binary.LittleEndian.PutUint64(ew.buf[8:16], tags1)
	binary.LittleEndian.PutUint64(ew.buf[16:24], tags2)
	var flags uint8
	if isOneway {
		flags |= 1
	}
	ew.buf[24] = flags
	ew.buf[25] = 0 // pad
	binary.LittleEndian.PutUint16(ew.buf[26:28], uint16(len(segments)))
	if _, err := ew.w.Write(ew.buf[:28]); err != nil {
		return fmt.Errorf("edgefile: write way header: %w", err)
	}

	// Encode each segment.
	segHdr := make([]byte, 16)
	for i := range segments {
		seg := &segments[i]
		binary.LittleEndian.PutUint32(segHdr[0:4], seg.FromID)
		binary.LittleEndian.PutUint32(segHdr[4:8], seg.ToID)
		binary.LittleEndian.PutUint32(segHdr[8:12], seg.Dist)
		geomCount := uint16(len(seg.GeomLat))
		binary.LittleEndian.PutUint16(segHdr[12:14], geomCount)
		segHdr[14] = 0 // pad
		segHdr[15] = 0 // pad
		if _, err := ew.w.Write(segHdr); err != nil {
			return fmt.Errorf("edgefile: write seg header: %w", err)
		}
		// Write Lat then Lon arrays (4 bytes each point).
		pt := make([]byte, 4)
		for _, v := range seg.GeomLat {
			binary.LittleEndian.PutUint32(pt, uint32(v))
			if _, err := ew.w.Write(pt); err != nil {
				return fmt.Errorf("edgefile: write geom lat: %w", err)
			}
		}
		for _, v := range seg.GeomLon {
			binary.LittleEndian.PutUint32(pt, uint32(v))
			if _, err := ew.w.Write(pt); err != nil {
				return fmt.Errorf("edgefile: write geom lon: %w", err)
			}
		}
	}

	ew.ways++
	return nil
}

// CloseWithNodeCount flushes the writer, patches header offsets 8–15 with
// wayCount and nodeCount, then closes the underlying file.
func (ew *EdgeFileWriter) CloseWithNodeCount(nodeCount uint32) error {
	if err := ew.w.Flush(); err != nil {
		ew.f.Close()
		return fmt.Errorf("edgefile: flush: %w", err)
	}
	// Patch header: wayCount at offset 8, nodeCount at offset 12.
	if _, err := ew.f.Seek(8, io.SeekStart); err != nil {
		ew.f.Close()
		return fmt.Errorf("edgefile: seek for patch: %w", err)
	}
	patch := make([]byte, 8)
	binary.LittleEndian.PutUint32(patch[0:4], ew.ways)
	binary.LittleEndian.PutUint32(patch[4:8], nodeCount)
	if _, err := ew.f.Write(patch); err != nil {
		ew.f.Close()
		return fmt.Errorf("edgefile: patch header: %w", err)
	}
	return ew.f.Close()
}

// NewEdgeFileReader opens the file at path and validates the 16-byte header.
func NewEdgeFileReader(path string) (*EdgeFileReader, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("edgefile: open %s: %w", path, err)
	}
	er := &EdgeFileReader{
		f:   f,
		r:   bufio.NewReaderSize(f, edgeFileBufSize),
		buf: make([]byte, 28),
	}
	// Read and validate header (16 bytes).
	if _, err := io.ReadFull(er.r, er.buf[:16]); err != nil {
		f.Close()
		return nil, fmt.Errorf("edgefile: read header: %w", err)
	}
	if string(er.buf[:4]) != edgeFileMagic {
		f.Close()
		return nil, fmt.Errorf("edgefile: bad magic %q", er.buf[:4])
	}
	version := binary.LittleEndian.Uint32(er.buf[4:8])
	if version != edgeFileVersion {
		f.Close()
		return nil, fmt.Errorf("edgefile: unsupported version %d", version)
	}
	er.wayCount = binary.LittleEndian.Uint32(er.buf[8:12])
	er.nodeCount = binary.LittleEndian.Uint32(er.buf[12:16])
	return er, nil
}

// WayCount returns the number of way records in the file.
func (er *EdgeFileReader) WayCount() uint32 { return er.wayCount }

// NodeCount returns the node count stored in the header.
func (er *EdgeFileReader) NodeCount() uint32 { return er.nodeCount }

// ReadWay reads the next way record. Returns io.EOF when all ways have been read.
func (er *EdgeFileReader) ReadWay() (osmID int64, tags1, tags2 uint64, isOneway bool, segments []WaySegment, err error) {
	// Read way header (28 bytes).
	if _, err = io.ReadFull(er.r, er.buf[:28]); err != nil {
		if err == io.EOF || err == io.ErrUnexpectedEOF {
			err = io.EOF
		}
		return
	}
	osmID = int64(binary.LittleEndian.Uint64(er.buf[0:8]))
	tags1 = binary.LittleEndian.Uint64(er.buf[8:16])
	tags2 = binary.LittleEndian.Uint64(er.buf[16:24])
	flags := er.buf[24]
	isOneway = flags&1 != 0
	segCount := binary.LittleEndian.Uint16(er.buf[26:28])

	segments = make([]WaySegment, segCount)
	segHdr := make([]byte, 16)
	pt := make([]byte, 4)
	for i := range segments {
		if _, err = io.ReadFull(er.r, segHdr); err != nil {
			err = fmt.Errorf("edgefile: read seg header: %w", err)
			return
		}
		seg := &segments[i]
		seg.FromID = binary.LittleEndian.Uint32(segHdr[0:4])
		seg.ToID = binary.LittleEndian.Uint32(segHdr[4:8])
		seg.Dist = binary.LittleEndian.Uint32(segHdr[8:12])
		geomCount := int(binary.LittleEndian.Uint16(segHdr[12:14]))
		seg.GeomLat = make([]int32, geomCount)
		seg.GeomLon = make([]int32, geomCount)
		for j := range seg.GeomLat {
			if _, err = io.ReadFull(er.r, pt); err != nil {
				err = fmt.Errorf("edgefile: read geom lat: %w", err)
				return
			}
			seg.GeomLat[j] = int32(binary.LittleEndian.Uint32(pt))
		}
		for j := range seg.GeomLon {
			if _, err = io.ReadFull(er.r, pt); err != nil {
				err = fmt.Errorf("edgefile: read geom lon: %w", err)
				return
			}
			seg.GeomLon[j] = int32(binary.LittleEndian.Uint32(pt))
		}
	}
	return
}

// Reset seeks back to the first way record (offset 16) for a second pass.
func (er *EdgeFileReader) Reset() error {
	if _, err := er.f.Seek(16, io.SeekStart); err != nil {
		return fmt.Errorf("edgefile: reset seek: %w", err)
	}
	er.r.Reset(er.f)
	return nil
}

// Close closes the underlying file.
func (er *EdgeFileReader) Close() error {
	return er.f.Close()
}
