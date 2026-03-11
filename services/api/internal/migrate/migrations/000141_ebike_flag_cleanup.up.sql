-- Migration 000139: E-Bike Taxonomy Cleanup
--
-- Problem: is_ebike was a GENERATED column (true only when sport_type='e_biking').
-- E-bike activities existed in TWO inconsistent forms:
--   1) sport_type='e_biking' (from Strava CSV) → is_ebike=true but unknown bike type
--   2) sport_type='cycling', sub_type='e_bike_fitness' (from Garmin FIT) → is_ebike=false(!)
--
-- Solution: Make is_ebike a regular boolean column. Any bike sport can be an e-bike.
-- Remove 'e_biking' as a sport_type entirely.

-- Step 1: Drop the generated column
ALTER TABLE activities DROP COLUMN is_ebike;

-- Step 2: Add as regular boolean
ALTER TABLE activities ADD COLUMN is_ebike BOOLEAN NOT NULL DEFAULT false;

-- Step 3: Migrate e_biking sport_type → cycling + is_ebike=true
UPDATE activities
SET sport_type = 'cycling', sub_type = COALESCE(NULLIF(sub_type, ''), 'generic'),
    is_ebike = true
WHERE sport_type = 'e_biking';

-- Step 4: Migrate e_bike_fitness sub_type → generic + is_ebike=true
UPDATE activities
SET sub_type = 'generic', is_ebike = true
WHERE sub_type = 'e_bike_fitness';

-- Step 5: Migrate e_bike_mountain sub_type → mountain + is_ebike=true
UPDATE activities
SET sub_type = 'mountain', is_ebike = true
WHERE sub_type = 'e_bike_mountain';

-- Step 6: Also fix 'e_bike' sport_type (Strava importer bug: returned 'e_bike' instead of 'e_biking')
UPDATE activities
SET sport_type = 'cycling', sub_type = COALESCE(NULLIF(sub_type, ''), 'generic'),
    is_ebike = true
WHERE sport_type = 'e_bike';

-- Step 7: Fix parsed_sport_type / parsed_sub_type references too
UPDATE activities SET parsed_sport_type = 'cycling'
WHERE parsed_sport_type IN ('e_biking', 'e_bike');

UPDATE activities SET parsed_sub_type = 'generic'
WHERE parsed_sub_type = 'e_bike_fitness';

UPDATE activities SET parsed_sub_type = 'mountain'
WHERE parsed_sub_type = 'e_bike_mountain';
