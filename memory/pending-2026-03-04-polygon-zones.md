## docs/07_DATABASE_SCHEMA.md — user_privacy_zones
**Suche nach:** user_privacy_zones
**Delta:**
- Migration 000065 fügt zwei neue Spalten hinzu:
  - `zone_type TEXT NOT NULL DEFAULT 'circle'` — 'circle' oder 'polygon'
  - `polygon GEOMETRY(Polygon, 4326)` NULLABLE — enthält Polygon-Geometrie wenn zone_type='polygon'
- Index `idx_user_privacy_zones_zone_type` auf zone_type

## docs/02_CORE_BACKEND.md — Privacy Zones
**Suche nach:** PrivacyZone
**Delta:**
- `model.PrivacyZone` Struct: neue Felder `ZoneType string` und `Polygon *string` (GeoJSON)
- `model.PrivacyZoneUpdate`: neue Felder `ZoneType *string` und `Polygon *string`
- `DB.CreatePrivacyZone()`: neue Parameter `zoneType string, polygonGeoJSON *string`
- `DB.ListPrivacyZones()`, `UpdatePrivacyZone()`, `getPrivacyZone()`: geben jetzt auch zone_type + ST_AsGeoJSON(polygon) zurück
- Handler `CreatePrivacyZone`: akzeptiert zone_type='circle'|'polygon' + polygon GeoJSON
- Strava-Importer: `CreatePrivacyZone`-Aufruf auf neue Signatur angepasst (immer 'circle', nil)

## docs/03_FRONTEND.md — Privacy Zone Polygon Editor
**Suche nach:** PrivacyZoneEditor
**Delta:**
- `PrivacyZoneEditor.tsx` komplett neu: Toggle Kreis/Polygon, Klick-Handler für Vertices, Doppelklick schließt Polygon, "Schließen"/"Zurücksetzen"-Buttons, Vertex-Drag-and-Drop, Polygon-Layer (Fill + Line), Vertex-Marker, Radius-Slider nur im Kreis-Modus
- `api/privacy.ts`: `PrivacyZone` Interface um `zone_type` und `polygon?` erweitert; create/update-Funktionen aktualisiert
- `PrivacySettingsPage.tsx` `OverviewMap`: Polygon-Zonen als GeoJSON-Polygon-Layer statt circlePolygon; Bounds-Berechnung für Polygone; Popup-Text angepasst
- `ZoneListItem`: zeigt "Polygon" statt Radius für Polygon-Zonen

## CHANGELOG.md
**Delta:**
- feat(privacy): add polygon zone type — draw custom polygon shapes for privacy zones
  - DB migration 000065: zone_type + polygon columns on user_privacy_zones
  - Go model, DB queries, handler updated
  - Frontend: PrivacyZoneEditor with circle/polygon toggle, click-to-add vertices, double-click to close, vertex drag-and-drop
  - OverviewMap renders polygon shapes from GeoJSON
