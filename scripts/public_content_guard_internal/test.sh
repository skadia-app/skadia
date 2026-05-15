#!/usr/bin/env bash
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
HOOK="${SKADIA_PUBLIC_CONTENT_GUARD:-$REPO_ROOT/.githooks/public-content-guard}"

LAST_STATUS=0
LAST_OUTPUT=""

fail() {
    echo "FAIL: $*" >&2
    exit 1
}

run_guard() {
    local diff_text="$1"
    local patterns_file="${2:-}"
    local diff_file
    diff_file="$(mktemp)"
    printf '%s\n' "$diff_text" > "$diff_file"
    set +e
    if [ -n "$patterns_file" ]; then
        LAST_OUTPUT="$(SKADIA_PUBLIC_CONTENT_PRIVATE_PATTERNS="$patterns_file" "$HOOK" --diff-file "$diff_file" 2>&1)"
    else
        LAST_OUTPUT="$("$HOOK" --diff-file "$diff_file" 2>&1)"
    fi
    LAST_STATUS=$?
    set -e
    rm -f "$diff_file"
}

assert_passes() {
    local diff_text="$1"
    run_guard "$diff_text"
    if [ "$LAST_STATUS" -ne 0 ]; then
        fail "expected diff to pass:
$LAST_OUTPUT"
    fi
}

assert_blocks_with() {
    local diff_text="$1"
    local needle="$2"
    local patterns_file="${3:-}"
    run_guard "$diff_text" "$patterns_file"
    if [ "$LAST_STATUS" -eq 0 ]; then
        fail "expected diff to be blocked"
    fi
    case "$LAST_OUTPUT" in
        *"$needle"*) ;;
        *)
            fail "expected output to contain '$needle':
$LAST_OUTPUT"
            ;;
    esac
}

public_changelog_diff='diff --git a/CHANGELOG.md b/CHANGELOG.md
--- a/CHANGELOG.md
+++ b/CHANGELOG.md
@@ -1,1 +1,2 @@
+- Fixed route around 48.0000, 11.0000.'

public_degree_coordinate_diff=$'diff --git a/CHANGELOG.md b/CHANGELOG.md\n--- a/CHANGELOG.md\n+++ b/CHANGELOG.md\n@@ -1,1 +1,2 @@\n+- Fixed route around 48.0000\302\260 N, 11.0000\302\260 E.'

public_latlon_coordinate_diff='diff --git a/CHANGELOG.md b/CHANGELOG.md
--- a/CHANGELOG.md
+++ b/CHANGELOG.md
@@ -1,1 +1,2 @@
+Fixed route around lat=48.0000 lon=11.0000.'

test_coordinate_diff='diff --git a/services/api/internal/routing/distance_test.go b/services/api/internal/routing/distance_test.go
--- a/services/api/internal/routing/distance_test.go
+++ b/services/api/internal/routing/distance_test.go
@@ -1,1 +1,2 @@
+const syntheticPoint = "48.0000, 11.0000"'

source_coordinate_diff='diff --git a/services/api/internal/routing/example.go b/services/api/internal/routing/example.go
--- a/services/api/internal/routing/example.go
+++ b/services/api/internal/routing/example.go
@@ -1,1 +1,2 @@
+const realPoint = "48.0000, 11.0000"'

host_path_diff='diff --git a/services/api/internal/example.go b/services/api/internal/example.go
--- a/services/api/internal/example.go
+++ b/services/api/internal/example.go
@@ -1,1 +1,2 @@
+const localPath = "/home/example-user/Skadia/private.fit"'

container_home_path_diff='diff --git a/docker-compose.yml b/docker-compose.yml
--- a/docker-compose.yml
+++ b/docker-compose.yml
@@ -1,1 +1,2 @@
+      - ${SKADIA_DATA_ROOT:-./data}/postgres:/home/postgres/pgdata${SKADIA_VOLUME_OPTS_RW_PRIVATE:-:Z}'

mixed_host_and_container_home_path_diff='diff --git a/docker-compose.yml b/docker-compose.yml
--- a/docker-compose.yml
+++ b/docker-compose.yml
@@ -1,1 +1,2 @@
+      - /var/home/example/Skadia/data/postgres:/home/postgres/pgdata'

public_filename_diff='diff --git a/docs/documentation/user/features/import.md b/docs/documentation/user/features/import.md
--- a/docs/documentation/user/features/import.md
+++ b/docs/documentation/user/features/import.md
@@ -1,1 +1,2 @@
+Do not mention morning-run.gpx in public examples.'

