---
name: Phase 8c Completion Docs Update
description: Pending docs changes from Phase 8c implementation (OSM schema rewrite, BRouter, routing activation)
type: project
---

## docs/05_GEO_SPATIAL.md — Phase 8c OSM Schema
**Suche nach:** osm_peaks
**Delta:** osm_peaks/passes/settlements/amenities GELÖSCHT → unified osm_pois (category/type/subtype); osm_trails → osm_ways (25+ neue Spalten); NEU: osm_pistes, osm_glaciers, osm_wetlands; PostGIS-Optimierungen: lz4 TOAST auf allen geom-Spalten, ST_QuantizeCoordinates(geom,6) nach Import

## docs/07_DATABASE_SCHEMA.md — Phase 8c OSM-Tabellen
**Suche nach:** osm_trails
**Delta:** osm_trails → osm_ways (Migration 000151); osm_pois unified table (ersetzt 4 Tabellen); NEU: osm_pistes (piste_type/difficulty/grooming), osm_glaciers, osm_wetlands; heatmap_trail_matches.trail_id → way_id

## docs/10_DEVELOPMENT_PLAN.md — Phase 8c Status
**Suche nach:** Phase 8c
**Delta:** Phase 8c abgeschlossen (OSM Schema Rewrite, Lua Rewrite, Import-Handler, Enrichment-Refs, Capabilities, BRouter-Pipeline+Profile, Martin Config, Frontend Import-Assistent+Settings+Types, ML-Refs, PostGIS-Optim); Sub-Phasen 8c-II (KI-Routing-Integration), 8c-III (KI-BRouter-Profil-Override), 8c-IV (Offline-Basemap/PMTiles) als Roadmap-Items

## CHANGELOG.md — Phase 8c
**Suche nach:** [Unreleased]
**Delta:** feat(osm): unified osm_pois table (replaces osm_peaks/passes/settlements/amenities), osm_trails→osm_ways rename with 25+ new columns, new osm_pistes/glaciers/wetlands tables; feat(osm): Lua rewrite with 22 import flags and 20+ POI categories; feat(brouter): BRouter rd5 build pipeline (scripts/brouter-build-segments.sh) + 9 routing profiles; fix: all Go/Python enrichment references updated to new OSM schema; feat(martin): new pois_mvt + pistes_mvt tile sources; feat(frontend): OSM Import-Assistent mit 21 Kategorien + BRouter-Build-Optionen; feat(settings): Settings-Seite Geo/Routing-Karte überarbeitet; PostGIS: lz4 TOAST + ST_QuantizeCoordinates nach Import; caps.OfflineMap = caps.Routing (kein PgOSM Flex mehr nötig)
