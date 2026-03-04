-- Strava import job tracking
CREATE TABLE strava_import_jobs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    status TEXT NOT NULL DEFAULT 'pending',
    source_path TEXT NOT NULL,
    total_modules INTEGER NOT NULL DEFAULT 0,
    completed_modules INTEGER NOT NULL DEFAULT 0,
    current_module TEXT,
    module_results JSONB DEFAULT '{}',
    errors JSONB DEFAULT '[]',
    started_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Strava activity ID for deduplication
ALTER TABLE activities ADD COLUMN IF NOT EXISTS strava_activity_id BIGINT;
CREATE UNIQUE INDEX IF NOT EXISTS uq_activities_strava_id
    ON activities (user_id, strava_activity_id)
    WHERE strava_activity_id IS NOT NULL;
