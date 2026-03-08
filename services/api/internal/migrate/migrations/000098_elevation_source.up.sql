ALTER TABLE activities ADD COLUMN IF NOT EXISTS elevation_source TEXT DEFAULT 'device';
COMMENT ON COLUMN activities.elevation_source IS 'Höhenquelle: device (Original), dem (DEM-korrigiert), manual (User-Edit)';
