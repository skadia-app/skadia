#!/usr/bin/env bash
# scripts/test_db_internal/lib.sh - shared functions for test_db.sh

: "${SKADIA_TEST_PG_HOST:=localhost}"
: "${SKADIA_TEST_PG_PORT:=5432}"
: "${SKADIA_TEST_PG_USER:=skadia}"
: "${SKADIA_TEST_PG_PASSWORD:=skadia_dev_password}"
: "${SKADIA_TEST_PG_ADMIN_DB:=postgres}"

worktree_root() {
  git rev-parse --show-toplevel 2>/dev/null
}

worktree_slug_for_path() {
  local path identity
  path="$(git -C "$1" rev-parse --show-toplevel 2>/dev/null)" || {
    echo "test_db.sh: not inside a git worktree" >&2
    return 1
  }
  identity="$(stat -Lc '%d:%i' "$path")" || return 1
  printf '%s' "$identity" | sha256sum | cut -c1-12
}

worktree_slug() {
  local path
  path="$(worktree_root)" || {
    echo "test_db.sh: not inside a git worktree" >&2
    return 1
  }
  worktree_slug_for_path "$path"
}

worktree_db_name() {
  local slug
  slug="$(worktree_slug)" || return 1
  printf 'skadia_test_%s' "$slug"
}

psql_admin() {
  PGPASSWORD="$SKADIA_TEST_PG_PASSWORD" psql \
    -h "$SKADIA_TEST_PG_HOST" -p "$SKADIA_TEST_PG_PORT" \
    -U "$SKADIA_TEST_PG_USER" -d "$SKADIA_TEST_PG_ADMIN_DB" \
    -v ON_ERROR_STOP=1 --no-psqlrc -tA "$@"
}

psql_test() {
  local db
  db="$(worktree_db_name)"
  PGPASSWORD="$SKADIA_TEST_PG_PASSWORD" psql \
    -h "$SKADIA_TEST_PG_HOST" -p "$SKADIA_TEST_PG_PORT" \
    -U "$SKADIA_TEST_PG_USER" -d "$db" \
    -v ON_ERROR_STOP=1 --no-psqlrc -tA "$@"
}

worktree_dsn() {
  local db
  db="$(worktree_db_name)"
  printf 'postgres://%s:%s@%s:%s/%s?sslmode=disable' \
    "$SKADIA_TEST_PG_USER" "$SKADIA_TEST_PG_PASSWORD" \
    "$SKADIA_TEST_PG_HOST" "$SKADIA_TEST_PG_PORT" "$db"
}

migrations_fingerprint() {
  local repo_root
  repo_root="$(worktree_root)"
  migrations_fingerprint_for_root "$repo_root"
}

migrations_fingerprint_for_root() {
  local repo_root="$1"
  (
    cd "$repo_root"
    find services/api/internal/migrate \
      \( -name '*.up.sql' -o -name '*.down.sql' \
      -o -name 'migrate.go' -o -name 'migrations.go' \
      -o -name 'migration_*_test.go' \) -print0 \
      | LC_ALL=C sort -z \
      | while IFS= read -r -d '' f; do
          printf '%s\0' "$f"
          cat "$f"
          printf '\0'
        done \
      | sha256sum | cut -c1-16
  )
}

