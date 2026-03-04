-- Add climbing_grade column to activity_splits for manual entry of route difficulty
-- (e.g. "6b", "V4", "7A+") since Garmin FIT files do not expose this in standard fields.
ALTER TABLE activity_splits ADD COLUMN IF NOT EXISTS climbing_grade TEXT;
