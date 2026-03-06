# Wetter-Tags als System-Kategorie — Plan Summary

**Status:** ✅ Plan erstellt und dokumentiert
**Datei:** `/path/to/skadia/docs/internal/2026-03-06-weather-tags-system-category.md`

## Kernprobleme (aktuell)

1. **Wetter-Tags sind isoliert** — nur in `activities.weather_tags` (TEXT[] Array)
2. **Nicht im Tag-System** — nicht in `activity_tags` verlinkt
3. **Nicht in TagsPage** — User kann Wetter-Tags nicht sehen/editieren
4. **Umlaute-Fehler** — "bewoelkt" statt "bewölkt"

## Lösung

Wetter-Tags **wie Geo-Tags** behandeln:
- ✅ Seed `tags` Tabelle: 6 Wetter-System-Tags pro User (sonnig, bewölkt, regen, schnee, nebel, gewitter)
- ✅ is_system=true, category='weather', is_auto=false
- ✅ Stage 1: Nach Weather-Fetch → Automatisch in `activity_tags` verlinken (assigned_by='auto')
- ✅ TagsPage zeigt alle Tags (bereits implementiert, nur Daten-Integration nötig)
- ✅ User kann Wetter-Tags editieren/löschen wie Benutzer-Tags
- ✅ Umlaute korrigieren in mapWeatherCode()

## Implementierungs-Schritte

| # | Komponente | Datei | Aufwand |
|---|---|---|---|
| 1 | **Migration 000078** | seed_weather_system_tags.up/down.sql | 20 Zeilen SQL |
| 2 | **Umlaute-Fix** | weather.go:83 | 1 Zeile Änderung |
| 3 | **Stage1 Linking** | stage1.go:196-210 | ~15 Zeilen nach FetchWeather |
| 4 | **DB Helper** | database.go | GetTagIDsByNamesAndCategory(), LinkActivityTag() (~40 Zeilen) |
| 5 | **Upload Handler** | enrichment.go | Call LinkActivityTag nach UpdateActivity (~10 Zeilen) |
| 6 | **Frontend** | TagsPage.tsx | ✅ Keine Änderung — zeigt bereits is_system Tags |
| 7 | **Recompute (opt)** | recompute.go | Weather-Tags-Sektion (~20 Zeilen) |
| 8 | **Tests** | weather_test.go, database_test.go | Unit + Integration (~100 Zeilen) |
| 9 | **Docs** | CHANGELOG, 07_DATABASE_SCHEMA | Aktualisierung |

**Gesamt-Aufwand:** ~200 Zeilen Code, 1-2h Implementierung

## Warum das besser ist als Status quo

| Aspekt | Vorher | Nachher |
|---|---|---|
| **Visibility** | Nur in activity.weather_tags Metadaten | Vollständig im Tag-System sichtbar |
| **User Control** | Keine Kontrolle | Editierbar, färbbar wie andere Tags |
| **Reporting** | weather_tags müssen speziell abgefragt werden | Über standard activity_tags Queries |
| **Auto-Tagging** | Nur Wetter-Namen, statisch | Regelbasiert erweiterbar (zB "if regen && nacht → tag 'Schlechtes Wetter'") |
| **Konsistenz** | Zwei parallele Tag-Systeme (geo + wetter) | Ein einheitliches System |

## Architektur

**Unterschied zu Geo-Tags:**
- **Geo-Tags** = Stage 2 (ML, async, strukturiert in activity_geo_tags, OSM-basiert)
- **Wetter-Tags** = Stage 1 (sync, in activity_tags, API-basiert)

→ Aber beide landen in `activity_tags` + werden in TagsPage angezeigt ✅

## Nächste Schritte

1. Plan reviewen & ggf. Fragen klären (Q1-Q4 im Dokument)
2. Migration schreiben
3. Code-Implementierung starten
4. Tests schreiben
5. Deploy + Backfill

---

**Mehr Details:** Siehe `/path/to/skadia/docs/internal/2026-03-06-weather-tags-system-category.md`
