## services/api/internal/rules/engine.go — set_field Action
**Suche nach:** case "set_field"
**Delta:** Neue Action `set_field` mit Whitelist (`is_round_trip`, `is_indoor`). `haversineM` → `HaversineM` (exportiert für externe Nutzung in database-Paket).

## services/api/internal/database/gear.go — GetActivityAsMap + ApplyRuleChange
**Suche nach:** GetActivityAsMap
**Delta:** Neue SELECT-Spalten: `a.started_at`, `ST_Y(a.end_location::geometry) AS end_lat`, `ST_X(a.end_location::geometry) AS end_lon`. Neue dynamische Felder: `sun_elevation` (via suncalc), `start_end_distance_m` (via rules.HaversineM). NULL-Guard: kein start_end_distance_m ohne GPS-Koordinaten. Neue Imports: `math`, `time`, `suncalc`, `rules`.

**Suche nach:** case "is_round_trip"
**Delta:** Neuer case `is_round_trip` in ApplyRuleChange() — UPDATE activities SET is_round_trip = $1.

## CHANGELOG.md
**Delta:**
- feat(rules): add `set_field` action (whitelist: `is_round_trip`, `is_indoor`) for direct boolean field updates via automation rules
- feat(rules): `GetActivityAsMap` now computes `sun_elevation` (via suncalc) and `start_end_distance_m` (Haversine) as dynamic condition fields
- feat(rules): export `HaversineM` for cross-package use
