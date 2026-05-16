#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"

fail() {
    echo "FAIL: $*" >&2
    exit 1
}

assert_contains() {
    local needle="$1"
    local file="$2"
    grep -Fq -- "$needle" "$file" || fail "missing command: $needle"
}

assert_env_matches() {
    local file="$1"
    local key="$2"
    local pattern="$3"
    local got
    got="$(grep "^${key}=" "$file" 2>/dev/null | head -1 | cut -d= -f2- || true)"
    [[ "$got" =~ $pattern ]] || fail "${key}=${got}, want pattern ${pattern}"
}

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

mkdir -p "$tmp/scripts/lib" "$tmp/bin"
cp "$ROOT/scripts/init-garage.sh" "$tmp/scripts/init-garage.sh"
cp "$ROOT/scripts/lib/secure_temp.sh" "$tmp/scripts/lib/secure_temp.sh"
cp "$ROOT/scripts/lib/secure_env.sh" "$tmp/scripts/lib/secure_env.sh"
chmod +x "$tmp/scripts/init-garage.sh" "$tmp/scripts/lib/secure_temp.sh" "$tmp/scripts/lib/secure_env.sh"

cat > "$tmp/.env" <<'ENV'
S3_BUCKET=skadia-library
S3_BUCKET_EXPORTS=custom-exports
S3_BUCKET_IMPORTS=custom-imports
S3_BUCKET_ACTIVITY=skadia-activities
S3_BUCKET_MEDIA=skadia-media
S3_ACCESS_KEY=custom-key
S3_SECRET_KEY=custom-secret
SKADIA_STORAGE_MODE=s3-local-garage
STORAGE_BACKEND=s3
SKADIA_S3_PROVIDER=local-garage
ENV
chmod 0600 "$tmp/.env"

cat > "$tmp/bin/podman" <<'SH'
#!/usr/bin/env bash
set -euo pipefail
printf '%q ' "$@" >> "$SKADIA_TEST_COMMAND_LOG"
printf '\n' >> "$SKADIA_TEST_COMMAND_LOG"
case "$*" in
    "compose version"*)
        echo "2.30.0"
        ;;
    "compose up -d garage"*)
        ;;
    "compose exec garage /garage status"*)
        echo "Node ID: 0123456789abcdef"
        ;;
    "compose exec garage /garage layout show"*)
        echo "Current cluster layout version 1"
        ;;
    *)
        ;;
esac
SH
chmod +x "$tmp/bin/podman"
cp "$tmp/bin/podman" "$tmp/bin/docker"

export PATH="$tmp/bin:$PATH"
export SKADIA_TEST_COMMAND_LOG="$tmp/commands.log"

(cd "$tmp" && ./scripts/init-garage.sh --start >/tmp/skadia-init-garage-test.out)

assert_contains "compose up -d garage" "$SKADIA_TEST_COMMAND_LOG"
assert_contains "compose exec garage /garage key import --yes -n skadia-dev-key custom-key custom-secret" "$SKADIA_TEST_COMMAND_LOG"

for bucket in skadia-library custom-exports custom-imports skadia-activities skadia-media; do
    assert_contains "compose exec garage /garage bucket create $bucket" "$SKADIA_TEST_COMMAND_LOG"
    assert_contains "compose exec garage /garage bucket allow $bucket --read --write --key custom-key" "$SKADIA_TEST_COMMAND_LOG"
done

fresh="$(mktemp -d)"
mkdir -p "$fresh/scripts/lib" "$fresh/bin"
cp "$ROOT/scripts/init-garage.sh" "$fresh/scripts/init-garage.sh"
cp "$ROOT/scripts/lib/secure_temp.sh" "$fresh/scripts/lib/secure_temp.sh"
cp "$ROOT/scripts/lib/secure_env.sh" "$fresh/scripts/lib/secure_env.sh"
chmod +x "$fresh/scripts/init-garage.sh" "$fresh/scripts/lib/secure_temp.sh" "$fresh/scripts/lib/secure_env.sh"
cp "$tmp/bin/podman" "$fresh/bin/podman"
cp "$tmp/bin/docker" "$fresh/bin/docker"

export PATH="$fresh/bin:$PATH"
export SKADIA_TEST_COMMAND_LOG="$fresh/commands.log"
cat > "$fresh/.env" <<'ENV'
SKADIA_STORAGE_MODE=s3-local-garage
STORAGE_BACKEND=s3
SKADIA_S3_PROVIDER=local-garage
ENV
chmod 0600 "$fresh/.env"

(cd "$fresh" && ./scripts/init-garage.sh --start >/tmp/skadia-init-garage-fresh-test.out)

