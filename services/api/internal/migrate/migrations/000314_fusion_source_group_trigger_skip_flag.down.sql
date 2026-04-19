-- Restore the pre-000314 version of fn_fusion_source_removes_group_member
-- (without the skip-flag check).

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
