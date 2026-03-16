## docs/07_DATABASE_SCHEMA.md — activity_tag_removals
**Suche nach:** activity_tags
**Delta:** Neue Tabelle activity_tag_removals (activity_id, tag_id, removed_at). Schützt User-Entscheidungen: Enrichment fügt System-Tags nicht wieder hinzu, wenn User sie explizit entfernt hat. Rundtour-Trigger bidirektional (is_round_trip ↔ activity_tags). weather_auto_tags Spalte in users (default true).

## docs/02_CORE_BACKEND.md — Weather Enrichment
**Suche nach:** Weather
**Delta:** SyncWeatherTagsToActivityTags() schreibt Weather-Tags nach Enrichment in activity_tags. 4 Call-Sites gepatcht (stage1.go, importer/service.go, handler/enrichment.go, handler/tags.go). Respektiert activity_tag_removals. weather_auto_tags User-Preference (default true). WeatherPrefs struct + GetWeatherPrefs/UpdateWeatherPrefs um AutoTags bool erweitert.

## CHANGELOG.md
**Delta:**
- feat(db): Migration 000173 — activity_tag_removals, Rundtour trigger bidirektional, Schnee text_color fix, weather tag backfill, weather_auto_tags pref
- feat(tags): activity_tag_removals DB-Helper (RecordTagRemoval/ClearTagRemoval/IsTagRemovalRecorded); System-Tag-Entfernungen werden gespeichert; Zuweisung löscht Entfernungseintrag
- feat(weather): SyncWeatherTagsToActivityTags() — Weather-Tags werden nach Enrichment in activity_tags geschrieben (4 Call-Sites)
- feat(recompute): Rundtour-Tag wird wie E-Bike in reprocessTags erhalten (trigger-managed)
- fix(seed): Rundtour ist jetzt trigger-managed (is_auto=false), nicht mehr autotag
