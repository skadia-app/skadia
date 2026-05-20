ALTER TABLE segment_versions
    DROP CONSTRAINT IF EXISTS segment_versions_validity_basis_chk;

ALTER TABLE segment_versions
    ADD CONSTRAINT segment_versions_validity_basis_chk
    CHECK (validity_basis IN ('unknown_start', 'legacy_date_utc', 'editor_asserted', 'import_asserted', 'rollback_retired'));

ALTER TABLE segment_versions
    DROP CONSTRAINT IF EXISTS segment_versions_course_gps_v1_no_overlap;

ALTER TABLE segment_versions
    ADD CONSTRAINT segment_versions_course_gps_v1_no_overlap
    EXCLUDE USING gist (
        segment_id WITH =,
        matching_contract_key WITH =,
        matching_contract_version WITH =,
        tstzrange(
            COALESCE(valid_from_at, '-infinity'::timestamptz),
            COALESCE(valid_until_at, 'infinity'::timestamptz),
            '[)'
        ) WITH &&
    )
    WHERE (validity_basis <> 'rollback_retired');

ALTER TABLE segment_version_revisions
    ADD COLUMN IF NOT EXISTS operation_input_hash TEXT,
    ADD COLUMN IF NOT EXISTS target_version_id UUID REFERENCES segment_versions(id) ON DELETE SET NULL,
    ADD COLUMN IF NOT EXISTS expected_before_snapshot_sha256 TEXT;

CREATE INDEX IF NOT EXISTS idx_segment_recalc_runs_actor_visible
    ON segment_recalculation_runs (actor_user_id, status, created_at DESC)
    WHERE metadata->>'visibility' = 'user_visible';
