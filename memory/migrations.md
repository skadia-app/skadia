# Skadia Migrations

Alle Migrationen im Überblick. Nächste freie: **000177**

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
