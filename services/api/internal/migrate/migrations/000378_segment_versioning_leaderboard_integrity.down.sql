DO $$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM segment_efforts
        WHERE superseded_at IS NOT NULL
          AND superseded_by_effort_id IS NULL
          AND COALESCE(superseded_reason, '') <> 'legacy_duplicate'
    ) THEN
        RAISE EXCEPTION 'unsafe rollback: Plan 2 removed/no-replacement efforts would become active';
    END IF;
END
$$;

DROP INDEX IF EXISTS idx_segment_recalc_run_activities_global_status;
DROP INDEX IF EXISTS idx_segment_recalc_run_activities_status_claim;
DROP INDEX IF EXISTS idx_segment_recalc_runs_status_claimed;
DROP INDEX IF EXISTS idx_segment_recalc_runs_segment_status;
DROP INDEX IF EXISTS idx_segment_efforts_leaderboard_partition;
DROP INDEX IF EXISTS idx_segment_efforts_active_match_identity;

ALTER TABLE segment_efforts
    DROP CONSTRAINT IF EXISTS segment_efforts_leaderboard_equivalence_group_fk,
    DROP CONSTRAINT IF EXISTS segment_efforts_created_by_recalc_run_fk,
    DROP CONSTRAINT IF EXISTS segment_efforts_superseded_by_recalc_run_fk,
    DROP CONSTRAINT IF EXISTS segment_efforts_superseded_reason_chk;

ALTER TABLE IF EXISTS segment_version_revisions
    DROP CONSTRAINT IF EXISTS segment_version_revisions_recalculation_run_fk;

DROP TABLE IF EXISTS segment_version_revisions;
DROP TABLE IF EXISTS segment_recalculation_run_activities;
DROP TABLE IF EXISTS segment_recalculation_runs;
DROP TABLE IF EXISTS segment_recalculation_segment_state;
DROP TABLE IF EXISTS segment_leaderboard_equivalence_group_versions;
DROP TABLE IF EXISTS segment_leaderboard_equivalence_groups;

ALTER TABLE segment_efforts
    DROP COLUMN IF EXISTS leaderboard_equivalence_group_id,
    DROP COLUMN IF EXISTS created_by_recalculation_run_id,
    DROP COLUMN IF EXISTS superseded_by_recalculation_run_id,
    DROP COLUMN IF EXISTS superseded_reason,
    DROP COLUMN IF EXISTS superseded_at;

CREATE UNIQUE INDEX IF NOT EXISTS idx_segment_efforts_active_match_identity
    ON segment_efforts (match_identity_key, started_at)
    WHERE superseded_by_effort_id IS NULL;

CREATE INDEX IF NOT EXISTS idx_segment_efforts_leaderboard_partition
    ON segment_efforts (
        segment_id,
        leaderboard_sport_type,
        leaderboard_sub_type,
        leaderboard_equipment_class,
        leaderboard_assist_class,
        leaderboard_indoor_policy_state,
        leaderboard_virtual_policy_state,
        leaderboard_electric_policy_state,
        leaderboard_direction,
        elapsed_time_s ASC
    )
    WHERE in_leaderboard = true AND superseded_by_effort_id IS NULL;
