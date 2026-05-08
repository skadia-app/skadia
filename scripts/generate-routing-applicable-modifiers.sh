#!/usr/bin/env bash
set -euo pipefail
repo_root="$(git rev-parse --show-toplevel)"
cd "$repo_root/services/api"
go run ./cmd/applicablemodifiersgen \
  --subsystem routefilter \
  --export-prefix Routing \
  --source "$repo_root/config/routing/applicable_modifiers.yaml" \
  --go-out "$repo_root/services/api/internal/routefilter/applicable_modifiers_generated.go" \
  --ts-out "$repo_root/frontend/src/lib/generatedRoutingApplicableModifiers.ts" \
  "$@"
