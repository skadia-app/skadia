-- 000261_indoor_system_tag.down.sql

-- Step 1: Drop combined trigger, restore simple electric-only trigger
DROP TRIGGER IF EXISTS trg_sync_system_flags_and_tags ON activities;
DROP FUNCTION IF EXISTS sync_system_flags_and_tags();

CREATE OR REPLACE FUNCTION sync_electric_from_tags() RETURNS TRIGGER AS $$
BEGIN
    IF NEW.tags IS DISTINCT FROM OLD.tags THEN
        NEW.is_electric := ('E-Bike' = ANY(COALESCE(NEW.tags, '{}')));
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_sync_electric_from_tags
    BEFORE UPDATE OF tags ON activities
    FOR EACH ROW
    EXECUTE FUNCTION sync_electric_from_tags();

-- Step 2: Remove Indoor tags from activities
UPDATE activities SET tags = array_remove(tags, 'Indoor') WHERE 'Indoor' = ANY(tags);

-- Step 3: Remove Indoor from activity_tags junction
DELETE FROM activity_tags WHERE tag_id IN (
    SELECT id FROM tags WHERE name = 'Indoor' AND is_system = true
);

-- Step 4: Delete Indoor system tags
DELETE FROM tags WHERE name = 'Indoor' AND is_system = true;

-- Step 5: Delete indoor_sync system rules
DELETE FROM automation_rules WHERE system_type = 'indoor_sync';

-- Step 6: Revert is_indoor to old logic (is_indoor = NOT has_gps)
UPDATE activities SET is_indoor = NOT has_gps;
