## docs/07_DATABASE_SCHEMA.md — activity_lift_candidates Tabelle
**Suche nach:** activity_detected_laps
**Delta:** Neue Tabelle `activity_lift_candidates` (Migration 000070): id UUID PK, activity_id FK→activities, started_at/ended_at TIMESTAMPTZ, duration_s INT, ascent_m DOUBLE, confidence TEXT, confirmed BOOL, created_at; UNIQUE(activity_id, started_at); Index auf activity_id

## docs/08_API_REFERENCE.md — Neue Map/Lift-Endpoints
**Suche nach:** /segments
**Delta:**
- `GET /api/v1/geo/visited-pois` — GeoJSON FeatureCollection aller named Stops (duration_s ≥ 900s) des Users
- `POST /api/v1/segments/import-lifts` — Importiert osm_lifts als lift-Segmente (idempotent), gibt `{"imported": N}` zurück
- `GET /api/v1/activities/{id}/lift-candidates` — Lift-Kandidaten einer Aktivität
- `POST /api/v1/activities/{id}/lift-candidates/{cid}/confirm` — Kandidat bestätigen (204)
- `DELETE /api/v1/activities/{id}/lift-candidates/{cid}` — Kandidat ablehnen/löschen (204)
- `GET /api/v1/admin/osm-status` — Zählt osm_peaks, osm_lifts, osm_settlements; gibt has_data flag zurück

## docs/10_DEVELOPMENT_PLAN.md — Map-Features Backend
**Suche nach:** Map-Features
**Delta:** Go-Backend für Map-Features implementiert: Migration 000070 (activity_lift_candidates), DB-Methoden GetVisitedPOIs/GetLiftCandidates/ConfirmLiftCandidate/ImportLiftSegments, Handler geo.go/lift_import.go/lift_candidates.go/osm_admin.go, alle Routen in main.go registriert

## CHANGELOG.md
**Delta:**
### Added
- Migration 000070: `activity_lift_candidates` Tabelle für ML-erkannte Liftfahrten
- `GET /api/v1/geo/visited-pois` — GeoJSON-Endpunkt für besuchte Named Stops
- `POST /api/v1/segments/import-lifts` — OSM-Lifte als Segmente importieren
- `GET/POST/DELETE /api/v1/activities/{id}/lift-candidates[/{cid}]` — Lift-Kandidaten verwalten
- `GET /api/v1/admin/osm-status` — OSM-Tabellen-Statistik
