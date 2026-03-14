## docs/07_DATABASE_SCHEMA.md — Neue OSM-Tabellen
**Suche nach:** osm_forests
**Delta:**
- `osm_landuse` Tabelle hinzugefügt (Migration 000161): farmland/meadow/orchard/vineyard/industrial/retail/commercial/quarry/residential; Spalten: id BIGSERIAL PK, osm_id BIGINT, name TEXT, landuse TEXT, geom GEOMETRY
- `osm_railways` Tabelle hinzugefügt (Migration 000161): rail/light_rail/narrow_gauge/subway; Spalten: id BIGSERIAL PK, osm_id BIGINT, name TEXT, railway TEXT, geom GEOMETRY(LineString)
- `osm_piste_areas` Tabelle hinzugefügt (Migration 000161): Pisten-Relations als Flächen (downhill/nordic/skitour etc.); Spalten: id BIGSERIAL PK, osm_id, name, piste_type, difficulty, grooming, lit BOOLEAN, tags JSONB, geom GEOMETRY
- `enrichment.raster_tiles` Tabelle + Schema hinzugefügt (Migration 000161): rid SERIAL PK, source TEXT, rast RASTER, resolution_m REAL; nutzt PostGIS Raster-Extension

## docs/05_GEO_SPATIAL.md — Enrichment-Architektur
**Suche nach:** forest_pct
**Delta:**
- Enrichment-Pipeline nutzt jetzt PostGIS Raster statt Polygon-Joins für forest_pct, protected_area, landuse_class
- RasterizeForest (10m), RasterizeProtected (25m, 3 Klassen), RasterizeLanduse (25m, 5 Klassen) konvertieren OSM-Polygone zu Raster-Tiles in `enrichment.raster_tiles`
- SampleForestPct/SampleProtectedArea/SampleLanduse nutzen PL/pgSQL DO-Blocks für Tile-Iteration (pgx kann PostgreSQL geometry nicht scannen)
- EnrichNoiseScore erweitert: optional railway_heavy (300m, 0.7) + railway_light (150m, 0.4) wenn osm_railways Daten enthält
- Neue landuse_class Werte: farmland(2), residential(3), industrial(4), commercial/retail(5), quarry(6)
- EnrichTrafficClass nutzt jetzt osm_landuse statt osm_boundaries für industrial_areas
- RunSpatialEnrichment(forceRebuild bool): true=TRUNCATE+rebuild, false=skip-if-exists (Resumability)
- Lua-Importer: env-Flags SKADIA_IMPORT_LANDUSE, SKADIA_IMPORT_RAILWAYS
- Pisten: offene Wege → osm_pistes (LineString), geschlossene Wege + Relations → osm_piste_areas (Polygon)

## CHANGELOG.md
**Delta:**
- feat: Raster-Overlay Enrichment — forest_pct, protected_area, landuse_class via PostGIS Raster statt O(n²) Polygon-Joins (>8min → Sekunden)
- feat: OSM Landnutzung (osm_landuse) + Eisenbahn (osm_railways) Import-Kategorien
- feat: Pisten-Relations als Flächen (osm_piste_areas)
- fix: landuse_class clamp 0→7 (quarry=6 Support)
- fix: traffic_class Reset bei RunSpatialEnrichment (verhindert Stale-Werte bei Re-Import)
