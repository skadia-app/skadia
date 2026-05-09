#!/usr/bin/env bash
set -euo pipefail
repo_root="$(git rev-parse --show-toplevel)"
cd "$repo_root/services/api"
go run ./cmd/applicablemodifiersgen \
    --all \
    --repo-root "$repo_root" \
    --registry "$repo_root/config/applicable-modifiers/subsystems.yaml" \
    --source "$repo_root/config/sport-taxonomy/modifiers.yaml" \
    --known-subsystems-out "$repo_root/services/api/internal/sporttaxgen/subsystems_generated.go" \
    --schema-enum-out "$repo_root/config/sport-taxonomy/schema/_subsystems_enum.schema.json" \
    --registry-schema "$repo_root/config/applicable-modifiers/schema/subsystems.schema.json" \
    --schema-embed-out "$repo_root/services/api/internal/applicablemodifiers/subsystem_registry_schema.json" \
    "$@"
