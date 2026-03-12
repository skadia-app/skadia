## docs/02_CORE_BACKEND.md — ImportService + pHash
**Suche nach:** Importer
**Delta:** Neuer `ImportService` in `services/api/internal/importer/service.go` vereint alle 6 Import-Einstiegspunkte. Bietet ProcessActivity(), ProcessPhoto(), ProcessOrphanPhoto(), ResolveQueuedPhoto(), PostProcess(). pHash-Paket in `internal/phash/` (goimagehash v1.1.0, threshold=3 Bits). Alle Importer (WebUI, URL, Hotfolder, Garmin, Strava, Komoot) nutzen ImportService.

## docs/07_DATABASE_SCHEMA.md — Neue Spalten 000149+000150
**Suche nach:** activity_photos
**Delta:** `activity_photos` hat neue Spalten: `phash BIGINT`, `width INTEGER`, `height INTEGER`. `user_enrichment_settings` hat `phash_threshold_bits INTEGER DEFAULT 3`. `import_queue.queue_type` unterstützt jetzt `'image_no_match'`.

## CHANGELOG.md
**Suche nach:** Unreleased
**Delta:** Cross-platform import pipeline Entry hinzugefügt (siehe oben)
