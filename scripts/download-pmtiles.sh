#!/usr/bin/env bash
# Download Protomaps PMTiles Region-Extract
# Usage: ./scripts/download-pmtiles.sh [bbox] [output]
# Default: DACH-Region (DE+AT+CH)
#
# Protomaps bietet kostenlose tägliche Builds des gesamten Planeten als PMTiles.
# Region-Extracts über das pmtiles CLI-Tool.
#
# Kein sac_scale im Protomaps-Schema — das kommt aus osm_ways via Martin MVT Overlay.
#
# Voraussetzung: pmtiles CLI
#   go install github.com/protomaps/go-pmtiles/cmd/pmtiles@latest
# oder:
#   npm install -g pmtiles
#
set -euo pipefail

# DACH bounding box (DE+AT+CH)
BBOX="${1:-5.87,45.82,17.17,55.06}"
OUTPUT="${2:-data/tiles/basemap.pmtiles}"
# Aktuelles Datum für Daily Build
DATE=$(date +%Y%m%d)
SOURCE="https://build.protomaps.com/${DATE}.pmtiles"

mkdir -p "$(dirname "$OUTPUT")"

echo "Extracting PMTiles for bbox=$BBOX from $SOURCE..."
echo "Output: $OUTPUT"
echo ""

if command -v pmtiles &>/dev/null; then
    pmtiles extract "$SOURCE" "$OUTPUT" \
        --bbox="$BBOX" \
        --maxzoom=15 \
        --download-threads=4
    echo ""
    echo "Done. File size: $(du -h "$OUTPUT" | cut -f1)"
    echo "Martin erkennt die Datei automatisch aus data/tiles/"
else
    echo "Error: pmtiles CLI nicht gefunden."
    echo "Installation: go install github.com/protomaps/go-pmtiles/cmd/pmtiles@latest"
    echo "Oder: npm install -g pmtiles"
    exit 1
fi
