-- Fix: keep tags.activity_count in sync when activity_tags rows are deleted
-- (e.g. via ON DELETE CASCADE when activities are deleted).
--
-- The trigger fires AFTER DELETE on activity_tags and recalculates activity_count
-- for every affected tag. This covers:
--   1. Explicit UnassignActivitiesFromTag calls (already updated in Go, but idempotent here)
--   2. CASCADE deletes triggered by activity deletion (was NOT updated before this fix)

CREATE OR REPLACE FUNCTION trg_update_tag_activity_count()
RETURNS TRIGGER LANGUAGE plpgsql AS $$
BEGIN
    UPDATE tags
    SET activity_count = (
        SELECT COUNT(DISTINCT activity_id)
        FROM activity_tags
        WHERE tag_id = OLD.tag_id
    )
    WHERE id = OLD.tag_id;
    RETURN NULL;
END;
$$;

CREATE TRIGGER trg_activity_tags_after_delete
AFTER DELETE ON activity_tags
FOR EACH ROW
EXECUTE FUNCTION trg_update_tag_activity_count();

-- Repair any stale activity_count values caused by the missing update on activity deletion.
UPDATE tags
SET activity_count = (
    SELECT COUNT(DISTINCT activity_id)
    FROM activity_tags
    WHERE tag_id = tags.id
);
