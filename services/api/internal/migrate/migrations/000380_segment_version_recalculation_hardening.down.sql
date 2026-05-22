DO $$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM segment_versions
        WHERE validity_basis = 'rollback_retired'
    ) THEN
        RAISE EXCEPTION 'cannot downgrade segment version recalculation hardening with rollback_retired rows'
            USING ERRCODE = 'check_violation';
    END IF;
END
$$;

DROP INDEX IF EXISTS idx_segment_recalc_runs_actor_visible;

ALTER TABLE segment_version_revisions
    DROP COLUMN IF EXISTS expected_before_snapshot_sha256,
    DROP COLUMN IF EXISTS target_version_id,
    DROP COLUMN IF EXISTS operation_input_hash;

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
    );

ALTER TABLE segment_versions
    DROP CONSTRAINT IF EXISTS segment_versions_validity_basis_chk;

ALTER TABLE segment_versions
    ADD CONSTRAINT segment_versions_validity_basis_chk
    CHECK (validity_basis IN ('unknown_start', 'legacy_date_utc', 'editor_asserted', 'import_asserted'));
