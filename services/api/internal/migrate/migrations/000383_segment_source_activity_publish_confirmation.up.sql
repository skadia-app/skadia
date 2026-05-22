ALTER TABLE segments
    ADD COLUMN IF NOT EXISTS source_activity_publish_confirmed_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS source_activity_publish_confirmed_by UUID REFERENCES users(id) ON DELETE SET NULL,
    ADD COLUMN IF NOT EXISTS source_activity_publish_confirmed_segment_version_id UUID REFERENCES segment_versions(id) ON DELETE SET NULL;

CREATE INDEX IF NOT EXISTS idx_segments_source_activity_publish_confirmation
    ON segments (source_activity_publish_confirmed_at)
    WHERE source_activity_publish_confirmed_at IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_segments_source_activity_publish_confirmation_version
    ON segments (source_activity_publish_confirmed_segment_version_id)
    WHERE source_activity_publish_confirmed_segment_version_id IS NOT NULL;
