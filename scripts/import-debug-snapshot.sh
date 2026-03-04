#!/usr/bin/env bash
# Kompaktes Diagnoseskript für Import-Debugging.
# Gibt nur das Wesentliche aus — kein Log-Flooding.
# Verwendung: bash scripts/import-debug-snapshot.sh

echo "=== DB-ZÄHLER ==="
distrobox-host-exec podman exec skadia-postgres-1 psql -U skadia -d skadia -c "
SELECT
  (SELECT COUNT(*) FROM activities)                  AS activities,
  (SELECT status          FROM garmin_import_jobs ORDER BY created_at DESC LIMIT 1) AS job_status,
  (SELECT completed_modules FROM garmin_import_jobs ORDER BY created_at DESC LIMIT 1) AS completed,
  (SELECT total_modules   FROM garmin_import_jobs ORDER BY created_at DESC LIMIT 1) AS total,
  (SELECT current_module  FROM garmin_import_jobs ORDER BY created_at DESC LIMIT 1) AS current_module;
"

echo ""
echo "=== FEHLER (gruppiert, Top 5 Typen) ==="
distrobox-host-exec podman logs skadia-api-1 2>&1 \
  | grep '"level":"WARN"\|"level":"ERROR"' \
  | python3 -c "
import sys, json, collections
errors = collections.Counter()
for line in sys.stdin:
    try:
        d = json.loads(line)
        errors[(d.get('msg','?'), str(d.get('error',''))[:100])] += 1
    except:
        pass
for (msg, err), n in errors.most_common(5):
    print(f'  {n:5}x  {msg}' + (f': {err}' if err else ''))
" || true

echo ""
echo "=== LETZTE 3 ERROR-ZEILEN ==="
distrobox-host-exec podman logs skadia-api-1 2>&1 \
  | grep '"level":"ERROR"\|panic' | tail -3 || echo "  (keine)"
