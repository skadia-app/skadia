#!/usr/bin/env bash
# finalize-migration.sh - normalize branch-only migration numbers before merge.
#
# Canonical migrations on main use monotonically increasing numeric prefixes.
# Feature branches may use provisional numeric prefixes while they are in
# progress. This script finds migrations introduced by the current branch,
# compares them with BASE_REF, and renumbers the branch-only set to the next
# contiguous numbers after BASE_REF.

set -euo pipefail

usage() {
  cat <<'EOF'
Usage: scripts/finalize-migration.sh [--check|--dry-run]

Modes:
  apply    Default. Rename branch-only migration files and patch references.
  --check  Do not modify files. Exit nonzero if finalization is required.
  --dry-run
           Print the planned renumbering without modifying files.

Environment:
  BASE_REF   Base branch/ref to compare against. Default: main.
  REPO_ROOT  Repository root. Default: parent of this script directory.
EOF
}

mode="apply"
while [[ $# -gt 0 ]]; do
  case "$1" in
    --check)
      mode="check"
      ;;
    --dry-run)
      mode="dry-run"
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "finalize-migration.sh: unknown argument: $1" >&2
      usage >&2
      exit 2
      ;;
  esac
  shift
done

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd -P)"
DEFAULT_REPO_ROOT="$(git -C "$SCRIPT_DIR/.." rev-parse --show-toplevel 2>/dev/null || true)"
REPO_ROOT="${REPO_ROOT:-$DEFAULT_REPO_ROOT}"
if [[ -z "$REPO_ROOT" ]]; then
  echo "finalize-migration.sh: cannot resolve the script worktree, set REPO_ROOT explicitly" >&2
  exit 1
fi

MIG_DIR="services/api/internal/migrate/migrations"
BASE_REF="${BASE_REF:-main}"

echo "finalize-migration.sh: using REPO_ROOT=$REPO_ROOT (branch=$(git -C "$REPO_ROOT" rev-parse --abbrev-ref HEAD), base=$BASE_REF, mode=$mode)" >&2
cd "$REPO_ROOT"

if ! git rev-parse --verify --quiet "$BASE_REF^{commit}" >/dev/null; then
  echo "ERROR: base ref '$BASE_REF' does not resolve to a commit" >&2
  exit 1
fi

merge_base="$(git merge-base "$BASE_REF" HEAD)"

format_num() {
  local value="$1"
  if (( value < 1000000 )); then
    printf '%06d' "$value"
  else
    printf '%d' "$value"
  fi
}

parse_migration_path() {
  local path="$1"
  local base num topic suffix
  base="$(basename "$path")"
  if [[ ! "$base" =~ ^([0-9]{6,})_([A-Za-z0-9][A-Za-z0-9_]*)\.(up|down)\.sql$ ]]; then
    return 1
  fi
  num="${BASH_REMATCH[1]}"
  topic="${BASH_REMATCH[2]}"
  suffix="${BASH_REMATCH[3]}"
  printf '%s\t%s\t%s\n' "$num" "$topic" "$suffix"
}

path_exists_at_ref() {
  local ref="$1"
  local path="$2"
  git cat-file -e "$ref:$path" 2>/dev/null
}

latest_base=0
while IFS= read -r path; do
  parsed="$(parse_migration_path "$path" || true)"
  [[ -n "${parsed:-}" ]] || continue
  IFS=$'\t' read -r num _topic suffix <<<"$parsed"
  [[ "$suffix" == "up" ]] || continue
  value=$((10#$num))
  if (( value > latest_base )); then
    latest_base="$value"
  fi
done < <(
  git ls-tree -r --name-only "$BASE_REF" -- "$MIG_DIR" \
    | grep -E "^${MIG_DIR}/[0-9]{6,}_[A-Za-z0-9][A-Za-z0-9_]*\.(up|down)\.sql$" || true
)

if (( latest_base == 0 )); then
  echo "ERROR: cannot determine latest migration number on $BASE_REF" >&2
  exit 1
fi

errors=()
while IFS= read -r line; do
  [[ -n "$line" ]] || continue
  status="${line%%$'\t'*}"
  rest="${line#*$'\t'}"
  path="$rest"
  if [[ "$status" == R* ]]; then
    path="${rest#*$'\t'}"
  fi
  parsed="$(parse_migration_path "$path" || true)"
  [[ -n "${parsed:-}" ]] || continue
  if [[ "$status" != A* ]]; then
    errors+=("existing migration SQL changed in branch: $path")
  fi
done < <(git diff --name-status "$merge_base"..HEAD -- "$MIG_DIR")

declare -A group_num=()
declare -A group_topic=()
declare -A group_up=()
declare -A group_down=()
groups=()

while IFS= read -r path; do
  parsed="$(parse_migration_path "$path" || true)"
  [[ -n "${parsed:-}" ]] || continue
  if path_exists_at_ref "$merge_base" "$path"; then
    continue
  fi
  IFS=$'\t' read -r num topic suffix <<<"$parsed"
  key="${num}|${topic}"
  if [[ -z "${group_num[$key]:-}" ]]; then
    groups+=("$key")
    group_num[$key]="$num"
    group_topic[$key]="$topic"
  fi
  case "$suffix" in
    up) group_up[$key]="$path" ;;
    down) group_down[$key]="$path" ;;
  esac
