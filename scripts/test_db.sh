#!/usr/bin/env bash
# scripts/test_db.sh - Per-worktree test-DB manager.
# See docs/documentation/dev/subsystems/test-db.md for full docs.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=test_db_internal/lib.sh
source "$SCRIPT_DIR/test_db_internal/lib.sh"

usage() {
  cat <<USAGE
Usage: $(basename "$0") <command> [args]

Commands:
  ensure                     Create/update test-DB for current worktree.
  emit-url                   Print 'export TEST_DATABASE_URL=...' line.
  run -- <cmd> [args...]     Set TEST_DATABASE_URL+DATABASE_URL and exec <cmd>.
  drop                       Drop test-DB for current worktree.
  gc                         Drop test-DBs whose worktree no longer exists.
  inspect                    List all skadia_test_* DBs with worktree mapping.
  help                       Show this message.

Environment:
  SKADIA_TEST_PG_HOST       Postgres host (default: localhost)
  SKADIA_TEST_PG_PORT       Postgres port (default: 5432)
  SKADIA_TEST_PG_USER       Postgres role (default: skadia)
  SKADIA_TEST_PG_PASSWORD   Password (default: skadia_dev_password)
  SKADIA_TEST_PG_ADMIN_DB   Admin/maintenance DB (default: postgres)
USAGE
}

[[ $# -ge 1 ]] || { usage; exit 1; }
cmd="$1"; shift

case "$cmd" in
  ensure) cmd_ensure "$@" ;;
  emit-url) cmd_emit_url "$@" ;;
  run) cmd_run "$@" ;;
  drop) cmd_drop "$@" ;;
  gc) cmd_gc "$@" ;;
  inspect) cmd_inspect "$@" ;;
  help|-h|--help) usage ;;
  *) usage; exit 1 ;;
esac