assert_env_matches "$fresh/.env" "S3_ACCESS_KEY" '^GK[0-9a-f]{24}$'
assert_env_matches "$fresh/.env" "S3_SECRET_KEY" '^[0-9a-f]{64}$'
assert_contains "compose exec garage /garage bucket allow skadia-exports --read --write --key GK" "$fresh/commands.log"
rm -rf "$fresh"

file_secrets="$(mktemp -d)"
mkdir -p "$file_secrets/scripts/lib" "$file_secrets/bin" "$file_secrets/secrets"
cp "$ROOT/scripts/init-garage.sh" "$file_secrets/scripts/init-garage.sh"
cp "$ROOT/scripts/lib/secure_temp.sh" "$file_secrets/scripts/lib/secure_temp.sh"
cp "$ROOT/scripts/lib/secure_env.sh" "$file_secrets/scripts/lib/secure_env.sh"
chmod +x "$file_secrets/scripts/init-garage.sh" "$file_secrets/scripts/lib/secure_temp.sh" "$file_secrets/scripts/lib/secure_env.sh"
cp "$tmp/bin/podman" "$file_secrets/bin/podman"
cp "$tmp/bin/docker" "$file_secrets/bin/docker"
printf 'file-key\n' > "$file_secrets/secrets/s3_access_key"
printf 'file-secret\n' > "$file_secrets/secrets/s3_secret_key"
cat > "$file_secrets/.env" <<ENV
S3_ACCESS_KEY_FILE=$file_secrets/secrets/s3_access_key
S3_SECRET_KEY_FILE=$file_secrets/secrets/s3_secret_key
SKADIA_STORAGE_MODE=s3-local-garage
STORAGE_BACKEND=s3
SKADIA_S3_PROVIDER=local-garage
ENV
chmod 0600 "$file_secrets/.env"

export PATH="$file_secrets/bin:$PATH"
export SKADIA_TEST_COMMAND_LOG="$file_secrets/commands.log"

(cd "$file_secrets" && ./scripts/init-garage.sh --start >/tmp/skadia-init-garage-file-secret-test.out)

assert_contains "compose exec garage /garage key import --yes -n skadia-dev-key file-key file-secret" "$file_secrets/commands.log"
if grep -Eq '^(S3_ACCESS_KEY|S3_SECRET_KEY|GARAGE_ACCESS_KEY|GARAGE_SECRET_KEY)=' "$file_secrets/.env"; then
    fail "init-garage converted _FILE secrets into inline .env secrets"
fi
rm -rf "$file_secrets"

legacy="$(mktemp -d)"
mkdir -p "$legacy/scripts/lib" "$legacy/bin"
cp "$ROOT/scripts/init-garage.sh" "$legacy/scripts/init-garage.sh"
cp "$ROOT/scripts/lib/secure_temp.sh" "$legacy/scripts/lib/secure_temp.sh"
cp "$ROOT/scripts/lib/secure_env.sh" "$legacy/scripts/lib/secure_env.sh"
chmod +x "$legacy/scripts/init-garage.sh" "$legacy/scripts/lib/secure_temp.sh" "$legacy/scripts/lib/secure_env.sh"
cp "$tmp/bin/podman" "$legacy/bin/podman"
cp "$tmp/bin/docker" "$legacy/bin/docker"
cat > "$legacy/.env" <<'ENV'
S3_ACCESS_KEY=skadia-dev-key
S3_SECRET_KEY=skadia-dev-secret
GARAGE_ACCESS_KEY=skadia-dev-key
GARAGE_SECRET_KEY=skadia-dev-secret
SKADIA_STORAGE_MODE=s3-local-garage
STORAGE_BACKEND=s3
SKADIA_S3_PROVIDER=local-garage
ENV
chmod 0600 "$legacy/.env"

export PATH="$legacy/bin:$PATH"
export SKADIA_TEST_COMMAND_LOG="$legacy/commands.log"

(cd "$legacy" && ./scripts/init-garage.sh --start >/tmp/skadia-init-garage-legacy-test.out)

assert_env_matches "$legacy/.env" "S3_ACCESS_KEY" '^GK[0-9a-f]{24}$'
assert_env_matches "$legacy/.env" "S3_SECRET_KEY" '^[0-9a-f]{64}$'
assert_contains "compose exec garage /garage bucket allow skadia-exports --read --write --key GK" "$legacy/commands.log"
rm -rf "$legacy"

