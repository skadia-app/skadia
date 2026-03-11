-- Revert: restore is_ebike as generated column, restore e_biking sport_type

-- Restore e_biking sport_type from is_ebike=true activities
UPDATE activities
SET sport_type = 'e_biking'
WHERE is_ebike = true AND sport_type IN ('cycling', 'mountain_biking', 'gravel_cycling');

-- Drop the regular column
ALTER TABLE activities DROP COLUMN is_ebike;

-- Re-add as generated column
ALTER TABLE activities ADD COLUMN is_ebike BOOLEAN GENERATED ALWAYS AS (sport_type = 'e_biking') STORED;
