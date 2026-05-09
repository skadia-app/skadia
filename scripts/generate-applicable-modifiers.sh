#!/usr/bin/env bash
set -euo pipefail
repo_root="$(git rev-parse --show-toplevel)"
cd "$repo_root/services/api"
source="$repo_root/config/sport-taxonomy/modifiers.yaml"

# Capture caller flags (e.g. --check) before they are shadowed by function positional args.
script_flags=("$@")

generate() {
  local sub="$1" prefix="$2" goout="$3" tsout="$4"
  local args=(--subsystem "$sub" --export-prefix "$prefix" --source "$source")
  [ -n "$goout" ] && args+=(--go-out "$goout")
  [ -n "$tsout" ] && args+=(--ts-out "$tsout")
  go run ./cmd/applicablemodifiersgen "${args[@]}" "${script_flags[@]}"
}

generate gear        Gear        "$repo_root/services/api/internal/gear/applicable_modifiers_generated.go"        "$repo_root/frontend/src/lib/generatedGearApplicableModifiers.ts"
generate rules       Rules       "$repo_root/services/api/internal/rules/applicable_modifiers_generated.go"       "$repo_root/frontend/src/lib/generatedRulesApplicableModifiers.ts"
generate dataimport  Dataimport  "$repo_root/services/api/internal/dataimport/applicable_modifiers_generated.go"  ""
generate shadowfs    Shadowfs    "$repo_root/services/api/internal/shadowfs/applicable_modifiers_generated.go"    ""
generate routefilter Routing     "$repo_root/services/api/internal/routefilter/applicable_modifiers_generated.go" "$repo_root/frontend/src/lib/generatedRoutingApplicableModifiers.ts"
generate bestefforts Records     "$repo_root/services/api/internal/bestefforts/applicable_modifiers_generated.go" ""
generate heatmap     Heatmap     "$repo_root/services/api/internal/heatmap/applicable_modifiers_generated.go"     "$repo_root/frontend/src/lib/generatedHeatmapApplicableModifiers.ts"
generate quick-edit  QuickEdit   ""                                                                                "$repo_root/frontend/src/lib/generatedQuickEditApplicableModifiers.ts"
