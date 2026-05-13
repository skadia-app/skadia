#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"

fail() {
    echo "FAIL: $*" >&2
    exit 1
}

assert_env() {
    local file="$1"
    local key="$2"
    local want="$3"
    local got
    got="$(grep "^${key}=" "$file" 2>/dev/null | head -1 | cut -d= -f2- || true)"
    [ "$got" = "$want" ] || fail "${key}=${got}, want ${want}"
}

assert_env_matches() {
    local file="$1"
    local key="$2"
    local pattern="$3"
    local got
    got="$(grep "^${key}=" "$file" 2>/dev/null | head -1 | cut -d= -f2- || true)"
    [[ "$got" =~ $pattern ]] || fail "${key}=${got}, want pattern ${pattern}"
}

assert_no_env_key() {
    local file="$1"
    local key="$2"
    if grep -q "^${key}=" "$file" 2>/dev/null; then
        fail "${key} unexpectedly set in ${file}"
    fi
}

assert_file_mode() {
    local file="$1"
    local want="$2"
    local got
    got="$(stat -c '%a' "$file")"
    [ "$got" = "$want" ] || fail "${file} mode ${got}, want ${want}"
}

assert_contains() {
    local needle="$1"
    local file="$2"
    grep -Fq -- "$needle" "$file" || fail "missing text: ${needle}"
}

assert_not_contains() {
    local needle="$1"
    local file="$2"
    if grep -Fq -- "$needle" "$file"; then
        fail "unexpected text: ${needle}"
    fi
}

assert_command_log_contains() {
    local needle="$1"
    local file="$2"
    grep -Fq -- "$needle" "$file" || fail "missing command log text: ${needle}"
}

assert_command_log_not_contains() {
    local needle="$1"
    local file="$2"
    if grep -Fq -- "$needle" "$file"; then
        fail "unexpected command log text: ${needle}"
    fi
}

new_fixture() {
    local tmp
    tmp="$(mktemp -d)"
    mkdir -p "$tmp/scripts/lib" "$tmp/bin"
    cp "$ROOT/scripts/setup.sh" "$tmp/scripts/setup.sh"
    cp "$ROOT/scripts/lib/secure_temp.sh" "$tmp/scripts/lib/secure_temp.sh"
    cp "$ROOT/scripts/lib/secure_env.sh" "$tmp/scripts/lib/secure_env.sh"
    cp "$ROOT/.env.example" "$tmp/.env.example"
    chmod +x "$tmp/scripts/setup.sh" "$tmp/scripts/lib/secure_temp.sh" "$tmp/scripts/lib/secure_env.sh"

    cat > "$tmp/bin/podman" <<'SH'
#!/usr/bin/env bash
set -euo pipefail
printf 'COMPOSE_PROJECT_NAME=%s SKADIA_DATA_ROOT=%s COMPOSE_FILE=%s\n' "${COMPOSE_PROJECT_NAME:-}" "${SKADIA_DATA_ROOT:-}" "${COMPOSE_FILE:-}" >> "${SKADIA_TEST_COMMAND_LOG:-/dev/null}"
printf '%q ' "$@" >> "${SKADIA_TEST_COMMAND_LOG:-/dev/null}"
printf '\n' >> "${SKADIA_TEST_COMMAND_LOG:-/dev/null}"
case "$*" in
    "compose version"*)
        echo "2.30.0"
        ;;
    "unshare true"*)
        exit 0
        ;;
    "unshare chown"*)
        exit 0
        ;;
    *)
        exit 0
        ;;
esac
SH
    chmod +x "$tmp/bin/podman"

    cat > "$tmp/bin/docker" <<'SH'
#!/usr/bin/env bash
set -euo pipefail
printf 'COMPOSE_PROJECT_NAME=%s SKADIA_DATA_ROOT=%s COMPOSE_FILE=%s\n' "${COMPOSE_PROJECT_NAME:-}" "${SKADIA_DATA_ROOT:-}" "${COMPOSE_FILE:-}" >> "${SKADIA_TEST_COMMAND_LOG:-/dev/null}"
printf '%q ' "$@" >> "${SKADIA_TEST_COMMAND_LOG:-/dev/null}"
printf '\n' >> "${SKADIA_TEST_COMMAND_LOG:-/dev/null}"
case "$*" in
    "compose version"*)
        exit 1
        ;;
    *)
        exit 1
        ;;
esac
SH
    chmod +x "$tmp/bin/docker"

    cat > "$tmp/bin/stat" <<'SH'
#!/usr/bin/env bash
set -euo pipefail
if [ "$1" = "-f" ]; then
    echo "ext2/ext3"
else
    /usr/bin/stat "$@"
fi
SH
    chmod +x "$tmp/bin/stat"

    cat > "$tmp/bin/ss" <<'SH'
#!/usr/bin/env bash
exit 1
SH
    chmod +x "$tmp/bin/ss"
    cp "$tmp/bin/ss" "$tmp/bin/netstat"

    printf '%s\n' "$tmp"
}

seed_fixture_brouter_profiles() {
    local tmp="$1"
    mkdir -p "$tmp/config/brouter/profiles"
    printf '%s\n' 'fixture official BRouter profile' > "$tmp/config/brouter/profiles/hiking-mountain.brf"
}

install_fake_smoke_runtime() {
    local tmp="$1"
    cat > "$tmp/bin/podman" <<'SH'
#!/usr/bin/env bash
set -euo pipefail
printf 'COMPOSE_PROJECT_NAME=%s SKADIA_DATA_ROOT=%s COMPOSE_FILE=%s\n' "${COMPOSE_PROJECT_NAME:-}" "${SKADIA_DATA_ROOT:-}" "${COMPOSE_FILE:-}" >> "${SKADIA_TEST_COMMAND_LOG:-/dev/null}"
printf 'LIBRARY_HOST_PATH=%s SHADOW_FS_HOST_PATH=%s HOTFOLDER_HOST_PATH=%s IMPORT_DATA_HOST_PATH=%s OSM_HOST_PATH=%s ROUTING_HOST_PATH=%s DEM_HOST_PATH=%s TILES_HOST_PATH=%s EXPORTS_HOST_PATH=%s IMPORTS_HOST_PATH=%s ML_HOST_PATH=%s\n' "${LIBRARY_HOST_PATH:-}" "${SHADOW_FS_HOST_PATH:-}" "${HOTFOLDER_HOST_PATH:-}" "${IMPORT_DATA_HOST_PATH:-}" "${OSM_HOST_PATH:-}" "${ROUTING_HOST_PATH:-}" "${DEM_HOST_PATH:-}" "${TILES_HOST_PATH:-}" "${EXPORTS_HOST_PATH:-}" "${IMPORTS_HOST_PATH:-}" "${ML_HOST_PATH:-}" >> "${SKADIA_TEST_COMMAND_LOG:-/dev/null}"
printf 'STORAGE_MODE=%s STORAGE_BACKEND=%s S3_ENDPOINT=%s S3_BUCKET=%s AWS_ACCESS_KEY_ID=%s GARAGE_ADMIN_TOKEN=%s COMPOSE_PROFILES=%s\n' "${STORAGE_MODE:-}" "${STORAGE_BACKEND:-}" "${S3_ENDPOINT:-}" "${S3_BUCKET:-}" "${AWS_ACCESS_KEY_ID:-}" "${GARAGE_ADMIN_TOKEN:-}" "${COMPOSE_PROFILES:-}" >> "${SKADIA_TEST_COMMAND_LOG:-/dev/null}"
printf '%q ' "$@" >> "${SKADIA_TEST_COMMAND_LOG:-/dev/null}"
printf '\n' >> "${SKADIA_TEST_COMMAND_LOG:-/dev/null}"
case "$*" in
    "compose version"*)
        echo "2.30.0"
        ;;
    "compose up "*|"compose down "*)
        exit 0
        ;;
    "compose port api 8080"*)
        echo "127.0.0.1:18080"
        ;;
    "ps "*)
        exit 0
        ;;
    *)
        exit 0
        ;;
esac
SH
    chmod +x "$tmp/bin/podman"

    cat > "$tmp/bin/curl" <<'SH'
#!/usr/bin/env bash
set -euo pipefail
printf '%s\n' "$*" >> "${SKADIA_TEST_CURL_LOG:-/dev/null}"
exit 0
SH
    chmod +x "$tmp/bin/curl"
}

run_setup() {
    local tmp="$1"
    run_setup_args "$tmp" --quiet
}

run_setup_args() {
    local tmp="$1"
    shift
    if [ -f "$tmp/.env" ] && [ "${SKADIA_TEST_KEEP_ENV_MODE:-false}" != "true" ]; then
        chmod 0600 "$tmp/.env"
    fi
    SKADIA_TEST_COMMAND_LOG="$tmp/commands.log" PATH="$tmp/bin:$PATH" "$tmp/scripts/setup.sh" "$@" > "$tmp/out" 2>&1
}

run_setup_interactive_input() {
    local tmp="$1"
    local input="$2"
    shift 2
    if [ -f "$tmp/.env" ] && [ "${SKADIA_TEST_KEEP_ENV_MODE:-false}" != "true" ]; then
        chmod 0600 "$tmp/.env"
    fi
    printf '%s' "$input" | SKADIA_TEST_COMMAND_LOG="$tmp/commands.log" PATH="$tmp/bin:$PATH" "$tmp/scripts/setup.sh" "$@" > "$tmp/out" 2>&1
}

