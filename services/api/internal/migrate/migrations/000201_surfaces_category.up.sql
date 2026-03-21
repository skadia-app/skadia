ALTER TABLE activity_surfaces ADD COLUMN IF NOT EXISTS category TEXT NOT NULL DEFAULT 'surface';
-- 'surface' = Oberflächen-Aggregation (asphalt, gravel, unknown, offroad)
-- 'highway' = Wegearten-Aggregation (path, track, residential, offroad)
-- Default 'surface' für Backward-Kompatibilität mit bestehenden Rows
