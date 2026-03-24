#!/usr/bin/env bash
# scripts/dem-download-all-regions.sh
# Downloads Copernicus GLO-30 DEM tiles for all imported regions and converts to HGT.
#
# Usage: ./scripts/dem-download-all-regions.sh
#
# Downloads GeoTIFFs from AWS Open Data, converts to SRTMHGT format (.hgt).
# HGT files are placed in ./data/dem/glo30/ for direct use by the tile builder.
# Skips already-existing HGT files.
#
# Requirements: curl, gdal_translate

set -euo pipefail

HGT_DIR="./data/dem/glo30"
TIF_DIR="./data/dem/glo30_tiles"
mkdir -p "$HGT_DIR" "$TIF_DIR"

BASE_URL="https://copernicus-dem-30m.s3.eu-central-1.amazonaws.com"

# All 10 regions: DE, AT, CH, LI, SI, IT, GR, PT, PE, NP
# Combined BBox: Lat -18 to 55, Lon -81 to 89
# Adjusted to actual country boundaries to avoid ocean tiles

declare -A REGIONS
# Format: "min_lat max_lat min_lon max_lon"
REGIONS[DE]="47 55 5 15"
REGIONS[AT]="46 49 9 17"
REGIONS[CH]="45 48 5 11"
REGIONS[SI]="45 47 13 17"
REGIONS[IT]="36 47 6 19"
REGIONS[GR]="34 42 19 30"
REGIONS[PT]="36 42 -10 -6"
REGIONS[LI]="47 47 9 10"
REGIONS[PE]="-18 0 -81 -68"
REGIONS[NP]="26 30 80 89"

# Collect unique tile coordinates
declare -A TILES
for region in "${!REGIONS[@]}"; do
    read -r min_lat max_lat min_lon max_lon <<< "${REGIONS[$region]}"
    for lat in $(seq $min_lat $max_lat); do
        for lon in $(seq $min_lon $max_lon); do
            TILES["${lat}_${lon}"]=1
        done
    done
done

TOTAL=${#TILES[@]}
echo "Total unique DEM tiles needed: $TOTAL"

# Count existing
EXISTING=0
for key in "${!TILES[@]}"; do
    IFS='_' read -r lat lon <<< "$key"
    if [ $lat -ge 0 ]; then
        lat_str=$(printf "N%02d" $lat)
    else
        lat_str=$(printf "S%02d" $((-lat)))
    fi
    if [ $lon -ge 0 ]; then
        lon_str=$(printf "E%03d" $lon)
    else
        lon_str=$(printf "W%03d" $((-lon)))
    fi
    hgt_file="${HGT_DIR}/${lat_str}${lon_str}.hgt"
    if [ -f "$hgt_file" ]; then
        EXISTING=$((EXISTING + 1))
    fi
done
echo "Already existing: $EXISTING"
echo "To download: $((TOTAL - EXISTING))"
echo ""

DOWNLOADED=0
CONVERTED=0
SKIPPED=0
FAILED=0

for key in $(echo "${!TILES[@]}" | tr ' ' '\n' | sort); do
    IFS='_' read -r lat lon <<< "$key"

    if [ $lat -ge 0 ]; then
        lat_str=$(printf "N%02d" $lat)
        lat_aws=$(printf "N%02d_00" $lat)
    else
        lat_str=$(printf "S%02d" $((-lat)))
        lat_aws=$(printf "S%02d_00" $((-lat)))
    fi
    if [ $lon -ge 0 ]; then
        lon_str=$(printf "E%03d" $lon)
        lon_aws=$(printf "E%03d_00" $lon)
    else
        lon_str=$(printf "W%03d" $((-lon)))
        lon_aws=$(printf "W%03d_00" $((-lon)))
    fi

    hgt_file="${HGT_DIR}/${lat_str}${lon_str}.hgt"
    tif_file="${TIF_DIR}/Copernicus_DSM_COG_10_${lat_aws}_${lon_aws}_DEM.tif"

    # Skip if HGT already exists
    if [ -f "$hgt_file" ]; then
        SKIPPED=$((SKIPPED + 1))
        continue
    fi

    # Download GeoTIFF if not cached
    if [ ! -f "$tif_file" ]; then
        filename="Copernicus_DSM_COG_10_${lat_aws}_${lon_aws}_DEM.tif"
        aws_key="Copernicus_DSM_COG_10_${lat_aws}_${lon_aws}_DEM/${filename}"
        url="${BASE_URL}/${aws_key}"

        if curl -sSfL -o "$tif_file" "$url" 2>/dev/null; then
            DOWNLOADED=$((DOWNLOADED + 1))
        else
            # Ocean tile or not available
            rm -f "$tif_file"
            FAILED=$((FAILED + 1))
            continue
        fi
    fi

    # Convert GeoTIFF to SRTMHGT (Copernicus is 3600x3600, HGT needs 3601x3601)
    if gdal_translate -of SRTMHGT -outsize 3601 3601 -r bilinear -q "$tif_file" "$hgt_file" 2>/dev/null; then
        CONVERTED=$((CONVERTED + 1))
        echo "  ✓ ${lat_str}${lon_str} (downloaded + converted)"
    else
        rm -f "$hgt_file"
        FAILED=$((FAILED + 1))
        echo "  ✗ ${lat_str}${lon_str} (conversion failed)"
    fi
done

echo ""
echo "=== Summary ==="
echo "Skipped (existing): $SKIPPED"
echo "Downloaded + converted: $CONVERTED"
echo "Failed/ocean: $FAILED"
echo "Total HGT files: $(ls "$HGT_DIR"/*.hgt 2>/dev/null | wc -l)"
echo ""
echo "HGT directory: $HGT_DIR"
echo "Ready for tile build."
