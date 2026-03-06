-- Add terrain_type and climb_type columns to segments table
ALTER TABLE segments ADD COLUMN IF NOT EXISTS terrain_type VARCHAR(32);
-- terrain_type values: 'standard', 'trail', 'lift'

ALTER TABLE segments ADD COLUMN IF NOT EXISTS climb_type VARCHAR(32) DEFAULT 'standard';
-- climb_type values: 'standard', 'bergauf', 'bergab', 'bergfahrt', 'talfahrt'
-- Only set on create if explicitly marked or lift auto-detected

CREATE INDEX idx_segments_terrain_type ON segments(terrain_type);
CREATE INDEX idx_segments_climb_type ON segments(climb_type);
