-- Migration 000140 (DOWN): Rollback fix for automation rule field names
--
-- WARNING: This migration is NOT fully reversible because it deletes records.
-- Reverting will remove the fix but not restore the deleted records.
-- The deleted incorrect rules were not functional anyway (field names didn't match).
--
-- To rollback to pre-000140 state:
--   1. This script removes the migration marker
--   2. Manual re-seeding via SeedDefaultAutoTags() will recreate rules
--   3. If you need the old incorrect rules back, restore from database backup

-- Remove the migration note if the tracking table exists
DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'schema_migrations_notes') THEN
    DELETE FROM schema_migrations_notes WHERE migration = '000140';
  END IF;
END $$;

-- Note: The deleted incorrect automation_rules cannot be restored from this script.
-- If rollback is necessary:
-- 1. Restore from a database backup taken before this migration, OR
-- 2. Run SeedDefaultAutoTags() which will recreate all rules correctly
