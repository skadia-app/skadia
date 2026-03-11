## CHANGELOG.md
**Suche nach:** `## [Unreleased]`
**Delta:**
- fix(tags): alle genutzten Tags in Workshop→Tags sichtbar: `ListTags` liefert jetzt auch Orphan-Tags (Namen in `activities.tags` TEXT[] ohne entsprechenden `tags`-Eintrag) mit `category='orphan'`; neue Sektion "Verwaiste Tags" in TagsPage mit "Entfernen"-Button
- fix(tags): `SeedDefaultAutoTags` erweitert: legt jetzt auch "Rundtour"-Auto-Tag + 6 Wetter-System-Tags an und führt anschließend auto. Migration von tag.conditions → automation_rules durch
- fix(tags): Auto-Migration beim TagsPage-Load: wenn Tags mit conditions gefunden werden, wird `migrateTagConditions` automatisch ausgelöst; invalidiert auch `['automation-rules']` Query
- fix(tags): Auto-Seed beim TagsPage-Load: `seedDefaultTags()` wird beim ersten Mount aufgerufen → Rundtour/Wetter-Tags + Automation-Rules werden automatisch angelegt
- fix(tags): "Tags migrieren"-Button von AutomationRulesPage nach TagsPage verschoben
- fix(tags): `MigrateTagConditionsToRules` — kritischer Bug: `jsonb_array_length()` liefert NULL für JSON-Objekte → WHERE-Klausel immer false → 0 Regeln angelegt; Fix: String-Vergleich statt jsonb_array_length; Go-seitiges Unwrapping alter Autotag-Format `{"operator":"...","rules":[...]}` zu flachem Array + condLogic
- feat(api): `DELETE /api/v1/tags/orphan` — entfernt Orphan-Tag-Namen aus allen activities.tags TEXT[]

## docs/02_CORE_BACKEND.md — Tags-System
**Suche nach:** Tag
**Delta:** `migrateExistingTagsToTable()` — sammelt alle Tag-Namen aus activities.tags TEXT[] und erstellt fehlende Einträge in tags table mit auto-assigned Farben (damit alles EXISTIERENDE Tags in der Übersicht sichtbar werden, nicht gelöscht); `SeedDefaultAutoTags` legt NUR fehlende Nacht/Frost/Rundtour + Wetter-Tags an; `MigrateTagConditionsToRules` Bug-Fix (jsonb_array_length → String-Vergleich + Go-seitiges Format-Unwrapping für alte JSON-Objekt-Format)
