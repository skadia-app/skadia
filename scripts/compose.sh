#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd -P)"
cd "$ROOT_DIR"

ENV_FILE="${SKADIA_ENV_FILE:-.env}"

env_get_file() {
    local key="$1"
    [ -f "$ENV_FILE" ] || return 1
    awk -F= -v key="$key" '$1 == key { sub(/^[^=]*=/, ""); print; found=1 } END { exit found ? 0 : 1 }' "$ENV_FILE"
}

env_set_file() {
    local key="$1"
    local value="$2"
    local tmp
    tmp="$(mktemp "${TMPDIR:-/tmp}/skadia-env.XXXXXX")"
    chmod 600 "$tmp"
    if [ -f "$ENV_FILE" ]; then
        awk -v key="$key" -v value="$value" '
            BEGIN { done=0 }
            $0 ~ "^" key "=" {
                print key "=" value
                done=1
                next
            }
            { print }
            END {
                if (!done) print key "=" value
            }
        ' "$ENV_FILE" > "$tmp"
    else
        printf '%s=%s\n' "$key" "$value" > "$tmp"
    fi
    mv "$tmp" "$ENV_FILE"
}

refresh_build_metadata() {
    if git rev-parse --is-inside-work-tree >/dev/null 2>&1; then
        env_set_file SKADIA_GIT_COMMIT "$(git rev-parse --short=12 HEAD 2>/dev/null || printf unknown)"
        env_set_file SKADIA_GIT_COMMIT_TIME "$(git show -s --format=%cI HEAD 2>/dev/null || printf unknown)"
    else
        env_set_file SKADIA_GIT_COMMIT unknown
        env_set_file SKADIA_GIT_COMMIT_TIME unknown
    fi
    env_set_file SKADIA_BUILD_TIME "$(date -u +%Y-%m-%dT%H:%M:%SZ)"
}

needs_build_metadata_refresh() {
    case " $* " in
        *" build "*) return 0 ;;
        *" up "*) ;;
        *) return 1 ;;
    esac
    for arg in "$@"; do
        case "$arg" in
            --build|--always-recreate-deps) return 0 ;;
        esac
    done
    return 1
}

runtime="${SKADIA_CONTAINER_RUNTIME:-}"
if [ -z "$runtime" ]; then
    runtime="$(env_get_file SKADIA_CONTAINER_RUNTIME 2>/dev/null || true)"
fi
if [ -z "$runtime" ]; then
    if command -v podman >/dev/null 2>&1; then
        runtime="podman"
    elif command -v docker >/dev/null 2>&1; then
        runtime="docker"
    else
        echo "Neither podman nor docker was found in PATH." >&2
        exit 127
    fi
fi

if needs_build_metadata_refresh "$@"; then
    refresh_build_metadata
fi

exec "$runtime" compose "$@"