run_smoke_args() {
    local tmp="$1"
    shift
    SKADIA_TEST_COMMAND_LOG="$tmp/commands.log" \
        SKADIA_TEST_CURL_LOG="$tmp/curl.log" \
        SKADIA_ALLOW_WORKTREE_SMOKE=true \
        PATH="$tmp/bin:$PATH" \
        "$ROOT/scripts/deployment_smoke.sh" "$@" > "$tmp/out" 2>&1
}

enable_fake_docker() {
    local tmp="$1"
    cat > "$tmp/bin/docker" <<'SH'
#!/usr/bin/env bash
set -euo pipefail
printf '%q ' "$@" >> "${SKADIA_TEST_COMMAND_LOG:-/dev/null}"
printf '\n' >> "${SKADIA_TEST_COMMAND_LOG:-/dev/null}"
case "$*" in
    "compose version"*)
        echo "Docker Compose version v2.30.0"
        ;;
    *)
        exit 0
        ;;
esac
SH
    chmod +x "$tmp/bin/docker"
}

test_kubernetes_target_exits_before_compose_state_mutation() {
    local tmp
    tmp="$(new_fixture)"
    trap 'rm -rf "$tmp"' RETURN

    if run_setup_args "$tmp" --quiet --target kubernetes; then
        fail "kubernetes target setup succeeded"
    fi

    assert_contains "Kubernetes" "$tmp/out"
    [ ! -f "$tmp/.env" ] || fail "kubernetes target wrote .env"
    [ ! -d "$tmp/data" ] || fail "kubernetes target created data directory"
    [ ! -s "$tmp/commands.log" ] || fail "kubernetes target probed container runtime"
}

test_env_file_is_created_private() {
    local tmp
    tmp="$(new_fixture)"
    trap 'rm -rf "$tmp"' RETURN

    run_setup "$tmp"

    assert_file_mode "$tmp/.env" "600"
}

test_secure_temp_helper_contract_exists() {
    [ -x "$ROOT/scripts/lib/secure_temp.sh" ] || fail "scripts/lib/secure_temp.sh missing or not executable"
    assert_contains 'mktemp' "$ROOT/scripts/lib/secure_temp.sh"
    assert_not_contains 'mktemp -d)' "$ROOT/scripts/lib/secure_temp.sh"
    assert_contains 'TMPDIR' "$ROOT/scripts/lib/secure_temp.sh"
    assert_contains '0700' "$ROOT/scripts/lib/secure_temp.sh"
}

test_secure_env_writer_uses_target_directory_tempfiles() {
    local tmp
    tmp="$(mktemp -d)"
    trap 'rm -rf "$tmp"' RETURN
    mkdir -p "$tmp/envdir"
    (
        # shellcheck source=scripts/lib/secure_env.sh
        source "$ROOT/scripts/lib/secure_env.sh"
        secure_env_install_from_example /dev/null "$tmp/envdir/.env"
        secure_env_set "$tmp/envdir/.env" "JWT_SECRET" "secret-value" true
    )

    assert_file_mode "$tmp/envdir/.env" "600"
    assert_env "$tmp/envdir/.env" "JWT_SECRET" "secret-value"
    if find "$tmp/envdir" -maxdepth 1 -name '.env.tmp.*' | grep -q .; then
        fail "secure_env left target-directory temp files behind"
    fi
    assert_contains 'secure_env_tmp_dir_for_file "$file"' "$ROOT/scripts/lib/secure_env.sh"
    assert_contains 'secure_env_tmp_dir_for_file "$target"' "$ROOT/scripts/lib/secure_env.sh"
    assert_not_contains "ENV_SET_VALUE" "$ROOT/scripts/lib/secure_env.sh"
}

test_quiet_mode_rejects_insecure_existing_env() {
    local tmp
    tmp="$(new_fixture)"
    trap 'rm -rf "$tmp"' RETURN
    cat > "$tmp/.env" <<'ENV'
SKADIA_DEPLOYMENT_PROFILE=core
ENV
    chmod 0644 "$tmp/.env"

    if SKADIA_TEST_KEEP_ENV_MODE=true run_setup "$tmp"; then
        fail "setup accepted an insecure existing .env in quiet mode"
    fi
    assert_contains "expected 600" "$tmp/out"
}

test_env_writers_use_shared_secret_safe_helper() {
    assert_not_contains 'tmp="$(mktemp .env.XXXXXX)"' "$ROOT/scripts/setup.sh"
    assert_not_contains 'tmp="$(mktemp .env.XXXXXX)"' "$ROOT/scripts/init-garage.sh"
    assert_contains "secure_env" "$ROOT/scripts/setup.sh"
    assert_contains "secure_env" "$ROOT/scripts/init-garage.sh"
}

test_compose_local_is_not_vite_dev() {
    local tmp
    tmp="$(new_fixture)"
    trap 'rm -rf "$tmp"' RETURN

    run_setup "$tmp"

    assert_env "$tmp/.env" "SKADIA_DEPLOYMENT_MODE" "compose-local"
    assert_env "$tmp/.env" "DOMAIN" "localhost"
    assert_env "$tmp/.env" "APP_URL" "http://localhost:8080"
    assert_env "$tmp/.env" "FRONTEND_URL" "http://localhost:8080"
    assert_env "$tmp/.env" "SKADIA_APP_PORT" "8080"
    assert_no_env_key "$tmp/.env" "TRAEFIK_HTTP_PORT"
    assert_no_env_key "$tmp/.env" "TRAEFIK_HTTPS_PORT"
    assert_env "$tmp/.env" "DEV_AUTH" "false"
    assert_env "$tmp/.env" "TRAEFIK_TLS_ENABLED" "false"
    assert_env "$tmp/.env" "TRAEFIK_CERTRESOLVER" ""
    assert_env "$tmp/.env" "SKADIA_DEPLOYMENT_PROFILE" "full"
    assert_env "$tmp/.env" "COMPOSE_FILE" "docker-compose.yml"
    assert_env_matches "$tmp/.env" "READONLY_PASSWORD" '^[0-9a-f]{32}$'
    assert_not_contains "localhost:5173" "$tmp/.env"
    assert_contains "Ports 8080 are free." "$tmp/out"
    assert_not_contains "Ports 80, 443, 8080 are free." "$tmp/out"
    assert_contains "Start services:  ./scripts/compose.sh up -d --build --remove-orphans" "$tmp/out"
}

test_compose_seeds_brouter_profiles_without_setup_wizard() {
    assert_contains "brouter-profile-init:" "$ROOT/docker-compose.yml"
    assert_contains "./config/brouter/profiles:/config/brouter/profiles" "$ROOT/docker-compose.yml"
    assert_contains "cp \"\$\$file\" \"\$\$target\"" "$ROOT/docker-compose.yml"
    assert_contains "condition: service_completed_successfully" "$ROOT/docker-compose.yml"
}

test_fresh_env_example_interactive_setup_prompts_for_install_decisions() {
    local tmp
    tmp="$(new_fixture)"
    trap 'rm -rf "$tmp"' RETURN
    cp "$tmp/.env.example" "$tmp/.env"
    chmod 0600 "$tmp/.env"

    run_setup_interactive_input "$tmp" $'\n\n\n\n\ny\nn\n\n\n'

    assert_contains "Deployment mode:" "$tmp/out"
    assert_contains "Deployment profile:" "$tmp/out"
    assert_contains "What storage backend do you want to use?" "$tmp/out"
    assert_env "$tmp/.env" "COMPOSE_PROFILES" "full-local-data,ml"
    assert_not_contains "Deployment mode (compose-local) — already configured, skipping." "$tmp/out"
    assert_not_contains "Deployment profile (full) — already configured, skipping." "$tmp/out"
    assert_not_contains "Storage (local / backend local) — already configured, skipping." "$tmp/out"
    assert_not_contains "Compose profiles (full-local-data) — already configured, skipping." "$tmp/out"
}

test_development_mode_uses_vite_urls_without_enabling_dev_auth() {
    local tmp
    tmp="$(new_fixture)"
    trap 'rm -rf "$tmp"' RETURN
    cat > "$tmp/.env" <<'ENV'
SKADIA_DEPLOYMENT_MODE=development
ENV

    run_setup "$tmp"

    assert_env "$tmp/.env" "DOMAIN" "localhost"
    assert_env "$tmp/.env" "APP_URL" "http://localhost:5173"
    assert_env "$tmp/.env" "FRONTEND_URL" "http://localhost:5173"
    assert_env "$tmp/.env" "SKADIA_APP_PORT" "8080"
    assert_no_env_key "$tmp/.env" "TRAEFIK_HTTP_PORT"
    assert_no_env_key "$tmp/.env" "TRAEFIK_HTTPS_PORT"
    assert_env "$tmp/.env" "DEV_AUTH" "false"
    assert_env "$tmp/.env" "TRAEFIK_TLS_ENABLED" "false"
    assert_env "$tmp/.env" "TRAEFIK_CERTRESOLVER" ""
    assert_env "$tmp/.env" "SKADIA_DEPLOYMENT_PROFILE" "full"
    assert_env "$tmp/.env" "COMPOSE_FILE" "docker-compose.yml"
    assert_contains "Ports 5173, 8080 are free." "$tmp/out"
}

