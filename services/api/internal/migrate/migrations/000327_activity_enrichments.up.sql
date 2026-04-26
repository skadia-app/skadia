CREATE TABLE IF NOT EXISTS activity_enrichments (
    activity_id UUID NOT NULL REFERENCES activities(id) ON DELETE CASCADE,
    category TEXT NOT NULL CHECK (category IN ('surface','highway','forest_pct','protected_pct','sac_scale','mtb_scale','vf_scale','vf_unknown')),
    type TEXT NOT NULL DEFAULT '',
    percentage DOUBLE PRECISION NOT NULL CHECK (percentage >= 0 AND percentage <= 100),
    length_m DOUBLE PRECISION CHECK (
        (category IN ('surface','highway') AND length_m IS NOT NULL) OR
        (category NOT IN ('surface','highway') AND length_m IS NULL)
    ),
    ranges JSONB CHECK (ranges IS NULL OR jsonb_typeof(ranges) = 'array'),
    PRIMARY KEY (activity_id, category, type)
);

CREATE INDEX IF NOT EXISTS idx_activity_enrichments_activity
    ON activity_enrichments(activity_id);

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM pg_constraint
        WHERE conrelid = 'activity_enrichments'::regclass
          AND conname = 'activity_enrichments_type_per_category_check'
    ) THEN
        ALTER TABLE activity_enrichments
            ADD CONSTRAINT activity_enrichments_type_per_category_check
            CHECK (
                (category IN ('forest_pct','protected_pct','vf_unknown') AND type = '') OR
                (category IN ('surface','highway','sac_scale','mtb_scale','vf_scale') AND type <> '')
            );
    END IF;
END $$;

ALTER TABLE user_enrichment_settings
    ADD COLUMN IF NOT EXISTS enable_difficulty_scales BOOLEAN NOT NULL DEFAULT true;

COMMENT ON COLUMN activity_enrichments.type IS
    'Subtype key within a category. Empty string is reserved for scalar categories without per-type buckets.';

COMMENT ON COLUMN activity_enrichments.ranges IS
    'JSON array of coordinate-index ranges: [{"from": int, "to": int}], zero-based in activity_tracks.track_full points, to exclusive.';
