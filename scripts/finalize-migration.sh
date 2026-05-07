#!/usr/bin/env bash
# finalize-migration.sh — Renumbert Platzhalter-Migrations (`000999_*`) im
# aktuellen Branch auf die nächste freie sequenzielle Nummer ab `main`.
#
# Hintergrund: parallele Feature-Branches/Worktrees würden bei einem statischen
# `next-free-number`-Ansatz (z. B. beide reservieren 000333) beim Merge
# kollidieren. Stattdessen schreiben Branches ihre Migration mit der Reserve-
# Nummer `000999`, und kurz vor dem Merge — nach `git rebase main` — vergibt
# dieses Skript die echte sequenzielle Nummer ab dem aktuellen `main`-Stand.
#
# Was es macht:
#   1. Findet alle `000999_<topic>.{up,down}.sql` im Diff vs. main.
#   2. Ermittelt die höchste committed Migration-Nummer auf main.
#   3. Vergibt pro unique Topic eine sequenzielle Nummer (next, next+1, …).
#   4. Renamed Migration-Files via `git mv`.
#   5. Renamed `migration_000999_<topic>_test.go` falls vorhanden.
#   6. Patcht alle Code-/Doc-/Test-Verweise im Branch-Diff (`*.go`, `*.ts`,
#      `*.tsx`, `*.md`, `*.sql`).
#   7. Warnt bei bare `000999`-Verweisen (Plan-Body-Erwähnungen, Kommentare),
#      die manuell reviewt werden sollten.
#
# Idempotent: keine Platzhalter im Branch → no-op + Exit 0.
#
# Workflow:
#   git rebase main                   # bring branch up to date
#   ./scripts/finalize-migration.sh   # renumber placeholders
#   go test ./internal/migrate/...    # verify renumbered tests pass
#   git add -u && git commit --amend  # fold renumber into existing migration commit
#
# Multi-Migration-Branch (z. B. zwei Migrations im selben Branch): pro
# Topic eine eigene Nummer, in alphabetischer Topic-Reihenfolge — schreibe
# den Branch so, dass die Reihenfolge der Migrations alphabetisch deinem
# gewünschten Apply-Order entspricht (oder benutze Prefixes wie `01_`, `02_`
# im Topic-Slug während der Branch-Lebensdauer).

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd -P)"
DEFAULT_REPO_ROOT="$(git -C "$SCRIPT_DIR/.." rev-parse --show-toplevel 2>/dev/null || true)"
REPO_ROOT="${REPO_ROOT:-$DEFAULT_REPO_ROOT}"
if [[ -z "$REPO_ROOT" ]]; then
    echo "finalize-migration.sh: cannot resolve the script worktree, set REPO_ROOT explicitly" >&2
    exit 1
fi
MIG_DIR="services/api/internal/migrate/migrations"
BASE_REF="${BASE_REF:-main}"
PLACEHOLDER="000999"

echo "finalize-migration.sh: using REPO_ROOT=$REPO_ROOT (branch=$(git -C "$REPO_ROOT" rev-parse --abbrev-ref HEAD))" >&2
cd "$REPO_ROOT"

