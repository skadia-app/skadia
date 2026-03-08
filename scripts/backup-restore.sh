#!/bin/bash
set -euo pipefail

BACKUP=${1:-LATEST}
TARGET_TIME=${3:-}

echo "=== Skadia PostgreSQL Restore ==="
echo "Backup: $BACKUP"
[ -n "${TARGET_TIME:-}" ] && echo "PITR Target: $TARGET_TIME"

echo "Stopping services..."
distrobox-host-exec podman compose stop api enrichment

echo "Available backups:"
distrobox-host-exec podman compose run --rm walg-backup wal-g backup-list

echo "Restoring..."
if [ "$BACKUP" = "LATEST" ]; then
    distrobox-host-exec podman compose run --rm walg-backup \
        wal-g backup-fetch /home/postgres/pgdata LATEST
else
    distrobox-host-exec podman compose run --rm walg-backup \
        wal-g backup-fetch /home/postgres/pgdata "$BACKUP"
fi

echo "Restarting..."
distrobox-host-exec podman compose restart postgres
sleep 10
distrobox-host-exec podman compose start api enrichment

echo "=== Restore complete ==="
