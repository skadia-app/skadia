-- 000260_electric_rename.up.sql

-- Step 1: Rename column (metadata-only, no table rewrite)
ALTER TABLE activities RENAME COLUMN is_ebike TO is_electric;

-- Step 2: Drop old trigger + function
DROP TRIGGER IF EXISTS trg_sync_ebike_from_tags ON activities;
DROP FUNCTION IF EXISTS sync_ebike_from_tags();

-- Step 3: Create renamed trigger function (temporary — replaced by combined trigger in 000261)
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

-- Step 4: Drop old CHECK constraint, update data, then add new constraint
ALTER TABLE automation_rules DROP CONSTRAINT IF EXISTS automation_rules_system_type_check;

-- Step 5: Rename system_type in automation_rules (no constraint active during update)
UPDATE automation_rules SET system_type = 'electric_sync' WHERE system_type = 'ebike_sync';

-- Step 6: Add new CHECK constraint (includes electric_sync + indoor_sync for 000261)
ALTER TABLE automation_rules ADD CONSTRAINT automation_rules_system_type_check
    CHECK (system_type IN ('electric_sync', 'round_trip', 'frost', 'nacht', 'virtual_sync', 'indoor_sync'));
