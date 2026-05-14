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

mkdir -p "$tmp/scripts" "$tmp/bin"
cp "$ROOT/scripts/init-garage.sh" "$tmp/scripts/init-garage.sh"
chmod +x "$tmp/scripts/init-garage.sh"

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
mkdir -p "$fresh/scripts" "$fresh/bin"
cp "$ROOT/scripts/init-garage.sh" "$fresh/scripts/init-garage.sh"
chmod +x "$fresh/scripts/init-garage.sh"
cp "$tmp/bin/podman" "$fresh/bin/podman"
cp "$tmp/bin/docker" "$fresh/bin/docker"

export PATH="$fresh/bin:$PATH"
export SKADIA_TEST_COMMAND_LOG="$fresh/commands.log"
cat > "$fresh/.env" <<'ENV'
SKADIA_STORAGE_MODE=s3-local-garage
STORAGE_BACKEND=s3
SKADIA_S3_PROVIDER=local-garage
ENV

(cd "$fresh" && ./scripts/init-garage.sh --start >/tmp/skadia-init-garage-fresh-test.out)

assert_env_matches "$fresh/.env" "S3_ACCESS_KEY" '^GK[0-9a-f]{24}$'
assert_env_matches "$fresh/.env" "S3_SECRET_KEY" '^[0-9a-f]{64}$'
assert_contains "compose exec garage /garage bucket allow skadia-exports --read --write --key GK" "$fresh/commands.log"
rm -rf "$fresh"

legacy="$(mktemp -d)"
mkdir -p "$legacy/scripts" "$legacy/bin"
cp "$ROOT/scripts/init-garage.sh" "$legacy/scripts/init-garage.sh"
chmod +x "$legacy/scripts/init-garage.sh"
cp "$tmp/bin/podman" "$legacy/bin/podman"
cp "$tmp/bin/docker" "$legacy/bin/docker"
cat > "$legacy/.env" <<'ENV'
S3_ACCESS_KEY=skadia-dev-key
S3_SECRET_KEY=skadia-dev-secret
GARAGE_ACCESS_KEY=skadia-dev-key
GARAGE_SECRET_KEY=skadia-dev-secret
SKADIA_S3_PROVIDER=local-garage
ENV

export PATH="$legacy/bin:$PATH"
export SKADIA_TEST_COMMAND_LOG="$legacy/commands.log"

(cd "$legacy" && ./scripts/init-garage.sh --start >/tmp/skadia-init-garage-legacy-test.out)

assert_env_matches "$legacy/.env" "S3_ACCESS_KEY" '^GK[0-9a-f]{24}$'
assert_env_matches "$legacy/.env" "S3_SECRET_KEY" '^[0-9a-f]{64}$'
assert_contains "compose exec garage /garage bucket allow skadia-exports --read --write --key GK" "$legacy/commands.log"
rm -rf "$legacy"

metachar="$(mktemp -d)"
mkdir -p "$metachar/scripts" "$metachar/bin"
cp "$ROOT/scripts/init-garage.sh" "$metachar/scripts/init-garage.sh"
chmod +x "$metachar/scripts/init-garage.sh"
cp "$tmp/bin/podman" "$metachar/bin/podman"
cp "$tmp/bin/docker" "$metachar/bin/docker"
cat > "$metachar/.env" <<'ENV'
S3_ACCESS_KEY=custom-key
# S3_SECRET_KEY=
GARAGE_SECRET_KEY=abc&def|ghi\jkl
SKADIA_S3_PROVIDER=local-garage
ENV

export PATH="$metachar/bin:$PATH"
export SKADIA_TEST_COMMAND_LOG="$metachar/commands.log"

(cd "$metachar" && ./scripts/init-garage.sh --start >/tmp/skadia-init-garage-metachar-test.out)

grep -Fq 'S3_SECRET_KEY=abc&def|ghi\jkl' "$metachar/.env" || fail "S3_SECRET_KEY metacharacters were not preserved"
rm -rf "$metachar"

external="$(mktemp -d)"
mkdir -p "$external/scripts" "$external/bin"
cp "$ROOT/scripts/init-garage.sh" "$external/scripts/init-garage.sh"
chmod +x "$external/scripts/init-garage.sh"
cp "$tmp/bin/podman" "$external/bin/podman"
cp "$tmp/bin/docker" "$external/bin/docker"
cat > "$external/.env" <<'ENV'
SKADIA_STORAGE_MODE=s3-external
SKADIA_S3_PROVIDER=external
S3_ENDPOINT=https://s3.example.invalid
ENV

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
mkdir -p "$local_storage/scripts" "$local_storage/bin"
cp "$ROOT/scripts/init-garage.sh" "$local_storage/scripts/init-garage.sh"
chmod +x "$local_storage/scripts/init-garage.sh"
cp "$tmp/bin/podman" "$local_storage/bin/podman"
cp "$tmp/bin/docker" "$local_storage/bin/docker"
cat > "$local_storage/.env" <<'ENV'
SKADIA_STORAGE_MODE=local
STORAGE_BACKEND=local
ENV

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

ambiguous="$(mktemp -d)"
mkdir -p "$ambiguous/scripts" "$ambiguous/bin"
cp "$ROOT/scripts/init-garage.sh" "$ambiguous/scripts/init-garage.sh"
chmod +x "$ambiguous/scripts/init-garage.sh"
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

echo "init-garage internal tests passed"
