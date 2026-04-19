-- When an activity becomes a fusion-source (INSERT into
-- activity_fusion_memberships with a non-null source_id), remove it
-- from any recording_group_members row. The existing DEFERRABLE
-- trg_group_members_sync trigger then handles primary-reelection and
-- group-dissolution at commit time.
--
-- Rationale: without this, ad-hoc fusion from a Tier-2 pair leaves a
-- zombie recording_group that references the source-activity (now
-- hidden from the activities list because source_id IS NOT NULL in
-- activity_fusion_memberships). Users saw X+Y in a group that was
-- supposed to be {B,X,Y} after merging (A,B).

-- One-time cleanup: existing zombie memberships (activities already fusion-sources
-- but still listed in recording_group_members). Trigger-driven cleanup from this
-- migration forward prevents new zombies.
DO $$
DECLARE
    n INTEGER;
BEGIN
    SELECT COUNT(*) INTO n
    FROM recording_group_members rgm
    WHERE EXISTS (
        SELECT 1 FROM activity_fusion_memberships m WHERE m.source_id = rgm.activity_id
    );
    IF n > 0 THEN
        RAISE NOTICE 'Migration 000313: cleaning up % zombie recording_group_members rows (fusion-source activities)', n;
    END IF;
END $$;

DELETE FROM recording_group_members rgm
WHERE EXISTS (
    SELECT 1 FROM activity_fusion_memberships m WHERE m.source_id = rgm.activity_id
);

CREATE OR REPLACE FUNCTION fn_fusion_source_removes_group_member()
RETURNS TRIGGER AS $$
BEGIN
    -- NEW.source_id is guaranteed NOT NULL by the memberships schema,
    -- but guard defensively.
    IF NEW.source_id IS NOT NULL THEN
        DELETE FROM recording_group_members
        WHERE activity_id = NEW.source_id;
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_fusion_source_removes_group_member
AFTER INSERT ON activity_fusion_memberships
FOR EACH ROW
EXECUTE FUNCTION fn_fusion_source_removes_group_member();
