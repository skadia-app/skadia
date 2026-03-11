## CHANGELOG.md
**Suche nach:** `## [Unreleased]`
**Delta:**
- fix(tags): alle genutzten Tags in Workshop→Tags sichtbar: `migrateExistingTagsToTable()` sammelt alle alten Tag-Namen aus `activities.tags` TEXT[] und erstellt fehlende `tags` Einträge mit auto-assigned Farben
- fix(tags): `migrateExistingTagsToTable()` erweitert: erstellt jetzt auch `activity_tags` Junction-Einträge für jede alte Aktivität mit altem Tag + synchronisiert `activity_count` korrekt
- fix(tags): Frost-Feldnamen korrigiert: `temperature_min_c` → `min_temperature_c`, `temperature_avg_c` → `avg_temperature_c`; fehlendes Go-Modell-Feld `MinTemperatureC`/`MaxTemperatureC` hinzugefügt; Frontend-Types aktualisiert
- fix(tags): Migration 000140 erstellt: Löscht alte falsche Automation-Rules mit `temperature_min_c` (unauswertbar), neue korrekte Rules werden beim API-Startup via `SeedDefaultAutoTags` geseedet
- fix(tags): `SeedDefaultAutoTags` erweitert: legt jetzt auch "Rundtour"-Auto-Tag + 6 Wetter-System-Tags an
- fix(tags): Auto-Migration beim TagsPage-Load: wenn Tags mit conditions gefunden werden, wird `migrateTagConditions` automatisch ausgelöst; invalidiert auch `['automation-rules']` Query
- fix(tags): Auto-Seed beim TagsPage-Load: `seedDefaultTags()` wird beim ersten Mount aufgerufen → Rundtour/Wetter-Tags + Automation-Rules werden automatisch angelegt
- fix(tags): "Tags migrieren"-Button von AutomationRulesPage nach TagsPage verschoben
- fix(tags): `MigrateTagConditionsToRules` — kritischer Bug: `jsonb_array_length()` liefert NULL für JSON-Objekte → WHERE-Klausel immer false → 0 Regeln angelegt; Fix: String-Vergleich statt jsonb_array_length; Go-seitiges Unwrapping alter Autotag-Format `{"operator":"...","rules":[...]}` zu flachem Array + condLogic
- feat(api): `DELETE /api/v1/tags/orphan` — entfernt Orphan-Tag-Namen aus allen activities.tags TEXT[]

## docs/02_CORE_BACKEND.md — Tags-System
**Suche nach:** Tag
**Delta:** `migrateExistingTagsToTable()` — sammelt alle Tag-Namen aus activities.tags TEXT[] und erstellt fehlende Einträge in tags table mit auto-assigned Farben (damit alles EXISTIERENDE Tags in der Übersicht sichtbar werden, nicht gelöscht); `SeedDefaultAutoTags` legt NUR fehlende Nacht/Frost/Rundtour + Wetter-Tags an; `MigrateTagConditionsToRules` Bug-Fix (jsonb_array_length → String-Vergleich + Go-seitiges Format-Unwrapping für alte JSON-Objekt-Format)

## docs/07_DATABASE_SCHEMA.md — is_ebike Spalte
**Suche nach:** is_ebike
**Delta:** is_ebike ist jetzt eine reguläre BOOLEAN-Spalte (nicht mehr GENERATED ALWAYS); Migration 000141; e_biking als sport_type existiert nicht mehr

## docs/02_CORE_BACKEND.md — E-Bike Normalisierung
**Suche nach:** e_biking
**Delta:** FIT/GPX/TCX/Strava-Parser normalisieren e_biking/e_bike_fitness/e_bike_mountain → cycling + is_ebike=true; e_biking ist kein gültiger sport_type mehr

## docs/09_FEATURE_CATALOG.md — Neue Sport-Typen
**Suche nach:** sledding
**Delta:** Neue Sport-Typen: sledding (Rodeln, Winter-Kategorie) + unicycling (Einrad, Sub-Type unter cycling)

## CHANGELOG.md — E-Bike Cleanup
**Delta:** refactor(taxonomy): e_biking sport_type durch is_ebike Boolean-Flag ersetzt; Migration 000141 migriert 54 Aktivitäten; sledding + unicycling hinzugefügt
