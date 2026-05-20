CREATE OR REPLACE FUNCTION validate_segment_leaderboard_group_version()
RETURNS trigger
LANGUAGE plpgsql
AS $$
DECLARE
    group_segment_id UUID;
    version_segment_id UUID;
    group_kind TEXT;
    active_count INTEGER;
BEGIN
    SELECT segment_id, equivalence_kind
    INTO group_segment_id, group_kind
    FROM segment_leaderboard_equivalence_groups
    WHERE id = NEW.group_id;

    IF group_segment_id IS NULL THEN
        RAISE EXCEPTION 'segment leaderboard equivalence group % not found', NEW.group_id
            USING ERRCODE = 'foreign_key_violation';
    END IF;

    SELECT segment_id
    INTO version_segment_id
    FROM segment_versions
    WHERE id = NEW.segment_version_id;

    IF version_segment_id IS NULL THEN
        RAISE EXCEPTION 'segment version % not found', NEW.segment_version_id
            USING ERRCODE = 'foreign_key_violation';
    END IF;

    IF group_segment_id <> version_segment_id THEN
        RAISE EXCEPTION 'segment leaderboard equivalence group and version belong to different segments'
            USING ERRCODE = 'check_violation';
    END IF;

    IF NEW.retired_at IS NULL AND group_kind = 'version_specific' THEN
        SELECT count(*)
        INTO active_count
        FROM segment_leaderboard_equivalence_group_versions gv
        WHERE gv.group_id = NEW.group_id
          AND gv.leaderboard_surface = NEW.leaderboard_surface
          AND gv.retired_at IS NULL
          AND NOT (
              gv.group_id = NEW.group_id
              AND gv.segment_version_id = NEW.segment_version_id
              AND gv.leaderboard_surface = NEW.leaderboard_surface
          );

        IF active_count > 0 THEN
            RAISE EXCEPTION 'version_specific leaderboard equivalence groups may have only one active version'
                USING ERRCODE = 'check_violation';
        END IF;
    END IF;

    RETURN NEW;
END;
$$;

DROP TRIGGER IF EXISTS trg_segment_leaderboard_group_version_validate
    ON segment_leaderboard_equivalence_group_versions;
CREATE TRIGGER trg_segment_leaderboard_group_version_validate
    BEFORE INSERT OR UPDATE ON segment_leaderboard_equivalence_group_versions
    FOR EACH ROW
    EXECUTE FUNCTION validate_segment_leaderboard_group_version();

CREATE OR REPLACE FUNCTION ensure_segment_version_default_equivalence_group()
RETURNS trigger
LANGUAGE plpgsql
AS $$
DECLARE
    created_group_id UUID;
BEGIN
    IF EXISTS (
        SELECT 1
        FROM segment_leaderboard_equivalence_group_versions gv
        WHERE gv.segment_version_id = NEW.id
          AND gv.leaderboard_surface = 'default'
          AND gv.retired_at IS NULL
    ) THEN
        RETURN NEW;
    END IF;

    INSERT INTO segment_leaderboard_equivalence_groups (
        segment_id, change_key, policy_key, policy_version, equivalence_kind,
        created_by, reason, metadata
    )
    VALUES (
        NEW.segment_id,
        'auto_version_specific:' || NEW.id::text,
        'segment_leaderboard_equivalence',
        1,
        'version_specific',
        NEW.created_by,
        'auto_version_specific_membership',
        jsonb_build_object(
            'created_by_trigger', TG_NAME,
            'version_id', NEW.id::text
        )
    )
    ON CONFLICT (segment_id, policy_key, policy_version, change_key) DO UPDATE
        SET metadata = EXCLUDED.metadata
    RETURNING id INTO created_group_id;

    INSERT INTO segment_leaderboard_equivalence_group_versions (
        group_id, segment_version_id, leaderboard_surface, join_reason
    )
    VALUES (created_group_id, NEW.id, 'default', 'auto_version_specific_membership')
    ON CONFLICT (group_id, segment_version_id, leaderboard_surface) DO NOTHING;

    RETURN NEW;
END;
$$;

DROP TRIGGER IF EXISTS trg_segment_versions_default_equivalence_group
    ON segment_versions;
CREATE TRIGGER trg_segment_versions_default_equivalence_group
    AFTER INSERT ON segment_versions
    FOR EACH ROW
    EXECUTE FUNCTION ensure_segment_version_default_equivalence_group();

CREATE OR REPLACE FUNCTION fill_and_validate_segment_effort_equivalence()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    IF NEW.superseded_at IS NOT NULL THEN
        RETURN NEW;
    END IF;

    IF NEW.leaderboard_equivalence_group_id IS NULL THEN
        SELECT gv.group_id
        INTO NEW.leaderboard_equivalence_group_id
        FROM segment_leaderboard_equivalence_group_versions gv
        JOIN segment_leaderboard_equivalence_groups g ON g.id = gv.group_id
        WHERE gv.segment_version_id = NEW.segment_version_id
          AND gv.leaderboard_surface = 'default'
          AND gv.retired_at IS NULL
          AND g.segment_id = NEW.segment_id
        ORDER BY gv.joined_at DESC, gv.group_id
        LIMIT 1;
    END IF;

    IF NEW.leaderboard_equivalence_group_id IS NULL THEN
        RAISE EXCEPTION 'active segment effort requires an active leaderboard equivalence group'
            USING ERRCODE = 'check_violation';
    END IF;

    IF NOT EXISTS (
        SELECT 1
        FROM segment_leaderboard_equivalence_group_versions gv
        JOIN segment_leaderboard_equivalence_groups g ON g.id = gv.group_id
        WHERE gv.group_id = NEW.leaderboard_equivalence_group_id
          AND gv.segment_version_id = NEW.segment_version_id
          AND gv.leaderboard_surface = 'default'
          AND gv.retired_at IS NULL
          AND g.segment_id = NEW.segment_id
    ) THEN
        RAISE EXCEPTION 'segment effort leaderboard equivalence group is not active for its segment version'
            USING ERRCODE = 'check_violation';
    END IF;

    RETURN NEW;
END;
$$;

DROP TRIGGER IF EXISTS trg_segment_efforts_fill_validate_equivalence
    ON segment_efforts;
CREATE TRIGGER trg_segment_efforts_fill_validate_equivalence
    BEFORE INSERT OR UPDATE OF segment_id, segment_version_id, superseded_at, leaderboard_equivalence_group_id
    ON segment_efforts
    FOR EACH ROW
    EXECUTE FUNCTION fill_and_validate_segment_effort_equivalence();

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM pg_constraint
        WHERE conname = 'segment_efforts_active_requires_equivalence_group_chk'
    ) THEN
        ALTER TABLE segment_efforts
            ADD CONSTRAINT segment_efforts_active_requires_equivalence_group_chk
            CHECK (superseded_at IS NOT NULL OR leaderboard_equivalence_group_id IS NOT NULL);
    END IF;
END
$$;
