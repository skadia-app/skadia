-- Phase 4b: Suunto session-level fields on activities
-- NOTE: activity_streams is a TimescaleDB hypertable with columnar compression.
-- After applying this migration, re-compress old chunks to include new columns:
--   SELECT compress_chunk(c, if_not_compressed => true)
--   FROM show_chunks('activity_streams') c;
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
