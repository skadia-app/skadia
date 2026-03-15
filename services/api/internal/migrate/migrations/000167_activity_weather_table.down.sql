-- 000167_activity_weather_table.down.sql

ALTER TABLE activities
    ADD COLUMN IF NOT EXISTS weather_temp_c       FLOAT8,
    ADD COLUMN IF NOT EXISTS weather_condition     TEXT,
    ADD COLUMN IF NOT EXISTS weather_humidity      INT,
    ADD COLUMN IF NOT EXISTS weather_wind_speed_ms FLOAT8,
    ADD COLUMN IF NOT EXISTS weather_tags          TEXT[];

-- Restore from activity_weather (first/single reading only)
UPDATE activities a
SET
    weather_temp_c        = aw.temp_c,
    weather_condition     = aw.condition,
    weather_humidity      = aw.humidity,
    weather_wind_speed_ms = aw.wind_speed_ms,
    weather_tags          = aw.tags
FROM (
    SELECT DISTINCT ON (activity_id)
        activity_id, temp_c, condition, humidity, wind_speed_ms, tags
    FROM activity_weather
    ORDER BY activity_id, created_at
) aw
WHERE a.id = aw.activity_id;

DROP TABLE IF EXISTS activity_weather;