test_core_profile_writes_core_compose_file_and_summary() {
    local tmp
    tmp="$(new_fixture)"
    trap 'rm -rf "$tmp"' RETURN
    cat > "$tmp/.env" <<'ENV'
SKADIA_DEPLOYMENT_PROFILE=core
ENV

    run_setup "$tmp"

    assert_env "$tmp/.env" "SKADIA_DEPLOYMENT_PROFILE" "core"
    assert_env "$tmp/.env" "COMPOSE_FILE" "docker-compose.yml"
    assert_no_env_key "$tmp/.env" "COMPOSE_PROFILES"
    assert_env "$tmp/.env" "OSM_DATA_DIR" "/data/disabled/osm"
    assert_env "$tmp/.env" "DEM_DATA_DIR" "/data/disabled/dem"
    assert_env "$tmp/.env" "ROUTING_GRAPH_PATH" "/data/disabled/routing/graph.bin"
    assert_env "$tmp/.env" "TILES_DIR" "/data/disabled/tiles"
    assert_env "$tmp/.env" "BADROUTER_PROFILE_DIR" "/data/disabled/badrouter"
    assert_contains "Core includes: uploads/imports, media, GDPR, BRouter/RD5, heuristic lifts, AI agent" "$tmp/out"
    assert_contains "Core excludes: local OSM import, BadRouter, DEM, Coverage, offline maps, ML pipelines" "$tmp/out"
}

test_existing_deployment_profile_is_not_overwritten() {
    local tmp
    tmp="$(new_fixture)"
    trap 'rm -rf "$tmp"' RETURN
    cat > "$tmp/.env" <<'ENV'
SKADIA_DEPLOYMENT_PROFILE=core
COMPOSE_FILE=docker-compose.yml:docker-compose.core.yml
ENV

    run_setup "$tmp"

    assert_env "$tmp/.env" "SKADIA_DEPLOYMENT_PROFILE" "core"
    assert_env "$tmp/.env" "COMPOSE_FILE" "docker-compose.yml"
    assert_no_env_key "$tmp/.env" "COMPOSE_PROFILES"
}

test_core_profile_does_not_create_full_or_garage_dirs() {
    local tmp
    tmp="$(new_fixture)"
    trap 'rm -rf "$tmp"' RETURN
    cat > "$tmp/.env" <<'ENV'
SKADIA_DEPLOYMENT_PROFILE=core
ENV

    run_setup "$tmp"

    [ -d "$tmp/data/postgres" ] || fail "core did not create postgres dir"
    [ -d "$tmp/data/nats" ] || fail "core did not create nats dir"
    [ -d "$tmp/data/library" ] || fail "core did not create library dir"
    [ -d "$tmp/data/brouter/segments" ] || fail "core did not create brouter segments dir"
    [ -d "$tmp/data/disabled/osm" ] || fail "core did not create disabled osm placeholder"
    [ ! -d "$tmp/data/garage/meta" ] || fail "core created local Garage meta dir"
    [ ! -d "$tmp/data/garage/data" ] || fail "core created local Garage data dir"
    [ ! -d "$tmp/data/osm" ] || fail "core created full OSM dir"
    [ ! -d "$tmp/data/dem" ] || fail "core created full DEM dir"
    [ ! -d "$tmp/data/tiles" ] || fail "core created full tiles dir"
    [ ! -d "$tmp/data/ml" ] || fail "core created ML dir"
    [ ! -d "$tmp/data/ollama" ] || fail "core created local model dir"
}

test_full_local_garage_creates_garage_dirs() {
    local tmp
    tmp="$(new_fixture)"
    trap 'rm -rf "$tmp"' RETURN
    cat > "$tmp/.env" <<'ENV'
SKADIA_DEPLOYMENT_PROFILE=full
SKADIA_STORAGE_MODE=s3-local-garage
STORAGE_BACKEND=s3
S3_ENDPOINT=http://garage:3900
ENV

    run_setup "$tmp"

    [ -d "$tmp/data/garage/meta" ] || fail "full local Garage did not create garage meta dir"
    [ -d "$tmp/data/garage/data" ] || fail "full local Garage did not create garage data dir"
    [ -d "$tmp/data/osm" ] || fail "full did not create OSM dir"
    [ -d "$tmp/data/dem" ] || fail "full did not create DEM dir"
}

test_custom_data_root_does_not_create_project_data_root() {
    local tmp
    tmp="$(new_fixture)"
    trap 'rm -rf "$tmp"' RETURN
    cat > "$tmp/.env" <<'ENV'
SKADIA_DATA_ROOT=./custom-data
ENV
    seed_fixture_brouter_profiles "$tmp"

    run_setup "$tmp"

    [ -d "$tmp/custom-data/postgres" ] || fail "custom data root did not receive postgres dir"
    [ -f "$tmp/custom-data/brouter/profiles/hiking-mountain.brf" ] || fail "custom data root did not receive shipped BRouter profiles"
    [ ! -d "$tmp/data/postgres" ] || fail "project data root was created despite custom SKADIA_DATA_ROOT"
}

test_empty_data_root_is_rejected() {
    local tmp
    tmp="$(new_fixture)"
    trap 'rm -rf "$tmp"' RETURN
    cat > "$tmp/.env" <<'ENV'
SKADIA_DATA_ROOT=
ENV

    if run_setup "$tmp"; then
        fail "setup accepted an explicit empty SKADIA_DATA_ROOT"
    fi
    assert_contains "SKADIA_DATA_ROOT must not be empty" "$tmp/out"
}

test_repo_root_data_root_is_rejected() {
    local tmp
    tmp="$(new_fixture)"
    trap 'rm -rf "$tmp"' RETURN
    cat > "$tmp/.env" <<'ENV'
SKADIA_DATA_ROOT=.
ENV

    if run_setup "$tmp"; then
        fail "setup accepted the project root as SKADIA_DATA_ROOT"
    fi
    assert_contains "must not point at the repository root" "$tmp/out"
}

test_parent_traversal_data_root_is_rejected() {
    local tmp
    tmp="$(new_fixture)"
    trap 'rm -rf "$tmp"' RETURN
    cat > "$tmp/.env" <<'ENV'
SKADIA_DATA_ROOT=../skadia-data
ENV

    if run_setup "$tmp"; then
        fail "setup accepted parent traversal in SKADIA_DATA_ROOT"
    fi
    assert_contains "must not contain parent-directory traversal" "$tmp/out"
}

test_data_root_symlink_to_repo_root_is_rejected() {
    local tmp
    tmp="$(new_fixture)"
    trap 'rm -rf "$tmp"' RETURN
    ln -s "$tmp" "$tmp/rootlink"
    cat > "$tmp/.env" <<'ENV'
SKADIA_DATA_ROOT=./rootlink
ENV

    if run_setup "$tmp"; then
        fail "setup accepted a SKADIA_DATA_ROOT symlink to the repository root"
    fi
    assert_contains "must not point at the repository root" "$tmp/out"
}

test_explicit_docker_runtime_is_first_class() {
    local tmp
    tmp="$(new_fixture)"
    trap 'rm -rf "$tmp"' RETURN
    enable_fake_docker "$tmp"
    cat > "$tmp/.env" <<'ENV'
SKADIA_CONTAINER_RUNTIME=docker
ENV

    run_setup "$tmp"

    assert_env "$tmp/.env" "SKADIA_CONTAINER_RUNTIME" "docker"
    assert_env "$tmp/.env" "DOCKER_SOCKET" "/var/run/docker.sock"
    assert_env "$tmp/.env" "SKADIA_API_USERNS_MODE" ""
    assert_env "$tmp/.env" "SKADIA_ML_USERNS_MODE" ""
    assert_env "$tmp/.env" "SKADIA_VOLUME_OPTS_RW_PRIVATE" ""
    assert_env "$tmp/.env" "SKADIA_VOLUME_OPTS_RW_SHARED" ""
    assert_env "$tmp/.env" "SKADIA_VOLUME_OPTS_RO_PRIVATE" ":ro"
    assert_env "$tmp/.env" "SKADIA_VOLUME_OPTS_RO_SHARED" ":ro"
}

test_docker_runtime_uses_existing_unix_socket_context() {
    local tmp
    tmp="$(new_fixture)"
    trap 'rm -rf "$tmp"' RETURN
    enable_fake_docker "$tmp"
    cat > "$tmp/.env" <<'ENV'
SKADIA_CONTAINER_RUNTIME=docker
DOCKER_SOCKET=/run/user/1000/docker.sock
ENV

    DOCKER_HOST=unix:///run/user/1000/docker.sock run_setup "$tmp"

    assert_env "$tmp/.env" "SKADIA_CONTAINER_RUNTIME" "docker"
    assert_env "$tmp/.env" "DOCKER_SOCKET" "/run/user/1000/docker.sock"
}

test_explicit_podman_runtime_sets_podman_socket() {
    local tmp
    tmp="$(new_fixture)"
    trap 'rm -rf "$tmp"' RETURN
    cat > "$tmp/.env" <<'ENV'
SKADIA_CONTAINER_RUNTIME=podman
ENV

    run_setup "$tmp"

    assert_env "$tmp/.env" "SKADIA_CONTAINER_RUNTIME" "podman"
    assert_env_matches "$tmp/.env" "DOCKER_SOCKET" '/podman/podman\.sock$'
    assert_env "$tmp/.env" "SKADIA_API_USERNS_MODE" "keep-id"
    assert_env "$tmp/.env" "SKADIA_ML_USERNS_MODE" "keep-id"
}

test_auto_runtime_preserves_existing_podman_choice_when_docker_is_available() {
    local tmp
    tmp="$(new_fixture)"
    trap 'rm -rf "$tmp"' RETURN
    enable_fake_docker "$tmp"
    cat > "$tmp/.env" <<'ENV'
SKADIA_CONTAINER_RUNTIME=podman
ENV

    run_setup "$tmp"

    assert_env "$tmp/.env" "SKADIA_CONTAINER_RUNTIME" "podman"
    assert_env "$tmp/.env" "SKADIA_API_USERNS_MODE" "keep-id"
}

