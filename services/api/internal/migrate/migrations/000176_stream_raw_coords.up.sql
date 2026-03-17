-- 000176_stream_raw_coords.up.sql
-- Store original GPS coordinates for points corrected by spatial outlier detection.
-- Only populated for corrected points (NULL = no correction applied).
-- NOTE: activity_streams is a TimescaleDB compressed hypertable.
-- Adding nullable columns with no default is supported since TimescaleDB 2.10+.
ALTER TABLE activity_streams
    ADD COLUMN raw_lat DOUBLE PRECISION,
    ADD COLUMN raw_lng DOUBLE PRECISION;

-- Count of corrected GPS points per activity (for UI badge/info).
ALTER TABLE activities ADD COLUMN gps_corrections_count SMALLINT NOT NULL DEFAULT 0;
