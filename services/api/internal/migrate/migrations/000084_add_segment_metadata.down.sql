DROP INDEX IF EXISTS idx_segments_climb_type;
DROP INDEX IF EXISTS idx_segments_terrain_type;
ALTER TABLE segments DROP COLUMN IF EXISTS climb_type;
ALTER TABLE segments DROP COLUMN IF EXISTS terrain_type;
