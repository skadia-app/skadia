package coverage

import (
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"syscall"
)

const (
	fcovHeaderSize = 32
	fcovVersion    = 1
)

var fcovMagic = [4]byte{'F', 'C', 'O', 'V'}

// FCOVHeader is the 32-byte fixed header for FCOV binary files.
type FCOVHeader struct {
	Magic   [4]byte
	Version uint16
	ResM    uint16
	Rows    uint32
	Cols    uint32
	MinLat  float32
	MinLon  float32
	MaxLat  float32
	MaxLon  float32
}

// WriteFCOV writes header + data to path. Creates parent directories.
func WriteFCOV(path string, h FCOVHeader, data []uint8) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	if err := binary.Write(f, binary.LittleEndian, &h); err != nil {
		return fmt.Errorf("fcov: write header: %w", err)
	}
	if _, err := f.Write(data); err != nil {
		return fmt.Errorf("fcov: write data: %w", err)
	}
	return nil
}

// ReadFCOV reads header + data from path (regular file I/O, no mmap).
func ReadFCOV(path string) (FCOVHeader, []uint8, error) {
	f, err := os.Open(path)
	if err != nil {
		return FCOVHeader{}, nil, err
	}
	defer f.Close()

	var h FCOVHeader
	if err := binary.Read(f, binary.LittleEndian, &h); err != nil {
		return FCOVHeader{}, nil, fmt.Errorf("fcov: read header: %w", err)
	}
	if h.Magic != fcovMagic {
		return FCOVHeader{}, nil, fmt.Errorf("fcov: invalid magic %q", h.Magic)
	}
	if h.Version != fcovVersion {
		return FCOVHeader{}, nil, fmt.Errorf("fcov: unsupported version %d", h.Version)
	}

	size := int(h.Rows) * int(h.Cols)
	info, statErr := f.Stat()
	if statErr != nil {
		return FCOVHeader{}, nil, fmt.Errorf("fcov: stat: %w", statErr)
	}
	fileDataSize := info.Size() - fcovHeaderSize
	if int64(size) > fileDataSize {
		return FCOVHeader{}, nil, fmt.Errorf("fcov: header claims %d bytes but file has %d data bytes", size, fileDataSize)
	}
	data := make([]uint8, size)
	if _, err := io.ReadFull(f, data); err != nil {
		return FCOVHeader{}, nil, fmt.Errorf("fcov: read data: %w", err)
	}
	return h, data, nil
}

// MmapFCOV opens a FCOV file and returns the header, grid data slice, and a
// cleanup function. Call cleanup() when done (munmap + close).
func MmapFCOV(path string) (FCOVHeader, []byte, func(), error) {
	f, err := os.Open(path)
	if err != nil {
		return FCOVHeader{}, nil, nil, err
	}

	info, err := f.Stat()
	if err != nil {
		f.Close()
		return FCOVHeader{}, nil, nil, err
	}

	fullData, err := syscall.Mmap(int(f.Fd()), 0, int(info.Size()),
		syscall.PROT_READ, syscall.MAP_SHARED)
	if err != nil {
		f.Close()
		return FCOVHeader{}, nil, nil, fmt.Errorf("fcov: mmap %s: %w", path, err)
	}

	cleanup := func() {
		syscall.Munmap(fullData)
		f.Close()
	}

	if len(fullData) < fcovHeaderSize {
		cleanup()
		return FCOVHeader{}, nil, nil, fmt.Errorf("fcov: file too small: %d bytes", len(fullData))
	}

	var h FCOVHeader
	h.Magic = [4]byte(fullData[0:4])
	h.Version = binary.LittleEndian.Uint16(fullData[4:6])
	h.ResM = binary.LittleEndian.Uint16(fullData[6:8])
	h.Rows = binary.LittleEndian.Uint32(fullData[8:12])
	h.Cols = binary.LittleEndian.Uint32(fullData[12:16])
	h.MinLat = math.Float32frombits(binary.LittleEndian.Uint32(fullData[16:20]))
	h.MinLon = math.Float32frombits(binary.LittleEndian.Uint32(fullData[20:24]))
	h.MaxLat = math.Float32frombits(binary.LittleEndian.Uint32(fullData[24:28]))
	h.MaxLon = math.Float32frombits(binary.LittleEndian.Uint32(fullData[28:32]))

	if h.Magic != fcovMagic {
		cleanup()
		return FCOVHeader{}, nil, nil, fmt.Errorf("fcov: invalid magic %q", h.Magic)
	}
	if h.Version != fcovVersion {
		cleanup()
		return FCOVHeader{}, nil, nil, fmt.Errorf("fcov: unsupported version %d", h.Version)
	}

	expectedSize := fcovHeaderSize + int(h.Rows)*int(h.Cols)
	if len(fullData) < expectedSize {
		cleanup()
		return FCOVHeader{}, nil, nil, fmt.Errorf("fcov: truncated file: have %d bytes, need %d", len(fullData), expectedSize)
	}

	return h, fullData[fcovHeaderSize:], cleanup, nil
}
