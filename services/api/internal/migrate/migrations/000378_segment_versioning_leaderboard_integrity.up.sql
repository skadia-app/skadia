ALTER TABLE segment_efforts
    ADD COLUMN IF NOT EXISTS superseded_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS superseded_reason TEXT,
    ADD COLUMN IF NOT EXISTS superseded_by_recalculation_run_id UUID,
    ADD COLUMN IF NOT EXISTS created_by_recalculation_run_id UUID,
    ADD COLUMN IF NOT EXISTS leaderboard_equivalence_group_id UUID;

UPDATE segment_efforts
SET superseded_at = COALESCE(created_at, NOW()),
    superseded_reason = COALESCE(superseded_reason, 'legacy_duplicate')
WHERE superseded_by_effort_id IS NOT NULL
  AND superseded_at IS NULL;

CREATE TABLE IF NOT EXISTS segment_leaderboard_equivalence_groups (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    segment_id UUID NOT NULL REFERENCES segments(id) ON DELETE CASCADE,
    change_key TEXT NOT NULL,
    policy_key TEXT NOT NULL,
    policy_version INTEGER NOT NULL,
    equivalence_kind TEXT NOT NULL,
    created_by UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    reason TEXT,
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    UNIQUE (segment_id, policy_key, policy_version, change_key),
    CONSTRAINT segment_leaderboard_equivalence_kind_chk
        CHECK (equivalence_kind IN ('version_specific', 'same_course'))
);

CREATE TABLE IF NOT EXISTS segment_leaderboard_equivalence_group_versions (
    group_id UUID NOT NULL REFERENCES segment_leaderboard_equivalence_groups(id) ON DELETE CASCADE,
    segment_version_id UUID NOT NULL REFERENCES segment_versions(id) ON DELETE CASCADE,
    leaderboard_surface TEXT NOT NULL DEFAULT 'default',
    joined_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    retired_at TIMESTAMPTZ,
    join_reason TEXT NOT NULL,
    PRIMARY KEY (group_id, segment_version_id, leaderboard_surface),
    CONSTRAINT segment_leaderboard_surface_chk CHECK (leaderboard_surface = 'default'),
    CONSTRAINT segment_leaderboard_membership_retired_chk CHECK (retired_at IS NULL OR retired_at > joined_at)
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_segment_leaderboard_group_versions_active
    ON segment_leaderboard_equivalence_group_versions (segment_version_id, leaderboard_surface)
    WHERE retired_at IS NULL;

CREATE TABLE IF NOT EXISTS segment_recalculation_segment_state (
    segment_id UUID PRIMARY KEY REFERENCES segments(id) ON DELETE CASCADE,
    generation BIGINT NOT NULL DEFAULT 0,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS segment_recalculation_runs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    segment_id UUID NOT NULL REFERENCES segments(id) ON DELETE CASCADE,
    actor_user_id UUID REFERENCES users(id) ON DELETE SET NULL,
    operation_key_hash TEXT NOT NULL,
    idempotency_key TEXT NOT NULL,
    generation BIGINT NOT NULL,
    reason TEXT NOT NULL,
    requested_from_at TIMESTAMPTZ,
    requested_until_at TIMESTAMPTZ,
    status TEXT NOT NULL DEFAULT 'queued',
    total_activities INTEGER NOT NULL DEFAULT 0,
    processed_activities INTEGER NOT NULL DEFAULT 0,
    superseded_efforts INTEGER NOT NULL DEFAULT 0,
    inserted_efforts INTEGER NOT NULL DEFAULT 0,
    failed_activities INTEGER NOT NULL DEFAULT 0,
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    started_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    claimed_at TIMESTAMPTZ,
    claimed_by TEXT,
    UNIQUE (idempotency_key),
    CONSTRAINT segment_recalculation_runs_status_chk
        CHECK (status IN ('queued', 'running', 'completed', 'failed', 'cancelled', 'obsolete')),
    CONSTRAINT segment_recalculation_runs_interval_chk
        CHECK (requested_until_at IS NULL OR requested_from_at IS NULL OR requested_until_at > requested_from_at)
);

CREATE TABLE IF NOT EXISTS segment_recalculation_run_activities (
    run_id UUID NOT NULL REFERENCES segment_recalculation_runs(id) ON DELETE CASCADE,
    activity_id UUID NOT NULL REFERENCES activities(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    status TEXT NOT NULL DEFAULT 'queued',
    error_code TEXT,
    error_message TEXT,
    attempt_count INTEGER NOT NULL DEFAULT 0,
    claim_token UUID,
    claimed_at TIMESTAMPTZ,
    claimed_until TIMESTAMPTZ,
    claimed_by TEXT,
    processed_at TIMESTAMPTZ,
    PRIMARY KEY (run_id, activity_id),
    CONSTRAINT segment_recalculation_run_activities_status_chk
        CHECK (status IN ('queued', 'running', 'completed', 'failed', 'skipped'))
);

CREATE TABLE IF NOT EXISTS segment_version_revisions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    segment_id UUID NOT NULL REFERENCES segments(id) ON DELETE CASCADE,
    actor_user_id UUID REFERENCES users(id) ON DELETE SET NULL,
    operation TEXT NOT NULL,
    reason TEXT,
    snapshot_schema_key TEXT NOT NULL DEFAULT 'segment_version_revision_v1',
    snapshot_schema_version INTEGER NOT NULL DEFAULT 1,
    before_snapshot JSONB NOT NULL,
    after_snapshot JSONB NOT NULL,
    before_snapshot_sha256 TEXT NOT NULL,
    after_snapshot_sha256 TEXT NOT NULL,
    affected_from_at TIMESTAMPTZ,
    affected_until_at TIMESTAMPTZ,
    recalculation_run_id UUID,
    rollback_of_revision_id UUID REFERENCES segment_version_revisions(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'segment_version_revisions_recalculation_run_fk'
    ) THEN
        ALTER TABLE segment_version_revisions
            ADD CONSTRAINT segment_version_revisions_recalculation_run_fk
            FOREIGN KEY (recalculation_run_id)
            REFERENCES segment_recalculation_runs(id)
            ON DELETE SET NULL;
    END IF;

    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'segment_efforts_superseded_by_recalc_run_fk'
    ) THEN
        ALTER TABLE segment_efforts
            ADD CONSTRAINT segment_efforts_superseded_by_recalc_run_fk
            FOREIGN KEY (superseded_by_recalculation_run_id)
            REFERENCES segment_recalculation_runs(id)
            ON DELETE SET NULL;
    END IF;

    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'segment_efforts_created_by_recalc_run_fk'
    ) THEN
        ALTER TABLE segment_efforts
            ADD CONSTRAINT segment_efforts_created_by_recalc_run_fk
            FOREIGN KEY (created_by_recalculation_run_id)
            REFERENCES segment_recalculation_runs(id)
            ON DELETE SET NULL;
    END IF;

    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'segment_efforts_leaderboard_equivalence_group_fk'
    ) THEN
        ALTER TABLE segment_efforts
            ADD CONSTRAINT segment_efforts_leaderboard_equivalence_group_fk
            FOREIGN KEY (leaderboard_equivalence_group_id)
            REFERENCES segment_leaderboard_equivalence_groups(id)
            ON DELETE NO ACTION
            DEFERRABLE INITIALLY DEFERRED;
    END IF;

    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'segment_efforts_superseded_reason_chk'
    ) THEN
        ALTER TABLE segment_efforts
            ADD CONSTRAINT segment_efforts_superseded_reason_chk
            CHECK (
                superseded_reason IS NULL OR superseded_reason IN (
                    'legacy_duplicate',
                    'recalculated_replaced',
                    'recalculated_removed',
                    'manual_rollback',
                    'admin_correction'
                )
            );
    END IF;