public_synthetic_filename_diff='diff --git a/docs/documentation/user/features/import.md b/docs/documentation/user/features/import.md
--- a/docs/documentation/user/features/import.md
+++ b/docs/documentation/user/features/import.md
@@ -1,1 +1,2 @@
+Use synthetic-activity.gpx in public examples.'

source_filename_diff='diff --git a/services/api/internal/imports/example.go b/services/api/internal/imports/example.go
--- a/services/api/internal/imports/example.go
+++ b/services/api/internal/imports/example.go
@@ -1,1 +1,2 @@
+const sample = "morning-run.gpx"'

public_workflow_diff='diff --git a/CHANGELOG.md b/CHANGELOG.md
--- a/CHANGELOG.md
+++ b/CHANGELOG.md
@@ -1,1 +1,2 @@
+Fixed phase 1 task 2 from cleanup plan.'

public_docs_workflow_diff='diff --git a/docs/00_OVERVIEW.md b/docs/00_OVERVIEW.md
--- a/docs/00_OVERVIEW.md
+++ b/docs/00_OVERVIEW.md
@@ -1,1 +1,2 @@
+Fixed critic findings from phase 2.'

public_internal_path_diff='diff --git a/CHANGELOG.md b/CHANGELOG.md
--- a/CHANGELOG.md
+++ b/CHANGELOG.md
@@ -1,1 +1,2 @@
+Moved cleanup notes under .private/.'

public_backtick_internal_path_diff='diff --git a/CHANGELOG.md b/CHANGELOG.md
--- a/CHANGELOG.md
+++ b/CHANGELOG.md
@@ -1,1 +1,2 @@
+Moved cleanup notes from `docs/internal/import.md`.'

public_agent_diff='diff --git a/CHANGELOG.md b/CHANGELOG.md
--- a/CHANGELOG.md
+++ b/CHANGELOG.md
@@ -1,1 +1,2 @@
+Synced OpenAI Codex GPT-5.5 notes.'

public_doc_provenance_metadata_diff='diff --git a/docs/documentation/user/how-to/example.md b/docs/documentation/user/how-to/example.md
--- a/docs/documentation/user/how-to/example.md
+++ b/docs/documentation/user/how-to/example.md
@@ -1,1 +1,2 @@
+*Last updated: 2026-05-14 (GPT-5.5)*'

public_doc_created_provenance_metadata_diff='diff --git a/docs/documentation/user/how-to/example.md b/docs/documentation/user/how-to/example.md
--- a/docs/documentation/user/how-to/example.md
+++ b/docs/documentation/user/how-to/example.md
@@ -1,1 +1,2 @@
+*Created: 2026-04-12 (Any Provider Model 123.4)*'

public_doc_wrong_metadata_agent_diff='diff --git a/docs/documentation/user/how-to/example.md b/docs/documentation/user/how-to/example.md
--- a/docs/documentation/user/how-to/example.md
+++ b/docs/documentation/user/how-to/example.md
@@ -1,1 +1,2 @@
+*Reviewed: 2026-05-14 (GPT-5.5)*'

public_doc_body_agent_diff='diff --git a/docs/documentation/user/how-to/example.md b/docs/documentation/user/how-to/example.md
--- a/docs/documentation/user/how-to/example.md
+++ b/docs/documentation/user/how-to/example.md
@@ -1,1 +1,2 @@
+This page was revised with GPT-5.5.'

public_tag_diff='diff --git a/docs/documentation/user/features/import.md b/docs/documentation/user/features/import.md
--- a/docs/documentation/user/features/import.md
+++ b/docs/documentation/user/features/import.md
@@ -1,1 +1,2 @@
+Do not expose #home in public examples.'

agents_coordinate_diff='diff --git a/AGENTS.md b/AGENTS.md
--- a/AGENTS.md
+++ b/AGENTS.md
@@ -1,1 +1,2 @@
+Keep this coordinate out: 48.0000, 11.0000.'

subtree_agents_tag_diff='diff --git a/services/api/AGENTS.md b/services/api/AGENTS.md
--- a/services/api/AGENTS.md
+++ b/services/api/AGENTS.md
@@ -1,1 +1,2 @@
+Keep #home out of agent guidance.'

