-- 000167_activity_weather_table.up.sql

CREATE TABLE activity_weather (
    id              UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    activity_id     UUID        NOT NULL REFERENCES activities(id) ON DELETE CASCADE,
    reading_type    TEXT        NOT NULL, -- 'single' | 'start' | 'summit' | 'midpoint'
    label           TEXT,                 -- display label e.g. '▲ Gipfel (2260m)', nullable
    recorded_at     TIMESTAMPTZ,
    lat             FLOAT8,
    lng             FLOAT8,
    elevation_m     FLOAT8,               -- elevation used for API call (mean/start/summit)
    temp_c          FLOAT8,
    apparent_temp_c FLOAT8,
    precipitation_mm FLOAT8,
    wind_speed_ms   FLOAT8,
    wind_gusts_ms   FLOAT8,
    humidity        INT,
    uv_index        FLOAT8,
    freezing_level_m FLOAT8,
    condition       TEXT,
    tags            TEXT[],
    model           TEXT,                 -- API model used e.g. 'dwd_icon_d2'
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_activity_weather_activity_id ON activity_weather(activity_id);
CREATE INDEX idx_activity_weather_tags ON activity_weather USING GIN(tags);

-- Migrate existing flat weather data as 'single' readings
INSERT INTO activity_weather (
    activity_id, reading_type, recorded_at, lat, lng,
    temp_c, humidity, wind_speed_ms, condition, tags
)
SELECT
    a.id,
    'single',
    a.started_at,
    ST_Y(a.start_location::geometry),
    ST_X(a.start_location::geometry),
    a.weather_temp_c,
    a.weather_humidity,
    a.weather_wind_speed_ms,
    a.weather_condition,
    COALESCE(a.weather_tags, '{}')
FROM activities a
WHERE a.weather_temp_c IS NOT NULL
  AND a.start_location IS NOT NULL;

-- Drop the old flat weather columns
ALTER TABLE activities
    DROP COLUMN IF EXISTS weather_temp_c,
    DROP COLUMN IF EXISTS weather_condition,
    DROP COLUMN IF EXISTS weather_humidity,
    DROP COLUMN IF EXISTS weather_wind_speed_ms,
    DROP COLUMN IF EXISTS weather_tags;

-- Cleanup: alte Weather-Tags aus activity_tags entfernen (jetzt in activity_weather.tags)
DELETE FROM activity_tags
WHERE tag_id IN (
    SELECT id FROM tags WHERE category = 'weather'
);
