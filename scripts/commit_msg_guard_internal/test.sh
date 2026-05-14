#!/usr/bin/env bash
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
HOOK="${SKADIA_COMMIT_MSG_HOOK:-$REPO_ROOT/.githooks/commit-msg}"

LAST_STATUS=0
LAST_OUTPUT=""

fail() {
    echo "FAIL: $*" >&2
    exit 1
}

run_hook() {
    local msg="$1"
    local tmp
    tmp="$(mktemp)"
    printf '%s\n' "$msg" > "$tmp"
    set +e
    LAST_OUTPUT="$("$HOOK" "$tmp" 2>&1)"
    LAST_STATUS=$?
    set -e
    rm -f "$tmp"
}

assert_passes() {
    local msg="$1"
    run_hook "$msg"
    if [ "$LAST_STATUS" -ne 0 ]; then
        fail "expected message to pass: $msg
$LAST_OUTPUT"
    fi
}

assert_blocks_with() {
    local msg="$1"
    local needle="$2"
    run_hook "$msg"
    if [ "$LAST_STATUS" -eq 0 ]; then
        fail "expected message to be blocked: $msg"
    fi
    case "$LAST_OUTPUT" in
        *"$needle"*) ;;
        *)
            fail "expected output to contain '$needle' for: $msg
$LAST_OUTPUT"
            ;;
    esac
}

assert_passes "feat(imports): improve review queue navigation"
assert_passes "fix(api): update services/api/internal/handler.go wiring"
assert_passes "fix(api): allow longer broker task requests"
assert_passes "test(routing): add synthetic coordinate fixtures"
assert_passes $'feat(imports): keep queue counters current\n\nCo-authored-by: GPT-5.5 <noreply@openai.com>'

wf_phase="$(printf 'pha%s' se)"
wf_review="$(printf 'criti%s' c)"
dir_plan="docs/$(printf 'pla%s' ns)"
dir_review="docs/$(printf 'revie%s' ws)"
dir_private=".$(printf 'priv%s' ate)"
agent_a="$(printf 'Co%s' dex)"
agent_b="$(printf 'Open%s' AI)"
model_a="$(printf 'GPT-%s' '5.5')"
local_path="/home/$(printf 'sas%s' cha)"

assert_blocks_with "fix(imports): apply ${wf_phase} 1 task 2 from cleanup plan" "workflow term"
assert_blocks_with "fix(imports): finish ${wf_phase}1 task2 cleanup" "workflow term"
assert_blocks_with "docs: update cleanup plans" "workflow term"
assert_blocks_with "fix: fixed ${wf_review} findings" "workflow term"
assert_blocks_with "docs: update ${dir_plan} import spec" "internal path"
assert_blocks_with "docs: update ${dir_review} import notes" "internal path"
assert_blocks_with "docs: move notes to \`${dir_private}/foo.md\`" "internal path"
assert_blocks_with "docs: add ${dir_private} cleanup note" "internal path"
assert_blocks_with "chore: sync ${agent_a} notes" "agent/model name"
assert_blocks_with "chore: sync ${model_a} notes" "agent/model name"
assert_blocks_with "chore: sync ${agent_b} notes" "agent/model name"
assert_blocks_with $'feat: update\n\nCo-authored-by: '"${agent_a}"$' <noreply@openai.com>' "agent/model name"
assert_blocks_with "docs: remove ${local_path} cleanup note" "local/private detail"
assert_blocks_with "fix: adjust route near 48.0000, 11.0000" "coordinate"
assert_blocks_with "fix: adjust route near lat=48.0000 lon=11.0000" "coordinate"
assert_blocks_with $'fix: adjust route near 48.0000\302\260 N, 11.0000\302\260 E' "coordinate"
assert_blocks_with 'test: add `morning-run.gpx` fixture' "filename/path"
assert_blocks_with "fix: clean #home tag" "private tag"

private_patterns="$(mktemp)"
printf '%s\n' 'PrivatePlaceForHookTest' > "$private_patterns"
SKADIA_COMMIT_MSG_PRIVATE_PATTERNS="$private_patterns" \
    assert_blocks_with "docs: mention PrivatePlaceForHookTest example" "local private pattern"
rm -f "$private_patterns"

invalid_patterns="$(mktemp)"
printf '%s\n' '[' > "$invalid_patterns"
SKADIA_COMMIT_MSG_PRIVATE_PATTERNS="$invalid_patterns" \
    assert_blocks_with "docs: harmless wording" "invalid regex"
rm -f "$invalid_patterns"

if [ "$(id -u)" != "0" ]; then
    unreadable_patterns="$(mktemp)"
    printf '%s\n' 'PrivatePlaceForHookTest' > "$unreadable_patterns"
    chmod 000 "$unreadable_patterns"
    SKADIA_COMMIT_MSG_PRIVATE_PATTERNS="$unreadable_patterns" \
        assert_blocks_with "docs: harmless wording" "not readable"
    chmod 600 "$unreadable_patterns"
    rm -f "$unreadable_patterns"
fi

echo "PASS: commit message guard"
