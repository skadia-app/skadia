#!/usr/bin/env bash
# Skadia — Initial Setup Script
#
# Run before first `podman compose up -d`.
# Safe to re-run — skips already-configured steps.
#
# Usage:
#   ./scripts/setup.sh              # interactive
#   ./scripts/setup.sh --quiet      # non-interactive (CI/automation)
#   ./scripts/setup.sh --reconfigure  # re-ask all questions (secrets are never overwritten)

set -euo pipefail

QUIET=false
RECONFIG=false
CLI_TARGET=""
CLI_RUNTIME=""
while [ "$#" -gt 0 ]; do
    arg="$1"
    case "$arg" in
        --quiet)       QUIET=true ;;
        --reconfigure) RECONFIG=true ;;
        --target)
            shift
            CLI_TARGET="${1:-}"
            ;;
        --target=*)    CLI_TARGET="${arg#--target=}" ;;
        --runtime)
            shift
            CLI_RUNTIME="${1:-}"
            ;;
        --runtime=*)   CLI_RUNTIME="${arg#--runtime=}" ;;
        *)
            echo "ERROR: unknown argument: $arg" >&2
            exit 1
            ;;
    esac
    shift
done

PROJECT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$PROJECT_ROOT"

# shellcheck source=scripts/lib/secure_env.sh
source "$PROJECT_ROOT/scripts/lib/secure_env.sh"

# ─── Colors ───────────────────────────────────────────────────────────────────

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
BOLD='\033[1m'
NC='\033[0m'

info()    { echo -e "${BLUE}[INFO]${NC}  $*"; }
ok()      { echo -e "${GREEN}[OK]${NC}    $*"; }
warn()    { echo -e "${YELLOW}[WARN]${NC}  $*"; }
error()   { echo -e "${RED}[ERROR]${NC} $*"; }
step()    { echo -e "\n${BOLD}${CYAN}══ $* ${NC}"; }
skipped() { echo -e "${GREEN}[OK]${NC}    $* — already configured, skipping."; }

# ─── Intro ────────────────────────────────────────────────────────────────────

echo ""
echo -e "${BOLD}Skadia Initial Setup${NC}"
echo "───────────────────────────────────────────────────────────────────────────"
echo "Experienced users: You can skip this wizard and configure .env directly."
echo "See .env.example for all available options."
echo "This script is safe to re-run — it skips already-configured steps."
if $RECONFIG; then
    echo -e "${YELLOW}--reconfigure: all prompts will be re-asked (secrets are never overwritten).${NC}"
fi
echo "───────────────────────────────────────────────────────────────────────────"
echo ""

# ─── Helper: set or update a key in .env ─────────────────────────────────────

# env_set KEY VALUE — sets or replaces KEY=VALUE in .env (also handles commented-out lines)
env_set() {
    local key="$1"
    local value="$2"
    secure_env_set ".env" "$key" "$value" "$QUIET"
}

# env_get KEY — prints current value (empty string if not set or commented out)
env_get() {
    local key="$1"
    secure_env_get ".env" "$key"
}

# env_is_set KEY — returns 0 if key exists with a non-empty value
env_is_set() {
    local key="$1"
    secure_env_is_set ".env" "$key"
}

env_is_set_or_file() {
    local key="$1"
    env_is_set "$key" || env_is_set "${key}_FILE"
}

env_get_secret_or_file() {
    local key="$1"
    local file
    file="$(env_get "${key}_FILE")"
    if [ -n "$file" ]; then
        if [ ! -r "$file" ]; then
            error "${key}_FILE points to an unreadable file: $file"
            exit 1
        fi
        sed -n '1p' "$file"
        return 0
    fi
    env_get "$key"
}

env_key_exists() {
    local key="$1"
    grep -q "^${key}=" .env 2>/dev/null
}

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
        *) path="$PROJECT_ROOT/$raw" ;;
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

DEPLOYMENT_TARGET="${CLI_TARGET:-${SKADIA_DEPLOYMENT_TARGET:-}}"
if [ -z "$DEPLOYMENT_TARGET" ]; then
    DEPLOYMENT_TARGET="$(env_get "SKADIA_DEPLOYMENT_TARGET")"
fi
DEPLOYMENT_TARGET="${DEPLOYMENT_TARGET:-compose}"

case "$DEPLOYMENT_TARGET" in
    compose) ;;
    kubernetes)
        echo "Kubernetes deployment is planned but not supported by scripts/setup.sh yet."
        echo "This target requires separate manifests, persistent-volume sizing, ingress, and secret management."
        exit 1
        ;;
    *)
        error "SKADIA_DEPLOYMENT_TARGET must be compose or kubernetes; got '$DEPLOYMENT_TARGET'"
        exit 1
        ;;
esac

SKADIA_DATA_ROOT_VALUE=""
if [ "${SKADIA_DATA_ROOT+x}" = "x" ]; then
    SKADIA_DATA_ROOT_VALUE="$SKADIA_DATA_ROOT"
elif env_key_exists "SKADIA_DATA_ROOT"; then
    SKADIA_DATA_ROOT_VALUE="$(env_get "SKADIA_DATA_ROOT")"
else
    SKADIA_DATA_ROOT_VALUE="./data"
fi
if [ -z "$SKADIA_DATA_ROOT_VALUE" ]; then
    error "SKADIA_DATA_ROOT must not be empty."
    exit 1
fi
if path_contains_parent_traversal "$SKADIA_DATA_ROOT_VALUE"; then
    error "SKADIA_DATA_ROOT must not contain parent-directory traversal: $SKADIA_DATA_ROOT_VALUE"
    exit 1
fi
DATA_ROOT="${SKADIA_DATA_ROOT_VALUE%/}"
if [ -z "$DATA_ROOT" ]; then
    DATA_ROOT="/"
fi
DATA_ROOT_CANONICAL="$(canonicalize_path "$SKADIA_DATA_ROOT_VALUE")"
PROJECT_ROOT_CANONICAL="$(canonicalize_path "$PROJECT_ROOT")"
case "$DATA_ROOT_CANONICAL" in
    "/")
        error "SKADIA_DATA_ROOT must not point at the filesystem root."
        exit 1
        ;;
esac
if [ "$DATA_ROOT_CANONICAL" = "$PROJECT_ROOT_CANONICAL" ]; then
    error "SKADIA_DATA_ROOT must not point at the repository root."
    exit 1
fi

# gen_secret — generates 32 random bytes as hex (64 chars)
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

profile_list_contains() {
    local list="$1"
    local needle="$2"
    local item
    IFS=',' read -ra _profile_items <<< "$list"
    for item in "${_profile_items[@]}"; do
        item="${item//[[:space:]]/}"
        if [ "$item" = "$needle" ]; then
            return 0
        fi
    done
    return 1
}

profile_list_remove() {
    local list="$1"
    local remove="$2"
    local item
    local result=""
    IFS=',' read -ra _profile_items <<< "$list"
    for item in "${_profile_items[@]}"; do
        item="${item//[[:space:]]/}"
        if [ -z "$item" ] || [ "$item" = "$remove" ]; then
            continue
        fi
        result="${result:+$result,}$item"
    done
    printf '%s\n' "$result"
}

profile_list_prepend_missing() {
    local list="$1"
    local add="$2"
    if profile_list_contains "$list" "$add"; then
        printf '%s\n' "$list"
    elif [ -n "$list" ]; then
        printf '%s,%s\n' "$add" "$list"
    else
        printf '%s\n' "$add"
    fi
}

profile_list_append_missing() {
    local list="$1"
    local add="$2"
    if profile_list_contains "$list" "$add"; then
        printf '%s\n' "$list"
    elif [ -n "$list" ]; then
        printf '%s,%s\n' "$list" "$add"
    else
        printf '%s\n' "$add"
    fi
}

is_local_garage_endpoint() {
    local endpoint="${1%/}"
    case "$endpoint" in
        "http://garage:3900"|"https://garage:3900"|"http://localhost:3900"|"http://127.0.0.1:3900") return 0 ;;
        *) return 1 ;;
    esac
}

storage_effective_backend_from_values() {
    local mode="$1"
    local backend="$2"
    local provider="$3"
    case "$mode" in
        local) printf 'local\n'; return 0 ;;
        s3|s3-*) printf 's3\n'; return 0 ;;
    esac
    case "$backend" in
        local|s3) printf '%s\n' "$backend"; return 0 ;;
    esac
    case "$provider" in
        local-garage|external) printf 's3\n'; return 0 ;;
    esac
    printf '\n'
}

storage_effective_backend() {
    storage_effective_backend_from_values \
        "$(env_get "SKADIA_STORAGE_MODE")" \
        "$(env_get "STORAGE_BACKEND")" \
        "$(env_get "SKADIA_S3_PROVIDER")"
}

render_garage_config() {
    local template="$1"
    local target="$2"
    local content
    local rpc_secret
    local admin_token
    local tmpdir
    local tmp

    rpc_secret="$(env_get "GARAGE_RPC_SECRET")"
    admin_token="$(env_get "GARAGE_ADMIN_TOKEN")"
    content="$(< "$template")"
    content="${content//\$\{GARAGE_RPC_SECRET\}/$rpc_secret}"
    content="${content//\$\{GARAGE_ADMIN_TOKEN\}/$admin_token}"
    tmpdir="$(secure_env_tmp_dir_for_file "$target")"
    tmp="$tmpdir/garage.toml"
    printf '%s\n' "$content" > "$tmp"
    chmod 0600 "$tmp"
    mv "$tmp" "$target"
    rmdir "$tmpdir" 2>/dev/null || rm -rf "$tmpdir"
    chmod 0600 "$target"
}