done < <(
  git ls-tree -r --name-only HEAD -- "$MIG_DIR" \
    | grep -E "^${MIG_DIR}/[0-9]{6,}_[A-Za-z0-9][A-Za-z0-9_]*\.(up|down)\.sql$" || true
)

if [[ ${#groups[@]} -eq 0 ]]; then
  if [[ ${#errors[@]} -gt 0 ]]; then
    printf 'ERROR: %s\n' "${errors[@]}" >&2
    exit 1
  fi
  echo "No branch-only migrations in branch vs $BASE_REF. No-op."
  exit 0
fi

declare -A topic_seen=()
declare -A num_count=()
for key in "${groups[@]}"; do
  num="${group_num[$key]}"
  topic="${group_topic[$key]}"
  if [[ -z "${group_up[$key]:-}" ]]; then
    errors+=("missing .up.sql for branch migration ${num}_${topic}")
  fi
  if [[ -z "${group_down[$key]:-}" ]]; then
    errors+=("missing .down.sql for branch migration ${num}_${topic}")
  fi
  if [[ -n "${topic_seen[$topic]:-}" ]]; then
    errors+=("topic appears with multiple migration numbers: $topic")
  fi
  topic_seen[$topic]=1
  num_count[$num]=$(( ${num_count[$num]:-0} + 1 ))
done

for key in "${groups[@]}"; do
  num="${group_num[$key]}"
  topic="${group_topic[$key]}"
  if (( ${num_count[$num]} > 1 )) && [[ ! "$topic" =~ ^[0-9]{2}_ ]]; then
    errors+=("same provisional number $num has multiple topics; use 01_/02_ topic prefixes to define apply order")
  fi
done

if [[ ${#errors[@]} -gt 0 ]]; then
  printf 'ERROR: %s\n' "${errors[@]}" >&2
  exit 1
fi

mapfile -t ordered_keys < <(
  for key in "${groups[@]}"; do
    printf '%d\t%s\t%s\n' "$((10#${group_num[$key]}))" "${group_topic[$key]}" "$key"
  done | sort -n -k1,1 -k2,2 | cut -f3-
)

declare -A new_num=()
declare -A old_token=()
declare -A new_token=()
next=$(( latest_base + 1 ))
changes_needed=0

for key in "${ordered_keys[@]}"; do
  assigned="$(format_num "$next")"
  new_num[$key]="$assigned"
  old_token[$key]="${group_num[$key]}_${group_topic[$key]}"
  new_token[$key]="${assigned}_${group_topic[$key]}"
  if [[ "${group_num[$key]}" != "$assigned" ]]; then
    changes_needed=1
  fi
  next=$(( next + 1 ))
done

echo "Latest migration on $BASE_REF: $(format_num "$latest_base")"
echo "Branch-only migration plan:"
for key in "${ordered_keys[@]}"; do
  if [[ "${group_num[$key]}" == "${new_num[$key]}" ]]; then
    echo "  ${old_token[$key]} -> ${new_token[$key]} (already final)"
  else
    echo "  ${old_token[$key]} -> ${new_token[$key]}"
  fi
done

if [[ "$mode" == "check" ]]; then
  if (( changes_needed )); then
    echo "finalization required: run scripts/finalize-migration.sh and commit the renumbered files" >&2
    exit 1
  fi
  echo "Migration numbering is finalized for branch vs $BASE_REF."
  exit 0
fi

if [[ "$mode" == "dry-run" ]]; then
  if (( changes_needed )); then
    echo "Dry run only; no files changed."
  else
    echo "No renumbering required."
  fi
  exit 0
fi

if (( ! changes_needed )); then
  echo "No renumbering required."
  exit 0
fi

mapfile -t affected < <(
  git diff --name-only "$merge_base"..HEAD -- \
      '*.go' '*.ts' '*.tsx' '*.md' '*.sql' '*.yml' '*.yaml' '*.sh' 2>/dev/null \
    | sort -u
)

for f in "${affected[@]}"; do
  [[ -f "$f" ]] || continue
  if [[ -L "$f" ]]; then
    echo "  skip symlink: $f"
    continue
  fi
  for key in "${ordered_keys[@]}"; do
    sed -i "s/${old_token[$key]}/${new_token[$key]}/g" "$f"
  done
done

tmp_paths=()
target_paths=()
declare -A rename_sources=()

mark_rename_source() {
  local old="$1"
  local new="$2"
  [[ "$old" != "$new" ]] || return 0
  [[ -e "$old" ]] || return 0
  rename_sources[$old]=1
}

queue_rename() {
  local old="$1"
  local new="$2"
  local tmp
  [[ "$old" != "$new" ]] || return 0
  if [[ ! -e "$old" ]]; then
    return 0
  fi
  if [[ -e "$new" && -z "${rename_sources[$new]:-}" ]]; then
    echo "ERROR: target already exists and is not part of this renumbering: $new" >&2
    exit 1
  fi
  tmp="$(dirname "$old")/.finalize-migration-$BASHPID-$(basename "$old")"
  tmp_paths+=("$tmp")
  target_paths+=("$new")
  git mv "$old" "$tmp"
}

finish_rename() {
  local index="$1"
  git mv "${tmp_paths[$index]}" "${target_paths[$index]}"
  echo "  ${target_paths[$index]}"
}

for key in "${ordered_keys[@]}"; do
  old_num="${group_num[$key]}"
  topic="${group_topic[$key]}"
  assigned="${new_num[$key]}"
  for suffix in up down; do
    mark_rename_source "$MIG_DIR/${old_num}_${topic}.${suffix}.sql" "$MIG_DIR/${assigned}_${topic}.${suffix}.sql"
  done
  mark_rename_source "$MIG_DIR/migration_${old_num}_${topic}_test.go" "$MIG_DIR/migration_${assigned}_${topic}_test.go"
  if [[ -f "$MIG_DIR/migration_${old_num}_test.go" && -z "${rename_sources[$MIG_DIR/migration_${old_num}_test.go]:-}" ]]; then
    if (( ${num_count[$old_num]} > 1 )); then
      echo "ERROR: generic test file is ambiguous for duplicated provisional number: $MIG_DIR/migration_${old_num}_test.go" >&2
      exit 1
    fi
    if path_exists_at_ref "$merge_base" "$MIG_DIR/migration_${old_num}_test.go"; then
      continue
    fi
    mark_rename_source "$MIG_DIR/migration_${old_num}_test.go" "$MIG_DIR/migration_${assigned}_test.go"
  fi
done

for key in "${ordered_keys[@]}"; do
  old_num="${group_num[$key]}"
  topic="${group_topic[$key]}"
  assigned="${new_num[$key]}"
  for suffix in up down; do
    queue_rename "$MIG_DIR/${old_num}_${topic}.${suffix}.sql" "$MIG_DIR/${assigned}_${topic}.${suffix}.sql"
  done

  topic_test_old="$MIG_DIR/migration_${old_num}_${topic}_test.go"
  topic_test_new="$MIG_DIR/migration_${assigned}_${topic}_test.go"
  queue_rename "$topic_test_old" "$topic_test_new"

  generic_test_old="$MIG_DIR/migration_${old_num}_test.go"
  generic_test_new="$MIG_DIR/migration_${assigned}_test.go"
  if [[ -f "$generic_test_old" && -n "${rename_sources[$generic_test_old]:-}" ]]; then
    queue_rename "$generic_test_old" "$generic_test_new"
  fi
done

if [[ ${#tmp_paths[@]} -gt 0 ]]; then
  echo "Renamed files:"
  for i in "${!tmp_paths[@]}"; do
    finish_rename "$i"
  done
fi

echo
echo "Renumber complete. Verify with:"
echo "  git status"
echo "  git diff --stat"
echo "  BASE_REF=$BASE_REF scripts/finalize-migration.sh --check"
echo "  cd services/api && go test ./internal/migrate/..."
