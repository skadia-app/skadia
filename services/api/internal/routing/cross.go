package routing

import "math"

// MakeGlobalNodeID encodes a tile ID and local node ID into a single uint64.
func MakeGlobalNodeID(tileID uint32, localNodeID uint32) uint64 {
	return uint64(tileID)<<32 | uint64(localNodeID)
}

// SplitGlobalNodeID decodes a global node ID into tile ID and local node ID.
func SplitGlobalNodeID(gid uint64) (tileID uint32, localNodeID uint32) {
	return uint32(gid >> 32), uint32(gid)
}

// equirectDistM returns approximate distance in meters using equirectangular projection.
func equirectDistM(lat1, lon1, lat2, lon2 float64) float64 {
	const degToRad = math.Pi / 180.0
	const earthRadius = 6371000.0
	dLat := (lat2 - lat1) * degToRad
	dLon := (lon2 - lon1) * degToRad * math.Cos((lat1+lat2)/2*degToRad)
	return math.Sqrt(dLat*dLat+dLon*dLon) * earthRadius
}
