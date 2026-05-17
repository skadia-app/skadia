#!/usr/bin/env bash
set -euo pipefail

# Initialize the local Garage S3 backend.
# Safe to re-run. Use --start on first installs to start Garage before creating
# the layout, buckets, and access key.

START_GARAGE=false
FORCE_LAYOUT=false
for arg in "$@"; do
    case "$arg" in
        --start) START_GARAGE=true ;;
        --force-layout) FORCE_LAYOUT=true ;;
        -h|--help)
            echo "Usage: $0 [--start] [--force-layout]"
            echo "  --start  Run '<runtime> compose up -d garage' before initialization."
            echo "  --force-layout  Re-assign the local Garage node layout even if it already appears in the layout."
            exit 0
            ;;
        *)
            echo "ERROR: unknown argument: $arg" >&2
            exit 1
            ;;
    esac
done

PROJECT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$PROJECT_ROOT"

# shellcheck source=scripts/lib/secure_env.sh
source "$PROJECT_ROOT/scripts/lib/secure_env.sh"

env_file_get() {
    local key="$1"
    secure_env_get ".env" "$key"
}

env_get() {
    local key="$1"
    if [ -n "${!key-}" ]; then
        printf '%s\n' "${!key}"
        return 0
    fi
    env_file_get "$key"
}

env_is_set() {
    local key="$1"
    [ -n "$(env_get "$key")" ]
}

read_secret_path() {
    local key="$1"
    local file="$2"
    if [ ! -r "$file" ]; then
        echo "ERROR: ${key}_FILE points to an unreadable file: $file" >&2
        exit 1
    fi
    sed -n '1p' "$file"
}

env_set() {
    local key="$1"
    local value="$2"
    secure_env_set ".env" "$key" "$value" "true"
}

env_default() {
    local key="$1"
    local fallback="$2"
    local value
    value="$(env_get "$key")"
    if [ -n "$value" ]; then
        printf '%s\n' "$value"
    else
        printf '%s\n' "$fallback"
    fi
}

gen_secret() {
    openssl rand -hex 32 2>/dev/null || head -c 32 /dev/urandom | xxd -p 2>/dev/null || echo ""
}

gen_garage_access_key() {
    local suffix
    suffix="$(openssl rand -hex 12 2>/dev/null || head -c 12 /dev/urandom | xxd -p 2>/dev/null || echo "")"
    if [ -n "$suffix" ]; then
        printf 'GK%s\n' "$suffix"
    fi
}

detect_compose() {
    local requested
    requested="$(env_get "SKADIA_CONTAINER_RUNTIME")"
    if [ -n "${SKADIA_COMPOSE_CMD:-}" ]; then
        # shellcheck disable=SC2206
        COMPOSE=($SKADIA_COMPOSE_CMD)
    elif [ "$requested" = "docker" ] && command -v docker &>/dev/null && docker compose version &>/dev/null; then
        COMPOSE=(docker compose)
    elif [ "$requested" = "podman" ] && command -v podman &>/dev/null && podman compose version &>/dev/null; then
        COMPOSE=(podman compose)
    elif command -v docker &>/dev/null && docker compose version &>/dev/null; then
        COMPOSE=(docker compose)
    elif command -v podman &>/dev/null && podman compose version &>/dev/null; then
        COMPOSE=(podman compose)
    else
        echo "ERROR: neither 'podman compose' nor 'docker compose' is available." >&2
        exit 1
    fi
}

compose() {
    "${COMPOSE[@]}" "$@"
}

append_bucket() {
    local bucket="$1"
    local existing
    if [ -z "$bucket" ]; then
        return
    fi
    for existing in "${BUCKETS[@]}"; do
        if [ "$existing" = "$bucket" ]; then
            return
        fi
    done
    BUCKETS+=("$bucket")
}

ensure_access_key() {
    local info
    local actual_key_id

    if compose exec garage /garage key import --yes -n skadia-dev-key "$S3_ACCESS_KEY" "$S3_SECRET_KEY" 2>/dev/null; then
        return
    fi

    if ! info="$(compose exec garage /garage key info "$S3_ACCESS_KEY" 2>/dev/null)"; then
        echo "ERROR: Could not import or find Garage key ${S3_ACCESS_KEY}." >&2
        exit 1
    fi

    actual_key_id="$(printf '%s\n' "$info" | awk -F: '/Key ID:/ {gsub(/^[ \t]+|[ \t]+$/, "", $2); print $2; exit}')"
    if [ "$actual_key_id" != "$S3_ACCESS_KEY" ]; then
        echo "ERROR: Garage key name '${S3_ACCESS_KEY}' resolves to key ID '${actual_key_id}'." >&2
        echo "Set S3_ACCESS_KEY to the real Key ID, or delete/recreate the legacy key deliberately." >&2
        exit 1
    fi

    echo "Key ${S3_ACCESS_KEY} already exists."
}

