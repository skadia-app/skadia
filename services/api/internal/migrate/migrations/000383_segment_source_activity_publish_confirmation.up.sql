ALTER TABLE segments
    ADD COLUMN IF NOT EXISTS source_activity_publish_confirmed_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS source_activity_publish_confirmed_by UUID REFERENCES users(id) ON DELETE SET NULL;

CREATE INDEX IF NOT EXISTS idx_segments_source_activity_publish_confirmation
    ON segments (source_activity_publish_confirmed_at)
    WHERE source_activity_publish_confirmed_at IS NOT NULL;
