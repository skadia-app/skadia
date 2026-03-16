-- 000168_user_weather_prefs.up.sql

ALTER TABLE users
    ADD COLUMN weather_reading_mode      TEXT    NOT NULL DEFAULT 'auto'
        CHECK (weather_reading_mode IN ('auto', 'always_one', 'always_two')),
    ADD COLUMN weather_mountain_threshold_m INT   NOT NULL DEFAULT 500,
    ADD COLUMN weather_long_duration_s   INT     NOT NULL DEFAULT 14400,
    ADD COLUMN weather_display_fields    TEXT[]  NOT NULL DEFAULT
        ARRAY['temp_c','apparent_temp_c','precipitation_mm','wind_speed_ms','humidity','condition'];

COMMENT ON COLUMN users.weather_reading_mode IS
    'auto=case-based; always_one=single reading; always_two=force two readings';
COMMENT ON COLUMN users.weather_mountain_threshold_m IS
    'Minimum elevation delta (m) to trigger mountain/valley case (default 500)';
COMMENT ON COLUMN users.weather_long_duration_s IS
    'Minimum activity duration (s) to trigger long-tour two-reading case (default 14400 = 4h)';
COMMENT ON COLUMN users.weather_display_fields IS
    'Fields shown in WeatherBadge UI: temp_c, apparent_temp_c, precipitation_mm, wind_speed_ms, wind_gusts_ms, humidity, uv_index, freezing_level_m, condition';
