#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$repo_root/services/api"

go run ./cmd/applicablemodifiersgen \
  --subsystem quick-edit \
  --export-prefix QuickEdit \
  --source "$repo_root/config/frontend/applicable_modifiers.yaml" \
  --ts-out "$repo_root/frontend/src/lib/generatedQuickEditApplicableModifiers.ts" \
  "$@"
# No --go-out — frontend-only consumer.
