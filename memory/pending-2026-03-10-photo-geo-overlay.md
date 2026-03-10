## docs/07_DATABASE_SCHEMA.md — Tags-System erweitert
**Suche nach:** SeedDefaultAutoTags
**Delta:**
- `SeedDefaultAutoTags` legt jetzt 3 Custom-Auto-Tags an: Nacht, Frost, **Rundtour** (is_round_trip-Bedingung, grün #10B981)
- Neu: 6 Weather-System-Tags (category='weather', is_system=true): Sonnig, Bewölkt, Regen, Schnee, Nebel, Gewitter
- Nach dem Seeden wird automatisch `MigrateTagConditionsToRules` aufgerufen (idempotent)

## docs/08_API_REFERENCE.md — Neue Route
**Suche nach:** tags/seed-defaults
**Delta:**
- `DELETE /api/v1/tags/orphan` — body: `{"name": "tagname"}` — entfernt Orphan-Tag-Name aus allen `activities.tags TEXT[]`; gibt `{"affected": N}` zurück

## docs/07_DATABASE_SCHEMA.md — Orphan-Tags
**Suche nach:** ListTags
**Delta:**
- `ListTags` hängt jetzt Orphan-Tags an (category='orphan', ID="") — Tag-Namen aus `activities.tags TEXT[]` ohne entsprechenden `tags`-Tabelleneintrag
- Neue DB-Funktion `DeleteOrphanTag(ctx, userID, tagName)` — UPDATE activities SET tags = array_remove(tags, $2) WHERE user_id=$1

## CHANGELOG.md
**Delta:**
- feat(tags): add Rundtour auto-tag and 6 weather system tags to SeedDefaultAutoTags
- feat(tags): ListTags now appends orphan tags (names in activities.tags with no tags table entry)
- feat(tags): new DeleteOrphanTag DB function and DELETE /api/v1/tags/orphan endpoint
- feat(tags): SeedDefaultAutoTags auto-calls MigrateTagConditionsToRules after seeding
- feat(tags): move "Tags migrieren" button from AutomationRulesPage to TagsPage with auto-migration on load
- feat(tags): add orphan tags section in TagsPage with remove button; filter orphans from main table and merge selects
- feat(tags): add deleteOrphanTag API function in frontend/src/api/tags.ts
