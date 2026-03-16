#!/usr/bin/env bash
# scripts/dem-download-glo30.sh
# Downloads Copernicus GLO-30 DEM tiles for DACH region
#
# Usage: ./scripts/dem-download-glo30.sh [output_dir]
# Default output: ./data/dem/glo30_tiles/
#
# Primary source: AWS Open Data (stable URLs)
# After download, import with: ./scripts/dem-import.sh glo30 ./data/dem/glo30_tiles/ 30

set -euo pipefail

OUT_DIR="${1:-./data/dem/glo30_tiles}"
mkdir -p "$OUT_DIR"

# AWS Open Data mirror (more stable than Copernicus direct)
BASE_URL="https://copernicus-dem-30m.s3.eu-central-1.amazonaws.com"

echo "Downloading Copernicus GLO-30 DEM tiles for DACH region..."
echo "Output: $OUT_DIR"

# DACH region: lat 45-49, lon 5-17
COUNT=0
for lat in $(seq 45 48); do
  for lon in $(seq 5 16); do
    lat_str=$(printf "N%02d_00" $lat)
    lon_str=$(printf "E%03d_00" $lon)
    # Resolution code is 10 (= 10 arc-seconds ≈ 30m), NOT 30
    filename="Copernicus_DSM_COG_10_${lat_str}_${lon_str}_DEM.tif"
    key="Copernicus_DSM_COG_10_${lat_str}_${lon_str}_DEM/${filename}"
    url="${BASE_URL}/${key}"

    if [ -f "$OUT_DIR/$filename" ]; then
      echo "  Skip (exists): $filename"
      continue
    fi

    echo "  Downloading: $filename"
    if curl -sSfL -o "$OUT_DIR/$filename" "$url" 2>/dev/null; then
      COUNT=$((COUNT + 1))
    else
      echo "  Warning: $filename not available (may be ocean tile)"
      rm -f "$OUT_DIR/$filename"
    fi
  done
done

echo "Downloaded $COUNT new tiles to $OUT_DIR"
echo ""
echo "Next steps:"
echo "  1. Import to DB: ./scripts/dem-import.sh glo30 $OUT_DIR 30"
echo "  2. Verify: SELECT dem.get_elevation(ST_SetSRID(ST_MakePoint(10.9853, 47.4211), 4326));"
echo "     Expected: ~2962 (Zugspitze)"
