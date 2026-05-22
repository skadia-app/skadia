DROP INDEX IF EXISTS idx_segments_source_activity_publish_confirmation;
DROP INDEX IF EXISTS idx_segments_source_activity_publish_confirmation_version;

ALTER TABLE segments
    DROP COLUMN IF EXISTS source_activity_publish_confirmed_segment_version_id,
    DROP COLUMN IF EXISTS source_activity_publish_confirmed_by,
    DROP COLUMN IF EXISTS source_activity_publish_confirmed_at;