ensure_garage_config() {
    local mode

    if [ -f garage/garage.toml ]; then
        mode="$(secure_env_stat_mode garage/garage.toml)"
        if $QUIET && [ -n "$mode" ] && [ "$mode" != "600" ]; then
            error "garage/garage.toml permissions are $mode; expected 600."
            exit 1
        fi
        chmod 0600 garage/garage.toml 2>/dev/null || {
            error "Cannot set private permissions on garage/garage.toml"
            exit 1
        }
        ok "garage/garage.toml already exists."
        return
    fi

    if [ -f garage/garage.toml.template ]; then
        render_garage_config garage/garage.toml.template garage/garage.toml
        ok "Generated garage/garage.toml from template."
    else
        warn "garage/garage.toml.template not found — configure Garage manually."
    fi
}

write_https_redirect_config() {
    mkdir -p traefik/dynamic
    cat > traefik/dynamic/redirect.yml <<'TRAEFIK_EOF'
# Generated by scripts/setup.sh. The setup script may replace or remove this file.
http:
  middlewares:
    https-redirect:
      redirectScheme:
        scheme: https
        permanent: true
  routers:
    http-catchall:
      entryPoints:
        - web
      rule: HostRegexp(`.+`)
      middlewares:
        - https-redirect
      service: dummy
      priority: 1
  services:
    dummy:
      loadBalancer:
        servers:
          - url: "http://localhost:1"
TRAEFIK_EOF
}

remove_generated_redirect_config() {
    if [ ! -f traefik/dynamic/redirect.yml ]; then
        return
    fi
    if grep -q "Generated by scripts/setup.sh" traefik/dynamic/redirect.yml; then
        rm -f traefik/dynamic/redirect.yml
    else
        warn "traefik/dynamic/redirect.yml exists and was not generated by setup.sh; leaving it unchanged."
    fi
}

# ─── Step 0: OS Detection ─────────────────────────────────────────────────────

step "Step 0: OS Detection"

OS="$(uname -s)"

case "$OS" in
    Linux)
        if grep -qi microsoft /proc/version 2>/dev/null; then
            info "WSL2 detected."
            if echo "$PROJECT_ROOT" | grep -qE '^/mnt/[A-Za-z]/'; then
                warn "Your project is on the Windows filesystem (/mnt/<drive>/)."
                warn "Move your project to the Linux filesystem for acceptable I/O performance."
                warn "Recommended: mv '$PROJECT_ROOT' ~/skadia && cd ~/skadia"
                if ! $QUIET; then
                    read -rp "Continue anyway? (y/N) " _choice
                    [[ "$_choice" =~ ^[Yy] ]] || exit 1
                fi
            else
                ok "Project is on Linux filesystem — WSL2 performance is acceptable."
            fi
        else
            ok "Linux detected."
        fi
        ;;
    Darwin)
        warn "macOS detected. The standard compose workflow is supported on Linux; macOS should be treated as experimental."
        ;;
    *)
        warn "Unknown OS: $OS — proceeding anyway."
        ;;
esac

# ─── Step 1: Prerequisites ────────────────────────────────────────────────────

step "Step 1: Prerequisites"

RUNTIME=""
REQUESTED_RUNTIME="${CLI_RUNTIME:-${SKADIA_CONTAINER_RUNTIME:-}}"
if [ -z "$REQUESTED_RUNTIME" ]; then
    REQUESTED_RUNTIME="$(env_get "SKADIA_CONTAINER_RUNTIME")"
fi
REQUESTED_RUNTIME="${REQUESTED_RUNTIME:-auto}"
case "$REQUESTED_RUNTIME" in
    auto|"") ;;
    docker|podman) ;;
    *)
        error "SKADIA_CONTAINER_RUNTIME must be auto, docker, or podman; got '$REQUESTED_RUNTIME'"
        exit 1
        ;;
esac

runtime_has_compose() {
    local runtime="$1"
    command -v "$runtime" &>/dev/null && "$runtime" compose version &>/dev/null
}

if [ "$REQUESTED_RUNTIME" = "docker" ]; then
    if runtime_has_compose docker; then
        RUNTIME=docker
    fi
elif [ "$REQUESTED_RUNTIME" = "podman" ]; then
    if runtime_has_compose podman; then
        RUNTIME=podman
    fi
else
    _existing_socket_hint="$(env_get "DOCKER_SOCKET")"
    if [[ "$_existing_socket_hint" == *podman* ]] && runtime_has_compose podman; then
        RUNTIME=podman
    elif runtime_has_compose docker; then
        RUNTIME=docker
    elif runtime_has_compose podman; then
        RUNTIME=podman
    fi
fi

if [ -z "$RUNTIME" ]; then
    error "Neither docker nor podman found for runtime '$REQUESTED_RUNTIME'. Install one first."
    exit 1
fi

if ! $RUNTIME compose version &>/dev/null; then
    error "'$RUNTIME compose' not available. Install the compose plugin."
    exit 1
fi
ok "Using: $RUNTIME compose ($($RUNTIME compose version --short 2>/dev/null || echo 'unknown'))"

SKADIA_SELECTED_CONTAINER_RUNTIME="$RUNTIME"
SKADIA_SELECTED_CONTAINER_USER="$(id -u):$(id -g)"
SKADIA_SELECTED_DOCKER_SOCKET=""
SKADIA_SELECTED_API_USERNS_MODE=""
SKADIA_SELECTED_ML_USERNS_MODE=""
SKADIA_SELECTED_VOLUME_OPTS_RW_PRIVATE=""
SKADIA_SELECTED_VOLUME_OPTS_RW_SHARED=""
SKADIA_SELECTED_VOLUME_OPTS_RO_PRIVATE=":ro"
SKADIA_SELECTED_VOLUME_OPTS_RO_SHARED=":ro"
if command -v getenforce >/dev/null 2>&1; then
    _selinux_state="$(getenforce 2>/dev/null || true)"
    if [ "$_selinux_state" = "Enforcing" ] || [ "$_selinux_state" = "Permissive" ]; then
        SKADIA_SELECTED_VOLUME_OPTS_RW_PRIVATE=":Z"
        SKADIA_SELECTED_VOLUME_OPTS_RW_SHARED=":z"
        SKADIA_SELECTED_VOLUME_OPTS_RO_PRIVATE=":ro,Z"
        SKADIA_SELECTED_VOLUME_OPTS_RO_SHARED=":ro,z"
    fi
