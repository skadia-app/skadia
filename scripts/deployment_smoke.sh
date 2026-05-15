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

case "$DATA_ROOT" in
    ""|"."|"data"|"./data"|"$PWD/data"|"$PWD/data/"*)
        echo "ERROR: refusing to run deployment smoke against the project data root." >&2
        exit 2
        ;;
esac

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

project_name="${SKADIA_DEPLOYMENT_SMOKE_PROJECT_NAME:-skadia-smoke}"
case "$project_name" in
    *[!A-Za-z0-9_.-]*|"")
        echo "ERROR: invalid smoke COMPOSE_PROJECT_NAME: $project_name" >&2
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

COMPOSE_PROJECT_NAME="$project_name" SKADIA_DATA_ROOT="$DATA_ROOT" COMPOSE_FILE=docker-compose.yml "$RUNTIME" "${compose_args[@]}"
COMPOSE_PROJECT_NAME="$project_name" SKADIA_DATA_ROOT="$DATA_ROOT" COMPOSE_FILE=docker-compose.yml "$RUNTIME" compose down --remove-orphans

if [ ! -f "$marker" ]; then
    echo "ERROR: data-root marker disappeared after compose down." >&2
    exit 1
fi

echo "deployment smoke passed for $RUNTIME with data root $DATA_ROOT"
