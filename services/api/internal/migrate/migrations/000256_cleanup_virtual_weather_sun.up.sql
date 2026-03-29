-- Remove weather and sunrise/sunset data incorrectly assigned to virtual activities
-- via location tag backfill (which didn't check is_virtual before 2026-03-29)
UPDATE activities SET
  sunrise_at = NULL, sunset_at = NULL, time_of_day = NULL
WHERE is_virtual = true AND (sunrise_at IS NOT NULL OR sunset_at IS NOT NULL);

DELETE FROM activity_weather WHERE activity_id IN (
  SELECT id FROM activities WHERE is_virtual = true
);
