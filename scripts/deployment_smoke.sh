#!/usr/bin/env bash
set -euo pipefail

RUNTIME=""
DATA_ROOT=""
NO_BUILD=false

while [ "$#" -gt 0 ]; do
    case "$1" in
        --runtime)
            shift
            RUNTIME="${1:-}"
            ;;
        --runtime=*)
            RUNTIME="${1#--runtime=}"
            ;;
        --data-root)
            shift
            DATA_ROOT="${1:-}"
            ;;
        --data-root=*)
            DATA_ROOT="${1#--data-root=}"
            ;;
        --no-build)
            NO_BUILD=true
            ;;
        -h|--help)
            echo "Usage: $0 --runtime docker|podman --data-root <path> [--no-build]"
            exit 0
            ;;
        *)
            echo "ERROR: unknown argument: $1" >&2
            exit 2
            ;;
    esac
    shift
done

case "$RUNTIME" in
    docker|podman) ;;
    *)
        echo "ERROR: --runtime must be docker or podman." >&2
        exit 2
        ;;
esac

if [ -z "$DATA_ROOT" ]; then
    echo "ERROR: --data-root is required." >&2
    exit 2
fi

path_contains_parent_traversal() {
    local path="$1"
    case "$path" in
        ".."|"../"*|*/".."|*/"../"*) return 0 ;;
        *) return 1 ;;
    esac
}

