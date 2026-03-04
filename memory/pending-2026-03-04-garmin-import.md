## docs/10_DEVELOPMENT_PLAN.md — Phase 7 / Garmin Import
**Suche nach:** Garmin Import
**Delta:** Garmin-Export-Import implementiert (Phase vor 7): registry pattern, 4 Importers (profile/gear/activities/health), DB-Layer, Frontend-UI

## docs/02_CORE_BACKEND.md — Garmin Import
**Suche nach:** import
**Delta:** Neues `services/api/internal/garmin/` Package: registry.go, source.go, importer_profile.go, importer_gear.go, importer_activities.go, importer_steps.go; handler/garmin_import.go mit 3 Endpoints; database/garmin_import.go + garmin_health.go

## docs/07_DATABASE_SCHEMA.md — Garmin IDs + Import-Tabellen
**Suche nach:** garmin
**Delta:**
- Migration 000047: `garmin_activity_id BIGINT` auf activities, `garmin_gear_id TEXT` auf gear (je unique index)
- Migration 000048: `garmin_import_jobs` Tabelle (async job tracking)
- Migration 000049: `garmin_import_mappings` Tabelle (source → skadia ID mapping)

## docs/08_API_REFERENCE.md — Garmin Import Endpoints
**Suche nach:** import
**Delta:**
- POST /api/v1/import/garmin/detect – Erkennt verfügbare Module im Export-Verzeichnis
- POST /api/v1/import/garmin/start – Startet async Import-Job
- GET /api/v1/import/garmin/jobs/{id} – Job-Status abfragen

## CHANGELOG.md
**Delta:**
### Added
- feat(garmin): Garmin-Export-Import — registry-basierter Importer für Profil, Ausrüstung, Aktivitäten (FIT/GPX/TCX) und Gesundheitsdaten (Schritte, Schlaf)
- Migrationen 000047–000049: garmin_activity_id/garmin_gear_id IDs, garmin_import_jobs, garmin_import_mappings
- Frontend ImportPage (/settings/import) mit Modul-Erkennung, Checkbox-Auswahl und Job-Fortschrittsanzeige
- Link in SettingsPage → "Datenimport"
