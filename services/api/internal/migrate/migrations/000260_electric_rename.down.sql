-- 000260_electric_rename.down.sql

DROP TRIGGER IF EXISTS trg_sync_electric_from_tags ON activities;
DROP FUNCTION IF EXISTS sync_electric_from_tags();

ALTER TABLE activities RENAME COLUMN is_electric TO is_ebike;

UPDATE automation_rules SET system_type = 'ebike_sync' WHERE system_type = 'electric_sync';

ALTER TABLE automation_rules DROP CONSTRAINT IF EXISTS automation_rules_system_type_check;
ALTER TABLE automation_rules ADD CONSTRAINT automation_rules_system_type_check
    CHECK (system_type IN ('ebike_sync', 'round_trip', 'frost', 'nacht', 'virtual_sync'));

CREATE OR REPLACE FUNCTION sync_ebike_from_tags() RETURNS TRIGGER AS $$
BEGIN
    IF NEW.tags IS DISTINCT FROM OLD.tags THEN
        NEW.is_ebike := ('E-Bike' = ANY(COALESCE(NEW.tags, '{}')));
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_sync_ebike_from_tags
    BEFORE UPDATE OF tags ON activities
    FOR EACH ROW
    EXECUTE FUNCTION sync_ebike_from_tags();
