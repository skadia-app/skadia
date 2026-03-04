ALTER TABLE activities ADD COLUMN IF NOT EXISTS garmin_activity_id BIGINT;
CREATE UNIQUE INDEX IF NOT EXISTS uq_activities_garmin_id ON activities (user_id, garmin_activity_id) WHERE garmin_activity_id IS NOT NULL;
ALTER TABLE gear ADD COLUMN IF NOT EXISTS garmin_gear_id TEXT;
CREATE UNIQUE INDEX IF NOT EXISTS uq_gear_garmin_id ON gear (user_id, garmin_gear_id) WHERE garmin_gear_id IS NOT NULL;
