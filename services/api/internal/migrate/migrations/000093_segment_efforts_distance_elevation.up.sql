-- Add distance_m and elevation_gain_m to segment_efforts for per-effort metrics
ALTER TABLE segment_efforts ADD COLUMN distance_m DOUBLE PRECISION;
ALTER TABLE segment_efforts ADD COLUMN elevation_gain_m DOUBLE PRECISION;
