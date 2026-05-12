#!/usr/bin/env bash
# Smoke tests for scripts/finalize-migration.sh. Each test creates a temporary
# Git repository so branch/base diff behavior is exercised with real git state.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd -P)"
SCRIPT_UNDER_TEST="${SCRIPT_UNDER_TEST:-$SCRIPT_DIR/../finalize-migration.sh}"
TMP_DIRS=()

cleanup() {
  if [[ ${#TMP_DIRS[@]} -gt 0 ]]; then
    rm -rf "${TMP_DIRS[@]}"
  fi
}
trap cleanup EXIT

run_test() {
  echo "=== $1 ==="
  "$1"
  echo "  PASS: $1"
}

fail() {
  echo "  FAIL: $*" >&2
  return 1
}

assert_file() {
  [[ -f "$1" ]] || fail "missing file: $1"
}

assert_no_file() {
  [[ ! -e "$1" ]] || fail "unexpected file remains: $1"
}

assert_contains() {
  local file="$1"
  local needle="$2"
  grep -Fq "$needle" "$file" || fail "$file does not contain $needle"
}

new_repo() {
  local repo
  repo="$(mktemp -d)"
  TMP_DIRS+=("$repo")
  git -C "$repo" init -q -b main
  git -C "$repo" config user.email "test@example.invalid"
  git -C "$repo" config user.name "Test User"
  mkdir -p "$repo/services/api/internal/migrate/migrations"
  add_migration "$repo" "000360" "base"
  git -C "$repo" add .
  git -C "$repo" commit -q -m "base migrations"
  git -C "$repo" switch -q -c feature
  printf '%s\n' "$repo"
}

add_migration() {
  local repo="$1"
  local num="$2"
  local topic="$3"
  local dir="$repo/services/api/internal/migrate/migrations"
  printf -- '-- up %s_%s\n' "$num" "$topic" > "$dir/${num}_${topic}.up.sql"
  printf -- '-- down %s_%s\n' "$num" "$topic" > "$dir/${num}_${topic}.down.sql"
}

add_migration_test() {
  local repo="$1"
  local num="$2"
  local topic="$3"
  local dir="$repo/services/api/internal/migrate/migrations"
  printf 'package migrations_test\n\nconst migrationRef = "%s_%s"\n' "$num" "$topic" \
    > "$dir/migration_${num}_${topic}_test.go"
}

finalize() {
  local repo="$1"
  shift
  REPO_ROOT="$repo" BASE_REF=main "$SCRIPT_UNDER_TEST" "$@"
}

test_check_fails_for_unfinalized_branch_migration() {
  local repo out code
  repo="$(new_repo)"
  add_migration "$repo" "900001" "alpha"
  git -C "$repo" add .
  git -C "$repo" commit -q -m "add provisional migration"

  set +e
  out="$(finalize "$repo" --check 2>&1)"
  code=$?
  set -e

  [[ "$code" -ne 0 ]] || fail "--check succeeded despite unfinalized migration"
  [[ "$out" == *"finalization required"* ]] || fail "missing finalization-required message: $out"
}

test_finalize_concrete_branch_numbers_and_references() {
  local repo dir
  repo="$(new_repo)"
  dir="$repo/services/api/internal/migrate/migrations"
  add_migration "$repo" "900001" "alpha"
  add_migration "$repo" "900002" "beta"
  add_migration_test "$repo" "900001" "alpha"
  printf '900001_alpha\n900002_beta\n' > "$repo/notes.md"
  git -C "$repo" add .
  git -C "$repo" commit -q -m "add provisional migrations"

  finalize "$repo" >/dev/null

  assert_file "$dir/000361_alpha.up.sql"
  assert_file "$dir/000361_alpha.down.sql"
  assert_file "$dir/000362_beta.up.sql"
  assert_file "$dir/000362_beta.down.sql"
  assert_file "$dir/migration_000361_alpha_test.go"
  assert_no_file "$dir/900001_alpha.up.sql"
  assert_no_file "$dir/migration_900001_alpha_test.go"
  assert_contains "$repo/notes.md" "000361_alpha"
  assert_contains "$repo/notes.md" "000362_beta"
  assert_contains "$dir/migration_000361_alpha_test.go" "000361_alpha"
}

test_refinalizes_colliding_branch_number_after_base_advances() {
  local repo dir
  repo="$(new_repo)"
  dir="$repo/services/api/internal/migrate/migrations"
  add_migration "$repo" "000361" "alpha"
  git -C "$repo" add .
  git -C "$repo" commit -q -m "add finalized migration"
  git -C "$repo" switch -q main
  add_migration "$repo" "000361" "other"
  git -C "$repo" add .
  git -C "$repo" commit -q -m "advance base"
  git -C "$repo" switch -q feature

  finalize "$repo" >/dev/null

  assert_file "$dir/000362_alpha.up.sql"
  assert_file "$dir/000362_alpha.down.sql"
  assert_no_file "$dir/000361_alpha.up.sql"
}

test_same_number_without_order_prefix_fails() {
  local repo out code
  repo="$(new_repo)"
  add_migration "$repo" "900001" "alpha"
  add_migration "$repo" "900001" "beta"
  git -C "$repo" add .
  git -C "$repo" commit -q -m "add ambiguous provisional migrations"

  set +e
  out="$(finalize "$repo" 2>&1)"
  code=$?
  set -e

  [[ "$code" -ne 0 ]] || fail "ambiguous same-number migrations were accepted"
  [[ "$out" == *"same provisional number"* ]] || fail "missing duplicate-number error: $out"
}

test_same_number_with_order_prefix_finalizes_in_topic_order() {
  local repo dir
  repo="$(new_repo)"
  dir="$repo/services/api/internal/migrate/migrations"
  add_migration "$repo" "900001" "02_beta"
  add_migration "$repo" "900001" "01_alpha"
  git -C "$repo" add .
  git -C "$repo" commit -q -m "add ordered provisional migrations"

  finalize "$repo" >/dev/null

  assert_file "$dir/000361_01_alpha.up.sql"
  assert_file "$dir/000362_02_beta.up.sql"
  assert_no_file "$dir/900001_01_alpha.up.sql"
  assert_no_file "$dir/900001_02_beta.up.sql"
}

test_base_999999_finalizes_to_seven_digits() {
  local repo dir
  repo="$(mktemp -d)"
  TMP_DIRS+=("$repo")
  git -C "$repo" init -q -b main
  git -C "$repo" config user.email "test@example.invalid"
  git -C "$repo" config user.name "Test User"
  mkdir -p "$repo/services/api/internal/migrate/migrations"
  add_migration "$repo" "999999" "base"
  git -C "$repo" add .
  git -C "$repo" commit -q -m "base migrations"
  git -C "$repo" switch -q -c feature
  add_migration "$repo" "1000001" "alpha"
  git -C "$repo" add .
  git -C "$repo" commit -q -m "add provisional migration"
  dir="$repo/services/api/internal/migrate/migrations"

  finalize "$repo" >/dev/null

  assert_file "$dir/1000000_alpha.up.sql"
  assert_file "$dir/1000000_alpha.down.sql"
  assert_no_file "$dir/1000001_alpha.up.sql"
}

test_placeholder_000999_still_finalizes() {
  local repo dir
  repo="$(new_repo)"
  dir="$repo/services/api/internal/migrate/migrations"
  add_migration "$repo" "000999" "alpha"
  git -C "$repo" add .
  git -C "$repo" commit -q -m "add legacy placeholder migration"

  finalize "$repo" >/dev/null

  assert_file "$dir/000361_alpha.up.sql"
  assert_no_file "$dir/000999_alpha.up.sql"
}

test_check_passes_after_finalize_commit() {
  local repo
  repo="$(new_repo)"
  add_migration "$repo" "900001" "alpha"
  git -C "$repo" add .
  git -C "$repo" commit -q -m "add provisional migration"

  finalize "$repo" >/dev/null
  git -C "$repo" add .
  git -C "$repo" commit -q -m "finalize migration"
  finalize "$repo" --check >/dev/null
}

run_test test_check_fails_for_unfinalized_branch_migration
run_test test_finalize_concrete_branch_numbers_and_references
run_test test_refinalizes_colliding_branch_number_after_base_advances
run_test test_same_number_without_order_prefix_fails
run_test test_same_number_with_order_prefix_finalizes_in_topic_order
run_test test_base_999999_finalizes_to_seven_digits
run_test test_placeholder_000999_still_finalizes
run_test test_check_passes_after_finalize_commit

echo
echo "=== all finalize-migration tests passed ==="
