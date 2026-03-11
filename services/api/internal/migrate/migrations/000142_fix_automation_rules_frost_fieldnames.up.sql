-- Migration 000142: Fix incorrect Frost/Nacht/Rundtour automation rule field names
--
-- Note: Migrations 000139/000140 were accidentally written to the wrong directory
-- (project root /migrations/ instead of services/api/internal/migrate/migrations/)
-- and therefore never executed. This migration replaces their intended logic.
--
-- Problem: Automation rules were created with incorrect field names:
--   - temperature_min_c  (incorrect) → should be min_temperature_c
--   - temperature_avg_c  (incorrect) → should be avg_temperature_c
--
-- Additionally, duplicate rules were created for Nacht and Rundtour.
--
-- This migration:
--   1. Deletes rules with incorrect field names (temperature_min_c/temperature_avg_c)
--   2. Removes duplicate correct rules (keeps only one per name)
--   3. SeedDefaultAutoTags() will re-seed correct rules on next API startup

-- Step 1: Delete all incorrect Frost rules (wrong field names, will never evaluate)
DELETE FROM automation_rules
WHERE name = 'Auto: Frost'
  AND (
    conditions::text LIKE '%temperature_min_c%'
    OR conditions::text LIKE '%temperature_avg_c%'
  );

-- Step 2: Remove duplicate Nacht rules (keep the one with lowest created_at)
DELETE FROM automation_rules
WHERE id IN (
  SELECT id FROM (
    SELECT id,
           ROW_NUMBER() OVER (PARTITION BY user_id, name ORDER BY created_at ASC) AS rn
    FROM automation_rules
    WHERE name = 'Auto: Nacht'
  ) ranked
  WHERE rn > 1
);

-- Step 3: Remove duplicate Rundtour rules (keep the one with lowest created_at)
DELETE FROM automation_rules
WHERE id IN (
  SELECT id FROM (
    SELECT id,
           ROW_NUMBER() OVER (PARTITION BY user_id, name ORDER BY created_at ASC) AS rn
    FROM automation_rules
    WHERE name = 'Auto: Rundtour'
  ) ranked
  WHERE rn > 1
);

-- Step 4: Fix activity_count denormalization for all tags
-- (replaces the intended 000139 migration that was also in the wrong directory)
UPDATE tags t
SET activity_count = (
  SELECT COUNT(DISTINCT activity_id)
  FROM activity_tags
  WHERE tag_id = t.id
);