test_core_strips_setup_managed_optional_profiles() {
    local tmp
    tmp="$(new_fixture)"
    trap 'rm -rf "$tmp"' RETURN
    cat > "$tmp/.env" <<'ENV'
SKADIA_DEPLOYMENT_PROFILE=core
COMPOSE_PROFILES=full-local-data,s3,ml,llm,custom_future
ENV

    run_setup "$tmp"

    assert_env "$tmp/.env" "COMPOSE_PROFILES" "custom_future"
}

test_full_backfills_legacy_profile_states() {
    local tmp
    tmp="$(new_fixture)"
    trap 'rm -rf "$tmp"' RETURN
    cat > "$tmp/.env" <<'ENV'
SKADIA_DEPLOYMENT_PROFILE=full
COMPOSE_PROFILES=s3
SKADIA_STORAGE_MODE=s3-local-garage
STORAGE_BACKEND=s3
S3_ENDPOINT=http://garage:3900
ENV

    run_setup "$tmp"

    assert_env "$tmp/.env" "COMPOSE_PROFILES" "full-local-data,s3"
}

test_external_s3_full_backfills_full_local_data_without_garage_profile() {
    local tmp
    tmp="$(new_fixture)"
    trap 'rm -rf "$tmp"' RETURN
    cat > "$tmp/.env" <<'ENV'
SKADIA_DEPLOYMENT_PROFILE=full
SKADIA_STORAGE_MODE=s3-external
STORAGE_BACKEND=s3
S3_ENDPOINT=https://s3.example.invalid
S3_ACCESS_KEY=external-key
S3_SECRET_KEY=external-secret
S3_BUCKET=external-library
S3_BUCKET_EXPORTS=external-exports
S3_BUCKET_IMPORTS=external-imports
S3_REGION=external-region
ENV

    run_setup "$tmp"

    assert_env "$tmp/.env" "COMPOSE_PROFILES" "full-local-data"
}

test_core_rejects_existing_s3_storage_in_quiet_mode() {
    local tmp
    tmp="$(new_fixture)"
    trap 'rm -rf "$tmp"' RETURN
    cat > "$tmp/.env" <<'ENV'
SKADIA_DEPLOYMENT_PROFILE=core
SKADIA_STORAGE_MODE=s3-local-garage
STORAGE_BACKEND=s3
SKADIA_S3_PROVIDER=local-garage
S3_ENDPOINT=http://garage:3900
COMPOSE_PROFILES=s3,full-local-data,custom_future
ENV

    if run_setup "$tmp"; then
        fail "core quiet setup silently rewrote existing S3 storage"
    fi

    assert_contains "Core profile cannot silently switch an existing S3/Garage storage configuration to local" "$tmp/out"
    assert_env "$tmp/.env" "SKADIA_STORAGE_MODE" "s3-local-garage"
    [ ! -d "$tmp/data/garage/meta" ] || fail "core created local Garage meta dir"
}

test_compose_server_defaults_to_external_s3_when_storage_unset() {
    local tmp
    tmp="$(new_fixture)"
    trap 'rm -rf "$tmp"' RETURN
    cat > "$tmp/.env" <<'ENV'
SKADIA_DEPLOYMENT_MODE=compose-server
DOMAIN=skadia.example.invalid
S3_ENDPOINT=https://s3.example.invalid
S3_ACCESS_KEY=external-key
S3_SECRET_KEY=external-secret
S3_BUCKET=external-library
S3_BUCKET_EXPORTS=external-exports
S3_BUCKET_IMPORTS=external-imports
S3_REGION=external-region
ENV

    run_setup "$tmp"

    assert_env "$tmp/.env" "SKADIA_DEPLOYMENT_PROFILE" "full"
    assert_env "$tmp/.env" "COMPOSE_FILE" "docker-compose.yml"
    assert_env "$tmp/.env" "SKADIA_STORAGE_MODE" "s3-external"
    assert_env "$tmp/.env" "STORAGE_BACKEND" "s3"
    assert_env "$tmp/.env" "COMPOSE_PROFILES" "full-local-data,proxy"
    assert_contains "Ports 80, 443, 8080 are free." "$tmp/out"
}

test_local_garage_generates_real_s3_credentials_and_profile() {
    local tmp
    tmp="$(new_fixture)"
    trap 'rm -rf "$tmp"' RETURN
    mkdir -p "$tmp/garage"
    cat > "$tmp/garage/garage.toml.template" <<'TOML'
rpc_secret = "${GARAGE_RPC_SECRET}"
admin_token = "${GARAGE_ADMIN_TOKEN}"
TOML
    cat > "$tmp/.env" <<'ENV'
STORAGE_BACKEND=s3
S3_ENDPOINT=http://garage:3900
GARAGE_ACCESS_KEY=skadia-dev-key
GARAGE_SECRET_KEY=skadia-dev-secret
ENV

    run_setup "$tmp"

    assert_env_matches "$tmp/.env" "S3_ACCESS_KEY" '^GK[0-9a-f]{24}$'
    assert_env_matches "$tmp/.env" "S3_SECRET_KEY" '^[0-9a-f]{64}$'
    assert_env "$tmp/.env" "GARAGE_ACCESS_KEY" "$(grep '^S3_ACCESS_KEY=' "$tmp/.env" | cut -d= -f2-)"
    assert_env "$tmp/.env" "GARAGE_SECRET_KEY" "$(grep '^S3_SECRET_KEY=' "$tmp/.env" | cut -d= -f2-)"
    assert_env "$tmp/.env" "COMPOSE_PROFILES" "full-local-data,s3"
    assert_env "$tmp/.env" "SKADIA_S3_PROVIDER" "local-garage"
    assert_env "$tmp/.env" "SKADIA_STORAGE_MODE" "s3-local-garage"
    assert_env "$tmp/.env" "S3_REGION" "us-east-1"
    assert_env "$tmp/.env" "S3_FORCE_PATH_STYLE" "true"
    assert_contains "Start services:    ./scripts/compose.sh up -d --build --remove-orphans" "$tmp/out"
    [ -f "$tmp/garage/garage.toml" ] || fail "local Garage config was not rendered"
    assert_file_mode "$tmp/garage/garage.toml" "600"
    assert_not_contains '${GARAGE_RPC_SECRET}' "$tmp/garage/garage.toml"
    assert_not_contains '${GARAGE_ADMIN_TOKEN}' "$tmp/garage/garage.toml"
    assert_contains "./scripts/init-garage.sh --start" "$tmp/out"
}

test_external_s3_does_not_enable_local_garage_profile() {
    local tmp
    tmp="$(new_fixture)"
    trap 'rm -rf "$tmp"' RETURN
    cat > "$tmp/.env" <<'ENV'
STORAGE_BACKEND=s3
S3_ENDPOINT=https://s3.example.invalid
S3_ACCESS_KEY=external-key
S3_SECRET_KEY=external-secret
S3_BUCKET=external-library
S3_BUCKET_EXPORTS=external-exports
S3_BUCKET_IMPORTS=external-imports
S3_REGION=external-region
ENV

    run_setup "$tmp"

    assert_env "$tmp/.env" "COMPOSE_PROFILES" "full-local-data"
    assert_env "$tmp/.env" "SKADIA_S3_PROVIDER" "external"
    assert_env "$tmp/.env" "SKADIA_STORAGE_MODE" "s3-external"
    assert_not_contains "./scripts/init-garage.sh --start" "$tmp/out"
}

test_external_s3_accepts_file_secret_vars() {
    local tmp
    tmp="$(new_fixture)"
    trap 'rm -rf "$tmp"' RETURN
    cat > "$tmp/.env" <<'ENV'
STORAGE_BACKEND=s3
SKADIA_S3_PROVIDER=external
SKADIA_STORAGE_MODE=s3-external
S3_ENDPOINT=https://s3.example.invalid
S3_ACCESS_KEY_FILE=/run/secrets/s3_access_key
S3_SECRET_KEY_FILE=/run/secrets/s3_secret_key
S3_BUCKET=external-library
S3_BUCKET_EXPORTS=external-exports
S3_BUCKET_IMPORTS=external-imports
S3_REGION=external-region
ENV

    run_setup "$tmp"

    assert_no_env_key "$tmp/.env" "S3_ACCESS_KEY"
    assert_no_env_key "$tmp/.env" "S3_SECRET_KEY"
    assert_not_contains "External S3 requires" "$tmp/out"
}

test_external_s3_quiet_requires_required_settings() {
    local tmp
    tmp="$(new_fixture)"
    trap 'rm -rf "$tmp"' RETURN
    cat > "$tmp/.env" <<'ENV'
STORAGE_BACKEND=s3
SKADIA_STORAGE_MODE=s3-external
ENV

    if run_setup "$tmp"; then
        fail "external S3 quiet setup succeeded without credentials"
    fi
    assert_contains "External S3 requires S3_ENDPOINT" "$tmp/out"
    assert_contains "S3_ACCESS_KEY" "$tmp/out"
    assert_contains "S3_REGION" "$tmp/out"
}

test_storage_backend_s3_without_endpoint_is_external_not_garage() {
    local tmp
    tmp="$(new_fixture)"
    trap 'rm -rf "$tmp"' RETURN
    cat > "$tmp/.env" <<'ENV'
STORAGE_BACKEND=s3
ENV

    if run_setup "$tmp"; then
        fail "quiet setup accepted ambiguous S3 without external settings"
    fi
    assert_contains "External S3 requires S3_ENDPOINT" "$tmp/out"
    assert_not_contains "./scripts/init-garage.sh --start" "$tmp/out"
}

