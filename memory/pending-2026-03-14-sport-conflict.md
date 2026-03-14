## CHANGELOG.md
- feat: Sport-Typ-Konflikt-Erkennung bei Strava/Komoot-Import (SegmentGroup-Vergleich, Queue, Modal)
- **Breaking Change**: Komoot-Import — bei unterschiedlichen Sport-Gruppen gewinnt jetzt die Gerätedatei statt Komoot-API

## docs/07_DATABASE_SCHEMA.md — activities Tabelle
**Suche nach:** sport_type_is_manual
**Delta:** Neue Spalte `sport_conflict_pending BOOLEAN NOT NULL DEFAULT false` + partieller Index

## docs/07_DATABASE_SCHEMA.md — import_queue Tabelle
**Suche nach:** import_queue_queue_type_check
**Delta:** Neuer queue_type 'sport_conflict' in CHECK constraint; funktionaler Index auf metadata->>'activity_id'

## docs/08_API_REFERENCE.md — Activities Endpoints
**Suche nach:** /activities/{id}/recompute
**Delta:** Neuer Endpoint: `PATCH /activities/{id}/resolve-sport-conflict` (Body: `{sport_type?}`, Response: `{status}`)

## docs/09_FEATURE_CATALOG.md
**Suche nach:** Sport-Taxonomie
**Delta:** Neues Feature: Sport-Typ-Konflikt-Erkennung & -Auflösung (Strava + Komoot Import)
