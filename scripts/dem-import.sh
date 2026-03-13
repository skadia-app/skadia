#!/usr/bin/env bash
# scripts/dem-import.sh
# Importiert GeoTIFF DEM-Dateien in PostGIS Raster (dem.raster_tiles)
#
# Usage:   ./dem-import.sh <source_name> <geotiff_dir> <resolution_m>
# Example: ./dem-import.sh eea10       ./data/dem/eea10/        10
#          ./dem-import.sh copernicus30 ./data/dem/copernicus30/ 30
#          ./dem-import.sh lidar_at     ./data/dem/lidar_at/     1
#
# Voraussetzungen:
#   - raster2pgsql (PostGIS-Tooling) muss im PATH sein
#   - psql muss im PATH sein
#   - DATABASE_URL muss gesetzt sein (oder Default-Wert passt)
#   - Die Migration 000130 muss bereits gelaufen sein (CREATE SCHEMA dem; CREATE TABLE dem.raster_tiles;)
#
# Tile-Größe 256x256 ist ein bewährter Kompromiss für ST_Value-Abfragen.
# -C: fügt constraints hinzu, -I: erstellt GiST-Index nach dem Import.
# Der Index wird pro Datei erstellt (append mode -a); am Ende ANALYZE laufen lassen.

set -euo pipefail

SOURCE="${1:?Usage: $0 <source_name> <geotiff_dir> <resolution_m>}"
DIR="${2:?Usage: $0 <source_name> <geotiff_dir> <resolution_m>}"
RESOLUTION="${3:?Usage: $0 <source_name> <geotiff_dir> <resolution_m>  (e.g. 30, 10, 1)}"
DB_URL="${DATABASE_URL:-postgresql://skadia:skadia@localhost:5432/skadia}"

if [ ! -d "$DIR" ]; then
  echo "Error: directory '$DIR' not found" >&2
  exit 1
fi

COUNT=0

for f in "$DIR"/*.tif "$DIR"/*.tiff "$DIR"/*.TIF "$DIR"/*.TIFF; do
  [ -f "$f" ] || continue

  echo "→ Importing $(basename "$f") (source=$SOURCE)..."
  raster2pgsql \
    -s 4326 \
    -t 256x256 \
    -I \
    -C \
    -a \
    "$f" \
    dem.raster_tiles \
    | PGAPPNAME=dem_import psql "$DB_URL" -q

  # source und resolution_m setzen (raster2pgsql kann keine Custom-Spalten)
  psql "$DB_URL" -q -c \
    "UPDATE dem.raster_tiles SET source = '$SOURCE', resolution_m = $RESOLUTION WHERE source IS NULL;"

  COUNT=$((COUNT + 1))
  echo "  Done: $(basename "$f")"
done

if [ "$COUNT" -eq 0 ]; then
  echo "Warning: no .tif/.tiff files found in '$DIR'" >&2
  exit 0
fi

echo "→ $COUNT file(s) imported. Running ANALYZE on dem.raster_tiles..."
psql "$DB_URL" -c "ANALYZE dem.raster_tiles;" -q

echo "→ Done. Imported $COUNT GeoTIFF file(s) from source '$SOURCE' into dem.raster_tiles."
echo "   Next: POST /api/v1/dem/import to trigger the background analyze job via the API."