test_external_s3_removes_stale_garage_profile() {
    local tmp
    tmp="$(new_fixture)"
    trap 'rm -rf "$tmp"' RETURN
    cat > "$tmp/.env" <<'ENV'
SKADIA_STORAGE_MODE=s3-external
STORAGE_BACKEND=s3
SKADIA_S3_PROVIDER=external
S3_ENDPOINT=https://s3.example.invalid
S3_ACCESS_KEY=external-key
S3_SECRET_KEY=external-secret
S3_BUCKET=external-library
S3_BUCKET_EXPORTS=external-exports
S3_BUCKET_IMPORTS=external-imports
S3_REGION=external-region
COMPOSE_PROFILES=ml,s3
ENV

    run_setup "$tmp"

    assert_env "$tmp/.env" "COMPOSE_PROFILES" "full-local-data,ml"
    assert_not_contains "./scripts/init-garage.sh --start" "$tmp/out"
}

test_external_s3_rejects_stale_garage_endpoint() {
    local tmp
    tmp="$(new_fixture)"
    trap 'rm -rf "$tmp"' RETURN
    cat > "$tmp/.env" <<'ENV'
SKADIA_STORAGE_MODE=s3-external
STORAGE_BACKEND=s3
SKADIA_S3_PROVIDER=external
S3_ENDPOINT=http://garage:3900
S3_ACCESS_KEY=external-key
S3_SECRET_KEY=external-secret
S3_BUCKET=external-library
S3_BUCKET_EXPORTS=external-exports
S3_BUCKET_IMPORTS=external-imports
S3_REGION=external-region
ENV

    if run_setup "$tmp"; then
        fail "external S3 quiet setup accepted local Garage endpoint"
    fi
    assert_env "$tmp/.env" "S3_ENDPOINT" ""
    assert_contains "External S3 requires S3_ENDPOINT" "$tmp/out"
}

test_local_storage_clears_stale_s3_provider_and_profile() {
    local tmp
    tmp="$(new_fixture)"
    trap 'rm -rf "$tmp"' RETURN
    cat > "$tmp/.env" <<'ENV'
SKADIA_STORAGE_MODE=local
STORAGE_BACKEND=s3
SKADIA_S3_PROVIDER=local-garage
COMPOSE_PROFILES=s3,ml
ENV

    run_setup "$tmp"

    assert_env "$tmp/.env" "STORAGE_BACKEND" "local"
    assert_env "$tmp/.env" "SKADIA_S3_PROVIDER" ""
    assert_env "$tmp/.env" "COMPOSE_PROFILES" "full-local-data,ml"
}

test_reconfigure_storage_backend_change_prints_offline_migration_steps() {
    local tmp
    tmp="$(new_fixture)"
    trap 'rm -rf "$tmp"' RETURN
    mkdir -p "$tmp/garage"
    cat > "$tmp/garage/garage.toml.template" <<'TOML'
rpc_secret = "${GARAGE_RPC_SECRET}"
admin_token = "${GARAGE_ADMIN_TOKEN}"
TOML
    cat > "$tmp/.env" <<'ENV'
SKADIA_DEPLOYMENT_MODE=compose-local
SKADIA_DEPLOYMENT_PROFILE=full
SKADIA_STORAGE_MODE=local
STORAGE_BACKEND=local
COMPOSE_PROFILES=full-local-data
POSTGRES_PASSWORD=existing-postgres
READONLY_PASSWORD=existing-readonly
JWT_SECRET=existing-jwt
OAUTH_ENCRYPTION_KEY=existing-oauth
LLM_ENCRYPTION_KEY=existing-llm
KOMOOT_ENCRYPTION_KEY=existing-komoot
SETTINGS_ENCRYPTION_KEY=existing-settings
ML_CALLBACK_SECRET=existing-callback
GARAGE_RPC_SECRET=existing-rpc
GARAGE_ADMIN_TOKEN=existing-admin
ENV

    run_setup_interactive_input "$tmp" $'\n\n\n2\n\n\n\n\n\n' --reconfigure

    assert_env "$tmp/.env" "SKADIA_STORAGE_MODE" "s3-local-garage"
    assert_env "$tmp/.env" "STORAGE_BACKEND" "s3"
    assert_contains "Storage backend changed from local to s3." "$tmp/out"
    assert_contains "setup.sh updated .env only; it did not migrate existing files." "$tmp/out"
    assert_contains "Run storage dry-run:    ./scripts/compose.sh run --rm api migrate-storage -target=s3 --dry-run" "$tmp/out"
    assert_contains "Run storage migration: ./scripts/compose.sh run --rm api migrate-storage -target=s3" "$tmp/out"
    assert_contains "Start services:         ./scripts/compose.sh up -d --build --remove-orphans" "$tmp/out"
}

test_reconfigure_legacy_missing_storage_defaults_to_local_for_migration_warning() {
    local tmp
    tmp="$(new_fixture)"
    trap 'rm -rf "$tmp"' RETURN
    mkdir -p "$tmp/garage"
    cat > "$tmp/garage/garage.toml.template" <<'TOML'
rpc_secret = "${GARAGE_RPC_SECRET}"
admin_token = "${GARAGE_ADMIN_TOKEN}"
TOML
    cat > "$tmp/.env" <<'ENV'
SKADIA_DEPLOYMENT_MODE=compose-local
SKADIA_DEPLOYMENT_PROFILE=full
COMPOSE_PROFILES=full-local-data
POSTGRES_PASSWORD=existing-postgres
READONLY_PASSWORD=existing-readonly
JWT_SECRET=existing-jwt
OAUTH_ENCRYPTION_KEY=existing-oauth
LLM_ENCRYPTION_KEY=existing-llm
KOMOOT_ENCRYPTION_KEY=existing-komoot
SETTINGS_ENCRYPTION_KEY=existing-settings
ML_CALLBACK_SECRET=existing-callback
GARAGE_RPC_SECRET=existing-rpc
GARAGE_ADMIN_TOKEN=existing-admin
ENV

    run_setup_interactive_input "$tmp" $'\n\n\n2\n\n\n\n\n\n' --reconfigure

    assert_env "$tmp/.env" "SKADIA_STORAGE_MODE" "s3-local-garage"
    assert_env "$tmp/.env" "STORAGE_BACKEND" "s3"
    assert_contains "Storage backend changed from local to s3." "$tmp/out"
    assert_contains "setup.sh updated .env only; it did not migrate existing files." "$tmp/out"
}

test_compose_server_requires_non_localhost_domain_in_quiet_mode() {
    local tmp
    tmp="$(new_fixture)"
    trap 'rm -rf "$tmp"' RETURN
    cat > "$tmp/.env" <<'ENV'
SKADIA_DEPLOYMENT_MODE=compose-server
DOMAIN=localhost
ENV

    if run_setup "$tmp"; then
        fail "compose-server quiet setup succeeded with DOMAIN=localhost"
    fi
    assert_contains "compose-server requires a non-localhost DOMAIN" "$tmp/out"
}

test_compose_server_requires_explicit_domain_in_quiet_mode() {
    local tmp
    tmp="$(new_fixture)"
    trap 'rm -rf "$tmp"' RETURN
    cat > "$tmp/.env" <<'ENV'
SKADIA_DEPLOYMENT_MODE=compose-server
ENV

    if run_setup "$tmp"; then
        fail "compose-server quiet setup succeeded without DOMAIN"
    fi
    assert_contains "compose-server requires a non-localhost DOMAIN" "$tmp/out"
}

test_local_garage_appends_s3_to_existing_profiles() {
    local tmp
    tmp="$(new_fixture)"
    trap 'rm -rf "$tmp"' RETURN
    cat > "$tmp/.env" <<'ENV'
STORAGE_BACKEND=s3
S3_ENDPOINT=http://garage:3900
COMPOSE_PROFILES=ml
ENV

    run_setup "$tmp"

    assert_env "$tmp/.env" "COMPOSE_PROFILES" "full-local-data,ml,s3"
}

test_unsafe_database_filesystem_fails_by_default() {
    local tmp
    tmp="$(new_fixture)"
    trap 'rm -rf "$tmp"' RETURN
    cat > "$tmp/bin/stat" <<'SH'
#!/usr/bin/env bash
set -euo pipefail
if [ "$1" = "-f" ]; then
    echo "fuseblk"
else
    /usr/bin/stat "$@"
fi
SH
    chmod +x "$tmp/bin/stat"

    if run_setup "$tmp"; then
        fail "setup accepted fuseblk for database-bearing SKADIA_DATA_ROOT"
    fi
    assert_contains "SKADIA_DATA_ROOT is on unsafe filesystem fuseblk" "$tmp/out"
}

test_unsafe_database_filesystem_allows_explicit_override() {
    local tmp
    tmp="$(new_fixture)"
    trap 'rm -rf "$tmp"' RETURN
    cat > "$tmp/bin/stat" <<'SH'
#!/usr/bin/env bash
set -euo pipefail
if [ "$1" = "-f" ]; then
    echo "fuseblk"
else
    /usr/bin/stat "$@"
fi
SH
    chmod +x "$tmp/bin/stat"

    SKADIA_ALLOW_UNSAFE_DATA_ROOT=true run_setup "$tmp"

    assert_contains "Unsafe database filesystem override accepted" "$tmp/out"
}

