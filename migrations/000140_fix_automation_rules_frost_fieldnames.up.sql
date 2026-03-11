-- Migration 000140: Fix incorrect Frost/Nacht/Rundtour automation rule field names
--
-- Problem: During development, automation rules were created with incorrect field names:
--   - temperature_min_c  (incorrect) → should be min_temperature_c
--   - temperature_avg_c  (incorrect) → should be avg_temperature_c
--
-- This migration:
--   1. Deletes old automation rules with incorrect field names
--   2. The correct rules will be re-seeded by SeedDefaultAutoTags() on next application startup
--   3. Future rule evaluations will use correct field names
--
-- Impact: Rules that were never evaluated due to field name mismatch will be recreated correctly.

-- Delete old incorrect automation rules (keep correct ones via the WHERE condition)
DELETE FROM automation_rules
WHERE name IN ('Auto: Frost', 'Auto: Nacht', 'Auto: Rundtour')
  AND (
    conditions::text LIKE '%temperature_min_c%'
    OR conditions::text LIKE '%temperature_avg_c%'
  );

-- Mark the fix in a note (if tracking table exists)
-- This helps trace when the fix was applied
DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'schema_migrations_notes') THEN
    INSERT INTO schema_migrations_notes (migration, note, applied_at)
    VALUES (
      '000140',
      'Removed incorrect Frost/Nacht/Rundtour automation rules with wrong field names (temperature_min_c/temperature_avg_c). Correct rules with field names (min_temperature_c/avg_temperature_c) will be re-seeded on next API startup.',
      NOW()
    ) ON CONFLICT DO NOTHING;
  END IF;
END $$;
