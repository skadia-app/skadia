-- Add has_gps column to activities.
-- Backfill: activities with a start_location have GPS track data.
-- This matches parser logic: data.HasGPS = len(data.TrackPoints) >= 2
ALTER TABLE activities
    ADD COLUMN has_gps BOOLEAN NOT NULL DEFAULT false;

UPDATE activities
    SET has_gps = (start_location IS NOT NULL);
