DROP TABLE IF EXISTS checkpoint_course_effort_controls;
DROP TABLE IF EXISTS checkpoint_course_projection;
DROP TABLE IF EXISTS course_version_controls;

DELETE FROM segments
WHERE course_kind = 'checkpoint_sequence';

ALTER TABLE target_course_eligibility_projection
    DROP CONSTRAINT IF EXISTS target_course_eligibility_projection_matching_contract_chk;
ALTER TABLE target_course_eligibility_projection
    ADD CONSTRAINT target_course_eligibility_projection_matching_contract_chk
    CHECK (matching_contract_key = 'course_gps_v1' AND matching_contract_version = 1);

ALTER TABLE segment_efforts
    DROP CONSTRAINT IF EXISTS segment_efforts_contract_bundle_chk;

ALTER TABLE segment_efforts
    DROP CONSTRAINT IF EXISTS segment_efforts_id_started_version_unique;

ALTER TABLE segment_efforts
    DROP CONSTRAINT IF EXISTS segment_efforts_matching_contract_chk;
ALTER TABLE segment_efforts
    ADD CONSTRAINT segment_efforts_matching_contract_chk
    CHECK (matching_contract_key = 'course_gps_v1' AND matching_contract_version = 1);

ALTER TABLE segment_efforts
    DROP CONSTRAINT IF EXISTS segment_efforts_leaderboard_contract_chk;
ALTER TABLE segment_efforts
    ADD CONSTRAINT segment_efforts_leaderboard_contract_chk
    CHECK (leaderboard_contract_key = 'course_gps_v1' AND leaderboard_contract_version = 1);

ALTER TABLE segment_efforts
    DROP CONSTRAINT IF EXISTS segment_efforts_leaderboard_policy_chk;
ALTER TABLE segment_efforts
    ADD CONSTRAINT segment_efforts_leaderboard_policy_chk
    CHECK (leaderboard_version_policy_key IN ('legacy_segment_wide'));

ALTER TABLE segment_versions
    DROP CONSTRAINT IF EXISTS segment_versions_matching_contract_chk;
ALTER TABLE segment_versions
    ADD CONSTRAINT segment_versions_matching_contract_chk
    CHECK (matching_contract_key = 'course_gps_v1' AND matching_contract_version = 1);

ALTER TABLE segment_versions
    DROP CONSTRAINT IF EXISTS segment_versions_id_segment_id_unique;

ALTER TABLE segments
    DROP CONSTRAINT IF EXISTS segments_course_kind_chk;

ALTER TABLE segments
    DROP COLUMN IF EXISTS course_kind;
