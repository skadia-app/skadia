# Skadia Migrations

Alle Migrationen im Überblick. Nächste freie: **000204**

| Nr    | Name                                | Beschreibung |
|-------|-------------------------------------|--------------|
| 000026–000160 | (ältere Migrationen)         | diverse Features |
| 000161 | osm_landuse_railways_piste         | osm_landuse/osm_railways/osm_piste_areas neue OSM-Tabellen |
| 000164 | osm_way_regions                    | osm_way_regions/osm_route_regions Junction-Tables; source_region auf OSM-Tabellen |
| 000167 | activity_weather                   | activity_weather Tabelle ersetzt flache weather_* Spalten |
| 000168 | weather_prefs                      | weather_reading_mode, weather_mountain_threshold_m, weather_long_duration_s, weather_display_fields in users |
| 000169 | hotfolder_imports_fix              | Hotfolder Import-Fix |
| 000171 | import_log                         | unified import_log Tabelle |
| 000172 | drop_hotfolder_imports             | Hotfolder imports Tabelle gelöscht |
| 000173 | import_log_batch_skipreason        | batch_id UUID + skip_reason TEXT Spalten in import_log |
| 000174 | import_log_status_check            | import_log Status-Check |
| 000175 | enrichment_tags_unification        | activity_tag_removals, Rundtour trigger bidirektional (is_round_trip ↔ activity_tags), Schnee text_color fix (#1F2937), weather tag backfill, weather_auto_tags pref in users |
| 000176 | tag_system_unification             | automation_rules: is_system/system_type/has_automation; System-Rules (ebike_sync/round_trip/frost/nacht) für alle User; Adjektive kleingeschrieben (sonnig/bewölkt); Rundtour-Trigger DROP |
| 000178 | add_has_automation_to_tags         | has_automation Boolean auf tags-Tabelle |
| 000179 | repair_tag_system_unification      | Idempotente Reparatur: is_system/system_type Spalten, System-Rules, Tag-Kategorien, Rundtour-Trigger-Drop (000176 wurde in alten Instanzen übersprungen) |
| 000180 | background_jobs_error              | error TEXT Spalte auf background_jobs |
| 000181 | fix_elevation_profile_pg18         | generate_series bigint-Cast für PostgreSQL 18 Kompatibilität |
| 000182 | osm_ways_way_id_index              | way_id Index auf osm_ways für Coverage Layer Enrichment |
| 000183 | coverage_columns                   | Coverage-Spalten auf osm_ways (forest_pct, landuse_class, water_proximity, protected_area, urbanization, noise_score, river_proximity) |
| 000184 | noise_settings                     | noise_settings JSONB auf osm_regions |
| 000185 | devices                            | devices-Tabelle (Device & Sensor Registry), activity_sensors foreign keys |
| 000186 | devices_backfill                   | Backfill: bestehende activity_sensors → devices-Tabelle |
| 000187 | devices_serial_index_fix           | Index-Fix für devices.serial_number |
| 000188 | segment_efforts_activity_index     | Index auf segment_efforts.activity_id |
| 000189 | fix_system_tag_text_colors         | Rundtour text_color fix (#10B981 → '#1a1a1a', 7.46:1 contrast) |
| 000190 | reset_stage2_module_flags          | Daten-Reset: alle stage-2 module flags auf true (Bug: false durch alten UpdateEnrichmentSettings-Pfad ohne merge) |
| 000191 | drop_pgrouting                     | Monolithischer Graph + pgRouting Extension entfernt; Isochrone + SuggestRoutes auf TiledGraphReader umgestellt |
| 000192 | consolidate_enrichment_fields      | Stage-1/Stage-2 eliminiert: 7 Renames zu _detection-Suffix, 4 Duplikat-Drops (peaks/stops/commute/ascents), climb→ascent Rename+Merge |
| 000193 | fix_geo_tags_map_section_geometry  | map_section LINESTRING → GEOMETRY (akzeptiert Multi/LineString, fix für water enrichment) |
| 000194 | drop_osm_ways_geog_idx             | DROP INDEX osm_ways_geog_idx (6.6 GB Geography-Index entfernt; geometry GiST reicht) |
| 000195 | osm_ways_dedup_osm_id              | UNIQUE INDEX idx_osm_ways_osm_id + Dedup ~224k Border-Duplikate; ON CONFLICT merge (~91s Table Lock) |
| 000196 | osm_landuse_coverage_geom_idx      | Partial GiST-Index osm_landuse(geom) WHERE landuse IN ('farmland',...) — Coverage-Performance |
| 000197 | geo_tags_source                    | activity_geo_tags +source TEXT +admin_level SMALLINT |
| 000198 | stops_poi_category                 | activity_stops +poi_category TEXT +confidence TEXT DEFAULT 'poi_matched' |
| 000199 | climbs_sport_class                 | activity_climbs +sport_class TEXT (S1–S5, leicht/mittel/schwer, HC–4) |
| 000200 | boundary_admin8_index              | Partieller GIST-Index osm_boundaries WHERE admin_level=8 AND boundary_type='administrative' |
| 000201 | surfaces_category                  | activity_surfaces +category TEXT NOT NULL DEFAULT 'surface' ('surface' oder 'highway') |
| 000202 | device_data_repair                 | Shadow Devices eliminieren + GarminProductInvalid-Strings bereinigen |
| 000203 | osm_pois_rename_peak_to_summit     | osm_pois.category 'peak'→'summit'; DROP idx_osm_pois_peak; CREATE idx_osm_pois_summit |