cmd_ensure() {
  local db slug fingerprint repo_root branch lock_file
  db="$(worktree_db_name)"
  slug="$(worktree_slug)"
  fingerprint="$(migrations_fingerprint)"
  repo_root="$(worktree_root)"
  branch="$(git rev-parse --abbrev-ref HEAD)"
  lock_file="/tmp/skadia-test-db-${slug}.lock"

  exec 9>"$lock_file"
  flock -x 9 || { echo "test_db.sh: could not acquire lock $lock_file" >&2; return 1; }

  if [ -n "${SKADIA_TEST_LOCK_HOLD_SECONDS:-}" ]; then
    sleep "$SKADIA_TEST_LOCK_HOLD_SECONDS"
  fi

  local needs_rebuild=1
  if psql_admin --set=db="$db" <<'SQL' | grep -q 1; then
SELECT 1 FROM pg_database WHERE datname = :'db';
SQL
    local current_fp
    current_fp="$(psql_test -c "SELECT migrations_fingerprint FROM _test_meta WHERE id = 1" 2>/dev/null | tr -d '[:space:]' || true)"
    if [ "$current_fp" = "$fingerprint" ]; then
      needs_rebuild=0
    fi
  fi

  if [ "$needs_rebuild" -eq 0 ]; then
    psql_test -c "UPDATE _test_meta SET last_used_at = now() WHERE id = 1" >/dev/null
    echo "test_db.sh: $db is current (fingerprint=$fingerprint)" >&2
    exec 9>&-
    return 0
  fi

  echo "test_db.sh: rebuilding $db (fingerprint=$fingerprint)" >&2

  psql_admin --set=db="$db" <<'SQL'
SELECT pg_terminate_backend(pid)
FROM pg_stat_activity
WHERE datname = :'db' AND pid <> pg_backend_pid();
SQL
  psql_admin --set=db="$db" <<'SQL'
DROP DATABASE IF EXISTS :"db";
SQL
  psql_admin --set=db="$db" --set=owner="$SKADIA_TEST_PG_USER" <<'SQL'
CREATE DATABASE :"db" OWNER :"owner";
SQL

  (
    cd "$repo_root/services/api"
    DATABASE_URL="$(worktree_dsn)" go run ./cmd/migrate
  ) >&2

  psql_test \
    --set=repo_root="$repo_root" \
    --set=branch="$branch" \
    --set=fingerprint="$fingerprint" \
    <<'SQL'
CREATE TABLE _test_meta (
  id INT PRIMARY KEY DEFAULT 1 CHECK (id = 1),
  worktree_path TEXT NOT NULL,
  branch TEXT NOT NULL,
  migrations_fingerprint TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  last_used_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
INSERT INTO _test_meta (worktree_path, branch, migrations_fingerprint)
VALUES (:'repo_root', :'branch', :'fingerprint');
SQL

  exec 9>&-
}
cmd_emit_url() { printf 'export TEST_DATABASE_URL="%s"\n' "$(worktree_dsn)"; }
cmd_run() {
  if [ "${1:-}" != "--" ]; then
    echo "test_db.sh run: missing '--' separator before command" >&2
    return 2
  fi
  shift
  if [ $# -eq 0 ]; then
    echo "test_db.sh run: no command given after '--'" >&2
    return 2
  fi
  cmd_ensure
  local dsn
  dsn="$(worktree_dsn)"
  TEST_DATABASE_URL="$dsn" DATABASE_URL="$dsn" exec "$@"
}
cmd_drop() {
  local db
  db="$(worktree_db_name)"
  psql_admin --set=db="$db" <<'SQL' >/dev/null
SELECT pg_terminate_backend(pid)
FROM pg_stat_activity
WHERE datname = :'db' AND pid <> pg_backend_pid();
SQL
  psql_admin --set=db="$db" <<'SQL' >/dev/null
DROP DATABASE IF EXISTS :"db";
SQL
  echo "test_db.sh: dropped $db" >&2
}
cmd_gc() {
  local dbs
  dbs="$(psql_admin -c "
    SELECT datname FROM pg_database
    WHERE datname LIKE 'skadia\_test\_%' ESCAPE '\'
    ORDER BY datname")"
  while IFS= read -r db; do
    [ -n "$db" ] || continue
    local path
    path="$(PGPASSWORD="$SKADIA_TEST_PG_PASSWORD" psql \
      -h "$SKADIA_TEST_PG_HOST" -p "$SKADIA_TEST_PG_PORT" \
      -U "$SKADIA_TEST_PG_USER" -d "$db" \
      -tA -v ON_ERROR_STOP=1 --no-psqlrc \
      -c "SELECT worktree_path FROM _test_meta WHERE id = 1" 2>/dev/null || true)"
    if [ -z "$path" ]; then
      echo "test_db.sh: $db has no _test_meta - leaving alone" >&2
      continue
    fi
    if [ ! -d "$path" ] || ! git -C "$path" rev-parse --is-inside-work-tree >/dev/null 2>&1; then
      echo "test_db.sh: dropping zombie $db (worktree $path gone)" >&2
      psql_admin --set=db="$db" <<'SQL' >/dev/null
SELECT pg_terminate_backend(pid)
FROM pg_stat_activity
WHERE datname = :'db' AND pid <> pg_backend_pid();
SQL
      psql_admin --set=db="$db" <<'SQL' >/dev/null
DROP DATABASE IF EXISTS :"db";
SQL
    fi
  done <<< "$dbs"
}

cmd_inspect() {
  printf 'db_name\tworktree_path\tbranch\tlast_used_at\tfingerprint\n'
  local dbs
  dbs="$(psql_admin -c "
    SELECT datname FROM pg_database
    WHERE datname LIKE 'skadia\_test\_%' ESCAPE '\'
    ORDER BY datname")"
  while IFS= read -r db; do
    [ -n "$db" ] || continue
    local meta
    meta="$(PGPASSWORD="$SKADIA_TEST_PG_PASSWORD" psql \
      -h "$SKADIA_TEST_PG_HOST" -p "$SKADIA_TEST_PG_PORT" \
      -U "$SKADIA_TEST_PG_USER" -d "$db" \
      -tA -F $'\t' -v ON_ERROR_STOP=1 --no-psqlrc \
      -c "SELECT worktree_path, branch, last_used_at, migrations_fingerprint FROM _test_meta WHERE id = 1" \
      2>/dev/null || printf '(no _test_meta)\t\t\t')"
    printf '%s\t%s\n' "$db" "$meta"
  done <<< "$dbs"
}
