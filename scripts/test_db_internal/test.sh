#!/usr/bin/env bash
# scripts/test_db_internal/test.sh - smoke tests for test_db.sh
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
TEST_DB_SH="$SCRIPT_DIR/../test_db.sh"
cleanup_paths=()
cleanup_dirs=()
restore_specs=()

cleanup() {
  local spec backup target
  for spec in "${restore_specs[@]:-}"; do
    backup="${spec%%::*}"
    target="${spec#*::}"
    if [ -f "$backup" ]; then
      cp "$backup" "$target"
      rm -f "$backup"
    fi
  done
  if [ "${#cleanup_paths[@]}" -gt 0 ]; then
    rm -f "${cleanup_paths[@]}"
  fi
  if [ "${#cleanup_dirs[@]}" -gt 0 ]; then
    rm -rf "${cleanup_dirs[@]}"
  fi
}
trap cleanup EXIT
trap 'cleanup; exit 130' INT TERM

assert_eq() {
  local actual="$1" expected="$2" name="$3"
  if [ "$actual" = "$expected" ]; then
    echo "  PASS: $name"
  else
    echo "  FAIL: $name"
    echo "    actual:   $actual"
    echo "    expected: $expected"
    return 1
  fi
}

run_test() {
  echo "=== $1 ==="
  "$@" || { echo "  test runner: FAILURE"; return 1; }
}

test_help_exits_zero() {
  "$TEST_DB_SH" help >/dev/null
}

test_emit_url_format() {
  local url
  url="$("$TEST_DB_SH" emit-url)"
  case "$url" in
    'export TEST_DATABASE_URL="postgres://'*'/skadia_test_'*'?sslmode=disable"')
      echo "  PASS: emit-url format" ;;
    *)
      echo "  FAIL: emit-url format mismatch"
      echo "    got: $url"
      return 1 ;;
  esac
}

test_slug_stable() {
  # shellcheck source=lib.sh
  source "$SCRIPT_DIR/lib.sh"
  local s1 s2
  s1="$(worktree_slug)"
  s2="$(worktree_slug)"
  assert_eq "$s1" "$s2" "slug stable across calls"
  [ "${#s1}" -eq 12 ] || { echo "  FAIL: slug length != 12 (got ${#s1})"; return 1; }
  echo "  PASS: slug length is 12"
}

