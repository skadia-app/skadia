package routing

// MakeGlobalNodeID encodes a tile ID and local node ID into a single uint64.
func MakeGlobalNodeID(tileID uint32, localNodeID uint32) uint64 {
	return uint64(tileID)<<32 | uint64(localNodeID)
}

// SplitGlobalNodeID decodes a global node ID into tile ID and local node ID.
func SplitGlobalNodeID(gid uint64) (tileID uint32, localNodeID uint32) {
	return uint32(gid >> 32), uint32(gid)
}

