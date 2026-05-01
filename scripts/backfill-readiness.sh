#!/usr/bin/env bash
set -euo pipefail

PG_CONTAINER="skadia-postgres-1"
NATS_CONTAINER="skadia-nats-1"

distrobox-host-exec podman exec "$PG_CONTAINER" \
  psql -U skadia -d skadia -At -c "
    SELECT user_id::text FROM activities
     WHERE started_at > NOW() - interval '30 days'
     GROUP BY user_id" | while read -r uid; do
  # Use if-guard instead of [ ] && continue: the latter exits under set -e when
  # the read produces an empty line (e.g. trailing newline from psql -At output).
  if [ -z "$uid" ]; then continue; fi
  # nats:2-alpine does not ship the nats CLI; use a one-shot nats-box container
  # on the same Compose network instead.
  distrobox-host-exec podman run --rm --network=skadia_default \
    synadia/nats-box nats --server nats://skadia-nats-1:4222 \
    pub skadia.analytics.invalidate "{\"user_id\":\"$uid\"}"
  echo "triggered $uid"
done
