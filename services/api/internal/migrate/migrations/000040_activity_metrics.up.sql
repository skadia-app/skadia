ALTER TABLE activities
  ADD COLUMN min_speed_ms        DOUBLE PRECISION,
  ADD COLUMN min_heart_rate      INTEGER,
  ADD COLUMN max_cadence         INTEGER,
  ADD COLUMN min_power_w         DOUBLE PRECISION,
  ADD COLUMN min_temperature_c   DOUBLE PRECISION,
  ADD COLUMN max_temperature_c   DOUBLE PRECISION,
  ADD COLUMN weather_temp_min_c  DOUBLE PRECISION,
  ADD COLUMN weather_temp_max_c  DOUBLE PRECISION,
  ADD COLUMN weather_wind_speed_max_ms DOUBLE PRECISION;