detect_compose

storage_mode="$(env_get SKADIA_STORAGE_MODE)"
storage_backend="$(env_get STORAGE_BACKEND)"
s3_provider="$(env_get SKADIA_S3_PROVIDER)"
s3_endpoint="$(env_get S3_ENDPOINT)"
garage_endpoint="$(env_get GARAGE_ENDPOINT)"
if [ -z "$storage_mode" ] && [ "$s3_provider" != "local-garage" ]; then
    echo "ERROR: Garage initialization requires SKADIA_STORAGE_MODE=s3-local-garage or SKADIA_S3_PROVIDER=local-garage." >&2
    exit 1
fi
if [ "$storage_backend" = "local" ]; then
    echo "ERROR: Garage initialization requires S3 storage, got STORAGE_BACKEND=local." >&2
    exit 1
fi
if [ "$storage_backend" != "s3" ]; then
    echo "ERROR: Garage initialization requires STORAGE_BACKEND=s3, got ${storage_backend:-unset}." >&2
    exit 1
fi
if [ -n "$storage_mode" ] && [ "$storage_mode" != "s3-local-garage" ]; then
    echo "ERROR: Garage initialization requires SKADIA_STORAGE_MODE=s3-local-garage, got ${storage_mode}." >&2
    exit 1
fi
if [ -n "$s3_provider" ] && [ "$s3_provider" != "local-garage" ]; then
    echo "ERROR: Garage initialization requires SKADIA_S3_PROVIDER=local-garage, got ${s3_provider}." >&2
    exit 1
fi
case "${s3_endpoint%/}" in
    ""|"http://garage:3900"|"https://garage:3900"|"http://localhost:3900"|"http://127.0.0.1:3900") ;;
    *)
        echo "ERROR: Garage initialization cannot run with external S3_ENDPOINT=${s3_endpoint}." >&2
        exit 1
        ;;
esac
case "${garage_endpoint%/}" in
    ""|"http://garage:3900"|"https://garage:3900"|"http://localhost:3900"|"http://127.0.0.1:3900") ;;
    *)
        echo "ERROR: Garage initialization cannot run with external GARAGE_ENDPOINT=${garage_endpoint}." >&2
        exit 1
        ;;
esac

if $START_GARAGE; then
    echo "Starting Garage service..."
    compose up -d garage
fi

echo "Waiting for Garage to be ready..."
for i in $(seq 1 30); do
    if compose exec garage /garage status &>/dev/null; then
        echo "Garage is ready."
        break
    fi
    if [ "$i" -eq 30 ]; then
        echo "ERROR: Garage did not become ready in time." >&2
        exit 1
    fi
    sleep 1
done

NODE_ID="$(compose exec garage /garage status 2>/dev/null | grep -oE '[a-f0-9]{16}' | head -1)"
if [ -z "$NODE_ID" ]; then
    echo "ERROR: Could not determine Garage node ID." >&2
    exit 1
fi
echo "Node ID: $NODE_ID"

LAYOUT_SHOW="$(compose exec garage /garage layout show 2>/dev/null || true)"
if $FORCE_LAYOUT || ! printf '%s\n' "$LAYOUT_SHOW" | grep -q "$NODE_ID"; then
    echo "Assigning layout..."
    compose exec garage /garage layout assign -z dc1 -c 1G "$NODE_ID"
    LAYOUT_SHOW="$(compose exec garage /garage layout show 2>/dev/null || true)"
else
    echo "Node already appears in Garage layout; leaving layout assignment unchanged."
fi

LAYOUT_VERSION="$(printf '%s\n' "$LAYOUT_SHOW" | grep -oE 'version [0-9]+' | awk '{print $2}' | head -1 || true)"
LAYOUT_VERSION="${LAYOUT_VERSION:-1}"
compose exec garage /garage layout apply --version "$LAYOUT_VERSION"
echo "Layout applied."

_s3_access_inline="$(env_get S3_ACCESS_KEY)"
_s3_access_file="$(env_get S3_ACCESS_KEY_FILE)"
_garage_access_inline="$(env_get GARAGE_ACCESS_KEY)"
_garage_access_file="$(env_get GARAGE_ACCESS_KEY_FILE)"
if [ -n "$_s3_access_file" ]; then
    S3_ACCESS_KEY="$(read_secret_path S3_ACCESS_KEY "$_s3_access_file")"
    access_source="s3-file"
elif [ -n "$_s3_access_inline" ]; then
    S3_ACCESS_KEY="$_s3_access_inline"
    access_source="s3-inline"
elif [ -n "$_garage_access_file" ]; then
    S3_ACCESS_KEY="$(read_secret_path GARAGE_ACCESS_KEY "$_garage_access_file")"
    access_source="garage-file"
