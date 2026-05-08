#!/usr/bin/env bash
set -euo pipefail
repo_root="$(git rev-parse --show-toplevel)"
cd "$repo_root/services/api"
go run ./cmd/applicablemodifiersgen \
  --subsystem rules \
  --export-prefix Rules \
  --source "$repo_root/config/rules/applicable_modifiers.yaml" \
  --go-out "$repo_root/services/api/internal/rules/applicable_modifiers_generated.go" \
  --ts-out "$repo_root/frontend/src/lib/generatedRulesApplicableModifiers.ts" \
  "$@"
