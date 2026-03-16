-- 000168_user_weather_prefs.down.sql

ALTER TABLE users
    DROP COLUMN IF EXISTS weather_reading_mode,
    DROP COLUMN IF EXISTS weather_mountain_threshold_m,
    DROP COLUMN IF EXISTS weather_long_duration_s,
    DROP COLUMN IF EXISTS weather_display_fields;
