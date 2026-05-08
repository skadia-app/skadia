#!/usr/bin/env bash
set -euo pipefail
repo_root="$(git rev-parse --show-toplevel)"
cd "$repo_root/services/api"
go run ./cmd/applicablemodifiersgen \
  --subsystem dataimport \
  --export-prefix Dataimport \
  --source "$repo_root/config/dataimport/applicable_modifiers.yaml" \
  --go-out "$repo_root/services/api/internal/dataimport/applicable_modifiers_generated.go" \
  "$@"
