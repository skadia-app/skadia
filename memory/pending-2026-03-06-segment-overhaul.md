## docs/05_GEO_SPATIAL.md — Segment System v2
**Suche nach:** Segment
**Delta:**
- `segment_favorites` Tabelle hinzugefügt (Migration 000083): user_id UUID FK, segment_id UUID FK, UNIQUE, Indexes
- `segments` Tabelle erweitert (Migration 000084): `terrain_type VARCHAR(32)` ('standard'/'trail'/'lift'), `climb_type VARCHAR(32)` DEFAULT 'standard' ('standard'/'bergauf'/'bergab'/'bergfahrt'/'talfahrt')
- Lift-Segmente bekommen automatisch `climb_type='bergfahrt'` beim Import/Erstellen
- Standard-Segmenttyp-Defaults: lift=bergfahrt, trail(rad)=bergab, trail(lauf)=bergauf
- Hinweis für später: Trail/Gondel/Standard können rad+fuß genutzt werden → Filtersystem statt Duplikate

## docs/03_FRONTEND.md — Segment Detail Page + Map Overlay
**Suche nach:** Segment
**Delta:**
- Neue Seite: `/segments/:id` → `SegmentDetailPage.tsx`
  - Links: `SegmentMinimap` (MapLibre, orange Linie, auto-fit), `SegmentActivityList` (Sort: Standard/Reverse/Alle, Sport-Typ-Filter)
  - Rechts: `FavoriteButton` (Heart-Toggle, GET/POST/DELETE /api/v1/segments/:id/favorite)
  - Unten: `SegmentPerformanceChart` (ECharts Scatter, beste Zeit gold, Medianline, Trendpfeil, Klick→Aktivität)
- SegmentsPage: Terrain-Typ-Filter (Standard/Trail/Lift-Checkboxen); Lift standardmäßig ausgeblendet; bei Lift-Auswahl "Alle anzeigen"-Checkbox (default aus = nur gefahrene Lifte)
- MapPage: `SegmentsOverlay`-Komponente (an/aus schaltbar); Farb-Unterscheidung: Standard=grau, Trail=pink gestrichelt, Lift=indigo; Favoriten=gold; gefahrene=dickere Linie; Klick→/segments/:id
- SegmentsPage: "Auf Karte"-Button → navigiert zu MapPage mit `state.activateSegmentsOverlay=true`
- SegmentEditorModal + SegmentEditModal: Berg-Checkbox (Bergauf/Bergab-Radio), terrain_type-Auswahl (Standard/Trail)

## docs/08_API_REFERENCE.md — Segment Endpoints
**Suche nach:** /segments
**Delta:**
- Neue Endpoints:
  - `GET /api/v1/segments/{id}/favorite` → `{favorite: bool}`
  - `POST /api/v1/segments/{id}/favorite` → Favorit setzen
  - `DELETE /api/v1/segments/{id}/favorite` → Favorit entfernen
  - `GET /api/v1/segments/{id}/performances` → Alle Segment-Efforts des Users mit Aktivitäts-Metadaten
- Segment-Responses enthalten jetzt: `terrain_type`, `climb_type`, `ridden_count`, `is_favorite`
- POST /segments: nimmt jetzt `terrain_type` und `climb_type` entgegen

## docs/07_DATABASE_SCHEMA.md — Neue Tabellen
**Suche nach:** segment_favorites
**Delta:**
- Neue Tabelle `segment_favorites`: id BIGSERIAL PK, user_id UUID FK→users, segment_id UUID FK→segments, created_at, UNIQUE(user_id, segment_id)
- `segments` Tabelle: +terrain_type VARCHAR(32), +climb_type VARCHAR(32) DEFAULT 'standard'

## docs/02_CORE_BACKEND.md — Sport Taxonomy
**Suche nach:** Paddeln
**Delta:**
- "Paddeln" umbenannt zu "Wassersport" in taxonomy.go (Go) und sportTaxonomy.ts (TypeScript)

## CHANGELOG.md
**Delta:**
### Added
- feat(segments): Segment-Favoriten — Herzchen-Button auf SegmentDetailPage, persistiert per User in `segment_favorites`; `GET/POST/DELETE /api/v1/segments/{id}/favorite`
- feat(segments): SegmentDetailPage (`/segments/:id`) — SegmentMinimap (MapLibre, orange Linie, auto-fit), SegmentActivityList mit Sort + Sport-Filter, SegmentPerformanceChart (ECharts Scatter, beste Zeit gold, Medianline, Trendpfeil, Klick→Aktivität)
- feat(segments): SegmentsOverlay auf MapPage — Standard=grau, Trail=pink gestrichelt, Lift=indigo; Favoriten=gold, gefahrene=dickere Linie; Klick→/segments/:id; "Auf Karte"-Button in SegmentsPage
- feat(segments): Terrain-Typ-Filter in SegmentsPage (Standard/Trail/Lift-Checkboxen); Lift standardmäßig ausgeblendet mit "Alle anzeigen"-Option (nur gefahrene Lifte)
- feat(segments): Berg-Checkbox bei Segmenterstellung (Bergauf/Bergab-Radio, Bergfahrt/Talfahrt für Lifte); `climb_type`-Feld; terrain_type-Auswahl (Standard/Trail) in Editoren
- feat(taxonomy): Wassersport-Kategorie (war: Paddeln) in Go taxonomy.go + Frontend sportTaxonomy.ts
### Database
- Migration 000083: `segment_favorites` Tabelle (user_id, segment_id, UNIQUE)
- Migration 000084: `terrain_type` + `climb_type` Spalten in `segments`
