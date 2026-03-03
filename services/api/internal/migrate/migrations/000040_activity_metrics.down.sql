ALTER TABLE activities
  DROP COLUMN IF EXISTS min_speed_ms,
  DROP COLUMN IF EXISTS min_heart_rate,
  DROP COLUMN IF EXISTS max_cadence,
  DROP COLUMN IF EXISTS min_power_w,
  DROP COLUMN IF EXISTS min_temperature_c,
  DROP COLUMN IF EXISTS max_temperature_c,
  DROP COLUMN IF EXISTS weather_temp_min_c,
  DROP COLUMN IF EXISTS weather_temp_max_c,
  DROP COLUMN IF EXISTS weather_wind_speed_max_ms;