END
$$;

WITH version_rows AS (
    SELECT sv.id AS version_id,
           sv.segment_id,
           sv.version_number
    FROM segment_versions sv
    WHERE sv.matching_contract_key = 'course_gps_v1'
      AND sv.matching_contract_version = 1
),
inserted AS (
    INSERT INTO segment_leaderboard_equivalence_groups (
        segment_id, change_key, policy_key, policy_version, equivalence_kind, reason, metadata
    )
    SELECT vr.segment_id,
           'legacy_version_specific:' || vr.version_id::text,
           'segment_leaderboard_equivalence',
           1,
           'version_specific',
           'legacy_version_specific_backfill',
           jsonb_build_object(
               'legacy_policy', 'legacy_segment_wide',
               'backfilled_version_id', vr.version_id::text,
               'version_number', vr.version_number,
               'merge_policy', 'no_automatic_historical_merge'
           )
    FROM version_rows vr
    ON CONFLICT (segment_id, policy_key, policy_version, change_key) DO UPDATE
        SET metadata = EXCLUDED.metadata
    RETURNING id, segment_id, metadata
)
INSERT INTO segment_leaderboard_equivalence_group_versions (
    group_id, segment_version_id, leaderboard_surface, join_reason
)
SELECT inserted.id,
       (inserted.metadata->>'backfilled_version_id')::uuid,
       'default',
       'legacy_version_specific_backfill'
FROM inserted
ON CONFLICT (group_id, segment_version_id, leaderboard_surface) DO NOTHING;

UPDATE segment_efforts se
SET leaderboard_equivalence_group_id = g.id
FROM segment_leaderboard_equivalence_groups g
JOIN segment_leaderboard_equivalence_group_versions gv ON gv.group_id = g.id
WHERE gv.segment_version_id = se.segment_version_id
  AND gv.leaderboard_surface = 'default'
  AND gv.retired_at IS NULL
  AND g.segment_id = se.segment_id
  AND se.leaderboard_equivalence_group_id IS NULL;

DROP INDEX IF EXISTS idx_segment_efforts_active_match_identity;
CREATE UNIQUE INDEX idx_segment_efforts_active_match_identity
    ON segment_efforts (match_identity_key, started_at)
    WHERE superseded_at IS NULL;

DROP INDEX IF EXISTS idx_segment_efforts_leaderboard_partition;
CREATE INDEX idx_segment_efforts_leaderboard_partition
    ON segment_efforts (
        segment_id,
        leaderboard_equivalence_group_id,
        leaderboard_sport_type,
        leaderboard_direction,
        elapsed_time_s ASC
    )
    WHERE in_leaderboard = true AND superseded_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_segment_recalc_runs_segment_status
    ON segment_recalculation_runs (segment_id, status, generation);

CREATE INDEX IF NOT EXISTS idx_segment_recalc_runs_status_claimed
    ON segment_recalculation_runs (status, claimed_at);

CREATE INDEX IF NOT EXISTS idx_segment_recalc_run_activities_status_claim
    ON segment_recalculation_run_activities (run_id, status, claimed_until, claimed_at);

CREATE INDEX IF NOT EXISTS idx_segment_recalc_run_activities_global_status
    ON segment_recalculation_run_activities (status, claimed_until)
    WHERE status IN ('queued', 'running');
