#!/usr/bin/env bash
set -euo pipefail

PG_CONTAINER="skadia-postgres-1"
NATS_CONTAINER="skadia-nats-1"

distrobox-host-exec podman exec "$PG_CONTAINER" \
  psql -U skadia -d skadia -At -c "
    SELECT user_id::text FROM activities
     WHERE started_at > NOW() - interval '30 days'
     GROUP BY user_id" | while read -r uid; do
  [ -z "$uid" ] && continue
  distrobox-host-exec podman exec "$NATS_CONTAINER" \
    nats pub skadia.analytics.invalidate "{\"user_id\":\"$uid\"}"
  echo "triggered $uid"
done
