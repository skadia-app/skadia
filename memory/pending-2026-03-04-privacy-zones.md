## docs/07_DATABASE_SCHEMA.md — user_privacy_zones Tabelle
**Suche nach:** user_privacy_zones
**Delta:** Neue Spalte `color TEXT NOT NULL DEFAULT '#F97316'` hinzugefügt (Migration 000064_privacy_zone_color). Nächste freie Migration: 000065.

## docs/02_CORE_BACKEND.md — Privacy Zones API
**Suche nach:** PrivacyZone
**Delta:** `model.PrivacyZone` + `model.PrivacyZoneUpdate` haben neues `Color string`-Feld. `CreatePrivacyZone()` DB-Funktion bekommt zusätzlichen `color string`-Parameter. Alle SQL-Queries (`SELECT`, `INSERT`, `UPDATE`, `RETURNING`) inkludieren jetzt `color`. Strava-Importer (`importer_privacy_zones.go`) nutzt Standardfarbe `#F97316`.

## docs/03_FRONTEND.md — Private Zonen UI
**Suche nach:** PrivacyZone
**Delta:**
- "Privacy-Zonen" → "Private Zonen" (alle Texte in PrivacySettingsPage, SettingsPage)
- `api/privacy.ts`: `PrivacyZone`-Interface + `createPrivacyZone`/`updatePrivacyZone` haben `color`-Feld
- `PrivacyZoneEditor.tsx`: ColorPicker-Integration (aus `@/components/ColorPicker`), Radius-Bereich 100–1000m (statt 50–500m), Kreis-Farbe dynamisch per Zone, `formatRadius()`-Funktion für lesbare Anzeige (km ab 1000m)
- `PrivacySettingsPage.tsx`:
  - Übersichtskarte höher (h-[480px] statt h-[340px])
  - Kreise mit individueller Zonenfarbe statt einheitlichem Orange (separate GeoJSON-Sources pro Zone)
  - `ZoneListItem`-Komponente mit Farbindikator-Streifen, Koordinaten-Anzeige und Reverse-Geocoding via Photon-API (`https://photon.komoot.io/reverse`)
  - React Query für Reverse-Geocoding: `queryKey: ['reverse-geocode', lat, lng]`, 1h staleTime
  - Lade-Text "Lade private Zonen…" statt "Lade Privacy-Zonen…"

## CHANGELOG.md
**Neue Einträge unter [Unreleased]:**
- feat(privacy): rename "Privacy-Zonen" to "Private Zonen" throughout UI
- feat(privacy): add color field to private zones (migration 000064, ColorPicker integration)
- feat(privacy): extend radius range to 100m–1km (was 50m–500m)
- feat(privacy): show coordinates + reverse geocode address (Photon API) in zone list
- feat(privacy): render zone circles with individual colors in overview map
- feat(privacy): increase overview map height to 480px

## TODO (nicht implementiert, für spätere Phase)
- Bug 4g: Polygon-Modus als Alternative zum Kreis — zu komplex für diesen Task, MapLibre Draw oder eigene Click-Polygon-Zeichnung benötigt
