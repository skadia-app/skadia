package routing

import "github.com/skadia-app/skadia/services/api/internal/routing/graph"

// MakeGlobalNodeID encodes a tile ID and local node ID into a single uint64.
func MakeGlobalNodeID(tileID uint32, localNodeID uint32) uint64 {
	return graph.MakeGlobalNodeID(tileID, localNodeID)
}

// SplitGlobalNodeID decodes a global node ID into tile ID and local node ID.
func SplitGlobalNodeID(gid uint64) (tileID uint32, localNodeID uint32) {
	return graph.SplitGlobalNodeID(gid)
}
