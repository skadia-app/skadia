-- Extend location_source CHECK constraint to allow 'manual' (user-placed pin)
ALTER TABLE activity_photos DROP CONSTRAINT IF EXISTS activity_photos_location_source_check;
ALTER TABLE activity_photos ADD CONSTRAINT activity_photos_location_source_check
    CHECK (location_source IN ('exif', 'activity_track', 'manual'));
