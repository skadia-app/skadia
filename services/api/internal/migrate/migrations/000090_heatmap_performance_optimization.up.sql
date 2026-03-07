-- Migration 000087: Heatmap performance optimization
-- - Add geom_3857 column for pre-transformed Web Mercator geometry
-- - Add missing indexes on activity_date and sub_type
-- - Create composite indexes for common filter combinations

-- Add new column for Web Mercator (EPSG:3857) geometry
ALTER TABLE heatmap_lines ADD COLUMN IF NOT EXISTS geom_3857 GEOMETRY(LineString, 3857);

-- Backfill existing geometries
UPDATE heatmap_lines
SET geom_3857 = ST_Transform(geom, 3857)
WHERE geom_3857 IS NULL;

-- Create GIST index on geom_3857 for fast spatial lookups in tile queries
CREATE INDEX IF NOT EXISTS idx_heatmap_lines_geom_3857 ON heatmap_lines USING GIST (geom_3857);

-- Add index on activity_date (was missing, causes full table scans on date filters)
CREATE INDEX IF NOT EXISTS idx_heatmap_lines_activity_date ON heatmap_lines (activity_date);

-- Add index on sub_type (was missing, causes full table scans on sub_type filters)
CREATE INDEX IF NOT EXISTS idx_heatmap_lines_sub_type ON heatmap_lines (sub_type);

-- Composite index: user_id + activity_date (common filter combo)
CREATE INDEX IF NOT EXISTS idx_heatmap_lines_user_date ON heatmap_lines (user_id, activity_date);

-- Composite index: user_id + sport_type + activity_date (most common filter combo)
CREATE INDEX IF NOT EXISTS idx_heatmap_lines_user_sport_date ON heatmap_lines (user_id, sport_type, activity_date);