canonicalize_path() {
    local raw="$1"
    local path
    local probe
    local suffix=""
    local base

    case "$raw" in
        /*) path="$raw" ;;
        *) path="$PWD/$raw" ;;
    esac

    if command -v realpath >/dev/null 2>&1; then
        realpath -m "$path" 2>/dev/null && return 0
    fi

    probe="$path"
    while [ ! -e "$probe" ] && [ "$probe" != "/" ]; do
        suffix="/$(basename "$probe")$suffix"
        probe="$(dirname "$probe")"
    done

    if [ -d "$probe" ]; then
        base="$(cd "$probe" && pwd -P)"
    else
        base="$(cd "$(dirname "$probe")" && pwd -P)/$(basename "$probe")"
    fi
    printf '%s%s\n' "$base" "$suffix"
}

if [ -z "$DATA_ROOT" ]; then
    echo "ERROR: --data-root must not be empty." >&2
    exit 2
fi
if path_contains_parent_traversal "$DATA_ROOT"; then
    echo "ERROR: --data-root must not contain parent-directory traversal: $DATA_ROOT" >&2
    exit 2
fi

project_root="$(git rev-parse --show-toplevel 2>/dev/null || pwd)"
project_root="$(canonicalize_path "$project_root")"
project_data_root="$(canonicalize_path "$project_root/data")"
data_root_canonical="$(canonicalize_path "$DATA_ROOT")"

case "$data_root_canonical" in
    "/"|"$project_root"|"$project_data_root"|"$project_data_root"/*)
        echo "ERROR: refusing to run deployment smoke against a dangerous or project-owned data root: $DATA_ROOT" >&2
        exit 2
        ;;
esac
DATA_ROOT="$data_root_canonical"

git_dir="$(cd "$(git rev-parse --git-dir)" && pwd -P)"
git_common="$(cd "$(git rev-parse --git-common-dir)" && pwd -P)"
if [ "$git_dir" != "$git_common" ] && [ "${SKADIA_ALLOW_WORKTREE_SMOKE:-false}" != "true" ]; then
    echo "ERROR: refusing to run runtime smoke from a linked worktree." >&2
    echo "Run this script from the main checkout, or set SKADIA_ALLOW_WORKTREE_SMOKE=true for an explicit local override." >&2
    exit 1
fi

if ! command -v "$RUNTIME" >/dev/null 2>&1 || ! "$RUNTIME" compose version >/dev/null 2>&1; then
    echo "ERROR: '$RUNTIME compose' is not available." >&2
    exit 1
fi

project_name="${SKADIA_DEPLOYMENT_SMOKE_PROJECT_NAME:-skadia-smoke-$(date +%Y%m%d%H%M%S)-$$}"
case "$project_name" in
    *[!A-Za-z0-9_.-]*|"")
        echo "ERROR: invalid smoke COMPOSE_PROJECT_NAME: $project_name" >&2
        exit 2
        ;;
esac
case "$project_name" in
    skadia-smoke-?*) ;;
    *)
        echo "ERROR: smoke COMPOSE_PROJECT_NAME must start with skadia-smoke-; got $project_name" >&2
        exit 2
        ;;
esac

if [ -e "$DATA_ROOT" ] && [ ! -d "$DATA_ROOT" ]; then
    echo "ERROR: --data-root exists but is not a directory: $DATA_ROOT" >&2
    exit 2
fi
if [ -d "$DATA_ROOT" ] && [ ! -f "$DATA_ROOT/.skadia-smoke-root" ]; then
    if find "$DATA_ROOT" -mindepth 1 -maxdepth 1 2>/dev/null | grep -q .; then
        echo "ERROR: refusing to run deployment smoke against a non-empty unmarked data root: $DATA_ROOT" >&2
        echo "Use a fresh empty directory, or a directory containing .skadia-smoke-root from a previous smoke run." >&2
        exit 2
    fi
fi

mkdir -p "$DATA_ROOT"
printf 'skadia deployment smoke root\n' > "$DATA_ROOT/.skadia-smoke-root"
marker="$DATA_ROOT/.skadia-smoke-before"
printf 'skadia deployment smoke\n' > "$marker"

compose_args=(compose)
if ! $NO_BUILD; then
    compose_args+=(up -d --build --remove-orphans)
else
    compose_args+=(up -d --remove-orphans)
fi

smoke_compose() {
    COMPOSE_PROJECT_NAME="$project_name" SKADIA_DATA_ROOT="$DATA_ROOT" COMPOSE_FILE=docker-compose.yml "$RUNTIME" compose "$@"
}

guard_smoke_project_containers() {
    local names
    local name
    names="$("$RUNTIME" ps -a --filter "label=com.docker.compose.project=$project_name" --format '{{.Names}}' 2>/dev/null || true)"
    while IFS= read -r name; do
        [ -n "$name" ] || continue
        case "$name" in
            *"$project_name"*|skadia-smoke-*) ;;
            *)
                echo "ERROR: refusing compose down; non-smoke container is labeled for $project_name: $name" >&2
                exit 1
                ;;
        esac
    done <<< "$names"
}

api_base_url() {
    local published
    local host
    local port
    published="$(smoke_compose port api 8080 2>/dev/null | head -1 || true)"
    [ -n "$published" ] || return 1
    port="${published##*:}"
    host="${published%:*}"
    host="${host#[}"
    host="${host%]}"
    case "$host" in
        ""|"0.0.0.0"|"::"|"[::]") host="127.0.0.1" ;;
    esac
    printf 'http://%s:%s\n' "$host" "$port"
}

wait_for_endpoint() {
    local url="$1"
    local timeout="${SKADIA_SMOKE_READY_TIMEOUT:-120}"
    local start
    start="$(date +%s)"
    while true; do
        if curl -fsS --max-time 5 "$url" >/dev/null; then
            return 0
        fi
        if [ $(( $(date +%s) - start )) -ge "$timeout" ]; then
            echo "ERROR: timed out waiting for $url" >&2
            return 1
        fi
        sleep 2
    done
}

check_ready_endpoints() {
    local base
    if ! command -v curl >/dev/null 2>&1; then
        echo "ERROR: curl is required for deployment smoke readiness checks." >&2
        return 1
    fi
    base="$(api_base_url || true)"
    if [ -z "$base" ]; then
        echo "WARN: api:8080 is not published by compose; skipping HTTP readiness checks." >&2
        return 0
    fi
    wait_for_endpoint "$base/readyz"
    wait_for_endpoint "$base/api/v1/capabilities"
}

SMOKE_STARTED=false
cleanup_smoke() {
    if $SMOKE_STARTED; then
        guard_smoke_project_containers
        smoke_compose down --remove-orphans
    fi
}
trap cleanup_smoke EXIT

COMPOSE_PROJECT_NAME="$project_name" SKADIA_DATA_ROOT="$DATA_ROOT" COMPOSE_FILE=docker-compose.yml "$RUNTIME" "${compose_args[@]}"
SMOKE_STARTED=true
check_ready_endpoints
cleanup_smoke
SMOKE_STARTED=false
trap - EXIT

if [ ! -f "$marker" ]; then
    echo "ERROR: data-root marker disappeared after compose down." >&2
    exit 1
fi

echo "deployment smoke passed for $RUNTIME with data root $DATA_ROOT"
