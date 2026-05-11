#!/usr/bin/env bash
set -euo pipefail
hits=$(grep -rln "default_for_sports" \
  services/api/internal/database/gear.go \
  services/api/internal/model/activity.go \
  services/api/internal/dataimport/sport_taxonomy.go \
  services/api/internal/dataimport/sport_taxonomy_test.go \
  services/api/internal/dataimport/engine_test.go \
  frontend/src/api/types.ts \
  frontend/src/components/GearForm.tsx \
  frontend/src/components/GearForm.test.tsx \
  frontend/src/pages/GearPage.tsx 2>/dev/null || true)
if [ -n "$hits" ]; then
  echo "FAIL: default_for_sports references remain in:"
  echo "$hits"
  exit 1
fi
echo "OK: code-sweep complete"