legacy_private_coordinate_diff='diff --git a/docs/internal/example.md b/docs/internal/example.md
--- a/docs/internal/example.md
+++ b/docs/internal/example.md
@@ -1,1 +1,2 @@
+Private planning note near 48.0000, 11.0000.'

legacy_private_filename_diff='diff --git a/docs/internal/example.md b/docs/internal/example.md
--- a/docs/internal/example.md
+++ b/docs/internal/example.md
@@ -1,1 +1,2 @@
+Private planning note mentioning morning-run.gpx.'

legacy_private_synthetic_filename_diff='diff --git a/docs/internal/example.md b/docs/internal/example.md
--- a/docs/internal/example.md
+++ b/docs/internal/example.md
@@ -1,1 +1,2 @@
+Private planning note mentioning synthetic-run.gpx.'

legacy_review_coordinate_diff='diff --git a/docs/reviews/example.md b/docs/reviews/example.md
--- a/docs/reviews/example.md
+++ b/docs/reviews/example.md
@@ -1,1 +1,2 @@
+Private review note near 48.0000, 11.0000.'

private_patterns="$(mktemp)"
printf '%s\n' 'PrivatePlaceForContentGuardTest' > "$private_patterns"
local_private_diff='diff --git a/services/api/internal/example.go b/services/api/internal/example.go
--- a/services/api/internal/example.go
+++ b/services/api/internal/example.go
@@ -1,1 +1,2 @@
+const label = "PrivatePlaceForContentGuardTest"'

assert_blocks_with "$public_changelog_diff" "coordinate in public/private text"
assert_blocks_with "$public_degree_coordinate_diff" "coordinate in public/private text"
assert_blocks_with "$public_latlon_coordinate_diff" "coordinate in public/private text"
assert_passes "$test_coordinate_diff"
assert_blocks_with "$source_coordinate_diff" "coordinate in public/private text"
assert_blocks_with "$host_path_diff" "local/private detail"
assert_passes "$container_home_path_diff"
assert_blocks_with "$mixed_host_and_container_home_path_diff" "local/private detail"
assert_blocks_with "$public_filename_diff" "sensitive filename in public/private text"
assert_passes "$public_synthetic_filename_diff"
assert_blocks_with "$source_filename_diff" "sensitive filename in public/private text"
assert_blocks_with "$public_workflow_diff" "internal workflow wording in public text"
assert_blocks_with "$public_docs_workflow_diff" "internal workflow wording in public text"
assert_blocks_with "$public_internal_path_diff" "internal path in public text"
assert_blocks_with "$public_backtick_internal_path_diff" "internal path in public text"
assert_blocks_with "$public_agent_diff" "agent/model name in public text"
assert_passes "$public_doc_provenance_metadata_diff"
assert_passes "$public_doc_created_provenance_metadata_diff"
assert_blocks_with "$public_doc_wrong_metadata_agent_diff" "agent/model name in public text"
assert_blocks_with "$public_doc_body_agent_diff" "agent/model name in public text"
assert_blocks_with "$public_tag_diff" "private tag marker in public text"
assert_blocks_with "$agents_coordinate_diff" "coordinate in public/private text"
assert_blocks_with "$subtree_agents_tag_diff" "private tag marker in public/private text"
assert_blocks_with "$legacy_private_coordinate_diff" "coordinate in public/private text"
assert_blocks_with "$legacy_private_filename_diff" "sensitive filename in public/private text"
assert_passes "$legacy_private_synthetic_filename_diff"
assert_blocks_with "$legacy_review_coordinate_diff" "coordinate in public/private text"
assert_blocks_with "$local_private_diff" "local private pattern" "$private_patterns"

rm -f "$private_patterns"

invalid_patterns="$(mktemp)"
printf '%s\n' '[' > "$invalid_patterns"
assert_blocks_with "$test_coordinate_diff" "invalid regex" "$invalid_patterns"
rm -f "$invalid_patterns"

if [ "$(id -u)" != "0" ]; then
    unreadable_patterns="$(mktemp)"
    printf '%s\n' 'PrivatePlaceForContentGuardTest' > "$unreadable_patterns"
    chmod 000 "$unreadable_patterns"
    assert_blocks_with "$test_coordinate_diff" "not readable" "$unreadable_patterns"
    chmod 600 "$unreadable_patterns"
    rm -f "$unreadable_patterns"
fi

echo "PASS: public content guard"