test_slug_same_for_home_aliases() {
  # shellcheck source=lib.sh
  source "$SCRIPT_DIR/lib.sh"
  local root alt_root s1 s2
  root="$(git rev-parse --show-toplevel)"
  case "$root" in
    /home/*) alt_root="/var${root}" ;;
    /var/home/*) alt_root="${root#/var}" ;;
    *) echo "  PASS: no /home <-> /var/home alias on this checkout"; return 0 ;;
  esac
  if [ ! -d "$alt_root" ]; then
    echo "  PASS: alias path $alt_root not present"
    return 0
  fi
  if [ "$(stat -Lc '%d:%i' "$root")" != "$(stat -Lc '%d:%i' "$alt_root")" ]; then
    echo "  PASS: alias path is not the same inode"
    return 0
  fi
  s1="$(worktree_slug_for_path "$root")"
  s2="$(worktree_slug_for_path "$alt_root")"
  assert_eq "$s1" "$s2" "slug stable across /home and /var/home aliases"
}

test_fingerprint_changes_on_content_edit() {
  # shellcheck source=lib.sh
  source "$SCRIPT_DIR/lib.sh"
  local fixture fp1 fp2 tmpfile
  fixture="$(make_fingerprint_fixture)"
  fp1="$(migrations_fingerprint_for_root "$fixture")"
  tmpfile="$(mktemp -p "$fixture/services/api/internal/migrate/migrations" \
    test_fingerprint_XXXXXX.up.sql)"
  echo "-- temporary marker" > "$tmpfile"
  fp2="$(migrations_fingerprint_for_root "$fixture")"
  if [ "$fp1" = "$fp2" ]; then
    echo "  FAIL: fingerprint unchanged after adding migration file"
    return 1
  fi
  echo "  PASS: fingerprint reacts to file add"
}

test_fingerprint_changes_on_filename_only_edit() {
  # shellcheck source=lib.sh
  source "$SCRIPT_DIR/lib.sh"
  local fixture target fp1 fp2
  fixture="$(make_fingerprint_fixture)"
  target="$fixture/services/api/internal/migrate/migrations/000001_init_extensions.up.sql"
  fp1="$(migrations_fingerprint_for_root "$fixture")"
  echo "-- modified" >> "$target"
  fp2="$(migrations_fingerprint_for_root "$fixture")"
  if [ "$fp1" = "$fp2" ]; then
    echo "  FAIL: fingerprint unchanged after content edit"
    return 1
  fi
  echo "  PASS: fingerprint reacts to content edit"
}

make_fingerprint_fixture() {
  local fixture
  fixture="$(mktemp -d)"
  cleanup_dirs+=("$fixture")
  mkdir -p "$fixture/services/api/internal/migrate/migrations"
  printf '%s\n' '-- fixture migration' > \
    "$fixture/services/api/internal/migrate/migrations/000001_init_extensions.up.sql"
  printf '%s\n' '-- fixture down migration' > \
    "$fixture/services/api/internal/migrate/migrations/000001_init_extensions.down.sql"
  printf '%s\n' 'package migrate' > "$fixture/services/api/internal/migrate/migrate.go"
  printf '%s\n' 'package migrations_test' > \
    "$fixture/services/api/internal/migrate/migrations/migration_000001_test.go"
  printf '%s\n' "$fixture"
}

test_ensure_creates_db_first_call() {
  "$TEST_DB_SH" drop >/dev/null 2>&1 || true

  "$TEST_DB_SH" ensure >/dev/null
  local url
  url="$("$TEST_DB_SH" emit-url | sed 's/^export TEST_DATABASE_URL="//; s/"$//')"
  PGPASSWORD=skadia_dev_password psql "$url" -c "SELECT 1 FROM activity_lift_candidates LIMIT 0" \
    >/dev/null 2>&1 \
    || { echo "  FAIL: activity_lift_candidates not present after ensure"; return 1; }
  echo "  PASS: ensure creates DB and applies migrations"
}

test_ensure_idempotent_when_fingerprint_unchanged() {
  "$TEST_DB_SH" ensure >/dev/null
  local first_created second_created
  first_created="$(source "$SCRIPT_DIR/lib.sh"; psql_test -c "SELECT created_at FROM _test_meta WHERE id = 1")"
  sleep 1
  "$TEST_DB_SH" ensure >/dev/null
  second_created="$(source "$SCRIPT_DIR/lib.sh"; psql_test -c "SELECT created_at FROM _test_meta WHERE id = 1")"
  if [ "$first_created" != "$second_created" ]; then
    echo "  FAIL: ensure rebuilt DB despite unchanged fingerprint"
    return 1
  fi
  echo "  PASS: ensure is idempotent on unchanged fingerprint"
}

test_ensure_concurrent_calls_serialize() {
  # shellcheck source=lib.sh
  source "$SCRIPT_DIR/lib.sh"
  local db
  db="$(worktree_db_name)"
  psql_admin -c "SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname = '$db' AND pid <> pg_backend_pid()" >/dev/null
  psql_admin -c "DROP DATABASE IF EXISTS \"$db\"" >/dev/null
  rm -f "/tmp/skadia-test-db-$(worktree_slug).lock"

  local pid1 pid2 start2 end2
  ( SKADIA_TEST_LOCK_HOLD_SECONDS=2 "$TEST_DB_SH" ensure \
      >/tmp/test_db_concurrent_1.log 2>&1 ) &
  pid1=$!
  sleep 0.5
  start2=$(date +%s%N)
  ( "$TEST_DB_SH" ensure >/tmp/test_db_concurrent_2.log 2>&1 ) &
  pid2=$!
  wait "$pid1"
  wait "$pid2"
  end2=$(date +%s%N)

  local pid2_wait_ms=$(( (end2 - start2) / 1000000 ))
  if [ "$pid2_wait_ms" -lt 1000 ]; then
    echo "  FAIL: pid2 finished in ${pid2_wait_ms}ms - lock did not block"
    return 1
  fi
  if ! grep -q "is current" /tmp/test_db_concurrent_2.log; then
    echo "  FAIL: pid2 did not see 'is current' - race or rebuild loop"
    cat /tmp/test_db_concurrent_2.log
    return 1
  fi

  rm -f /tmp/test_db_concurrent_1.log /tmp/test_db_concurrent_2.log
  echo "  PASS: concurrent ensure serializes (pid2 waited ${pid2_wait_ms}ms)"
}

test_run_sets_both_urls_in_subprocess() {
  "$TEST_DB_SH" ensure >/dev/null
  local out expected_test expected_main
  out="$("$TEST_DB_SH" run -- env | grep -E '^(TEST_DATABASE_URL|DATABASE_URL)=' | sort)"
  expected_test="$("$TEST_DB_SH" emit-url | sed 's/^export //; s/"$//; s/="/=/')"
  expected_main="$(echo "$expected_test" | sed 's/^TEST_DATABASE_URL=/DATABASE_URL=/')"
  if [ "$out" != "$(printf '%s\n%s\n' "$expected_main" "$expected_test" | sort)" ]; then
    echo "  FAIL: run did not set both URLs"
    echo "    got: $out"
    return 1
  fi
  echo "  PASS: run sets both DATABASE_URL and TEST_DATABASE_URL"
}

test_run_passes_through_exit_code() {
  set +e
  "$TEST_DB_SH" run -- bash -c "exit 42"
  local code=$?
  set -e
  if [ "$code" -ne 42 ]; then
    echo "  FAIL: exit code passthrough (got $code, want 42)"
    return 1
  fi
  echo "  PASS: run passes through exit code"
}

test_drop_removes_db() {
  "$TEST_DB_SH" ensure >/dev/null
  "$TEST_DB_SH" drop >/dev/null
  local db exists
  db="$(source "$SCRIPT_DIR/lib.sh"; worktree_db_name)"
  exists="$(source "$SCRIPT_DIR/lib.sh"; psql_admin -c "SELECT 1 FROM pg_database WHERE datname = '$db'" 2>/dev/null || true)"
  if [ -n "$exists" ]; then
    echo "  FAIL: DB $db still exists after drop"
    return 1
  fi
  echo "  PASS: drop removes DB"
}

test_drop_idempotent() {
  "$TEST_DB_SH" drop >/dev/null
  "$TEST_DB_SH" drop >/dev/null
  echo "  PASS: drop is idempotent"
}

test_inspect_lists_current_db() {
  "$TEST_DB_SH" ensure >/dev/null
  local db out
  db="$(source "$SCRIPT_DIR/lib.sh"; worktree_db_name)"
  out="$("$TEST_DB_SH" inspect)"
  if ! echo "$out" | grep -q "$db"; then
    echo "  FAIL: inspect did not list current DB"
    return 1
  fi
  echo "  PASS: inspect lists current DB"
}

test_gc_drops_zombie_db() {
  "$TEST_DB_SH" ensure >/dev/null
  # shellcheck source=lib.sh
  source "$SCRIPT_DIR/lib.sh"
  local zombie="skadia_test_zombie000000"
  psql_admin --set=zombie="$zombie" --set=owner="$SKADIA_TEST_PG_USER" <<'SQL' >/dev/null
SELECT pg_terminate_backend(pid)
FROM pg_stat_activity
WHERE datname = :'zombie' AND pid <> pg_backend_pid();
DROP DATABASE IF EXISTS :"zombie";
CREATE DATABASE :"zombie" OWNER :"owner";
SQL
  PGPASSWORD="$SKADIA_TEST_PG_PASSWORD" psql \
    -h "$SKADIA_TEST_PG_HOST" -p "$SKADIA_TEST_PG_PORT" \
    -U "$SKADIA_TEST_PG_USER" -d "$zombie" \
    -v ON_ERROR_STOP=1 --no-psqlrc >/dev/null <<'SQL'
CREATE TABLE _test_meta (
  id INT PRIMARY KEY DEFAULT 1 CHECK (id = 1),
  worktree_path TEXT,
  branch TEXT,
  migrations_fingerprint TEXT,
  created_at TIMESTAMPTZ DEFAULT now(),
  last_used_at TIMESTAMPTZ DEFAULT now()
);
INSERT INTO _test_meta (worktree_path, branch, migrations_fingerprint)
VALUES ('/tmp/nonexistent-zombie-path', 'feature/zombie', 'deadbeef');
SQL

  "$TEST_DB_SH" gc >/dev/null

  local still_there
  still_there="$(psql_admin --set=zombie="$zombie" <<'SQL' 2>/dev/null || true
SELECT 1 FROM pg_database WHERE datname = :'zombie';
SQL
)"
  if [ -n "$still_there" ]; then
    echo "  FAIL: gc did not drop zombie DB"
    return 1
  fi
  echo "  PASS: gc drops zombie DB"
}

run_test test_help_exits_zero
run_test test_emit_url_format
run_test test_slug_stable
run_test test_slug_same_for_home_aliases
run_test test_fingerprint_changes_on_content_edit
run_test test_fingerprint_changes_on_filename_only_edit
run_test test_ensure_creates_db_first_call
run_test test_ensure_idempotent_when_fingerprint_unchanged
run_test test_ensure_concurrent_calls_serialize
run_test test_run_sets_both_urls_in_subprocess
run_test test_run_passes_through_exit_code
run_test test_drop_removes_db
run_test test_drop_idempotent
run_test test_inspect_lists_current_db
run_test test_gc_drops_zombie_db

echo ""
echo "=== all tests passed ==="