test_btrfs_existing_db_files_without_prior_nocow_fail_even_if_chattr_succeeds() {
    local tmp
    tmp="$(new_fixture)"
    trap 'rm -rf "$tmp"' RETURN
    mkdir -p "$tmp/data/postgres/data"
    printf '16\n' > "$tmp/data/postgres/data/PG_VERSION"
    cat > "$tmp/bin/stat" <<'SH'
#!/usr/bin/env bash
set -euo pipefail
if [ "$1" = "-f" ]; then
    echo "btrfs"
else
    /usr/bin/stat "$@"
fi
SH
    chmod +x "$tmp/bin/stat"
    cat > "$tmp/bin/lsattr" <<'SH'
#!/usr/bin/env bash
set -euo pipefail
printf '---------------------- %s\n' "${@: -1}"
SH
    chmod +x "$tmp/bin/lsattr"
    cat > "$tmp/bin/chattr" <<'SH'
#!/usr/bin/env bash
exit 0
SH
    chmod +x "$tmp/bin/chattr"

    if run_setup "$tmp"; then
        fail "setup accepted existing btrfs DB files that lacked nocow before chattr"
    fi
    assert_contains "already contains database files without nocow" "$tmp/out"
    assert_contains "migrate the existing database files into a nocow directory" "$tmp/out"
}

test_btrfs_nocow_failure_hints_use_absolute_data_root_paths() {
    local tmp
    tmp="$(new_fixture)"
    trap 'rm -rf "$tmp"' RETURN
    local data_root="$tmp/absolute-data"
    cat > "$tmp/.env" <<ENV
SKADIA_DATA_ROOT=$data_root
ENV
    cat > "$tmp/bin/stat" <<'SH'
#!/usr/bin/env bash
set -euo pipefail
if [ "$1" = "-f" ]; then
    echo "btrfs"
else
    /usr/bin/stat "$@"
fi
SH
    chmod +x "$tmp/bin/stat"
    cat > "$tmp/bin/lsattr" <<'SH'
#!/usr/bin/env bash
set -euo pipefail
printf '---------------------- %s\n' "${@: -1}"
SH
    chmod +x "$tmp/bin/lsattr"
    cat > "$tmp/bin/chattr" <<'SH'
#!/usr/bin/env bash
exit 1
SH
    chmod +x "$tmp/bin/chattr"
    cat > "$tmp/bin/sudo" <<'SH'
#!/usr/bin/env bash
exit 1
SH
    chmod +x "$tmp/bin/sudo"

    if run_setup "$tmp"; then
        fail "setup succeeded after failing to set nocow on btrfs"
    fi
    assert_contains "sudo chattr +C $data_root/postgres" "$tmp/out"
    assert_not_contains "$tmp/$data_root/postgres" "$tmp/out"
}

test_smoke_rejects_non_smoke_project_name_before_down() {
    local tmp
    tmp="$(mktemp -d)"
    trap 'rm -rf "$tmp"' RETURN
    mkdir -p "$tmp/bin"
    install_fake_smoke_runtime "$tmp"

    if SKADIA_DEPLOYMENT_SMOKE_PROJECT_NAME=skadia run_smoke_args "$tmp" --runtime podman --data-root "$tmp/smoke-data" --no-build; then
        fail "deployment smoke accepted a non-smoke project name"
    fi
    assert_contains "COMPOSE_PROJECT_NAME must start with skadia-smoke-" "$tmp/out"
    assert_command_log_not_contains "compose down" "$tmp/commands.log"
}

test_smoke_overrides_data_related_host_paths() {
    local tmp
    tmp="$(mktemp -d)"
    trap 'rm -rf "$tmp"' RETURN
    mkdir -p "$tmp/bin"
    install_fake_smoke_runtime "$tmp"

    LIBRARY_HOST_PATH=/real/library \
        SHADOW_FS_HOST_PATH=/real/shadow \
        HOTFOLDER_HOST_PATH=/real/hotfolder \
        IMPORT_DATA_HOST_PATH=/real/import-data \
        OSM_HOST_PATH=/real/osm \
        ROUTING_HOST_PATH=/real/routing \
        DEM_HOST_PATH=/real/dem \
        TILES_HOST_PATH=/real/tiles \
        EXPORTS_HOST_PATH=/real/exports \
        IMPORTS_HOST_PATH=/real/imports \
        ML_HOST_PATH=/real/ml \
        run_smoke_args "$tmp" --runtime podman --data-root "$tmp/smoke-data" --no-build

    assert_command_log_not_contains "/real/" "$tmp/commands.log"
    assert_command_log_contains "LIBRARY_HOST_PATH=$tmp/smoke-data/library" "$tmp/commands.log"
    assert_command_log_contains "SHADOW_FS_HOST_PATH=$tmp/smoke-data/shadow-fs" "$tmp/commands.log"
    assert_command_log_contains "IMPORT_DATA_HOST_PATH=$tmp/smoke-data/import-data" "$tmp/commands.log"
    assert_command_log_contains "ML_HOST_PATH=$tmp/smoke-data/ml" "$tmp/commands.log"
}

test_smoke_does_not_inherit_external_storage_or_profile_env() {
    local tmp
    tmp="$(new_fixture)"
    trap 'rm -rf "$tmp"' RETURN

    install_fake_smoke_runtime "$tmp"

    STORAGE_MODE=s3-external \
        STORAGE_BACKEND=s3 \
        S3_ENDPOINT=https://object-store.example.invalid \
        S3_BUCKET=real-bucket \
        AWS_ACCESS_KEY_ID=real-access-key \
        GARAGE_ADMIN_TOKEN=real-admin-token \
        COMPOSE_PROFILES=ml,garage \
        run_smoke_args "$tmp" --runtime podman --data-root "$tmp/smoke-data" --no-build

    assert_command_log_not_contains "S3_ENDPOINT=https://object-store.example.invalid" "$tmp/commands.log"
    assert_command_log_not_contains "S3_BUCKET=real-bucket" "$tmp/commands.log"
    assert_command_log_not_contains "AWS_ACCESS_KEY_ID=real-access-key" "$tmp/commands.log"
    assert_command_log_not_contains "GARAGE_ADMIN_TOKEN=real-admin-token" "$tmp/commands.log"
    assert_command_log_not_contains "COMPOSE_PROFILES=ml,garage" "$tmp/commands.log"
    assert_command_log_contains "STORAGE_MODE=local STORAGE_BACKEND=local" "$tmp/commands.log"
    assert_command_log_contains "COMPOSE_PROFILES=" "$tmp/commands.log"
}

test_smoke_fails_when_api_port_is_not_published() {
    local tmp
    tmp="$(mktemp -d)"
    trap 'rm -rf "$tmp"' RETURN
    mkdir -p "$tmp/bin"
    install_fake_smoke_runtime "$tmp"
    cat > "$tmp/bin/podman" <<'SH'
#!/usr/bin/env bash
set -euo pipefail
printf 'COMPOSE_PROJECT_NAME=%s SKADIA_DATA_ROOT=%s COMPOSE_FILE=%s\n' "${COMPOSE_PROJECT_NAME:-}" "${SKADIA_DATA_ROOT:-}" "${COMPOSE_FILE:-}" >> "${SKADIA_TEST_COMMAND_LOG:-/dev/null}"
printf '%q ' "$@" >> "${SKADIA_TEST_COMMAND_LOG:-/dev/null}"
printf '\n' >> "${SKADIA_TEST_COMMAND_LOG:-/dev/null}"
case "$*" in
    "compose version"*)
        echo "2.30.0"
        ;;
    "compose up "*|"compose down "*)
        exit 0
        ;;
    "compose port api 8080"*)
        exit 1
        ;;
    "ps "*)
        exit 0
        ;;
    *)
        exit 0
        ;;
esac
SH
    chmod +x "$tmp/bin/podman"

    if run_smoke_args "$tmp" --runtime podman --data-root "$tmp/smoke-data" --no-build; then
        fail "deployment smoke succeeded without a checkable API port"
    fi
    assert_contains "api:8080 is not published" "$tmp/out"
    assert_command_log_contains "compose down" "$tmp/commands.log"
}

test_smoke_uses_unique_project_name_and_checks_ready_endpoints() {
    local tmp
    tmp="$(mktemp -d)"
    trap 'rm -rf "$tmp"' RETURN
    mkdir -p "$tmp/bin"
    install_fake_smoke_runtime "$tmp"

    run_smoke_args "$tmp" --runtime podman --data-root "$tmp/smoke-data" --no-build

    assert_command_log_contains "COMPOSE_PROJECT_NAME=skadia-smoke-" "$tmp/commands.log"
    assert_command_log_contains "compose down" "$tmp/commands.log"
    assert_contains "http://127.0.0.1:18080/readyz" "$tmp/curl.log"
    assert_contains "http://127.0.0.1:18080/api/v1/capabilities" "$tmp/curl.log"
}

