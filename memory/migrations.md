# Skadia Migrations

Alle Migrationen im Überblick. Nächste freie: **000174**

| Nr    | Name                                | Beschreibung |
|-------|-------------------------------------|--------------|
| 000026–000160 | (ältere Migrationen)         | diverse Features |
| 000161 | osm_landuse_railways_piste         | osm_landuse/osm_railways/osm_piste_areas neue OSM-Tabellen |
| 000164 | osm_way_regions                    | osm_way_regions/osm_route_regions Junction-Tables; source_region auf OSM-Tabellen |
| 000167 | activity_weather                   | activity_weather Tabelle ersetzt flache weather_* Spalten |
| 000168 | weather_prefs                      | weather_reading_mode, weather_mountain_threshold_m, weather_long_duration_s, weather_display_fields in users |
| 000169 | hotfolder_imports_fix              | Hotfolder Import-Fix |
| 000170 | (leer / reserviert)                | — |
| 000171 | (leer / reserviert)                | — |
| 000172 | (leer / reserviert)                | — |
| 000173 | enrichment_tags_unification        | activity_tag_removals, Rundtour trigger bidirektional (is_round_trip ↔ activity_tags), Schnee text_color fix (#1F2937), weather tag backfill, weather_auto_tags pref in users |