fi
case "$RUNTIME" in
    docker)
        _existing_docker_socket="$(env_get "DOCKER_SOCKET")"
        if [[ "${DOCKER_HOST:-}" == unix://* ]]; then
            SKADIA_SELECTED_DOCKER_SOCKET="${DOCKER_HOST#unix://}"
        elif [ -n "$_existing_docker_socket" ]; then
            SKADIA_SELECTED_DOCKER_SOCKET="$_existing_docker_socket"
        else
            SKADIA_SELECTED_DOCKER_SOCKET="/var/run/docker.sock"
        fi
        ;;
    podman)
        _podman_socket="${XDG_RUNTIME_DIR:-/run/user/$(id -u)}/podman/podman.sock"
        SKADIA_SELECTED_DOCKER_SOCKET="$_podman_socket"
        if [ "$(id -u)" != "0" ]; then
            SKADIA_SELECTED_API_USERNS_MODE="keep-id"
            SKADIA_SELECTED_ML_USERNS_MODE="keep-id"
        fi
        ;;
esac

prepare_private_runtime_dir() {
    local dir="$1"
    prepare_api_writable_runtime_path "$dir" 0700
}

prepare_optional_runtime_dirs() {
    local d
    local optional_dirs=()

    if [ "$DEPLOYMENT_PROFILE" = "full" ]; then
        optional_dirs+=(
            "$DATA_ROOT/routing"
            "$DATA_ROOT/osm"
            "$DATA_ROOT/dem"
            "$DATA_ROOT/litellm"
            "$DATA_ROOT/ml"
            "$DATA_ROOT/ollama"
            "$DATA_ROOT/models"
            "$DATA_ROOT/tiles"
            "$DATA_ROOT/profiles/badrouter"
        )
    fi
    if $GARAGE_INIT_REQUIRED; then
        optional_dirs+=(
            "$DATA_ROOT/garage/meta"
            "$DATA_ROOT/garage/data"
        )
    fi

    for d in "${optional_dirs[@]}"; do
        mkdir -p "$d"
    done

    if [ "$DEPLOYMENT_PROFILE" = "full" ]; then
        prepare_api_writable_runtime_path "$DATA_ROOT/ml" 0755
        prepare_api_writable_runtime_path "$DATA_ROOT/litellm" 0750
        if [ ! -f "$DATA_ROOT/litellm/config.yaml" ]; then
            if [ -f services/litellm/config.yaml ]; then
                cp services/litellm/config.yaml "$DATA_ROOT/litellm/config.yaml"
            else
                printf 'model_list: []\n\nlitellm_settings:\n  drop_params: true\n  request_timeout: 120\n' > "$DATA_ROOT/litellm/config.yaml"
            fi
        fi
        prepare_api_writable_runtime_path "$DATA_ROOT/litellm/config.yaml" 0640
    fi

    if $GARAGE_INIT_REQUIRED && [ "${FS_TYPE:-}" = "btrfs" ]; then
        if check_nocow "$DATA_ROOT/garage/meta"; then
            ok "$DATA_ROOT/garage/meta — nocow already set."
        elif set_nocow "$DATA_ROOT/garage/meta"; then
            ok "$DATA_ROOT/garage/meta — nocow set."
        else
            error "$DATA_ROOT/garage/meta — failed to set nocow. Run manually:"
            error "  sudo chattr +C $(canonicalize_path "$DATA_ROOT/garage/meta")"
            exit 1
        fi
    fi
}

prepare_api_writable_runtime_path() {
    local path="$1"
    local mode="$2"
    if chmod "$mode" "$path" 2>/dev/null; then
        return
    fi
    if [ "$RUNTIME" = "podman" ] && podman unshare true &>/dev/null; then
        if podman unshare chown 0:0 "$path" 2>/dev/null && podman unshare chmod "$mode" "$path" 2>/dev/null; then
            return
        fi
    fi

    error "Cannot make $path writable for the host user and API/ML keep-id containers."
    error "Check ownership manually, then re-run setup. With rootless Podman, this usually repairs it:"
    error "  podman unshare chown 0:0 $path && chmod $mode $path"
    exit 1
}

seed_brouter_profiles() {
    local src="$PROJECT_ROOT/config/brouter/profiles"
    local dest="$DATA_ROOT/brouter/profiles"
    local src_canonical
    local dest_canonical
    local file
    local target

    [ -d "$src" ] || return 0
    src_canonical="$(canonicalize_path "$src")"
    dest_canonical="$(canonicalize_path "$dest")"
    [ "$src_canonical" != "$dest_canonical" ] || return 0

    for file in "$src"/*; do
        [ -f "$file" ] || continue
        target="$dest/$(basename "$file")"
        [ -e "$target" ] && continue
        cp "$file" "$target"
    done
}

# Activate local git hooks (.githooks/pre-commit + helper hooks). Catches
# secrets, personal fitness tracks, public-surface privacy slips, and internal
# workflow wording before they reach the remote. CI runs the generic rules from
# .github/workflows/pre-commit-guard.yml; per-clone private patterns stay local.
if git -C "$PROJECT_ROOT" rev-parse --is-inside-work-tree >/dev/null 2>&1 \
    && [ -x "$PROJECT_ROOT/.githooks/pre-commit" ] \
    && [ -x "$PROJECT_ROOT/.githooks/public-content-guard" ] \
    && [ -x "$PROJECT_ROOT/.githooks/commit-msg" ]; then
    current_hooks_path=$(git -C "$PROJECT_ROOT" config --get core.hooksPath 2>/dev/null || echo "")
    if [ "$current_hooks_path" = ".githooks" ]; then
        skipped "git hooks (core.hooksPath=.githooks)"
    else
        git -C "$PROJECT_ROOT" config core.hooksPath .githooks
        ok "git hooks activated (core.hooksPath=.githooks)"
    fi
elif ! git -C "$PROJECT_ROOT" rev-parse --is-inside-work-tree >/dev/null 2>&1; then
    info "Not a git checkout — skipping git hook activation."
else
    warn ".githooks/pre-commit, .githooks/public-content-guard, or .githooks/commit-msg not found or not executable — git hooks NOT activated."
fi

# ─── Step 2: Data Directories ─────────────────────────────────────────────────

step "Step 2: Data directories"

dirs=(
    "$DATA_ROOT/postgres"
    "$DATA_ROOT/brouter/segments"
    "$DATA_ROOT/brouter/profiles"
    "$DATA_ROOT/shadow-fs"
    "$DATA_ROOT/hotfolder"
    "$DATA_ROOT/library"
    "$DATA_ROOT/exports"
    "$DATA_ROOT/imports"
    "$DATA_ROOT/nats"
    "$DATA_ROOT/disabled/osm"
    "$DATA_ROOT/disabled/dem"
    "$DATA_ROOT/disabled/routing"
    "$DATA_ROOT/disabled/tiles"
    "$DATA_ROOT/disabled/badrouter"
)
for d in "${dirs[@]}"; do
    mkdir -p "$d"
done
prepare_api_writable_runtime_path "$DATA_ROOT/library" 0755
prepare_private_runtime_dir "$DATA_ROOT/exports"
prepare_private_runtime_dir "$DATA_ROOT/imports"
seed_brouter_profiles
ok "Data directories ready."

# ─── Step 3: Filesystem checks ────────────────────────────────────────────────

step "Step 3: Filesystem checks"

DB_DIRS=("$DATA_ROOT/postgres" "$DATA_ROOT/nats")

detect_fs_type() {
    local path="$1"
    if command -v stat &>/dev/null; then
        stat -f -c %T "$path" 2>/dev/null || echo "unknown"
    else
        echo "unknown"
    fi
}

check_nocow() {
    local path="$1"
    if ! command -v lsattr &>/dev/null; then return 2; fi
    local attrs
    attrs=$(lsattr -d "$path" 2>/dev/null) || return 2
    echo "$attrs" | grep -q "C" && return 0 || return 1
}

set_nocow() {
    local path="$1"
    chattr +C "$path" 2>/dev/null && return 0
    sudo chattr +C "$path" 2>/dev/null && return 0
    return 1
}

db_dir_has_existing_files() {
    local path="$1"
    find "$path" -mindepth 1 -maxdepth 3 2>/dev/null | grep -q .
}

print_btrfs_existing_data_migration_hint() {
    local dir="$1"
    error "$dir already contains database files without nocow."
    error "chattr +C only affects new files; migrate the existing database files into a nocow directory before continuing."
    error "Stop Skadia, create a new directory with chattr +C, copy the database files there, then swap it into place."
}

FS_TYPE=$(detect_fs_type "$DATA_ROOT/")

case "$FS_TYPE" in
    btrfs)
        info "Filesystem: btrfs detected."
        info "PostgreSQL on btrfs requires nocow (Copy-on-Write disabled) to prevent"
        info "data corruption on crash or OOM. See: https://wiki.postgresql.org/wiki/Btrfs"

        NOCOW_NEEDED=false
        EXISTING_DB_WITHOUT_NOCOW=false
        for dir in "${DB_DIRS[@]}"; do
            if check_nocow "$dir"; then
                ok "$dir — nocow already set."
            elif db_dir_has_existing_files "$dir"; then
                print_btrfs_existing_data_migration_hint "$dir"
                EXISTING_DB_WITHOUT_NOCOW=true
            else
                NOCOW_NEEDED=true
                warn "$dir — nocow NOT set."
            fi
        done

        if $EXISTING_DB_WITHOUT_NOCOW; then
            exit 1
        fi

        if $NOCOW_NEEDED; then
            info "Setting nocow on database directories..."
            FAILED=false
            for dir in "${DB_DIRS[@]}"; do
                if ! check_nocow "$dir" 2>/dev/null; then
                    if set_nocow "$dir"; then
                        ok "$dir — nocow set."
                    else
                        error "$dir — failed to set nocow. Run manually:"
                        error "  sudo chattr +C $(canonicalize_path "$dir")"
                        FAILED=true
                    fi
                fi
            done

            if $FAILED; then
                echo ""
                error "Some directories could not be configured."
                error "PostgreSQL on btrfs WITHOUT nocow WILL corrupt data on OOM or crash."
                if [ -d "$DATA_ROOT/postgres/data" ]; then
                    warn "$DATA_ROOT/postgres/ already contains data. chattr +C only affects NEW files."
                    warn "To fix existing data:"
                    warn "  1. podman compose stop postgres"
                    warn "  2. sudo mkdir $DATA_ROOT/postgres-new && sudo chattr +C $DATA_ROOT/postgres-new"
                    warn "  3. sudo cp -a $DATA_ROOT/postgres/. $DATA_ROOT/postgres-new/"
                    warn "  4. sudo mv $DATA_ROOT/postgres $DATA_ROOT/postgres-old && sudo mv $DATA_ROOT/postgres-new $DATA_ROOT/postgres"
                    warn "  5. podman compose start postgres"
                    warn "  6. After verification: sudo rm -rf $DATA_ROOT/postgres-old"
                fi
                if $QUIET; then
                    exit 1
                fi
                if ! $QUIET; then
                    read -rp "Continue anyway? (y/N) " _choice
                    [[ "$_choice" =~ ^[Yy] ]] || exit 1
                fi
            fi
        else
            ok "All database directories have nocow set."
        fi
        ;;
    ext*|xfs|tmpfs)
        ok "Filesystem: $FS_TYPE — no special configuration needed."
        ;;
    zfs)
        warn "Filesystem: ZFS detected."
        warn "For PostgreSQL on ZFS, set recordsize=8K on the dataset:"
        warn "  zfs set recordsize=8K <pool>/skadia/postgres"
        warn "See: https://wiki.postgresql.org/wiki/ZFS"
        ;;
    msdos|fuseblk|ntfs|ntfs3|nfs|nfs4|cifs|smb2|9p|fuse.sshfs|sshfs)
        if [ "${SKADIA_ALLOW_UNSAFE_DATA_ROOT:-false}" = "true" ]; then
            warn "Unsafe database filesystem override accepted for SKADIA_DATA_ROOT on $FS_TYPE."
        else
            error "SKADIA_DATA_ROOT is on unsafe filesystem $FS_TYPE for database storage."
            error "Use a native Linux filesystem (ext4, xfs, or btrfs with nocow), or set SKADIA_ALLOW_UNSAFE_DATA_ROOT=true after accepting the durability risk."
            exit 1
        fi
        ;;
    *)
        if [[ "$(uname -s)" == "Darwin" ]]; then
            ok "macOS/APFS — no special configuration needed."
        else
            info "Filesystem: $FS_TYPE — skipping filesystem checks."
        fi
        ;;
esac

# ─── Step 4: Environment file & secrets ───────────────────────────────────────

step "Step 4: Environment file & secrets"

if [ ! -f .env ]; then
    if [ -f .env.example ]; then
        secure_env_install_from_example .env.example .env
        info "Created .env from .env.example."
    else
        warn ".env.example not found — create .env manually."
        secure_env_install_from_example /dev/null .env
    fi
else
    secure_env_ensure_private .env "$QUIET"
    ok ".env already exists."
fi

env_matches_example_value() {
    local key="$1"
    [ -f .env.example ] || return 1
    [ "$(env_get "$key")" = "$(secure_env_get ".env.example" "$key")" ]
}

_env_looks_like_fresh_example=false
if ! $QUIET && ! $RECONFIG \
    && env_matches_example_value "SKADIA_DEPLOYMENT_MODE" \
    && env_matches_example_value "SKADIA_DEPLOYMENT_PROFILE" \
    && env_matches_example_value "SKADIA_STORAGE_MODE" \
    && env_matches_example_value "STORAGE_BACKEND" \
    && env_matches_example_value "COMPOSE_PROFILES" \
    && { [ "$(env_get "POSTGRES_PASSWORD")" = "skadia_dev_password" ] || [ -z "$(env_get "POSTGRES_PASSWORD")" ]; } \
    && { [ "$(env_get "READONLY_PASSWORD")" = "skadia_readonly_password" ] || [ -z "$(env_get "READONLY_PASSWORD")" ]; } \
    && ! env_is_set_or_file "JWT_SECRET" \
    && ! env_is_set_or_file "OAUTH_ENCRYPTION_KEY"; then
    _env_looks_like_fresh_example=true
fi

should_prompt_fresh_example_decision() {
    local key="$1"
    ! $QUIET && ! $RECONFIG && $_env_looks_like_fresh_example && env_matches_example_value "$key"
}

STORAGE_BACKEND_BEFORE="$(storage_effective_backend)"
if [ -z "$STORAGE_BACKEND_BEFORE" ] && ! $_env_looks_like_fresh_example; then
    STORAGE_BACKEND_BEFORE="local"
fi
STORAGE_MIGRATION_REQUIRED=false
STORAGE_MIGRATION_TARGET=""

_existing_compose_file="$(env_get "COMPOSE_FILE")"
case "$_existing_compose_file" in
    ""|"docker-compose.yml"|"docker-compose.yml:docker-compose.core.yml") ;;
    *)
        error "Custom COMPOSE_FILE is set to '$_existing_compose_file'."
        error "Move custom overrides into a reviewed deployment plan before re-running setup."
        exit 1
        ;;
esac
if [ -f docker-compose.override.yml ] && [ "${SKADIA_ALLOW_COMPOSE_OVERRIDE:-false}" != "true" ]; then
    error "docker-compose.override.yml is present and would be loaded implicitly by Compose."
    error "Review it, remove it, or set SKADIA_ALLOW_COMPOSE_OVERRIDE=true for this run."
    exit 1
fi

env_set "SKADIA_DEPLOYMENT_TARGET" "$DEPLOYMENT_TARGET"
env_set "SKADIA_DATA_ROOT" "$SKADIA_DATA_ROOT_VALUE"
env_set "SKADIA_CONTAINER_RUNTIME" "$SKADIA_SELECTED_CONTAINER_RUNTIME"
env_set "SKADIA_CONTAINER_USER" "$SKADIA_SELECTED_CONTAINER_USER"
env_set "DOCKER_SOCKET" "$SKADIA_SELECTED_DOCKER_SOCKET"
env_set "SKADIA_API_USERNS_MODE" "$SKADIA_SELECTED_API_USERNS_MODE"
env_set "SKADIA_ML_USERNS_MODE" "$SKADIA_SELECTED_ML_USERNS_MODE"
env_set "SKADIA_VOLUME_OPTS_RW_PRIVATE" "$SKADIA_SELECTED_VOLUME_OPTS_RW_PRIVATE"
env_set "SKADIA_VOLUME_OPTS_RW_SHARED" "$SKADIA_SELECTED_VOLUME_OPTS_RW_SHARED"
env_set "SKADIA_VOLUME_OPTS_RO_PRIVATE" "$SKADIA_SELECTED_VOLUME_OPTS_RO_PRIVATE"
env_set "SKADIA_VOLUME_OPTS_RO_SHARED" "$SKADIA_SELECTED_VOLUME_OPTS_RO_SHARED"

if git rev-parse --is-inside-work-tree >/dev/null 2>&1; then
    SKADIA_GIT_COMMIT_VALUE="$(git rev-parse --short=12 HEAD 2>/dev/null || printf unknown)"
    SKADIA_GIT_COMMIT_TIME_VALUE="$(git show -s --format=%cI HEAD 2>/dev/null || printf unknown)"
else
    SKADIA_GIT_COMMIT_VALUE="unknown"
    SKADIA_GIT_COMMIT_TIME_VALUE="unknown"
fi
SKADIA_BUILD_TIME_VALUE="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
env_set "SKADIA_GIT_COMMIT" "$SKADIA_GIT_COMMIT_VALUE"
env_set "SKADIA_GIT_COMMIT_TIME" "$SKADIA_GIT_COMMIT_TIME_VALUE"
env_set "SKADIA_BUILD_TIME" "$SKADIA_BUILD_TIME_VALUE"
if ! env_is_set "SKADIA_CONFIG_VERSION"; then
    env_set "SKADIA_CONFIG_VERSION" "1"
fi

# Track which secrets were freshly generated (for summary display)
declare -A GENERATED_SECRETS=()

postgres_cluster_initialized() {
    [ -d "$DATA_ROOT/postgres/data" ] \
        || [ -d "$DATA_ROOT/postgres/base" ] \
        || [ -d "$DATA_ROOT/postgres/global" ]
}

# POSTGRES_PASSWORD — generate if still set to hardcoded default or empty
_pg_val=$(env_get "POSTGRES_PASSWORD")
if [[ "$_pg_val" == "skadia_dev_password" || -z "$_pg_val" ]]; then
    if postgres_cluster_initialized; then
        warn "POSTGRES_PASSWORD still uses the setup placeholder, but an existing PostgreSQL data directory is present."
        warn "Leaving POSTGRES_PASSWORD and DATABASE_URL unchanged; rotate the database password explicitly instead."
    else
        _pg_new=$(openssl rand -hex 16 2>/dev/null || gen_secret | head -c 32)
        env_set "POSTGRES_PASSWORD" "$_pg_new"
        _db_url=$(env_get "DATABASE_URL")
        if [[ "$_db_url" == *"skadia_dev_password"* ]]; then
            env_set "DATABASE_URL" "${_db_url//skadia_dev_password/$_pg_new}"
        fi
        GENERATED_SECRETS["POSTGRES_PASSWORD"]="$_pg_new"
        ok "POSTGRES_PASSWORD generated."
    fi
else
    ok "POSTGRES_PASSWORD already set."
fi

_readonly_val=$(env_get "READONLY_PASSWORD")
if [[ "$_readonly_val" == "skadia_readonly_password" || -z "$_readonly_val" ]]; then
    _readonly_new=$(openssl rand -hex 16 2>/dev/null || gen_secret | head -c 32)
    env_set "READONLY_PASSWORD" "$_readonly_new"
    _readonly_url=$(env_get "READONLY_DATABASE_URL")
    if [[ "$_readonly_url" == *"skadia_readonly_password"* ]]; then
        env_set "READONLY_DATABASE_URL" "${_readonly_url//skadia_readonly_password/$_readonly_new}"
    fi
    GENERATED_SECRETS["READONLY_PASSWORD"]="$_readonly_new"
    ok "READONLY_PASSWORD generated."
else
    ok "READONLY_PASSWORD already set."
fi

# Secrets that are never overwritten
for key in JWT_SECRET OAUTH_ENCRYPTION_KEY LLM_ENCRYPTION_KEY KOMOOT_ENCRYPTION_KEY SETTINGS_ENCRYPTION_KEY ML_CALLBACK_SECRET GARAGE_RPC_SECRET GARAGE_ADMIN_TOKEN; do
    if [[ "$key" == "GARAGE_RPC_SECRET" || "$key" == "GARAGE_ADMIN_TOKEN" ]]; then
        _secret_present=false
        env_is_set "$key" && _secret_present=true
    else
        _secret_present=false
        env_is_set_or_file "$key" && _secret_present=true
    fi
    if ! $_secret_present; then
        _val=$(gen_secret)
        if [ -n "$_val" ]; then
            env_set "$key" "$_val"
            GENERATED_SECRETS["$key"]="$_val"
            ok "$key generated."
        else
            warn "Could not generate $key — set it manually in .env"
        fi
    else
        ok "$key already set."
    fi
done

ensure_local_garage_s3_credentials() {
    local _access
    local _secret
    local _garage_access
    local _garage_secret
    local _access_source=""
    local _secret_source=""

    _garage_access=$(env_get_secret_or_file "GARAGE_ACCESS_KEY")
    _garage_secret=$(env_get_secret_or_file "GARAGE_SECRET_KEY")

    _access=$(env_get_secret_or_file "S3_ACCESS_KEY")
    if [ -z "$_access" ] || [ "$_access" = "skadia-dev-key" ]; then
        if [ -n "$_garage_access" ] && [ "$_garage_access" != "skadia-dev-key" ]; then
            _access="$_garage_access"
            if env_is_set "GARAGE_ACCESS_KEY"; then
                _access_source="garage-inline"
            else
                _access_source="garage-file"
            fi
        else
            _access=$(gen_garage_access_key)
            _access_source="generated"
        fi
        if [ "$_access_source" != "garage-file" ]; then
            env_set "S3_ACCESS_KEY" "$_access"
        fi
        ok "S3_ACCESS_KEY set for local Garage."
    elif env_is_set "S3_ACCESS_KEY"; then
        _access_source="s3-inline"
    else
        _access_source="s3-file"
    fi
    if { [ -z "$_garage_access" ] || [ "$_garage_access" = "skadia-dev-key" ]; } \
        && { [ "$_access_source" = "generated" ] || [ "$_access_source" = "s3-inline" ]; }; then
        env_set "GARAGE_ACCESS_KEY" "$_access"
    fi

    _secret=$(env_get_secret_or_file "S3_SECRET_KEY")
    if [ -z "$_secret" ] || [ "$_secret" = "skadia-dev-secret" ]; then
        if [ -n "$_garage_secret" ] && [ "$_garage_secret" != "skadia-dev-secret" ]; then
            _secret="$_garage_secret"
            if env_is_set "GARAGE_SECRET_KEY"; then
                _secret_source="garage-inline"
            else
                _secret_source="garage-file"
            fi
        else
            _secret=$(gen_secret)
            _secret_source="generated"
        fi
        if [ -n "$_secret" ]; then
            if [ "$_secret_source" != "garage-file" ]; then
                env_set "S3_SECRET_KEY" "$_secret"
                GENERATED_SECRETS["S3_SECRET_KEY"]="$_secret"
            fi
            ok "S3_SECRET_KEY generated for local Garage."
        else
            warn "Could not generate S3_SECRET_KEY — set it manually in .env"
        fi
    elif env_is_set "S3_SECRET_KEY"; then
        _secret_source="s3-inline"
    else
        _secret_source="s3-file"
    fi
    if { [ -z "$_garage_secret" ] || [ "$_garage_secret" = "skadia-dev-secret" ]; } \
        && { [ "$_secret_source" = "generated" ] || [ "$_secret_source" = "s3-inline" ]; }; then
        env_set "GARAGE_SECRET_KEY" "$(env_get "S3_SECRET_KEY")"
    fi
}

# ─── Step 5: Deployment mode ──────────────────────────────────────────────────

step "Step 5: Deployment mode"

_mode_already_set=false
if ! $RECONFIG && env_is_set "SKADIA_DEPLOYMENT_MODE" && ! should_prompt_fresh_example_decision "SKADIA_DEPLOYMENT_MODE"; then
    DEPLOYMENT_MODE=$(env_get "SKADIA_DEPLOYMENT_MODE")
    skipped "Deployment mode ($DEPLOYMENT_MODE)"
    _mode_already_set=true
fi

if ! $_mode_already_set; then
    DEPLOYMENT_MODE="compose-local"
    if ! $QUIET; then
        echo ""
        echo "Deployment mode:"
        echo "  1) Private/local Compose install (localhost, built app container)"
        echo "  2) Private server Compose install (domain + HTTPS)"
        echo "  3) Development (Vite frontend on :5173)"
        echo ""
        read -rp "Deployment mode [1]: " _deployment_choice
        case "${_deployment_choice:-1}" in
            2) DEPLOYMENT_MODE="compose-server" ;;
            3) DEPLOYMENT_MODE="development" ;;
            *) DEPLOYMENT_MODE="compose-local" ;;
        esac
    fi
    env_set "SKADIA_DEPLOYMENT_MODE" "$DEPLOYMENT_MODE"
    ok "Deployment mode: $DEPLOYMENT_MODE"
fi

case "$DEPLOYMENT_MODE" in
    compose-local|compose-server|development) ;;
    *)
        error "SKADIA_DEPLOYMENT_MODE must be compose-local, compose-server, or development; got '$DEPLOYMENT_MODE'"
        exit 1
        ;;
esac

# ─── Step 6: Domain & URLs ────────────────────────────────────────────────────

step "Step 6: Domain & URLs"

if ! env_is_set "SKADIA_APP_PORT"; then
    env_set "SKADIA_APP_PORT" "8080"
fi

_domain_already_set=false
if [ "$DEPLOYMENT_MODE" = "compose-server" ] && ! $RECONFIG && env_is_set "DOMAIN"; then
    _existing_domain=$(env_get "DOMAIN")
    if [[ "$_existing_domain" != "localhost" ]]; then
        skipped "Domain ($_existing_domain)"
        _domain_already_set=true
        DOMAIN="$_existing_domain"
    fi
fi

if [ "$DEPLOYMENT_MODE" = "development" ]; then
    DOMAIN="localhost"
    env_set "DOMAIN" "$DOMAIN"
    env_set "TRAEFIK_ENTRYPOINT" "web"
    env_set "TRAEFIK_TLS_ENABLED" "false"
    env_set "TRAEFIK_CERTRESOLVER" ""
    env_set "APP_URL" "http://localhost:5173"
    env_set "FRONTEND_URL" "http://localhost:5173"
    if ! env_is_set "DEV_AUTH"; then
        env_set "DEV_AUTH" "false"
    fi
    ok "Development mode: Vite frontend URL configured, DEV_AUTH unchanged unless absent."
    mkdir -p traefik/dynamic
    remove_generated_redirect_config
elif [ "$DEPLOYMENT_MODE" = "compose-local" ]; then
    DOMAIN="localhost"
    env_set "DOMAIN" "$DOMAIN"
    env_set "TRAEFIK_ENTRYPOINT" "web"
    env_set "TRAEFIK_TLS_ENABLED" "false"
    env_set "TRAEFIK_CERTRESOLVER" ""
    env_set "APP_URL" "http://localhost:8080"
    env_set "FRONTEND_URL" "http://localhost:8080"
    env_set "DEV_AUTH" "false"
    ok "Local Compose mode: HTTP only, built app container, DEV_AUTH disabled."
    mkdir -p traefik/dynamic
    remove_generated_redirect_config
elif ! $_domain_already_set; then
    CURRENT_DOMAIN=$(env_get "DOMAIN")
    DEFAULT_DOMAIN=${CURRENT_DOMAIN:-}

    if $QUIET; then
        if [ -z "$DEFAULT_DOMAIN" ] || [ "$DEFAULT_DOMAIN" = "localhost" ]; then
            error "compose-server requires a non-localhost DOMAIN"
            exit 1
        fi
        DOMAIN=$DEFAULT_DOMAIN
    else
        if [ -n "$DEFAULT_DOMAIN" ]; then
            read -rp "Domain for Skadia [$DEFAULT_DOMAIN]: " DOMAIN
            DOMAIN=${DOMAIN:-$DEFAULT_DOMAIN}
        else
            read -rp "Domain for Skadia: " DOMAIN
        fi
    fi

    env_set "DOMAIN" "$DOMAIN"

    if [ "$DOMAIN" = "localhost" ]; then
        error "compose-server requires a non-localhost DOMAIN"
        exit 1
    fi
fi

if [ "$DEPLOYMENT_MODE" = "compose-server" ]; then
    if ! $QUIET; then
        read -rp "ACME email for Let's Encrypt: " ACME_EMAIL
    fi
    ACME_EMAIL=${ACME_EMAIL:-}
    if [ -n "$ACME_EMAIL" ]; then
        env_set "ACME_EMAIL" "$ACME_EMAIL"
        ok "ACME_EMAIL set."
    else
        warn "No ACME_EMAIL provided — set it manually in .env for TLS."
    fi
    env_set "TRAEFIK_ENTRYPOINT" "websecure"
    env_set "TRAEFIK_TLS_ENABLED" "true"
    env_set "TRAEFIK_CERTRESOLVER" "letsencrypt"
    env_set "APP_URL" "https://$DOMAIN"
    env_set "FRONTEND_URL" "https://$DOMAIN"
    env_set "DEV_AUTH" "false"
    ok "Server Compose mode: domain=$DOMAIN, HTTPS via Let's Encrypt, DEV_AUTH disabled."
    write_https_redirect_config
fi

# ─── Step 7: Database tuning ──────────────────────────────────────────────────

step "Step 7: Database tuning"

_db_tuning_done=false
if [ -f docker-compose.override.yml ] && grep -q "random_page_cost" docker-compose.override.yml 2>/dev/null; then
    warn "docker-compose.override.yml contains legacy PostgreSQL command tuning."
    warn "Compose replaces the whole postgres command when command is overridden;"
    warn "move these settings to POSTGRES_* variables in .env to keep the base safety flags."
fi

if ! $RECONFIG \
    && env_is_set "POSTGRES_RANDOM_PAGE_COST" \
    && env_is_set "POSTGRES_EFFECTIVE_IO_CONCURRENCY" \
    && env_is_set "POSTGRES_EFFECTIVE_CACHE_SIZE" \
    && env_is_set "POSTGRES_MAINTENANCE_WORK_MEM"; then
    skipped "Database tuning ($(env_get POSTGRES_RANDOM_PAGE_COST), $(env_get POSTGRES_EFFECTIVE_CACHE_SIZE))"
    _db_tuning_done=true
fi

if ! $_db_tuning_done; then
    DB_STORAGE_TYPE="nvme"
    if ! $QUIET; then
        echo ""
        echo "What type of storage does your database use?"
        echo "  1) NVMe/SSD (recommended)"
        echo "  2) HDD"
        echo ""
        echo "Note: SSD/NVMe is strongly recommended. HDD could experience significant"
        echo "performance issues with OSM import and routing features."
        echo ""
        read -rp "Storage type [1]: " _db_choice
        case "${_db_choice:-1}" in
            2) DB_STORAGE_TYPE="hdd" ;;
            *) DB_STORAGE_TYPE="nvme" ;;
        esac
    fi

    if [ "$DB_STORAGE_TYPE" = "hdd" ]; then
        _rpc=4.0; _eic=2; _ecs="2GB"; _mwm="256MB"
        _label="HDD"
    else
        _rpc=1.1; _eic=200; _ecs="4GB"; _mwm="512MB"
        _label="NVMe/SSD"
    fi

    env_set "POSTGRES_RANDOM_PAGE_COST" "$_rpc"
    env_set "POSTGRES_EFFECTIVE_IO_CONCURRENCY" "$_eic"
    env_set "POSTGRES_EFFECTIVE_CACHE_SIZE" "$_ecs"
    env_set "POSTGRES_MAINTENANCE_WORK_MEM" "$_mwm"
    ok "Database tuning written to .env ($_label profile)."
    DB_TUNING_LABEL="$_label optimized"
else
    DB_TUNING_LABEL="already configured"
fi

# ─── Step 8: Deployment profile ───────────────────────────────────────────────

step "Step 8: Deployment profile"

profile_summary() {
    local profile="$1"
    case "$profile" in
        core)
            echo "Core includes: uploads/imports, media, GDPR, BRouter/RD5, heuristic lifts, AI agent"
            echo "Core excludes: local OSM import, BadRouter, DEM, Coverage, offline maps, ML pipelines"
            ;;
        *)
            echo "Full includes: Core plus local OSM import, BadRouter, DEM, Coverage, offline maps, and optional ML pipelines"
            echo "Full excludes: local LLM containers unless the llm profile is enabled; storage-specific local path actions on S3"
            ;;
    esac
}

_deployment_profile_already_set=false
if ! $RECONFIG && env_is_set "SKADIA_DEPLOYMENT_PROFILE" && ! should_prompt_fresh_example_decision "SKADIA_DEPLOYMENT_PROFILE"; then
    skipped "Deployment profile ($(env_get SKADIA_DEPLOYMENT_PROFILE))"
    _deployment_profile_already_set=true
    DEPLOYMENT_PROFILE="$(env_get "SKADIA_DEPLOYMENT_PROFILE")"
fi

if ! $_deployment_profile_already_set; then
    DEPLOYMENT_PROFILE="full"
    if ! $QUIET; then
        echo ""
        echo "Deployment profile:"
        echo "  1) Full (default) — all local geodata/routing features when data is present"
        echo "  2) Core — reduced install without local OSM, BadRouter, DEM, Coverage, offline maps, or ML pipelines"
        echo ""
        profile_summary "full"
        echo ""
        profile_summary "core"
        echo ""
        read -rp "Deployment profile [1]: " _profile_choice
        case "${_profile_choice:-1}" in
            2) DEPLOYMENT_PROFILE="core" ;;
            *) DEPLOYMENT_PROFILE="full" ;;
        esac
    fi
    env_set "SKADIA_DEPLOYMENT_PROFILE" "$DEPLOYMENT_PROFILE"
fi

case "$DEPLOYMENT_PROFILE" in
    core)
        env_set "COMPOSE_FILE" "docker-compose.yml"
        env_set "OSM_DATA_DIR" "/data/disabled/osm"
        env_set "DEM_DATA_DIR" "/data/disabled/dem"
        env_set "ROUTING_GRAPH_PATH" "/data/disabled/routing/graph.bin"
        env_set "TILES_DIR" "/data/disabled/tiles"
        env_set "BADROUTER_PROFILE_DIR" "/data/disabled/badrouter"
        env_set "OSM_HOST_PATH" "$DATA_ROOT/disabled/osm"
        env_set "DEM_HOST_PATH" "$DATA_ROOT/disabled/dem"
        env_set "ROUTING_HOST_PATH" "$DATA_ROOT/disabled/routing"
        env_set "TILES_HOST_PATH" "$DATA_ROOT/disabled/tiles"
        env_set "BADROUTER_CONFIG_HOST_PATH" "$DATA_ROOT/disabled/badrouter"
        DEPLOYMENT_PROFILE_LABEL="Core"
        ;;
    full|"")
        DEPLOYMENT_PROFILE="full"
        env_set "SKADIA_DEPLOYMENT_PROFILE" "full"
        env_set "COMPOSE_FILE" "docker-compose.yml"
        env_set "OSM_DATA_DIR" ""
        env_set "DEM_DATA_DIR" ""
        env_set "ROUTING_GRAPH_PATH" ""
        env_set "TILES_DIR" ""
        env_set "BADROUTER_PROFILE_DIR" ""
        env_set "OSM_HOST_PATH" ""
        env_set "DEM_HOST_PATH" ""
        env_set "ROUTING_HOST_PATH" ""
        env_set "TILES_HOST_PATH" ""
        env_set "BADROUTER_CONFIG_HOST_PATH" ""
        DEPLOYMENT_PROFILE_LABEL="Full"
        ;;
    *)
        error "SKADIA_DEPLOYMENT_PROFILE must be core or full, got '$DEPLOYMENT_PROFILE'"
        exit 1
        ;;
esac
profile_summary "$DEPLOYMENT_PROFILE"

# ─── Step 9: Storage backend ──────────────────────────────────────────────────

step "Step 9: Storage backend"

GARAGE_INIT_REQUIRED=false
_storage_already_set=false
if [ "$DEPLOYMENT_PROFILE" = "core" ]; then
    _existing_storage_mode="$(env_get "SKADIA_STORAGE_MODE")"
    _existing_storage_backend="$(env_get "STORAGE_BACKEND")"
    _existing_s3_provider="$(env_get "SKADIA_S3_PROVIDER")"
    if ! $RECONFIG && { [ "$_existing_storage_backend" = "s3" ] || [[ "$_existing_storage_mode" == s3-* ]] || [ "$_existing_s3_provider" = "local-garage" ]; }; then
        error "Core profile cannot silently switch an existing S3/Garage storage configuration to local."
        error "Choose Full, or run --reconfigure and deliberately migrate storage before selecting Core."
        exit 1
    fi
    _storage_already_set=true
    STORAGE="local"
    env_set "SKADIA_STORAGE_MODE" "local"
    env_set "STORAGE_BACKEND" "local"
    env_set "SKADIA_S3_PROVIDER" ""
    env_set "GDPR_ARTIFACT_BACKEND" "auto"
    ok "Core profile uses local filesystem storage."
fi
if ! $_storage_already_set && ! $RECONFIG && { env_is_set "SKADIA_STORAGE_MODE" || env_is_set "STORAGE_BACKEND"; } \
    && ! should_prompt_fresh_example_decision "SKADIA_STORAGE_MODE"; then
    skipped "Storage ($(env_get SKADIA_STORAGE_MODE) / backend $(env_get STORAGE_BACKEND))"
    _storage_already_set=true
    STORAGE=$(env_get "SKADIA_STORAGE_MODE")
    if [ -z "$STORAGE" ]; then
        STORAGE=$(env_get "STORAGE_BACKEND")
    fi
fi

if ! $_storage_already_set; then
    STORAGE="local"
    if [ "$(env_get "SKADIA_DEPLOYMENT_MODE")" = "compose-server" ]; then
        STORAGE="s3-external"
    fi
    if ! $QUIET; then
        echo ""
        echo "What storage backend do you want to use?"
        echo "  1) Local filesystem (simplest, recommended for single-server)"
        echo "  2) Local S3 via Garage (S3 API on this machine, included in Skadia)"
        echo "  3) External S3-compatible endpoint"
        echo ""
        _storage_default="1"
        if [ "$(env_get "SKADIA_DEPLOYMENT_MODE")" = "compose-server" ]; then
            _storage_default="3"
        fi
        read -rp "Storage backend [$_storage_default]: " _storage_choice
        case "${_storage_choice:-$_storage_default}" in
            2) STORAGE="s3-garage" ;;
            3) STORAGE="s3-external" ;;
            *) STORAGE="local" ;;
        esac
    fi

    case "$STORAGE" in
        s3-garage)
            env_set "SKADIA_STORAGE_MODE" "s3-local-garage"
            env_set "STORAGE_BACKEND" "s3"
            env_set "SKADIA_S3_PROVIDER" "local-garage"
            env_set "GDPR_ARTIFACT_BACKEND" "auto"
            if ! env_is_set "S3_ENDPOINT"; then env_set "S3_ENDPOINT" "http://garage:3900"; fi
            if ! env_is_set "S3_BUCKET"; then env_set "S3_BUCKET" "skadia-originals"; fi
            if ! env_is_set "S3_BUCKET_EXPORTS"; then env_set "S3_BUCKET_EXPORTS" "skadia-exports"; fi
            if ! env_is_set "S3_BUCKET_IMPORTS"; then env_set "S3_BUCKET_IMPORTS" "skadia-imports"; fi
            if ! env_is_set "S3_EXPORTS_PREFIX"; then env_set "S3_EXPORTS_PREFIX" "gdpr/exports"; fi
            if ! env_is_set "S3_IMPORTS_PREFIX"; then env_set "S3_IMPORTS_PREFIX" "gdpr/imports"; fi
            if ! env_is_set "S3_REGION"; then env_set "S3_REGION" "us-east-1"; fi
            if ! env_is_set "S3_FORCE_PATH_STYLE"; then env_set "S3_FORCE_PATH_STYLE" "true"; fi
            ensure_local_garage_s3_credentials
            ok "Storage: Local Garage S3 selected."
            ensure_garage_config
            STORAGE_LABEL="Garage (local S3)"
            GARAGE_INIT_REQUIRED=true
            ;;
        s3-external)
            env_set "SKADIA_STORAGE_MODE" "s3-external"
            env_set "STORAGE_BACKEND" "s3"
            env_set "SKADIA_S3_PROVIDER" "external"
            env_set "GDPR_ARTIFACT_BACKEND" "auto"
            if is_local_garage_endpoint "$(env_get "S3_ENDPOINT")"; then
                env_set "S3_ENDPOINT" ""
            fi
            if ! env_is_set "S3_BUCKET_EXPORTS"; then env_set "S3_BUCKET_EXPORTS" "skadia-exports"; fi
            if ! env_is_set "S3_BUCKET_IMPORTS"; then env_set "S3_BUCKET_IMPORTS" "skadia-imports"; fi
            if ! env_is_set "S3_EXPORTS_PREFIX"; then env_set "S3_EXPORTS_PREFIX" "gdpr/exports"; fi
            if ! env_is_set "S3_IMPORTS_PREFIX"; then env_set "S3_IMPORTS_PREFIX" "gdpr/imports"; fi
            echo ""
            info "Set the following S3 variables in .env:"
            info "  S3_ENDPOINT, S3_ACCESS_KEY, S3_SECRET_KEY, S3_BUCKET"
            info "  S3_BUCKET_ACTIVITY, S3_BUCKET_MEDIA, S3_REGION, S3_FORCE_PATH_STYLE"
            info "  S3_BUCKET_EXPORTS, S3_BUCKET_IMPORTS, S3_EXPORTS_PREFIX, S3_IMPORTS_PREFIX"
            STORAGE_LABEL="External S3"
            ;;
        *)
            env_set "SKADIA_STORAGE_MODE" "local"
            env_set "STORAGE_BACKEND" "local"
            env_set "SKADIA_S3_PROVIDER" ""
            env_set "GDPR_ARTIFACT_BACKEND" "auto"
            ok "Storage: local filesystem selected."
            STORAGE_LABEL="Local filesystem"
            ;;
    esac
else
    case "$STORAGE" in
        s3-local-garage)
            env_set "STORAGE_BACKEND" "s3"
            env_set "SKADIA_S3_PROVIDER" "local-garage"
            GARAGE_INIT_REQUIRED=true
            if ! env_is_set "S3_ENDPOINT"; then env_set "S3_ENDPOINT" "http://garage:3900"; fi
            if ! env_is_set "S3_REGION"; then env_set "S3_REGION" "us-east-1"; fi
            if ! env_is_set "S3_FORCE_PATH_STYLE"; then env_set "S3_FORCE_PATH_STYLE" "true"; fi
            ensure_local_garage_s3_credentials
            ensure_garage_config
            STORAGE_LABEL="Garage (local S3)"
            ;;
        s3-external)
            env_set "STORAGE_BACKEND" "s3"
            env_set "SKADIA_S3_PROVIDER" "external"
            if is_local_garage_endpoint "$(env_get "S3_ENDPOINT")"; then
                env_set "S3_ENDPOINT" ""
            fi
            STORAGE_LABEL="External S3"
            ;;
        s3)
            STORAGE_LABEL="S3"
            _endpoint=$(env_get "S3_ENDPOINT")
            _provider=$(env_get "SKADIA_S3_PROVIDER")
            if [ "$_provider" = "local-garage" ] || { [ -z "$_provider" ] && is_local_garage_endpoint "$_endpoint"; }; then
                GARAGE_INIT_REQUIRED=true
                env_set "SKADIA_STORAGE_MODE" "s3-local-garage"
                if [ -z "$_provider" ]; then env_set "SKADIA_S3_PROVIDER" "local-garage"; fi
                if ! env_is_set "S3_REGION"; then env_set "S3_REGION" "us-east-1"; fi
                if ! env_is_set "S3_FORCE_PATH_STYLE"; then env_set "S3_FORCE_PATH_STYLE" "true"; fi
                ensure_local_garage_s3_credentials
                ensure_garage_config
                STORAGE_LABEL="Garage (local S3)"
            else
                env_set "SKADIA_STORAGE_MODE" "s3-external"
                if [ -z "$_provider" ]; then env_set "SKADIA_S3_PROVIDER" "external"; fi
                if is_local_garage_endpoint "$(env_get "S3_ENDPOINT")"; then
                    env_set "S3_ENDPOINT" ""
                fi
                STORAGE_LABEL="External S3"
            fi
            ;;
        *)
            env_set "SKADIA_STORAGE_MODE" "local"
            env_set "STORAGE_BACKEND" "local"
            env_set "SKADIA_S3_PROVIDER" ""
            STORAGE_LABEL="Local filesystem"
            ;;
    esac
fi

if [ "$(env_get "STORAGE_BACKEND")" = "s3" ] && ! $GARAGE_INIT_REQUIRED; then
    missing_s3=()
    for key in S3_ENDPOINT S3_ACCESS_KEY S3_SECRET_KEY S3_BUCKET S3_BUCKET_EXPORTS S3_BUCKET_IMPORTS S3_REGION; do
        if [[ "$key" == "S3_ACCESS_KEY" || "$key" == "S3_SECRET_KEY" ]]; then
            env_is_set_or_file "$key" || missing_s3+=("$key")
        elif ! env_is_set "$key"; then
            missing_s3+=("$key")
        fi
    done
    if [ "${#missing_s3[@]}" -gt 0 ]; then
        if $QUIET; then
            error "External S3 requires ${missing_s3[*]}"
            exit 1
        fi
        warn "External S3 is missing: ${missing_s3[*]}"
        warn "Set these values in .env before starting the API."
    fi
fi

STORAGE_BACKEND_AFTER="$(storage_effective_backend)"
if ! $_env_looks_like_fresh_example \
    && [ -n "$STORAGE_BACKEND_BEFORE" ] \
    && [ -n "$STORAGE_BACKEND_AFTER" ] \
    && [ "$STORAGE_BACKEND_BEFORE" != "$STORAGE_BACKEND_AFTER" ]; then
    STORAGE_MIGRATION_REQUIRED=true
    STORAGE_MIGRATION_TARGET="$STORAGE_BACKEND_AFTER"
    warn "Storage backend changed from $STORAGE_BACKEND_BEFORE to $STORAGE_BACKEND_AFTER."
    warn "setup.sh updated .env only; it did not migrate existing files."
    warn "Keep the API stopped, back up the database and storage, then run migrate-storage before the next start."
fi

# ─── Step 10: SMTP ────────────────────────────────────────────────────────────

step "Step 10: SMTP (optional)"

SMTP_CONFIGURED=false
if ! $RECONFIG && env_is_set "SMTP_HOST"; then
    skipped "SMTP ($(env_get SMTP_HOST))"
    SMTP_CONFIGURED=true
fi

SMTP_LABEL="not configured — use Admin UI"

if ! $SMTP_CONFIGURED; then
    if ! $QUIET; then
        echo ""
        echo "Configure SMTP for email notifications?"
        echo "You can also configure this later in the Admin UI under System > Email."
        echo "  1) Configure now"
        echo "  2) Skip (configure later)"
        echo ""
        read -rp "SMTP [2]: " _smtp_choice
        if [[ "${_smtp_choice:-2}" == "1" ]]; then
            read -rp "SMTP host: " _smtp_host
            read -rp "SMTP port [587]: " _smtp_port
            _smtp_port=${_smtp_port:-587}
            read -rp "SMTP username: " _smtp_user
            read -rsp "SMTP password: " _smtp_pass; echo ""
            read -rp "SMTP from address: " _smtp_from
            echo "TLS mode:"
            echo "  1) starttls (default, port 587)"
            echo "  2) ssl (port 465)"
            echo "  3) none"
            read -rp "TLS mode [1]: " _smtp_tls_choice
            case "${_smtp_tls_choice:-1}" in
                2) _smtp_tls="ssl" ;;
                3) _smtp_tls="none" ;;
                *) _smtp_tls="starttls" ;;
            esac

            if [ -n "$_smtp_host" ]; then
                env_set "SMTP_HOST"     "$_smtp_host"
                env_set "SMTP_PORT"     "$_smtp_port"
                env_set "SMTP_USERNAME" "$_smtp_user"
                env_set "SMTP_PASSWORD" "$_smtp_pass"
                env_set "SMTP_FROM"     "$_smtp_from"
                env_set "SMTP_TLS"      "$_smtp_tls"
                ok "SMTP configured (host: $_smtp_host, port: $_smtp_port, tls: $_smtp_tls)."
                SMTP_LABEL="$_smtp_host:$_smtp_port ($_smtp_tls)"
            else
                warn "No SMTP host entered — skipping."
            fi
        else
            info "SMTP skipped — configure later in Admin UI > System > Email."
        fi
    else
        info "SMTP skipped in quiet mode — configure in .env or Admin UI."
    fi
fi

# ─── Step 11: Registration mode ───────────────────────────────────────────────

step "Step 11: Registration mode"

_reg_already_set=false
if ! $RECONFIG && env_is_set "REGISTRATION_MODE"; then
    skipped "Registration mode ($(env_get REGISTRATION_MODE))"
    _reg_already_set=true
    REG_LABEL=$(env_get "REGISTRATION_MODE")
fi

if ! $_reg_already_set; then
    REG_MODE="invite"
    if ! $QUIET; then
        echo ""
        echo "Registration mode:"
        echo "  1) Invite-only (default — admin creates accounts)"
        echo "  2) Open (anyone can register, email verification required)"
        echo "  3) Approval (anyone can register, admin must approve)"
        echo ""
        read -rp "Registration mode [1]: " _reg_choice
        case "${_reg_choice:-1}" in
            2) REG_MODE="open" ;;
            3) REG_MODE="approval" ;;
            *) REG_MODE="invite" ;;
        esac
    fi
    env_set "REGISTRATION_MODE" "$REG_MODE"
    ok "Registration mode: $REG_MODE"
    REG_LABEL="$REG_MODE"
fi

# ─── Step 12: Optional service profiles ───────────────────────────────────────

step "Step 12: Optional service profiles"

_profiles_already_set=false
if ! $RECONFIG && env_is_set "COMPOSE_PROFILES" && ! should_prompt_fresh_example_decision "COMPOSE_PROFILES"; then
    skipped "Compose profiles ($(env_get COMPOSE_PROFILES))"
    _profiles_already_set=true
    PROFILES_LABEL=$(env_get "COMPOSE_PROFILES")
fi

if ! $_profiles_already_set; then
    PROFILES=""
    if [ "$DEPLOYMENT_PROFILE" = "full" ]; then
        PROFILES="full-local-data"
    fi
    # Carry over local Garage only. External S3 must not start the bundled Garage service.
    if $GARAGE_INIT_REQUIRED; then
        PROFILES="$(profile_list_append_missing "$PROFILES" "s3")"
    fi
    if [ "$DEPLOYMENT_MODE" = "compose-server" ]; then
        PROFILES="$(profile_list_append_missing "$PROFILES" "proxy")"
    fi

    if ! $QUIET && [ "$DEPLOYMENT_PROFILE" = "full" ]; then
        read -rp "Enable ML service (analytics/document jobs; photo embeddings require S3 media)? [y/N]: " _enable_ml
        if [[ "${_enable_ml:-}" =~ ^[Yy] ]]; then
            PROFILES="$(profile_list_append_missing "$PROFILES" "ml")"
        fi
        read -rp "Enable local LLM containers? External LLM providers do not need this profile. [y/N]: " _enable_llm
        if [[ "${_enable_llm:-}" =~ ^[Yy] ]]; then
            PROFILES="$(profile_list_append_missing "$PROFILES" "llm")"
        fi
    fi

    if [ -n "$PROFILES" ]; then
        env_set "COMPOSE_PROFILES" "$PROFILES"
        ok "Compose profiles: $PROFILES"
        PROFILES_LABEL="$PROFILES"
    else
        ok "No optional profiles enabled (core services only)."
        PROFILES_LABEL="none (core only)"
        if $RECONFIG; then
            env_set "COMPOSE_PROFILES" ""
        fi
    fi
fi

if $_profiles_already_set; then
    _profiles="$(env_get "COMPOSE_PROFILES")"
    if [ "$DEPLOYMENT_PROFILE" = "core" ]; then
        for _blocked_profile in full-local-data s3 ml llm; do
            _profiles="$(profile_list_remove "$_profiles" "$_blocked_profile")"
        done
    else
        _profiles="$(profile_list_prepend_missing "$_profiles" "full-local-data")"
        if $GARAGE_INIT_REQUIRED; then
            _profiles="$(profile_list_append_missing "$_profiles" "s3")"
        else
            _profiles="$(profile_list_remove "$_profiles" "s3")"
        fi
    fi
    if [ "$DEPLOYMENT_MODE" = "compose-server" ]; then
        _profiles="$(profile_list_append_missing "$_profiles" "proxy")"
    else
        _profiles="$(profile_list_remove "$_profiles" "proxy")"
    fi
    env_set "COMPOSE_PROFILES" "$_profiles"
    PROFILES_LABEL="${_profiles:-none (core only)}"
    ok "Compose profiles normalized: ${PROFILES_LABEL}."
fi

prepare_optional_runtime_dirs
ok "Profile-specific data directories ready."

# ─── Step 13: TimescaleDB compression ─────────────────────────────────────────

step "Step 13: TimescaleDB compression"

skipped "TimescaleDB compression is enabled by database migrations and can be changed later in Admin settings."
TS_LABEL="managed in Admin settings"

# ─── Step 14: Admin account ───────────────────────────────────────────────────

step "Step 14: Admin account"

ADMIN_LABEL="create in browser (first user becomes admin)"

_admin_already_set=false
if ! $RECONFIG && env_is_set "BOOTSTRAP_ADMIN_USER"; then
    skipped "Admin account ($(env_get BOOTSTRAP_ADMIN_USER))"
    _admin_already_set=true
    ADMIN_LABEL=$(env_get "BOOTSTRAP_ADMIN_USER")
fi

if ! $_admin_already_set; then
    if ! $QUIET; then
        echo ""
        echo "Create initial admin account?"
        echo "Alternatively, the first user created in the browser becomes admin."
        echo "  1) Create now"
        echo "  2) Skip (create in browser)"
        echo ""
        read -rp "Admin account [2]: " _admin_choice
        if [[ "${_admin_choice:-2}" == "1" ]]; then
            read -rp "Admin username: " _admin_user
            while true; do
                read -rsp "Admin password: " _admin_pass; echo ""
                read -rsp "Confirm password: " _admin_pass2; echo ""
                if [ "$_admin_pass" = "$_admin_pass2" ]; then
                    break
                fi
                warn "Passwords do not match — try again."
            done

            if [ -n "$_admin_user" ] && [ -n "$_admin_pass" ]; then
                # Ensure the block is present in .env
                if ! grep -q "^# Bootstrap admin" .env 2>/dev/null && \
                   ! grep -q "BOOTSTRAP_ADMIN_USER" .env 2>/dev/null; then
                    echo "" >> .env
                    echo "# Bootstrap admin (optional, consumed on first start when no users exist)" >> .env
                    echo "# SECURITY: Remove BOOTSTRAP_ADMIN_PASSWORD after first start" >> .env
                fi
                env_set "BOOTSTRAP_ADMIN_USER"     "$_admin_user"
                env_set "BOOTSTRAP_ADMIN_PASSWORD"  "$_admin_pass"
                ok "Admin account set: $_admin_user"
                warn "SECURITY: Remove BOOTSTRAP_ADMIN_PASSWORD from .env after first start."
                ADMIN_LABEL="$_admin_user (bootstrap)"
            else
                warn "Username or password empty — skipping admin account creation."
            fi
        else
            info "Admin account skipped — first user registered in browser becomes admin."
        fi
    else
        info "Admin account skipped in quiet mode."
    fi
fi

# ─── Step 15: Port collision check ────────────────────────────────────────────

step "Step 15: Port collision check"

_app_port="$(env_get "SKADIA_APP_PORT")"
_app_port="${_app_port:-8080}"
case "$_app_port" in
    ''|*[!0-9]*)
        warn "SKADIA_APP_PORT is not numeric ($_app_port); skipping app port collision check."
        _ports_to_check=()
        ;;
    *)
        case "$DEPLOYMENT_MODE" in
            compose-server)
                _ports_to_check=(80 443 "$_app_port")
                ;;
            development)
                _ports_to_check=(5173 "$_app_port")
                ;;
            *)
                _ports_to_check=("$_app_port")
                ;;
        esac
        ;;
esac

_port_ok=true
for port in "${_ports_to_check[@]}"; do
    if ss -tlnp 2>/dev/null | grep -q ":$port " || \
       netstat -tlnp 2>/dev/null | grep -q ":$port " 2>/dev/null; then
        warn "Port $port is already in use — Skadia may not start correctly."
        _port_ok=false
    fi
done
if $_port_ok && [ "${#_ports_to_check[@]}" -gt 0 ]; then
    _ports_label=""
    for port in "${_ports_to_check[@]}"; do
        if [ -z "$_ports_label" ]; then
            _ports_label="$port"
        else
            _ports_label="$_ports_label, $port"
        fi
    done
    ok "Ports $_ports_label are free."
fi

# ─── Step 16: Compose render check ────────────────────────────────────────────

step "Step 16: Compose render check"

_compose_file_for_check="$(env_get "COMPOSE_FILE")"
_compose_profiles_for_check="$(env_get "COMPOSE_PROFILES")"
if COMPOSE_FILE="${_compose_file_for_check:-docker-compose.yml}" \
   COMPOSE_PROFILES="${_compose_profiles_for_check:-}" \
   "$RUNTIME" compose config >/dev/null; then
    ok "Compose configuration renders successfully."
else
    error "Compose configuration did not render. Review .env and docker-compose.yml."
    exit 1
fi

# ─── Step 17: Summary ─────────────────────────────────────────────────────────

step "Step 17: Summary"

DOMAIN_DISPLAY=$(env_get "DOMAIN")
DOMAIN_URL=$(env_get "APP_URL")
if [ -z "$DOMAIN_URL" ]; then
    [[ "$DOMAIN_DISPLAY" == "localhost" ]] && DOMAIN_URL="http://localhost:8080" || DOMAIN_URL="https://$DOMAIN_DISPLAY"
fi

echo ""
echo -e "${BOLD}${CYAN}===============================================================${NC}"
echo -e "${BOLD}  Skadia Setup Complete${NC}"
echo -e "${BOLD}${CYAN}===============================================================${NC}"
echo ""
printf "  %-25s %s\n" "Domain:"           "$DOMAIN_URL"
printf "  %-25s %s\n" "Deployment profile:" "${DEPLOYMENT_PROFILE_LABEL:-Full}"
printf "  %-25s %s\n" "Storage:"          "${STORAGE_LABEL:-local filesystem}"
printf "  %-25s %s\n" "Database:"         "${DB_TUNING_LABEL:-configured}"
printf "  %-25s %s\n" "SMTP:"             "${SMTP_LABEL:-not configured — use Admin UI}"
printf "  %-25s %s\n" "Registration:"     "${REG_LABEL:-invite}"
printf "  %-25s %s\n" "Profiles:"         "${PROFILES_LABEL:-none (core only)}"
printf "  %-25s %s\n" "Compression:"      "${TS_LABEL:-true}"
printf "  %-25s %s\n" "Admin account:"    "${ADMIN_LABEL:-create in browser}"

if [ ${#GENERATED_SECRETS[@]} -gt 0 ]; then
    echo ""
    if $QUIET; then
        echo -e "  ${YELLOW}Generated secrets saved in .env; values suppressed in quiet mode.${NC}"
        for key in "${!GENERATED_SECRETS[@]}"; do
            printf "    %-30s = %s\n" "$key" "<saved in .env>"
        done
    else
        echo -e "  ${YELLOW}Generated secrets (shown only once — saved in .env):${NC}"
        for key in "${!GENERATED_SECRETS[@]}"; do
            printf "    %-30s = %s\n" "$key" "${GENERATED_SECRETS[$key]}"
        done
    fi
fi

echo ""
echo "  Next steps:"
echo "    1. Review .env for additional settings"
if $STORAGE_MIGRATION_REQUIRED; then
    _next_step=2
    if $GARAGE_INIT_REQUIRED; then
        echo "    $_next_step. Initialize Garage:      ./scripts/init-garage.sh --start"
        _next_step=$((_next_step + 1))
    fi
    echo "    $_next_step. Keep API stopped:       ./scripts/compose.sh stop api ml"
    _next_step=$((_next_step + 1))
    echo "    $_next_step. Run storage dry-run:    ./scripts/compose.sh run --rm api migrate-storage -target=$STORAGE_MIGRATION_TARGET --dry-run"
    _next_step=$((_next_step + 1))
    echo "    $_next_step. Run storage migration: ./scripts/compose.sh run --rm api migrate-storage -target=$STORAGE_MIGRATION_TARGET"
    _next_step=$((_next_step + 1))
    echo "    $_next_step. Start services:         ./scripts/compose.sh up -d --build --remove-orphans"
    _next_step=$((_next_step + 1))
    echo "    $_next_step. Open in browser:        $DOMAIN_URL"
    _next_step=$((_next_step + 1))
    echo "    $_next_step. Verify SMTP:            Admin UI > System > Email > Send Test Email"
    echo "    Storage migration guide: docs/documentation/user/how-to/howto-storage-backend-migration.md"
elif $GARAGE_INIT_REQUIRED; then
    echo "    2. Initialize Garage: ./scripts/init-garage.sh --start"
    echo "    3. Start services:    ./scripts/compose.sh up -d --build --remove-orphans"
    echo "    4. Open in browser:   $DOMAIN_URL"
    echo "    5. Verify SMTP:       Admin UI > System > Email > Send Test Email"
else
    echo "    2. Start services:  ./scripts/compose.sh up -d --build --remove-orphans"
    echo "    3. Open in browser: $DOMAIN_URL"
    echo "    4. Verify SMTP:     Admin UI > System > Email > Send Test Email"
fi
echo ""
echo "  For Kubernetes deployments, see:"
echo "  docs/documentation/dev/subsystems/kubernetes-deployment.md"
echo ""
echo -e "${BOLD}${CYAN}===============================================================${NC}"
echo ""
