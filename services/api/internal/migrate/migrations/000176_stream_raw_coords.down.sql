-- 000176_stream_raw_coords.down.sql
ALTER TABLE activity_streams
    DROP COLUMN IF EXISTS raw_lat,
    DROP COLUMN IF EXISTS raw_lng;
ALTER TABLE activities DROP COLUMN IF EXISTS gps_corrections_count;
