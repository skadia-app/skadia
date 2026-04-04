#!/bin/bash
# Skadia — btrfs/COW check for PostgreSQL data directory
#
# This script runs during first-time database initialization only.
# It warns if the data directory is on btrfs without nocow (Copy-on-Write disabled).
#
# On btrfs with COW enabled, any interruption (OOM, SIGKILL, power loss) during
# a PostgreSQL write can corrupt data pages permanently. PostgreSQL's WAL replay
# cannot fix COW-level inconsistencies.
#
# Fix: Run `chattr +C <data-dir>` on the HOST before first start.
# See: https://wiki.postgresql.org/wiki/Btrfs
# See: docs/operations/POSTGRES_INTEGRITY.md

PGDATA="${PGDATA:-/home/postgres/pgdata}"

FS_TYPE=$(stat -f -c %T "$PGDATA" 2>/dev/null || echo "unknown")

if [ "$FS_TYPE" = "btrfs" ]; then
    ATTRS=$(lsattr -d "$PGDATA" 2>/dev/null || echo "")
    if echo "$ATTRS" | grep -q "C"; then
        echo "Skadia: btrfs with nocow detected — PostgreSQL data integrity protected."
    else
        echo "============================================================"
        echo "CRITICAL: PostgreSQL data directory is on btrfs WITHOUT nocow!"
        echo ""
        echo "This WILL cause data corruption on OOM, container stop, or power loss."
        echo ""
        echo "Fix (on the HOST, not inside this container):"
        echo "  1. Stop this container"
        echo "  2. sudo chattr +C <host-path>/data/postgres"
        echo "  3. If data already exists: migrate to a new nocow directory"
        echo "     (see docs/operations/POSTGRES_INTEGRITY.md)"
        echo ""
        echo "See: https://wiki.postgresql.org/wiki/Btrfs"
        echo "============================================================"
    fi
fi
