ALTER TABLE segment_efforts
    DROP CONSTRAINT IF EXISTS segment_efforts_active_requires_equivalence_group_chk;

DROP TRIGGER IF EXISTS trg_segment_efforts_fill_validate_equivalence
    ON segment_efforts;
DROP FUNCTION IF EXISTS fill_and_validate_segment_effort_equivalence();

DROP TRIGGER IF EXISTS trg_segment_versions_default_equivalence_group
    ON segment_versions;
DROP FUNCTION IF EXISTS ensure_segment_version_default_equivalence_group();

DROP TRIGGER IF EXISTS trg_segment_leaderboard_group_version_validate
    ON segment_leaderboard_equivalence_group_versions;
DROP FUNCTION IF EXISTS validate_segment_leaderboard_group_version();
