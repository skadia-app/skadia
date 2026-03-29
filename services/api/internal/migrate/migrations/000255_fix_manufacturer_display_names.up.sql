-- Fix manufacturer display names for existing activities and devices.
-- New imports get correct names from parser.go/manufacturer.go changes.
-- This migration corrects historical data.

-- Activities: development → real manufacturer
UPDATE activities SET source = 'Apple'
WHERE source = 'development' AND source_device ~ '^Watch[0-9]';

UPDATE activities SET source = 'Amazfit'
WHERE source = 'development' AND LOWER(source_device) LIKE 'amazfit%';

UPDATE activities SET source = 'Amazfit'
WHERE source = 'development' AND LOWER(source_device) LIKE 'huami amazfit%';

UPDATE activities SET source = 'Samsung'
WHERE source = 'development' AND (LOWER(source_device) LIKE 'samsung%' OR LOWER(source_device) LIKE 'galaxy watch%');

UPDATE activities SET source = 'Google'
WHERE source = 'development' AND LOWER(source_device) LIKE '%pixel watch%';

UPDATE activities SET source = 'Polar'
WHERE source = 'development' AND LOWER(source_device) LIKE 'polar %';

-- Activities: virtual platform display names
UPDATE activities SET source = 'Rouvy' WHERE source = 'virtualtraining';
UPDATE activities SET source = 'TrainerRoad' WHERE source = 'trainer_road';
UPDATE activities SET source = 'Wahoo SYSTM' WHERE source = 'the_sufferfest';
UPDATE activities SET source = 'WHOOP' WHERE LOWER(source) = 'whoop' AND source != 'WHOOP';
UPDATE activities SET source = 'Wahoo' WHERE source = 'wahoo_fitness';
UPDATE activities SET source = 'Bkool' WHERE source = 'bkool';
UPDATE activities SET source = 'MyWhoosh' WHERE source = 'mywhoosh';
UPDATE activities SET source = 'Peloton' WHERE source = 'peloton';
UPDATE activities SET source = 'TrainingPeaks' WHERE source = 'peaksware';

-- Devices: same corrections
UPDATE devices SET manufacturer = 'Apple'
WHERE manufacturer = 'Development' AND product_name ~ '^Watch[0-9]';

UPDATE devices SET manufacturer = 'WHOOP'
WHERE LOWER(manufacturer) = 'whoop' AND manufacturer != 'WHOOP';

UPDATE devices SET manufacturer = 'Rouvy'
WHERE manufacturer = 'Virtualtraining';

UPDATE devices SET manufacturer = 'TrainerRoad'
WHERE manufacturer = 'Trainer Road' AND product_name IS NULL;
