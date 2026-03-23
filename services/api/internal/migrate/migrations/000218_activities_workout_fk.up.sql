ALTER TABLE activities ADD COLUMN IF NOT EXISTS workout_id UUID REFERENCES workouts(id) ON DELETE SET NULL;
CREATE INDEX IF NOT EXISTS idx_activities_workout ON activities(workout_id) WHERE workout_id IS NOT NULL;
