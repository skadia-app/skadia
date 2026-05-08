#!/usr/bin/env bash
set -euo pipefail
repo_root="$(git rev-parse --show-toplevel)"
cd "$repo_root/services/api"
go run ./cmd/applicablemodifiersgen \
  --subsystem heatmap \
  --export-prefix Heatmap \
  --source "$repo_root/config/heatmap/applicable_modifiers.yaml" \
  --go-out "$repo_root/services/api/internal/heatmap/applicable_modifiers_generated.go" \
  --ts-out "$repo_root/frontend/src/lib/generatedHeatmapApplicableModifiers.ts" \
  "$@"
