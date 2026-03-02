#!/usr/bin/env bash
set -euo pipefail

# Skadia OSM Import Script
# Usage: ./import.sh [PBF_FILE]
# If no file given, downloads Bayern extract from Geofabrik

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DATA_DIR="${SCRIPT_DIR}/data"
LUA_SCRIPT="${SCRIPT_DIR}/skadia-osm.lua"
DB_NAME="${SKADIA_DB:-skadia}"
DB_HOST="${SKADIA_DB_HOST:-localhost}"
DB_PORT="${SKADIA_DB_PORT:-5432}"
DB_USER="${SKADIA_DB_USER:-postgres}"
PBF_FILE="${1:-}"

mkdir -p "$DATA_DIR"

# Download if no file specified
if [ -z "$PBF_FILE" ]; then
    PBF_FILE="${DATA_DIR}/bayern-latest.osm.pbf"
    if [ ! -f "$PBF_FILE" ]; then
        echo "Downloading Bayern extract from Geofabrik..."
        curl -L -o "$PBF_FILE" "https://download.geofabrik.de/europe/germany/bayern-latest.osm.pbf"
    else
        echo "Using existing PBF: $PBF_FILE"
    fi
fi

echo "Importing OSM data from: $PBF_FILE"
echo "Target database: $DB_NAME@$DB_HOST:$DB_PORT"
echo ""

# Run osm2pgsql in Flex mode
osm2pgsql \
    -d "$DB_NAME" \
    -H "$DB_HOST" \
    -P "$DB_PORT" \
    -U "$DB_USER" \
    -S "$LUA_SCRIPT" \
    -O flex \
    --log-progress true \
    "$PBF_FILE"

echo ""
echo "Creating spatial indexes..."

psql -h "$DB_HOST" -p "$DB_PORT" -U "$DB_USER" -d "$DB_NAME" <<'SQL'
-- Indexes (IF NOT EXISTS to be safe)
CREATE INDEX IF NOT EXISTS idx_osm_boundaries_geom ON osm_boundaries USING GIST (geom);
CREATE INDEX IF NOT EXISTS idx_osm_boundaries_admin ON osm_boundaries (admin_level);
CREATE INDEX IF NOT EXISTS idx_osm_peaks_geom ON osm_peaks USING GIST (geom);
CREATE INDEX IF NOT EXISTS idx_osm_forests_geom ON osm_forests USING GIST (geom);
CREATE INDEX IF NOT EXISTS idx_osm_waters_geom ON osm_waters USING GIST (geom);
CREATE INDEX IF NOT EXISTS idx_osm_waterways_geom ON osm_waterways USING GIST (geom);
CREATE INDEX IF NOT EXISTS idx_osm_settlements_geom ON osm_settlements USING GIST (geom);
CREATE INDEX IF NOT EXISTS idx_osm_amenities_geom ON osm_amenities USING GIST (geom);

-- Stats
SELECT 'osm_boundaries' AS table_name, COUNT(*) AS rows FROM osm_boundaries
UNION ALL SELECT 'osm_peaks', COUNT(*) FROM osm_peaks
UNION ALL SELECT 'osm_forests', COUNT(*) FROM osm_forests
UNION ALL SELECT 'osm_waters', COUNT(*) FROM osm_waters
UNION ALL SELECT 'osm_waterways', COUNT(*) FROM osm_waterways
UNION ALL SELECT 'osm_settlements', COUNT(*) FROM osm_settlements
UNION ALL SELECT 'osm_amenities', COUNT(*) FROM osm_amenities;
SQL

echo ""
echo "OSM import complete!"
