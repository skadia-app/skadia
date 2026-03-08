#!/bin/bash
set -euo pipefail

echo "=== Skadia Backup Verify ==="
LAST=$(distrobox-host-exec podman compose run --rm walg-backup \
    wal-g backup-list 2>/dev/null | tail -1)
echo "Letztes Backup: $LAST"

# Prüfe ob WAL-Archivierung aktiv
distrobox-host-exec podman compose exec postgres \
    psql -U postgres -c "SELECT pg_walfile_name(pg_current_wal_lsn());"
