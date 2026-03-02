-- Rollback: Heatmap-Points
DROP TABLE IF EXISTS heatmap_points;
ALTER TABLE users DROP COLUMN IF EXISTS heatmap_sample_interval_m;
