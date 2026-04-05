-- Phase 4b: Suunto session-level fields on activities
ALTER TABLE activities ADD COLUMN IF NOT EXISTS suunto_vo2max REAL;
ALTER TABLE activities ADD COLUMN IF NOT EXISTS suunto_peak_epoc REAL;
ALTER TABLE activities ADD COLUMN IF NOT EXISTS suunto_recovery_time_h REAL;

-- Radar developer fields promoted to stream columns
ALTER TABLE activity_streams ADD COLUMN IF NOT EXISTS radar_vehicle_count SMALLINT;
ALTER TABLE activity_streams ADD COLUMN IF NOT EXISTS radar_nearest_range_m SMALLINT;
ALTER TABLE activity_streams ADD COLUMN IF NOT EXISTS radar_total_count SMALLINT;

-- Running smoothness (Garmin HRM-Pro developer field)
ALTER TABLE activity_streams ADD COLUMN IF NOT EXISTS running_smoothness_pct REAL;

-- Glucose (Dexcom CGM developer field)
ALTER TABLE activity_streams ADD COLUMN IF NOT EXISTS glucose_mg_dl REAL;
