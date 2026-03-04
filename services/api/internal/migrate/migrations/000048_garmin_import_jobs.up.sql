CREATE TABLE garmin_import_jobs (
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
