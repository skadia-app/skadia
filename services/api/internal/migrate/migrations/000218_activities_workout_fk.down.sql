DROP INDEX IF EXISTS idx_activities_workout;
ALTER TABLE activities DROP COLUMN IF EXISTS workout_id;