metachar="$(mktemp -d)"
mkdir -p "$metachar/scripts/lib" "$metachar/bin"
cp "$ROOT/scripts/init-garage.sh" "$metachar/scripts/init-garage.sh"
cp "$ROOT/scripts/lib/secure_temp.sh" "$metachar/scripts/lib/secure_temp.sh"
cp "$ROOT/scripts/lib/secure_env.sh" "$metachar/scripts/lib/secure_env.sh"
chmod +x "$metachar/scripts/init-garage.sh" "$metachar/scripts/lib/secure_temp.sh" "$metachar/scripts/lib/secure_env.sh"
cp "$tmp/bin/podman" "$metachar/bin/podman"
cp "$tmp/bin/docker" "$metachar/bin/docker"
cat > "$metachar/.env" <<'ENV'
S3_ACCESS_KEY=custom-key
# S3_SECRET_KEY=
GARAGE_SECRET_KEY=abc&def|ghi\jkl
SKADIA_STORAGE_MODE=s3-local-garage
STORAGE_BACKEND=s3
SKADIA_S3_PROVIDER=local-garage
ENV
chmod 0600 "$metachar/.env"

export PATH="$metachar/bin:$PATH"
export SKADIA_TEST_COMMAND_LOG="$metachar/commands.log"

(cd "$metachar" && ./scripts/init-garage.sh --start >/tmp/skadia-init-garage-metachar-test.out)

grep -Fq 'S3_SECRET_KEY=abc&def|ghi\jkl' "$metachar/.env" || fail "S3_SECRET_KEY metacharacters were not preserved"
rm -rf "$metachar"

external="$(mktemp -d)"
mkdir -p "$external/scripts/lib" "$external/bin"
cp "$ROOT/scripts/init-garage.sh" "$external/scripts/init-garage.sh"
cp "$ROOT/scripts/lib/secure_temp.sh" "$external/scripts/lib/secure_temp.sh"
cp "$ROOT/scripts/lib/secure_env.sh" "$external/scripts/lib/secure_env.sh"
chmod +x "$external/scripts/init-garage.sh" "$external/scripts/lib/secure_temp.sh" "$external/scripts/lib/secure_env.sh"
cp "$tmp/bin/podman" "$external/bin/podman"
cp "$tmp/bin/docker" "$external/bin/docker"
cat > "$external/.env" <<'ENV'
SKADIA_STORAGE_MODE=s3-external
STORAGE_BACKEND=s3
SKADIA_S3_PROVIDER=external
S3_ENDPOINT=https://s3.example.invalid
ENV
chmod 0600 "$external/.env"

export PATH="$external/bin:$PATH"
export SKADIA_TEST_COMMAND_LOG="$external/commands.log"

if (cd "$external" && ./scripts/init-garage.sh --start >"$external/out" 2>&1); then
    fail "init-garage accepted external S3 mode"
fi
grep -Fq "Garage initialization requires SKADIA_STORAGE_MODE=s3-local-garage" "$external/out" || fail "missing external S3 rejection"
if [ -f "$external/commands.log" ] && grep -Fq "compose up -d garage" "$external/commands.log"; then
    fail "init-garage started Garage for external S3"
fi
rm -rf "$external"

local_storage="$(mktemp -d)"
mkdir -p "$local_storage/scripts/lib" "$local_storage/bin"
cp "$ROOT/scripts/init-garage.sh" "$local_storage/scripts/init-garage.sh"
cp "$ROOT/scripts/lib/secure_temp.sh" "$local_storage/scripts/lib/secure_temp.sh"
cp "$ROOT/scripts/lib/secure_env.sh" "$local_storage/scripts/lib/secure_env.sh"
chmod +x "$local_storage/scripts/init-garage.sh" "$local_storage/scripts/lib/secure_temp.sh" "$local_storage/scripts/lib/secure_env.sh"
cp "$tmp/bin/podman" "$local_storage/bin/podman"
cp "$tmp/bin/docker" "$local_storage/bin/docker"
cat > "$local_storage/.env" <<'ENV'
SKADIA_STORAGE_MODE=local
STORAGE_BACKEND=local
ENV
chmod 0600 "$local_storage/.env"

export PATH="$local_storage/bin:$PATH"
export SKADIA_TEST_COMMAND_LOG="$local_storage/commands.log"

if (cd "$local_storage" && ./scripts/init-garage.sh --start >"$local_storage/out" 2>&1); then
    fail "init-garage accepted local storage mode"
fi
grep -Fq "Garage initialization requires S3 storage" "$local_storage/out" || fail "missing local storage rejection"
if [ -f "$local_storage/commands.log" ] && grep -Fq "compose up -d garage" "$local_storage/commands.log"; then
    fail "init-garage started Garage for local storage"
fi
rm -rf "$local_storage"

provider_only="$(mktemp -d)"
mkdir -p "$provider_only/scripts/lib" "$provider_only/bin"
cp "$ROOT/scripts/init-garage.sh" "$provider_only/scripts/init-garage.sh"
cp "$ROOT/scripts/lib/secure_temp.sh" "$provider_only/scripts/lib/secure_temp.sh"
cp "$ROOT/scripts/lib/secure_env.sh" "$provider_only/scripts/lib/secure_env.sh"
chmod +x "$provider_only/scripts/init-garage.sh" "$provider_only/scripts/lib/secure_temp.sh" "$provider_only/scripts/lib/secure_env.sh"
cp "$tmp/bin/podman" "$provider_only/bin/podman"
cp "$tmp/bin/docker" "$provider_only/bin/docker"
cat > "$provider_only/.env" <<'ENV'
SKADIA_S3_PROVIDER=local-garage
ENV
chmod 0600 "$provider_only/.env"

