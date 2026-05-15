#!/usr/bin/env bash
set -euo pipefail

secure_temp_dir() {
    local purpose="${1:-tmp}"
    local parent="${TMPDIR:-/tmp}"
    local tmp=""

    if ! command -v mktemp >/dev/null 2>&1; then
        echo "ERROR: mktemp is required to create private temporary directories." >&2
        return 1
    fi

    mkdir -p "$parent"
    tmp="$(umask 077 && mktemp -d "$parent/skadia-${purpose}.XXXXXXXXXX" 2>/dev/null)" \
        || tmp="$(umask 077 && mktemp -d -t "skadia-${purpose}.XXXXXXXXXX" 2>/dev/null)" \
        || {
            echo "ERROR: could not create a private temporary directory under $parent." >&2
            return 1
        }

    chmod 0700 "$tmp" 2>/dev/null || {
        echo "ERROR: could not set private permissions on temporary directory: $tmp" >&2
        rm -rf "$tmp"
        return 1
    }
    printf '%s\n' "$tmp"
}

if [[ "${BASH_SOURCE[0]}" == "$0" ]]; then
    case "${1:-}" in
        dir)
            shift
            secure_temp_dir "${1:-tmp}"
            ;;
        *)
            echo "Usage: $0 dir [purpose]" >&2
            exit 2
            ;;
    esac
fi
