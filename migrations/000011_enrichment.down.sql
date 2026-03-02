-- Rollback Phase 4a: Enrichment

DROP TABLE IF EXISTS geo_name_gender_cache;
DROP TABLE IF EXISTS commute_clusters;
DROP TABLE IF EXISTS user_enrichment_settings;
DROP TABLE IF EXISTS activity_stops;
DROP TABLE IF EXISTS activity_climbs;
DROP TABLE IF EXISTS activity_geo_tags;

ALTER TABLE activities DROP COLUMN IF EXISTS start_city;
ALTER TABLE activities DROP COLUMN IF EXISTS end_city;
ALTER TABLE activities DROP COLUMN IF EXISTS start_state;
ALTER TABLE activities DROP COLUMN IF EXISTS start_country;
ALTER TABLE activities DROP COLUMN IF EXISTS start_country_code;
ALTER TABLE activities DROP COLUMN IF EXISTS weather_temp_c;
ALTER TABLE activities DROP COLUMN IF EXISTS weather_condition;
ALTER TABLE activities DROP COLUMN IF EXISTS weather_humidity;
ALTER TABLE activities DROP COLUMN IF EXISTS weather_wind_speed_ms;
ALTER TABLE activities DROP COLUMN IF EXISTS weather_tags;
ALTER TABLE activities DROP COLUMN IF EXISTS is_round_trip;
ALTER TABLE activities DROP COLUMN IF EXISTS intensity;
ALTER TABLE activities DROP COLUMN IF EXISTS time_of_day;
