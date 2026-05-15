#!/usr/bin/env bash
set -euo pipefail

_secure_env_lib_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=scripts/lib/secure_temp.sh
source "$_secure_env_lib_dir/secure_temp.sh"

secure_env_stat_mode() {
    local file="$1"
    stat -c '%a' "$file" 2>/dev/null || stat -f '%Lp' "$file" 2>/dev/null || echo ""
}

secure_env_ensure_private() {
    local file="$1"
    local quiet="${2:-false}"
    local mode

    if [ ! -e "$file" ]; then
        (umask 077 && : > "$file")
    fi
    mode="$(secure_env_stat_mode "$file")"
    if [ "$quiet" = "true" ] && [ -n "$mode" ] && [ "$mode" != "600" ]; then
        echo "ERROR: $file permissions are $mode; expected 600." >&2
        return 1
    fi
    chmod 0600 "$file" 2>/dev/null || {
        if [ "$quiet" = "true" ]; then
            echo "ERROR: cannot set private permissions on $file" >&2
            return 1
        fi
        echo "WARN: cannot set private permissions on $file; fix permissions manually." >&2
        return 1
    }

    mode="$(secure_env_stat_mode "$file")"
    if [ -n "$mode" ] && [ "$mode" != "600" ]; then
        echo "ERROR: $file permissions are $mode; expected 600." >&2
        return 1
    fi
}

secure_env_get() {
    local file="$1"
    local key="$2"
    grep "^${key}=" "$file" 2>/dev/null | head -1 | cut -d= -f2- || true
}

secure_env_is_set() {
    local file="$1"
    local key="$2"
    grep -q "^${key}=.\+" "$file" 2>/dev/null
}

secure_env_tmp_dir_for_file() {
    local file="$1"
    local dir
    local base

    dir="$(dirname "$file")"
    base="$(basename "$file")"
    mkdir -p "$dir"
    if ! command -v mktemp >/dev/null 2>&1; then
        echo "ERROR: mktemp is required to update $file safely." >&2
        return 1
    fi
    (umask 077 && mktemp -d "$dir/.${base}.tmp.XXXXXXXXXX")
}

secure_env_set() {
    local file="$1"
    local key="$2"
    local value="$3"
    local quiet="${4:-false}"
    local tmpdir
    local tmp

    secure_env_ensure_private "$file" "$quiet"
    tmpdir="$(secure_env_tmp_dir_for_file "$file")"
    tmp="$tmpdir/env"
    ENV_SET_VALUE="$value" awk -v key="$key" '
        BEGIN {
            value = ENVIRON["ENV_SET_VALUE"]
            written = 0
        }
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
    ' "$file" > "$tmp"
    chmod 0600 "$tmp"
    mv "$tmp" "$file"
    rmdir "$tmpdir" 2>/dev/null || rm -rf "$tmpdir"
}

secure_env_install_from_example() {
    local example="$1"
    local target="$2"
    local tmpdir
    local tmp

    tmpdir="$(secure_env_tmp_dir_for_file "$target")"
    tmp="$tmpdir/env"
    if [ -f "$example" ]; then
        (umask 077 && cp "$example" "$tmp")
    else
        (umask 077 && : > "$tmp")
    fi
    chmod 0600 "$tmp"
    mv "$tmp" "$target"
    rmdir "$tmpdir" 2>/dev/null || rm -rf "$tmpdir"
    chmod 0600 "$target"
}
