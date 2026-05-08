#!/usr/bin/env bash
set -euo pipefail
repo_root="$(git rev-parse --show-toplevel)"
cd "$repo_root/services/api"
go run ./cmd/tagsgen \
  --source "$repo_root/config/tags/system_tags.yaml" \
  --schema "$repo_root/config/tags/schema/system_tags.schema.json" \
  --go-out "$repo_root/services/api/internal/tags/system_tags_generated.go" \
  --ts-out "$repo_root/frontend/src/lib/generatedSystemTags.ts" \
  "$@"
