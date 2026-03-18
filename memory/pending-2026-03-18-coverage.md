## docs/05_GEO_SPATIAL.md — Coverage Layer
**Suche nach:** forest_pct
**Delta:**
- Coverage Layer: 7 Grid-Layer in Go (forest 25m, landuse 25m, water 25m, protected 100m,
  urbanization 100m, river_proximity 25m, noise 100m)
- paulmach/orb für WKB-Parsing, Polygon-Operationen, Geo-Distanzen
- Noise: 7 konfigurierbare Quellkategorien, Weber-Fechner-Decay, quadratische Kombination
- User-Settings: noise_settings JSONB mit Kategorien + Buffer-Distanzen (Phase 2 routing-time)
- O(1) pro Sample-Punkt, ~59 MB pro Tile, 4 parallele Workers
- Phase 1: osm_ways Spalten. Phase 2: CoverageReader direkt im Graph-Builder
- rasterize.go (SQL-basierte Polygon-Joins) entfernt — ersetzt durch coverage/ Package

## docs/07_DATABASE_SCHEMA.md
**Suche nach:** landuse_class
**Delta:**
- Migration 000183: urbanization + river_proximity SMALLINT auf osm_ways
- Migration 000184: noise_settings JSONB auf users (Defaults für 7 Kategorien)
- EdgeTags2 Bits 57-59 urbanization (3 Bit, 0-7 Klassen), 60-62 river_proximity (3 Bit)
- Bit 63 reserve (1 Bit, war vorher 7 Bit reserve)

## CHANGELOG.md
**Delta:**
- feat: Coverage Layer v2 — 7 Grid-Layer, Go In-Memory-Rasterisierung (paulmach/orb)
- feat: Noise konfigurierbar (7 Kategorien, Weber-Fechner, quadratische Kombination)
- feat: Urbanization (Population-Decay, BRouter town_class Ersatz)
- feat: River-Proximity (Waterway-Nähe als Routing-Faktor)
- feat: Water Layer (Seen/Gewässerflächen)
- perf: Enrichment Slowenien <5 Min (war >1h), Europa geschätzt <50 Min
- refactor: rasterize.go entfernt (SQL-Polygon-Joins ersetzt durch Coverage Builder)