# Step 1: Find placeholder migrations in current branch (vs main).
mapfile -t placeholders < <(
    git diff --name-only "$BASE_REF"...HEAD -- "$MIG_DIR" 2>/dev/null \
        | grep -E "$MIG_DIR/${PLACEHOLDER}_.*\.(up|down)\.sql$" \
        | sort -u
)
if [[ ${#placeholders[@]} -eq 0 ]]; then
    echo "No placeholder migrations (${PLACEHOLDER}_*) in branch vs $BASE_REF. No-op."
    exit 0
fi

echo "Found ${#placeholders[@]} placeholder migration file(s):"
printf '  %s\n' "${placeholders[@]}"
echo

# Step 2: Highest committed migration on $BASE_REF.
latest_base=$(
    git ls-tree -r --name-only "$BASE_REF" -- "$MIG_DIR" \
        | grep -oE "^${MIG_DIR}/[0-9]{6}_" \
        | grep -oE "[0-9]{6}" \
        | sort -u | tail -1
)
if [[ -z "$latest_base" ]]; then
    echo "ERROR: cannot determine latest migration number on $BASE_REF" >&2
    exit 1
fi
next=$(printf "%06d" $((10#$latest_base + 1)))
echo "Latest migration on $BASE_REF: $latest_base — first available: $next"
echo

# Step 3: Group placeholders by topic (suffix after PLACEHOLDER_).
declare -A topics_seen=()
topics=()
for p in "${placeholders[@]}"; do
    base=$(basename "$p")
    topic="${base#${PLACEHOLDER}_}"
    topic="${topic%.up.sql}"
    topic="${topic%.down.sql}"
    if [[ -z "${topics_seen[$topic]:-}" ]]; then
        topics+=("$topic")
        topics_seen[$topic]=1
    fi
done
# Sort topics for deterministic ordering across runs.
mapfile -t topics < <(printf '%s\n' "${topics[@]}" | sort)

# Step 4: Assign + rename.
declare -A topic_to_num=()
for topic in "${topics[@]}"; do
    topic_to_num[$topic]="$next"
    next=$(printf "%06d" $((10#$next + 1)))
done

for topic in "${topics[@]}"; do
    new_num="${topic_to_num[$topic]}"
    echo "Renumbering '$topic': $PLACEHOLDER -> $new_num"

    for suffix in up down; do
        old="$MIG_DIR/${PLACEHOLDER}_${topic}.${suffix}.sql"
        new="$MIG_DIR/${new_num}_${topic}.${suffix}.sql"
        if [[ -f "$old" ]]; then
            git mv "$old" "$new"
            echo "  $old -> $new"
        fi
    done

    # 4b. Optional Test-File.
    test_old="$MIG_DIR/migration_${PLACEHOLDER}_${topic}_test.go"
    test_new="$MIG_DIR/migration_${new_num}_${topic}_test.go"
    if [[ -f "$test_old" ]]; then
        git mv "$test_old" "$test_new"
        echo "  $test_old -> $test_new"
    fi
    # Generic migration_NNN_test.go (no topic suffix) is renamed only if
    # the file is in the current branch's diff (avoid touching unrelated tests).
    test_generic_old="$MIG_DIR/migration_${PLACEHOLDER}_test.go"
    test_generic_new="$MIG_DIR/migration_${new_num}_test.go"
    if [[ -f "$test_generic_old" ]] \
       && git diff --name-only "$BASE_REF"...HEAD -- "$test_generic_old" | grep -q .; then
        git mv "$test_generic_old" "$test_generic_new"
        echo "  $test_generic_old -> $test_generic_new"
    fi
done

# Step 5: Patch references in branch-diff files.
mapfile -t affected < <(
    git diff --name-only "$BASE_REF"...HEAD -- '*.go' '*.ts' '*.tsx' '*.md' '*.sql' 2>/dev/null \
        | xargs -r grep -l "${PLACEHOLDER}_" 2>/dev/null \
        | sort -u
)
for f in "${affected[@]}"; do
    [[ -f "$f" ]] || continue
    for topic in "${topics[@]}"; do
        new_num="${topic_to_num[$topic]}"
        sed -i "s/${PLACEHOLDER}_${topic}/${new_num}_${topic}/g" "$f"
    done
    # Also handle migration_NNN_test.go-style references.
    for topic in "${topics[@]}"; do
        new_num="${topic_to_num[$topic]}"
        # Topic-less but PLACEHOLDER-prefixed test names get the topic's number.
        # Only if exactly one topic — otherwise ambiguous, leave for manual fix.
        if [[ ${#topics[@]} -eq 1 ]]; then
            sed -i "s/migration_${PLACEHOLDER}_test/migration_${new_num}_test/g" "$f"
        fi
    done
done

# Step 6: Bare PLACEHOLDER references (e.g., "Migration 000999" in prose).
bare_hits=$(
    git diff "$BASE_REF"...HEAD -- '*.go' '*.ts' '*.tsx' '*.md' \
        | grep -nE "(^\\+).*\\b${PLACEHOLDER}\\b" \
        | grep -v "${PLACEHOLDER}_" || true
)
if [[ -n "$bare_hits" ]]; then
    echo
    echo "WARN: bare '${PLACEHOLDER}' references in branch diff — review manually:"
    echo "$bare_hits" | head -20
    echo "(showing first 20 — use 'git grep ${PLACEHOLDER}' for full list)"
fi

echo
echo "Renumber complete. Verify with:"
echo "  git status"
echo "  git diff --stat"
echo "  cd services/api && go test ./internal/migrate/..."
echo "Then: git add -u && git commit --amend (or new commit if separate from migration content)"