elif [ -n "$_garage_access_inline" ]; then
    S3_ACCESS_KEY="$_garage_access_inline"
    access_source="garage-inline"
else
    S3_ACCESS_KEY=""
    access_source=""
fi
if [ -z "$S3_ACCESS_KEY" ] || [ "$S3_ACCESS_KEY" = "skadia-dev-key" ]; then
    S3_ACCESS_KEY="$(gen_garage_access_key)"
    if [ -n "$S3_ACCESS_KEY" ]; then
        echo "Generated S3_ACCESS_KEY for local Garage and saved it to .env."
        env_set S3_ACCESS_KEY "$S3_ACCESS_KEY"
        env_set GARAGE_ACCESS_KEY "$S3_ACCESS_KEY"
        access_source="generated"
    fi
fi
if [ "$access_source" = "garage-inline" ] && { [ -z "$(env_file_get S3_ACCESS_KEY)" ] || [ "$(env_file_get S3_ACCESS_KEY)" = "skadia-dev-key" ]; }; then
    env_set S3_ACCESS_KEY "$S3_ACCESS_KEY"
fi
if [ "$access_source" = "s3-inline" ] && { [ -z "$(env_file_get GARAGE_ACCESS_KEY)" ] || [ "$(env_file_get GARAGE_ACCESS_KEY)" = "skadia-dev-key" ]; }; then
    env_set GARAGE_ACCESS_KEY "$S3_ACCESS_KEY"
fi

_s3_secret_inline="$(env_get S3_SECRET_KEY)"
_s3_secret_file="$(env_get S3_SECRET_KEY_FILE)"
_garage_secret_inline="$(env_get GARAGE_SECRET_KEY)"
_garage_secret_file="$(env_get GARAGE_SECRET_KEY_FILE)"
if [ -n "$_s3_secret_file" ]; then
    S3_SECRET_KEY="$(read_secret_path S3_SECRET_KEY "$_s3_secret_file")"
    secret_source="s3-file"
elif [ -n "$_s3_secret_inline" ]; then
    S3_SECRET_KEY="$_s3_secret_inline"
    secret_source="s3-inline"
else
    S3_SECRET_KEY=""
    secret_source=""
fi
if [ -z "$S3_SECRET_KEY" ] || [ "$S3_SECRET_KEY" = "skadia-dev-secret" ]; then
    if [ -n "$_garage_secret_file" ]; then
        garage_secret="$(read_secret_path GARAGE_SECRET_KEY "$_garage_secret_file")"
        garage_secret_source="garage-file"
    else
        garage_secret="$_garage_secret_inline"
        garage_secret_source="garage-inline"
    fi
    if [ -n "$garage_secret" ] && [ "$garage_secret" != "skadia-dev-secret" ]; then
        S3_SECRET_KEY="$garage_secret"
        secret_source="$garage_secret_source"
    else
        S3_SECRET_KEY="$(gen_secret)"
        secret_source="generated"
    fi
    if [ -n "$S3_SECRET_KEY" ]; then
        if [ "$secret_source" = "generated" ] || [ "$secret_source" = "garage-inline" ]; then
            env_set S3_SECRET_KEY "$S3_SECRET_KEY"
        fi
        if [ "$secret_source" = "generated" ] || [ "$secret_source" = "s3-inline" ]; then
            env_set GARAGE_SECRET_KEY "$S3_SECRET_KEY"
        fi
        echo "Generated S3_SECRET_KEY for local Garage and saved it to .env."
    fi
fi
if [ -z "$S3_ACCESS_KEY" ] || [ -z "$S3_SECRET_KEY" ]; then
    echo "ERROR: S3_ACCESS_KEY and S3_SECRET_KEY are required for Garage initialization." >&2
    exit 1
fi

BUCKETS=()
append_bucket "$(env_default S3_BUCKET skadia-originals)"
append_bucket "$(env_default S3_BUCKET_EXPORTS skadia-exports)"
append_bucket "$(env_default S3_BUCKET_IMPORTS skadia-imports)"
append_bucket "$(env_get S3_BUCKET_ACTIVITY)"
append_bucket "$(env_get S3_BUCKET_MEDIA)"

for bucket in "${BUCKETS[@]}"; do
    echo "Creating bucket ${bucket}..."
    compose exec garage /garage bucket create "$bucket" 2>/dev/null || echo "Bucket ${bucket} already exists."
done

echo "Importing API key..."
ensure_access_key

echo "Granting access..."
for bucket in "${BUCKETS[@]}"; do
    compose exec garage /garage bucket allow "$bucket" --read --write --key "$S3_ACCESS_KEY"
done

echo ""
echo "Garage initialization complete."
echo "Key info:"
compose exec garage /garage key info "$S3_ACCESS_KEY"
