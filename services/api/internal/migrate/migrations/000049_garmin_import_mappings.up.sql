CREATE TABLE garmin_import_mappings (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    import_job_id UUID REFERENCES garmin_import_jobs(id) ON DELETE CASCADE,
    source_type TEXT NOT NULL,
    source_id TEXT NOT NULL,
    skadia_id UUID NOT NULL,
    skadia_table TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (user_id, source_type, source_id)
);
