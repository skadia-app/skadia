#!/usr/bin/env bash
set -euo pipefail

repo_root="$(git rev-parse --show-toplevel)"
cd "$repo_root/services/api"

go run ./cmd/sporttaxgen --repo-root "$repo_root" "$@"
