#!/usr/bin/env bash
set -euo pipefail
repo_root="$(git rev-parse --show-toplevel)"
cd "$repo_root/services/api"
go run ./cmd/applicablemodifiersgen \
  --subsystem shadowfs \
  --export-prefix Shadowfs \
  --source "$repo_root/config/shadowfs/applicable_modifiers.yaml" \
  --go-out "$repo_root/services/api/internal/shadowfs/applicable_modifiers_generated.go" \
  "$@"
