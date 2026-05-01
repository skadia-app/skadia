#!/usr/bin/env bash
# Backfill readiness invalidation messages for recently active users.
#
# COMPOSE_PROJECT_NAME (optional override): set this env var to match the
# Compose project name used in your worktree or deployment. Defaults to
# "skadia". Example:
#   COMPOSE_PROJECT_NAME=skadia-readiness bash scripts/backfill-readiness.sh
set -euo pipefail

: "${COMPOSE_PROJECT_NAME:=skadia}"

PG_CONTAINER="${COMPOSE_PROJECT_NAME}-postgres-1"
NATS_CONTAINER="${COMPOSE_PROJECT_NAME}-nats-1"
NETWORK="${COMPOSE_PROJECT_NAME}_default"

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
  distrobox-host-exec podman run --rm --network="$NETWORK" \
    synadia/nats-box nats --server "nats://${NATS_CONTAINER}:4222" \
    pub skadia.analytics.invalidate "{\"user_id\":\"$uid\"}"
  echo "triggered $uid"
done
