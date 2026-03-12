#!/bin/bash
# brouter-build-segments.sh — Builds BRouter rd5 segments from PBF file
# Usage: ./brouter-build-segments.sh <pbf-file> [srtm-dir] [srtm-fallback-dir]
#
# Prerequisites:
#   - Java 17+ on PATH
#   - BRouter JAR in data/brouter/brouter-server-all.jar
#   - BRouter profiles in data/brouter/profiles/
#
# Output:
#   - rd5 segment files in data/brouter/segments/
set -euo pipefail

PBF_FILE="${1:?Usage: $0 <pbf-file> [srtm-dir] [srtm-fallback-dir]}"
SRTM_PATH="${2:-}"
SRTM_FALLBACK="${3:-}"

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_DIR="$(dirname "$SCRIPT_DIR")"
SEGMENTS_DIR="${PROJECT_DIR}/data/brouter/segments"
PROFILES_DIR="${PROJECT_DIR}/data/brouter/profiles"
BROUTER_JAR="${PROJECT_DIR}/data/brouter/brouter-server-all.jar"
WORK_DIR=$(mktemp -d)

# Cleanup on exit
trap 'rm -rf "$WORK_DIR"' EXIT

# Check prerequisites
if [ ! -f "$BROUTER_JAR" ]; then
    echo "ERROR: BRouter JAR not found at $BROUTER_JAR"
    echo "Extract from Docker: docker run --rm -v ${PROJECT_DIR}/data/brouter:/out ghcr.io/abrensch/brouter:latest sh -c 'cp /brouter/brouter-server-*-all.jar /out/brouter-server-all.jar'"
    exit 1
fi

if [ ! -f "$PROFILES_DIR/lookups.dat" ]; then
    echo "ERROR: lookups.dat not found in $PROFILES_DIR"
    echo "Run: cd $PROFILES_DIR && curl -sLO https://raw.githubusercontent.com/abrensch/brouter/master/misc/profiles2/lookups.dat"
    exit 1
fi

if [ ! -f "$PBF_FILE" ]; then
    echo "ERROR: PBF file not found: $PBF_FILE"
    exit 1
fi

JAVA="${JAVA_HOME:+$JAVA_HOME/bin/java}"
JAVA="${JAVA:-java}"
JAVA_OPTS="-Xmx2600m -Xms2600m -Xmn32m"

echo "=== BRouter Segment Build ==="
echo "PBF:      $PBF_FILE"
echo "SRTM:     ${SRTM_PATH:-none}"
echo "Profiles: $PROFILES_DIR"
echo "Output:   $SEGMENTS_DIR"
echo "Workdir:  $WORK_DIR"
echo ""

cd "$WORK_DIR"

# Phase 1: OsmCutter — extracts nodes relevant for routing profiles
echo "[1/7] OsmCutter..."
mkdir nodetiles
$JAVA $JAVA_OPTS -cp "$BROUTER_JAR" -DavoidMapPolling=true \
    btools.mapcreator.OsmCutter \
    "${PROFILES_DIR}/lookups.dat" nodetiles ways.dat relations.dat restrictions.dat \
    "${PROFILES_DIR}/all.brf" "$PBF_FILE"

# Phase 2: NodeFilter — removes irrelevant nodes
echo "[2/7] NodeFilter..."
mkdir ftiles
$JAVA $JAVA_OPTS -cp "$BROUTER_JAR" -Ddeletetmpfiles=true -DuseDenseMaps=true \
    btools.mapcreator.NodeFilter nodetiles ways.dat ftiles

# Phase 3: RelationMerger — merges route relations into ways
echo "[3/7] RelationMerger..."
$JAVA $JAVA_OPTS -cp "$BROUTER_JAR" -Ddeletetmpfiles=true -DuseDenseMaps=true \
    btools.mapcreator.RelationMerger ways.dat ways2.dat relations.dat \
    "${PROFILES_DIR}/lookups.dat" "${PROFILES_DIR}/trekking.brf" "${PROFILES_DIR}/softaccess.brf"

# Phase 4: WayCutter — cuts ways at node intersections
echo "[4/7] WayCutter..."
mkdir waytiles
$JAVA $JAVA_OPTS -cp "$BROUTER_JAR" -Ddeletetmpfiles=true -DuseDenseMaps=true \
    btools.mapcreator.WayCutter ftiles ways2.dat waytiles

# Phase 5: WayCutter5 — creates 5-degree tiles
echo "[5/7] WayCutter5..."
mkdir waytiles55
$JAVA $JAVA_OPTS -cp "$BROUTER_JAR" -Ddeletetmpfiles=true -DuseDenseMaps=true \
    btools.mapcreator.WayCutter5 ftiles waytiles waytiles55 bordernids.dat

# Phase 6: NodeCutter + PosUnifier — with optional SRTM elevation
echo "[6/7] NodeCutter + PosUnifier..."
mkdir nodes55 unodes55
$JAVA $JAVA_OPTS -cp "$BROUTER_JAR" -Ddeletetmpfiles=true -DuseDenseMaps=true \
    btools.mapcreator.NodeCutter ftiles nodes55

SRTM_ARGS=()
if [ -n "$SRTM_PATH" ]; then
    SRTM_ARGS+=("$SRTM_PATH")
    [ -n "$SRTM_FALLBACK" ] && SRTM_ARGS+=("$SRTM_FALLBACK")
fi
$JAVA $JAVA_OPTS -cp "$BROUTER_JAR" -Ddeletetmpfiles=true -DuseDenseMaps=true \
    btools.mapcreator.PosUnifier nodes55 unodes55 bordernids.dat bordernodes.dat "${SRTM_ARGS[@]+"${SRTM_ARGS[@]}"}"

# Phase 7: WayLinker — creates final rd5 segment files
echo "[7/7] WayLinker → rd5..."
mkdir segments
$JAVA $JAVA_OPTS -cp "$BROUTER_JAR" -DuseDenseMaps=true \
    btools.mapcreator.WayLinker unodes55 waytiles55 bordernodes.dat restrictions.dat \
    "${PROFILES_DIR}/lookups.dat" "${PROFILES_DIR}/all.brf" segments rd5

# Move rd5 files to target directory
echo ""
echo "Moving segments to $SEGMENTS_DIR..."
mkdir -p "$SEGMENTS_DIR"
mv segments/*.rd5 "$SEGMENTS_DIR/"

RD5_COUNT=$(ls "$SEGMENTS_DIR"/*.rd5 2>/dev/null | wc -l)
echo ""
echo "=== Done: ${RD5_COUNT} rd5 files in $SEGMENTS_DIR ==="