test_static_install_defaults_are_compose_safe() {
    assert_contains "SKADIA_DEPLOYMENT_MODE=compose-local" "$ROOT/.env.example"
    assert_contains "SKADIA_DEPLOYMENT_PROFILE=full" "$ROOT/.env.example"
    assert_contains "COMPOSE_FILE=docker-compose.yml" "$ROOT/.env.example"
    assert_contains "SKADIA_APP_PORT=8080" "$ROOT/.env.example"
    assert_contains "APP_URL=http://localhost:8080" "$ROOT/.env.example"
    assert_contains "FRONTEND_URL=http://localhost:8080" "$ROOT/.env.example"
    assert_contains "TRAEFIK_ENTRYPOINT=web" "$ROOT/.env.example"
    assert_contains "TRAEFIK_TLS_ENABLED=false" "$ROOT/.env.example"
    assert_contains "TRAEFIK_CERTRESOLVER=" "$ROOT/.env.example"
    assert_not_contains "TRAEFIK_HTTP_PORT=" "$ROOT/.env.example"
    assert_not_contains "TRAEFIK_HTTPS_PORT=" "$ROOT/.env.example"
    assert_contains "SKADIA_VOLUME_OPTS_RW_PRIVATE=" "$ROOT/.env.example"
    assert_contains "DOCKER_SOCKET=" "$ROOT/.env.example"
    assert_not_contains "SKADIA_VOLUME_OPTS_RW_PRIVATE=:Z" "$ROOT/.env.example"
    assert_not_contains "DOCKER_SOCKET=/var/run/docker.sock" "$ROOT/.env.example"
    assert_contains 'traefik.http.routers.app.tls=${TRAEFIK_TLS_ENABLED:-true}' "$ROOT/docker-compose.yml"
    assert_not_contains "APP_URL=http://localhost:5173" "$ROOT/.env.example"
    assert_not_contains "FRONTEND_URL=http://localhost:5173" "$ROOT/.env.example"
    assert_contains "GARAGE_ACCESS_KEY=" "$ROOT/.env.example"
    assert_contains "GARAGE_SECRET_KEY=" "$ROOT/.env.example"
    assert_not_contains "GARAGE_SECRET_KEY=skadia-dev-secret" "$ROOT/.env.example"
    assert_contains 'context: .' "$ROOT/docker-compose.yml"
    assert_contains 'dockerfile: services/api/Dockerfile' "$ROOT/docker-compose.yml"
    assert_contains 'APP_URL: ${APP_URL:-http://localhost:8080}' "$ROOT/docker-compose.yml"
    assert_contains 'FRONTEND_URL: ${FRONTEND_URL:-http://localhost:8080}' "$ROOT/docker-compose.yml"
    assert_contains 'FRONTEND_STATIC_DIR: ${FRONTEND_STATIC_DIR:-/app/frontend}' "$ROOT/docker-compose.yml"
    assert_contains '127.0.0.1:${SKADIA_APP_PORT:-8080}:8080' "$ROOT/docker-compose.yml"
    assert_contains 'profiles: ["proxy"]' "$ROOT/docker-compose.yml"
    assert_contains 'data/' "$ROOT/.dockerignore"
    assert_contains 'frontend/node_modules/' "$ROOT/.dockerignore"
    assert_contains 'services/api/vendor/' "$ROOT/.dockerignore"
    assert_contains '  proxy:' "$ROOT/docker-compose.yml"
    assert_not_contains '  traefik:' "$ROOT/docker-compose.yml"
    assert_not_contains 'TRAEFIK_HTTP_PORT' "$ROOT/docker-compose.yml"
    assert_not_contains 'TRAEFIK_HTTPS_PORT' "$ROOT/docker-compose.yml"
    assert_not_contains 'skadia-frontend' "$ROOT/docker-compose.yml"
    assert_not_contains 'image: nginx:1.27-alpine' "$ROOT/docker-compose.yml"
    assert_not_contains 'frontend:' "$ROOT/docker-compose.yml"
    assert_contains 'S3_ACCESS_KEY: ${S3_ACCESS_KEY:-${GARAGE_ACCESS_KEY:-}}' "$ROOT/docker-compose.yml"
    assert_contains 'S3_SECRET_KEY: ${S3_SECRET_KEY:-${GARAGE_SECRET_KEY:-}}' "$ROOT/docker-compose.yml"
    assert_contains 'GARAGE_ENDPOINT: ${GARAGE_ENDPOINT:-}' "$ROOT/docker-compose.yml"
    assert_contains 'GARAGE_ACCESS_KEY: ${GARAGE_ACCESS_KEY:-}' "$ROOT/docker-compose.yml"
    assert_contains 'GARAGE_SECRET_KEY: ${GARAGE_SECRET_KEY:-}' "$ROOT/docker-compose.yml"
    assert_contains 'S3_ACCESS_KEY_FILE: ${S3_ACCESS_KEY_FILE:-}' "$ROOT/docker-compose.yml"
    assert_contains 'S3_SECRET_KEY_FILE: ${S3_SECRET_KEY_FILE:-}' "$ROOT/docker-compose.yml"
    assert_contains 'GARAGE_ACCESS_KEY_FILE: ${GARAGE_ACCESS_KEY_FILE:-}' "$ROOT/docker-compose.yml"
    assert_contains 'GARAGE_SECRET_KEY_FILE: ${GARAGE_SECRET_KEY_FILE:-}' "$ROOT/docker-compose.yml"
    assert_contains 'S3_REGION: ${S3_REGION:-}' "$ROOT/docker-compose.yml"
    assert_contains 'SKADIA_STORAGE_MODE: ${SKADIA_STORAGE_MODE:-}' "$ROOT/docker-compose.yml"
    assert_contains 'SKADIA_DEPLOYMENT_PROFILE: ${SKADIA_DEPLOYMENT_PROFILE:-}' "$ROOT/docker-compose.yml"
    assert_contains 'SKADIA_S3_PROVIDER: ${SKADIA_S3_PROVIDER:-}' "$ROOT/docker-compose.yml"
    assert_contains 'S3_ENDPOINT: ${S3_ENDPOINT:-${GARAGE_ENDPOINT:-}}' "$ROOT/docker-compose.yml"
    assert_contains 'S3_FORCE_PATH_STYLE: ${S3_FORCE_PATH_STYLE:-}' "$ROOT/docker-compose.yml"
    assert_contains 'ML_CALLBACK_SECRET: ${ML_CALLBACK_SECRET:-}' "$ROOT/docker-compose.yml"
    assert_contains 'ML_CALLBACK_SECRET_FILE: ${ML_CALLBACK_SECRET_FILE:-}' "$ROOT/docker-compose.yml"
    assert_contains 'ML_LLM_BROKER_TOKEN_FILE: ${ML_LLM_BROKER_TOKEN_FILE:-}' "$ROOT/docker-compose.yml"
    assert_not_contains "docker-compose.core.yml" "$ROOT/.env.example"
    [ ! -e "$ROOT/docker-compose.core.yml" ] || fail "docker-compose.core.yml should be removed"
    assert_contains "COMPOSE_PROFILES=full-local-data" "$ROOT/.env.example"
    assert_contains "Options: full-local-data, s3, ml, llm" "$ROOT/.env.example"
    assert_contains "profiles: [\"full-local-data\"]" "$ROOT/docker-compose.yml"
    assert_contains 'userns_mode: ${SKADIA_API_USERNS_MODE:-}' "$ROOT/docker-compose.yml"
    assert_contains 'user: ${SKADIA_CONTAINER_USER:-}' "$ROOT/docker-compose.yml"
    assert_contains '${SKADIA_DATA_ROOT:-./data}/postgres' "$ROOT/docker-compose.yml"
    assert_contains '"COMPOSE_PROJECT_NAME=$project_name"' "$ROOT/scripts/deployment_smoke.sh"
    assert_contains 'skadia-smoke' "$ROOT/scripts/deployment_smoke.sh"
    assert_contains '.skadia-smoke-root' "$ROOT/scripts/deployment_smoke.sh"
    assert_contains 'Compose render check' "$ROOT/scripts/setup.sh"
}

test_template_readonly_url_does_not_contain_placeholder_password() {
    if grep -E '^# READONLY_DATABASE_URL=.*skadia_readonly_password' "$ROOT/.env.example" >/dev/null; then
        fail ".env.example contains a commented READONLY_DATABASE_URL with skadia_readonly_password"
    fi
}

test_setup_does_not_source_dotenv_when_rendering_garage_config() {
    assert_not_contains "source .env" "$ROOT/scripts/setup.sh"
    assert_contains "render_garage_config garage/garage.toml.template garage/garage.toml" "$ROOT/scripts/setup.sh"
}

test_hook_activation_supports_git_worktrees() {
    assert_contains "rev-parse --is-inside-work-tree" "$ROOT/scripts/setup.sh"
    assert_not_contains '[ -d "$PROJECT_ROOT/.git" ]' "$ROOT/scripts/setup.sh"
}

test_generated_postgres_password_updates_direct_database_url() {
    local tmp
    tmp="$(new_fixture)"
    trap 'rm -rf "$tmp"' RETURN

    run_setup "$tmp"

    if grep '^DATABASE_URL=' "$tmp/.env" | grep -q "skadia_dev_password"; then
        fail "DATABASE_URL still contains the old POSTGRES_PASSWORD placeholder"
    fi
}

test_existing_postgres_cluster_does_not_rotate_placeholder_password() {
    local tmp
    tmp="$(new_fixture)"
    trap 'rm -rf "$tmp"' RETURN
    cat > "$tmp/.env" <<'ENV'
POSTGRES_PASSWORD=skadia_dev_password
DATABASE_URL=postgres://skadia:skadia_dev_password@postgres:5432/skadia?sslmode=disable
ENV
    mkdir -p "$tmp/data/postgres/data"

    run_setup "$tmp"

    assert_env "$tmp/.env" "POSTGRES_PASSWORD" "skadia_dev_password"
    assert_env "$tmp/.env" "DATABASE_URL" "postgres://skadia:skadia_dev_password@postgres:5432/skadia?sslmode=disable"
    assert_contains "Leaving POSTGRES_PASSWORD and DATABASE_URL unchanged" "$tmp/out"
}

test_generated_readonly_password_updates_direct_readonly_database_url() {
    local tmp
    tmp="$(new_fixture)"
    trap 'rm -rf "$tmp"' RETURN
    cat > "$tmp/.env" <<'ENV'
READONLY_DATABASE_URL=postgres://skadia_readonly:skadia_readonly_password@localhost:5432/skadia?sslmode=disable
ENV

    run_setup "$tmp"

    if grep '^READONLY_DATABASE_URL=' "$tmp/.env" | grep -q "skadia_readonly_password"; then
        fail "READONLY_DATABASE_URL still contains the old READONLY_PASSWORD placeholder"
    fi
}

