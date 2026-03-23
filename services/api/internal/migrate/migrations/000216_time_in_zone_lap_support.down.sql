DROP INDEX IF EXISTS idx_time_in_zone_segment_lap;
DROP INDEX IF EXISTS idx_time_in_zone_lap;
DROP INDEX IF EXISTS uq_activity_time_in_zone_v2;
-- Alten UNIQUE Index wiederherstellen
CREATE UNIQUE INDEX IF NOT EXISTS uq_activity_time_in_zone ON activity_time_in_zone (activity_id, zone_type, zone_index);
ALTER TABLE activity_time_in_zone DROP COLUMN IF EXISTS segment_lap_id;
ALTER TABLE activity_time_in_zone DROP COLUMN IF EXISTS lap_id;
