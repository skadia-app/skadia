ALTER TABLE segment_versions
    DROP CONSTRAINT IF EXISTS segment_versions_course_gps_v1_no_overlap;

DROP INDEX IF EXISTS idx_segment_efforts_leaderboard_partition;
DROP INDEX IF EXISTS idx_segment_versions_course_gps_v1_validity;
DROP INDEX IF EXISTS idx_segment_efforts_active_match_identity;
DROP INDEX IF EXISTS idx_segments_elevation_kind;

ALTER TABLE segment_efforts
    DROP CONSTRAINT IF EXISTS segment_efforts_policy_state_chk,
    DROP CONSTRAINT IF EXISTS segment_efforts_leaderboard_policy_chk,
    DROP CONSTRAINT IF EXISTS segment_efforts_leaderboard_direction_chk,
    DROP CONSTRAINT IF EXISTS segment_efforts_match_source_type_chk,
    DROP CONSTRAINT IF EXISTS segment_efforts_leaderboard_contract_chk,
    DROP CONSTRAINT IF EXISTS segment_efforts_matching_contract_chk;

ALTER TABLE segment_versions
    DROP CONSTRAINT IF EXISTS segment_versions_validity_order_chk,
    DROP CONSTRAINT IF EXISTS segment_versions_validity_basis_chk,
    DROP CONSTRAINT IF EXISTS segment_versions_eligibility_elevation_kind_chk,
    DROP CONSTRAINT IF EXISTS segment_versions_eligibility_segment_type_chk,
    DROP CONSTRAINT IF EXISTS segment_versions_matching_contract_chk;

ALTER TABLE segments
    DROP CONSTRAINT IF EXISTS segments_lap_elevation_kind_chk,
    DROP CONSTRAINT IF EXISTS segments_elevation_kind_chk;

DROP TRIGGER IF EXISTS trg_segment_versions_validity_defaults ON segment_versions;
DROP FUNCTION IF EXISTS fn_segment_versions_validity_defaults();

DROP TRIGGER IF EXISTS trg_segment_efforts_match_metadata_defaults ON segment_efforts;
DROP FUNCTION IF EXISTS fn_segment_efforts_match_metadata_defaults();

DROP TRIGGER IF EXISTS trg_segments_normalize_elevation_kind ON segments;
DROP FUNCTION IF EXISTS normalize_segment_elevation_kind();

ALTER TABLE segment_efforts
    DROP COLUMN IF EXISTS leaderboard_contract_version,
    DROP COLUMN IF EXISTS leaderboard_contract_key,
    DROP COLUMN IF EXISTS leaderboard_version_policy_key,
    DROP COLUMN IF EXISTS leaderboard_direction,
    DROP COLUMN IF EXISTS leaderboard_electric_policy_state,
    DROP COLUMN IF EXISTS leaderboard_virtual_policy_state,
    DROP COLUMN IF EXISTS leaderboard_indoor_policy_state,
    DROP COLUMN IF EXISTS leaderboard_assist_class,
    DROP COLUMN IF EXISTS leaderboard_equipment_class,
    DROP COLUMN IF EXISTS leaderboard_sub_type,
    DROP COLUMN IF EXISTS leaderboard_sport_type,
    DROP COLUMN IF EXISTS superseded_by_effort_id,
    DROP COLUMN IF EXISTS occurrence_hash,
    DROP COLUMN IF EXISTS occurrence_index,
    DROP COLUMN IF EXISTS match_identity_key,
    DROP COLUMN IF EXISTS match_source_id,
    DROP COLUMN IF EXISTS match_source_type,
    DROP COLUMN IF EXISTS matching_contract_version,
    DROP COLUMN IF EXISTS matching_contract_key;

ALTER TABLE segment_versions
    DROP COLUMN IF EXISTS eligibility_elevation_kind,
    DROP COLUMN IF EXISTS eligibility_visibility,
    DROP COLUMN IF EXISTS eligibility_bidirectional,
    DROP COLUMN IF EXISTS eligibility_allowed_predicates_set,
    DROP COLUMN IF EXISTS eligibility_allowed_predicates,
    DROP COLUMN IF EXISTS eligibility_sport_group,
    DROP COLUMN IF EXISTS eligibility_segment_type,
    DROP COLUMN IF EXISTS published_at,
    DROP COLUMN IF EXISTS validity_basis,
    DROP COLUMN IF EXISTS valid_until_at,
    DROP COLUMN IF EXISTS valid_from_at,
    DROP COLUMN IF EXISTS matching_contract_version,
    DROP COLUMN IF EXISTS matching_contract_key;

ALTER TABLE segments
    DROP COLUMN IF EXISTS elevation_kind;
