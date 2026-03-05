CREATE TABLE IF NOT EXISTS enrichment_errors (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    activity_id UUID REFERENCES activities(id) ON DELETE CASCADE,
    module TEXT NOT NULL,
    error_message TEXT,
    created_at TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_enrichment_errors_activity_id ON enrichment_errors(activity_id);
CREATE INDEX IF NOT EXISTS idx_enrichment_errors_created_at ON enrichment_errors(created_at DESC);
