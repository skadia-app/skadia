-- NOTE: DROP COLUMN on compressed TimescaleDB hypertable chunks may require
-- decompression first. If this fails, run:
--   SELECT decompress_chunk(c, if_compressed => true)
--   FROM show_chunks('activity_streams') c;
-- Then re-run this migration, then re-compress.
ALTER TABLE activities DROP COLUMN IF EXISTS suunto_vo2max;
ALTER TABLE activities DROP COLUMN IF EXISTS suunto_peak_epoc;
ALTER TABLE activities DROP COLUMN IF EXISTS suunto_recovery_time_h;
ALTER TABLE activity_streams DROP COLUMN IF EXISTS radar_vehicle_count;
ALTER TABLE activity_streams DROP COLUMN IF EXISTS radar_nearest_range_m;
ALTER TABLE activity_streams DROP COLUMN IF EXISTS radar_total_count;
ALTER TABLE activity_streams DROP COLUMN IF EXISTS running_smoothness_pct;
ALTER TABLE activity_streams DROP COLUMN IF EXISTS glucose_mg_dl;
