package graph

import (
	"fmt"
	"math"
	"path/filepath"
)

const BorderBuffer = 0.02

func TileIDFromCoord(lat, lon float64) uint32 {
	latFloor := int(math.Floor(lat))
	lonFloor := int(math.Floor(lon))
	return uint32((latFloor+90)*360 + (lonFloor + 180))
}

func TileBBox(tileID uint32) [4]float64 {
	latFloor := int(tileID/360) - 90
	lonFloor := int(tileID%360) - 180
	return [4]float64{float64(latFloor), float64(lonFloor), float64(latFloor + 1), float64(lonFloor + 1)}
}

func TileBBoxExpanded(tileID uint32, buffer float64) [4]float64 {
	bbox := TileBBox(tileID)
	return [4]float64{bbox[0] - buffer, bbox[1] - buffer, bbox[2] + buffer, bbox[3] + buffer}
}

func TileLatLon(tileID uint32) (latFloor, lonFloor int) {
	latFloor = int(tileID/360) - 90
	lonFloor = int(tileID%360) - 180
	return
}

func TileFileName(tileID uint32) string {
	return fmt.Sprintf("tile_%d.bin", tileID)
}

func TileFilePath(outputDir string, tileID uint32) string {
	return filepath.Join(outputDir, fmt.Sprintf("tile_%d.bin", tileID))
}
