#!/usr/bin/env bash
set -euo pipefail

# Run database migrations using golang-migrate Docker image.
# Usage: ./scripts/migrate.sh [up|down|version|force VERSION]

DIRECTION="${1:-up}"
shift 2>/dev/null || true

# Load .env if present
if [ -f .env ]; then
    set -a
    source .env
    set +a
fi

POSTGRES_PASSWORD="${POSTGRES_PASSWORD:?POSTGRES_PASSWORD is required}"
DB_URL="postgres://skadia:${POSTGRES_PASSWORD}@postgres:5432/skadia?sslmode=disable"

echo "Running migrations: $DIRECTION $*"

docker run --rm \
    -v "$(pwd)/migrations:/migrations" \
    --network "$(basename "$(pwd)")_default" \
    migrate/migrate \
    -path=/migrations \
    -database "$DB_URL" \
    "$DIRECTION" "$@"

echo "Migration complete."
