#!/bin/bash
# scripts/brouter-download-segments.sh
# Downloads BRouter rd5 segments for specified region
# Usage: ./brouter-download-segments.sh [region]
# Regions: alps, dach, europe

set -euo pipefail

BASE_URL="https://brouter.de/brouter/segments4"
TARGET_DIR="$(dirname "$0")/../data/brouter/segments"

mkdir -p "$TARGET_DIR"

case "${1:-alps}" in
  alps)
    FILES="E005N45.rd5 E005N50.rd5 E010N45.rd5 E010N50.rd5 E015N45.rd5 E015N50.rd5"
    ;;
  dach)
    FILES="E005N45.rd5 E005N50.rd5 E010N45.rd5 E010N50.rd5 E015N45.rd5 E015N50.rd5 E000N50.rd5 E005N55.rd5 E010N55.rd5 E015N55.rd5"
    ;;
  europe)
    # All segments covering Europe (roughly W010-E035, N35-N70)
    for lon in $(seq -10 5 35); do
      for lat in $(seq 35 5 70); do
        printf -v lonf "%s%03d" "$([ $lon -ge 0 ] && echo E || echo W)" "${lon#-}"
        printf -v latf "N%02d" "$lat"
        FILES="${FILES:-} ${lonf}${latf}.rd5"
      done
    done
    ;;
  *)
    echo "Usage: $0 [alps|dach|europe]"
    exit 1
    ;;
esac

for f in $FILES; do
  if [ ! -f "$TARGET_DIR/$f" ]; then
    echo "Downloading $f..."
    curl -fSL -o "$TARGET_DIR/$f" "$BASE_URL/$f" || echo "  Skipped $f (not available)"
  else
    echo "Already exists: $f"
  fi
done

echo "Done. Segments in $TARGET_DIR"
