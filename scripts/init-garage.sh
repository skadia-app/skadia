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

env_file_get() {
    local key="$1"
    grep "^${key}=" .env 2>/dev/null | head -1 | cut -d= -f2- || true
}

env_get() {
    local key="$1"
    if [ -n "${!key-}" ]; then
        printf '%s\n' "${!key}"
        return 0
    fi
    env_file_get "$key"
}

env_set() {
    local key="$1"
    local value="$2"
    local tmp
    touch .env
    tmp="$(mktemp .env.XXXXXX)"
    awk -v key="$key" -v value="$value" '
        BEGIN { written = 0 }
        $0 ~ "^" key "=" {
            print key "=" value
            written = 1
            next
        }
        $0 ~ "^#[[:space:]]*" key "=" {
            print key "=" value
            written = 1
            next
        }
        { print }
        END {
            if (!written) {
                print key "=" value
            }
        }
    ' .env > "$tmp"
    mv "$tmp" .env
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
    if [ -n "${SKADIA_COMPOSE_CMD:-}" ]; then
        # shellcheck disable=SC2206
        COMPOSE=($SKADIA_COMPOSE_CMD)
    elif command -v podman &>/dev/null && podman compose version &>/dev/null; then
        COMPOSE=(podman compose)
    elif command -v docker &>/dev/null && docker compose version &>/dev/null; then
        COMPOSE=(docker compose)
    else
        echo "ERROR: neither 'podman compose' nor 'docker compose' is available." >&2
        exit 1
    fi
    if [ "${COMPOSE[0]}" = "docker" ] && [ "${SKADIA_ALLOW_DOCKER_EXPERIMENTAL:-}" != "true" ]; then
        echo "ERROR: Docker runtime is not currently supported by this compose file." >&2
        echo "Use Podman, or set SKADIA_ALLOW_DOCKER_EXPERIMENTAL=true after adding a reviewed Docker override for userns/volume ownership." >&2
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
if [ -z "$storage_mode" ] && [ "$s3_provider" != "local-garage" ]; then
    echo "ERROR: Garage initialization requires SKADIA_STORAGE_MODE=s3-local-garage or SKADIA_S3_PROVIDER=local-garage." >&2
    exit 1
fi
if [ "$storage_backend" = "local" ]; then
    echo "ERROR: Garage initialization requires S3 storage, got STORAGE_BACKEND=local." >&2
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

S3_ACCESS_KEY="$(env_default S3_ACCESS_KEY "$(env_default GARAGE_ACCESS_KEY "")")"
if [ -z "$S3_ACCESS_KEY" ] || [ "$S3_ACCESS_KEY" = "skadia-dev-key" ]; then
    S3_ACCESS_KEY="$(gen_garage_access_key)"
    if [ -n "$S3_ACCESS_KEY" ]; then
        echo "Generated S3_ACCESS_KEY for local Garage and saved it to .env."
    fi
fi
if [ -z "$(env_file_get S3_ACCESS_KEY)" ] || [ "$(env_file_get S3_ACCESS_KEY)" = "skadia-dev-key" ]; then
    env_set S3_ACCESS_KEY "$S3_ACCESS_KEY"
fi
if [ -z "$(env_file_get GARAGE_ACCESS_KEY)" ] || [ "$(env_file_get GARAGE_ACCESS_KEY)" = "skadia-dev-key" ]; then
    env_set GARAGE_ACCESS_KEY "$S3_ACCESS_KEY"
fi

S3_SECRET_KEY="$(env_get S3_SECRET_KEY)"
if [ -z "$S3_SECRET_KEY" ] || [ "$S3_SECRET_KEY" = "skadia-dev-secret" ]; then
    garage_secret="$(env_get GARAGE_SECRET_KEY)"
    if [ -n "$garage_secret" ] && [ "$garage_secret" != "skadia-dev-secret" ]; then
        S3_SECRET_KEY="$garage_secret"
    else
        S3_SECRET_KEY="$(gen_secret)"
    fi
    if [ -n "$S3_SECRET_KEY" ]; then
        env_set S3_SECRET_KEY "$S3_SECRET_KEY"
        env_set GARAGE_SECRET_KEY "$S3_SECRET_KEY"
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