export PATH="$provider_only/bin:$PATH"
export SKADIA_TEST_COMMAND_LOG="$provider_only/commands.log"

if (cd "$provider_only" && ./scripts/init-garage.sh --start >"$provider_only/out" 2>&1); then
    fail "init-garage accepted provider-only local Garage config"
fi
grep -Fq "Garage initialization requires STORAGE_BACKEND=s3" "$provider_only/out" || fail "missing provider-only storage backend rejection"
if [ -f "$provider_only/commands.log" ] && grep -Fq "compose up -d garage" "$provider_only/commands.log"; then
    fail "init-garage started Garage for provider-only config"
fi
rm -rf "$provider_only"

ambiguous="$(mktemp -d)"
mkdir -p "$ambiguous/scripts/lib" "$ambiguous/bin"
cp "$ROOT/scripts/init-garage.sh" "$ambiguous/scripts/init-garage.sh"
cp "$ROOT/scripts/lib/secure_temp.sh" "$ambiguous/scripts/lib/secure_temp.sh"
cp "$ROOT/scripts/lib/secure_env.sh" "$ambiguous/scripts/lib/secure_env.sh"
chmod +x "$ambiguous/scripts/init-garage.sh" "$ambiguous/scripts/lib/secure_temp.sh" "$ambiguous/scripts/lib/secure_env.sh"
cp "$tmp/bin/podman" "$ambiguous/bin/podman"
cp "$tmp/bin/docker" "$ambiguous/bin/docker"

export PATH="$ambiguous/bin:$PATH"
export SKADIA_TEST_COMMAND_LOG="$ambiguous/commands.log"

if (cd "$ambiguous" && ./scripts/init-garage.sh --start >"$ambiguous/out" 2>&1); then
    fail "init-garage accepted ambiguous storage mode"
fi
grep -Fq "Garage initialization requires SKADIA_STORAGE_MODE=s3-local-garage" "$ambiguous/out" || fail "missing ambiguous storage rejection"
if [ -f "$ambiguous/commands.log" ] && grep -Fq "compose up -d garage" "$ambiguous/commands.log"; then
    fail "init-garage started Garage for ambiguous storage"
fi
rm -rf "$ambiguous"

garage_endpoint_external="$(mktemp -d)"
mkdir -p "$garage_endpoint_external/scripts/lib" "$garage_endpoint_external/bin"
cp "$ROOT/scripts/init-garage.sh" "$garage_endpoint_external/scripts/init-garage.sh"
cp "$ROOT/scripts/lib/secure_temp.sh" "$garage_endpoint_external/scripts/lib/secure_temp.sh"
cp "$ROOT/scripts/lib/secure_env.sh" "$garage_endpoint_external/scripts/lib/secure_env.sh"
chmod +x "$garage_endpoint_external/scripts/init-garage.sh" "$garage_endpoint_external/scripts/lib/secure_temp.sh" "$garage_endpoint_external/scripts/lib/secure_env.sh"
cp "$tmp/bin/podman" "$garage_endpoint_external/bin/podman"
cp "$tmp/bin/docker" "$garage_endpoint_external/bin/docker"
cat > "$garage_endpoint_external/.env" <<'ENV'
SKADIA_STORAGE_MODE=s3-local-garage
STORAGE_BACKEND=s3
SKADIA_S3_PROVIDER=local-garage
GARAGE_ENDPOINT=https://s3.example.invalid
ENV
chmod 0600 "$garage_endpoint_external/.env"

export PATH="$garage_endpoint_external/bin:$PATH"
export SKADIA_TEST_COMMAND_LOG="$garage_endpoint_external/commands.log"

if (cd "$garage_endpoint_external" && ./scripts/init-garage.sh --start >"$garage_endpoint_external/out" 2>&1); then
    fail "init-garage accepted external GARAGE_ENDPOINT for local Garage mode"
fi
grep -Fq "Garage initialization cannot run with external GARAGE_ENDPOINT=https://s3.example.invalid" "$garage_endpoint_external/out" || fail "missing external GARAGE_ENDPOINT rejection"
if [ -f "$garage_endpoint_external/commands.log" ] && grep -Fq "compose up -d garage" "$garage_endpoint_external/commands.log"; then
    fail "init-garage started Garage with external GARAGE_ENDPOINT"
fi
rm -rf "$garage_endpoint_external"

echo "init-garage internal tests passed"
