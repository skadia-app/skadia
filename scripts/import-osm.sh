#!/usr/bin/env bash
# Usage: bash scripts/import-osm.sh [--region bavaria|dach|europe]
# Downloads PBF from GeoFabrik and imports into PostGIS via osm2pgsql Flex.

set -euo pipefail

REGION="${1:-dach}"
shift 2>/dev/null || true

case "$REGION" in
  --region) REGION="$1" ;;
esac

case "$REGION" in
  bavaria|bayern)
    URL="https://download.geofabrik.de/europe/germany/bavaria-latest.osm.pbf"
    FILE="bavaria-latest.osm.pbf"
    ;;
  dach)
    URL="https://download.geofabrik.de/europe/dach-latest.osm.pbf"
    FILE="dach-latest.osm.pbf"
    ;;
  europe)
    URL="https://download.geofabrik.de/europe-latest.osm.pbf"
    FILE="europe-latest.osm.pbf"
    ;;
  *)
    echo "Unknown region: $REGION. Use: bavaria, dach, europe" >&2
    exit 1
    ;;
esac

DATA_DIR="${OSM_DATA_DIR:-./data/osm}"
mkdir -p "$DATA_DIR"
PBF="$DATA_DIR/$FILE"

echo "→ Downloading $REGION PBF from GeoFabrik..."
curl -L --progress-bar -o "$PBF" "$URL"

echo "→ Importing into PostGIS via osm2pgsql..."
osm2pgsql \
  --database "${DATABASE_URL:-skadia}" \
  --output=flex \
  --style "$(dirname "$0")/../services/osm/skadia-osm.lua" \
  --slim \
  "$PBF"

echo "→ Done. Imported $REGION OSM data."