test_api_writable_runtime_paths_keep_host_owner_for_keep_id() {
    local tmp
    tmp="$(new_fixture)"
    trap 'rm -rf "$tmp"' RETURN

    run_setup "$tmp"

    grep -Eq 'unshare chown .*data/(litellm|library|ml|exports|imports)' "$tmp/commands.log" \
        && fail "setup must not podman-unshare chown fresh API/ML writable paths; keep-id maps the host owner to the container UID"
    [ "$(stat -c '%a' "$tmp/data/library")" = "755" ] || fail "data/library mode is not 755"
    [ "$(stat -c '%a' "$tmp/data/ml")" = "755" ] || fail "data/ml mode is not 755"
    [ "$(stat -c '%a' "$tmp/data/exports")" = "700" ] || fail "data/exports mode is not 700"
    [ "$(stat -c '%a' "$tmp/data/imports")" = "700" ] || fail "data/imports mode is not 700"
    [ "$(stat -c '%a' "$tmp/data/litellm")" = "750" ] || fail "data/litellm mode is not 750"
    [ "$(stat -c '%a' "$tmp/data/litellm/config.yaml")" = "640" ] || fail "data/litellm/config.yaml mode is not 640"
}

test_api_writable_runtime_paths_repair_legacy_podman_subuid_owner() {
    local tmp
    tmp="$(new_fixture)"
    trap 'rm -rf "$tmp"' RETURN

    cat > "$tmp/bin/chmod" <<'SH'
#!/usr/bin/env bash
set -euo pipefail
if [ "$*" = "0750 data/litellm" ]; then
    exit 1
fi
exec /usr/bin/chmod "$@"
SH
    chmod +x "$tmp/bin/chmod"

    run_setup "$tmp"

    grep -q 'unshare chown 0:0 data/litellm' "$tmp/commands.log" \
        || fail "setup did not repair legacy subuid ownership for data/litellm"
    grep -q 'unshare chmod 0750 data/litellm' "$tmp/commands.log" \
        || fail "setup did not restore the data/litellm mode after ownership repair"
    grep -q 'unshare chown 999:999' "$tmp/commands.log" \
        && fail "setup must not recreate legacy subuid ownership"
}

test_database_tuning_uses_env_not_compose_command_override() {
    local tmp
    tmp="$(new_fixture)"
    trap 'rm -rf "$tmp"' RETURN

    run_setup "$tmp"

    [ ! -f "$tmp/docker-compose.override.yml" ] || fail "setup created docker-compose.override.yml"
    assert_env "$tmp/.env" "POSTGRES_RANDOM_PAGE_COST" "1.1"
    assert_env "$tmp/.env" "POSTGRES_EFFECTIVE_IO_CONCURRENCY" "200"
    assert_env "$tmp/.env" "POSTGRES_EFFECTIVE_CACHE_SIZE" "4GB"
    assert_env "$tmp/.env" "POSTGRES_MAINTENANCE_WORK_MEM" "512MB"
    assert_contains 'shared_buffers=${POSTGRES_SHARED_BUFFERS:-4GB}' "$ROOT/docker-compose.yml"
    assert_contains 'random_page_cost=${POSTGRES_RANDOM_PAGE_COST:-1.1}' "$ROOT/docker-compose.yml"
    assert_contains 'effective_io_concurrency=${POSTGRES_EFFECTIVE_IO_CONCURRENCY:-200}' "$ROOT/docker-compose.yml"
}

test_env_updates_preserve_secret_metacharacters() {
    local tmp
    tmp="$(new_fixture)"
    trap 'rm -rf "$tmp"' RETURN
    cat > "$tmp/.env" <<'ENV'
STORAGE_BACKEND=s3
S3_ENDPOINT=http://garage:3900
# S3_SECRET_KEY=
GARAGE_ACCESS_KEY=skadia-dev-key
GARAGE_SECRET_KEY=abc&def|ghi\jkl
ENV

    run_setup "$tmp"

    assert_env "$tmp/.env" "S3_SECRET_KEY" 'abc&def|ghi\jkl'
}

test_timescale_compression_is_admin_managed_not_setup_managed() {
    local tmp
    tmp="$(new_fixture)"
    trap 'rm -rf "$tmp"' RETURN

    run_setup "$tmp"

    assert_not_contains "Enable TimescaleDB compression for activity data?" "$tmp/out"
    assert_contains "TimescaleDB compression is enabled by database migrations and can be changed later in Admin settings." "$tmp/out"
    assert_no_env_key "$tmp/.env" "TIMESCALE_COMPRESSION"
}

test_build_metadata_written_from_git_stub() {
    local tmp
    tmp="$(new_fixture)"
    trap 'rm -rf "$tmp"' RETURN

    cat > "$tmp/bin/git" <<'SH'
#!/usr/bin/env bash
set -euo pipefail
case "$*" in
    "rev-parse --is-inside-work-tree")
        exit 0
        ;;
    "rev-parse --short=12 HEAD")
        printf 'abc123def456\n'
        ;;
    "show -s --format=%cI HEAD")
        printf '2026-05-17T09:58:00+00:00\n'
        ;;
    *)
        exit 1
        ;;
esac
SH
    chmod +x "$tmp/bin/git"

    run_setup "$tmp"

    assert_env "$tmp/.env" "SKADIA_GIT_COMMIT" "abc123def456"
    assert_env "$tmp/.env" "SKADIA_GIT_COMMIT_TIME" "2026-05-17T09:58:00+00:00"
    assert_env_matches "$tmp/.env" "SKADIA_BUILD_TIME" '^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}Z$'
    assert_env "$tmp/.env" "SKADIA_CONFIG_VERSION" "1"
}

test_setup_step_numbers_are_unique_and_ordered() {
    local steps
    steps="$(grep -o 'step "Step [0-9][0-9]*:' "$ROOT/scripts/setup.sh" | awk '{print $3}' | tr -d ':')"
    [ "$steps" = "$(seq 0 17)" ] || fail "setup step sequence is: $(printf '%s' "$steps" | tr '\n' ' ')"
}

test_kubernetes_target_exits_before_compose_state_mutation
test_env_file_is_created_private
test_secure_temp_helper_contract_exists
test_secure_env_writer_uses_target_directory_tempfiles
test_quiet_mode_rejects_insecure_existing_env
test_env_writers_use_shared_secret_safe_helper
test_compose_local_is_not_vite_dev
test_compose_seeds_brouter_profiles_without_setup_wizard
test_fresh_env_example_interactive_setup_prompts_for_install_decisions
test_development_mode_uses_vite_urls_without_enabling_dev_auth
test_core_profile_writes_core_compose_file_and_summary
test_existing_deployment_profile_is_not_overwritten
test_core_profile_does_not_create_full_or_garage_dirs
test_full_local_garage_creates_garage_dirs
test_custom_data_root_does_not_create_project_data_root
test_empty_data_root_is_rejected
test_repo_root_data_root_is_rejected
test_parent_traversal_data_root_is_rejected
test_data_root_symlink_to_repo_root_is_rejected
test_explicit_docker_runtime_is_first_class
test_docker_runtime_uses_existing_unix_socket_context
test_explicit_podman_runtime_sets_podman_socket
test_auto_runtime_preserves_existing_podman_choice_when_docker_is_available
test_core_strips_setup_managed_optional_profiles
test_full_backfills_legacy_profile_states
test_external_s3_full_backfills_full_local_data_without_garage_profile
test_core_rejects_existing_s3_storage_in_quiet_mode
test_compose_server_defaults_to_external_s3_when_storage_unset
test_local_garage_generates_real_s3_credentials_and_profile
test_external_s3_does_not_enable_local_garage_profile
test_external_s3_quiet_requires_required_settings
test_storage_backend_s3_without_endpoint_is_external_not_garage
test_external_s3_removes_stale_garage_profile
test_external_s3_rejects_stale_garage_endpoint
test_local_storage_clears_stale_s3_provider_and_profile
test_reconfigure_storage_backend_change_prints_offline_migration_steps
test_reconfigure_legacy_missing_storage_defaults_to_local_for_migration_warning
test_compose_server_requires_non_localhost_domain_in_quiet_mode
test_compose_server_requires_explicit_domain_in_quiet_mode
test_local_garage_appends_s3_to_existing_profiles
test_unsafe_database_filesystem_fails_by_default
test_unsafe_database_filesystem_allows_explicit_override
test_btrfs_existing_db_files_without_prior_nocow_fail_even_if_chattr_succeeds
test_btrfs_nocow_failure_hints_use_absolute_data_root_paths
test_smoke_rejects_non_smoke_project_name_before_down
test_smoke_overrides_data_related_host_paths
test_smoke_does_not_inherit_external_storage_or_profile_env
test_smoke_fails_when_api_port_is_not_published
test_smoke_uses_unique_project_name_and_checks_ready_endpoints
test_external_s3_accepts_file_secret_vars
test_static_install_defaults_are_compose_safe
test_template_readonly_url_does_not_contain_placeholder_password
test_setup_does_not_source_dotenv_when_rendering_garage_config
test_hook_activation_supports_git_worktrees
test_generated_postgres_password_updates_direct_database_url
test_existing_postgres_cluster_does_not_rotate_placeholder_password
test_generated_readonly_password_updates_direct_readonly_database_url
test_api_writable_runtime_paths_keep_host_owner_for_keep_id
test_database_tuning_uses_env_not_compose_command_override
test_env_updates_preserve_secret_metacharacters
test_build_metadata_written_from_git_stub
test_timescale_compression_is_admin_managed_not_setup_managed
test_setup_step_numbers_are_unique_and_ordered

echo "setup internal tests passed"
