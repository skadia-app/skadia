package graph

import (
	"encoding/binary"
	"fmt"
	"slices"
)

type BorderNodeEntry struct {
	CoordKey    int64
	LocalNodeID uint32
}

type BorderNodeIndex struct {
	Entries []BorderNodeEntry
}

// CoordKeyToInt64 packs a CoordKey into an int64 for compact storage.
// EXPORTED for cross-tile routing (cross.go needs it).
func CoordKeyToInt64(k CoordKey) int64 {
	return int64(k.Lat)<<32 | int64(uint32(k.Lon))
}

func IdentifyBorderNodes(tileID uint32, nodeLat, nodeLon []int32, borderDist float64) []uint32 {
	bbox := TileBBox(tileID)
	bdLat := int32(borderDist * 1e7)
	bdLon := int32(borderDist * 1e7)
	minLat := int32(bbox[0] * 1e7)
	minLon := int32(bbox[1] * 1e7)
	maxLat := int32(bbox[2] * 1e7)
	maxLon := int32(bbox[3] * 1e7)

	var ids []uint32
	for i := range nodeLat {
		lat := nodeLat[i]
		lon := nodeLon[i]
		if lat-minLat < bdLat || maxLat-lat < bdLat ||
			lon-minLon < bdLon || maxLon-lon < bdLon {
			ids = append(ids, uint32(i))
		}
	}
	return ids
}

func BuildBorderNodeIndex(borderNodeIDs []uint32, nodeLat, nodeLon []int32) *BorderNodeIndex {
	entries := make([]BorderNodeEntry, len(borderNodeIDs))
	for i, id := range borderNodeIDs {
		key := CoordKey{Lat: nodeLat[id], Lon: nodeLon[id]}
		entries[i] = BorderNodeEntry{
			CoordKey:    CoordKeyToInt64(key),
			LocalNodeID: id,
		}
	}
	slices.SortFunc(entries, func(a, b BorderNodeEntry) int {
		if a.CoordKey < b.CoordKey {
			return -1
		}
		if a.CoordKey > b.CoordKey {
			return 1
		}
		return 0
	})
	return &BorderNodeIndex{Entries: entries}
}

func (idx *BorderNodeIndex) Lookup(coordKey int64) (uint32, bool) {
	i, found := slices.BinarySearchFunc(idx.Entries, coordKey, func(e BorderNodeEntry, target int64) int {
		if e.CoordKey < target {
			return -1
		}
		if e.CoordKey > target {
			return 1
		}
		return 0
	})
	if !found {
		return 0, false
	}
	return idx.Entries[i].LocalNodeID, true
}

func (idx *BorderNodeIndex) Serialize() []byte {
	buf := make([]byte, 4+len(idx.Entries)*12)
	le.PutUint32(buf[0:4], uint32(len(idx.Entries)))
	for i, e := range idx.Entries {
		off := 4 + i*12
		binary.LittleEndian.PutUint64(buf[off:off+8], uint64(e.CoordKey))
		binary.LittleEndian.PutUint32(buf[off+8:off+12], e.LocalNodeID)
	}
	return buf
}

func DeserializeBorderNodeIndex(data []byte) (*BorderNodeIndex, error) {
	if len(data) < 4 {
		return nil, fmt.Errorf("border node index too short: %d bytes", len(data))
	}
	count := le.Uint32(data[0:4])
	expected := 4 + int(count)*12
	if len(data) < expected {
		return nil, fmt.Errorf("border node index truncated: have %d, need %d", len(data), expected)
	}
	entries := make([]BorderNodeEntry, count)
	for i := range count {
		off := 4 + int(i)*12
		entries[i] = BorderNodeEntry{
			CoordKey:    int64(binary.LittleEndian.Uint64(data[off : off+8])),
			LocalNodeID: binary.LittleEndian.Uint32(data[off+8 : off+12]),
		}
	}
	return &BorderNodeIndex{Entries: entries}, nil
}
