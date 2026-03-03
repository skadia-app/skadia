-- Reverse normalization (best-effort, some sub_type→sport_type promotions are lossy).
-- NOTE: This cannot perfectly restore the original state because multiple sport_types
-- were merged into one (e.g. mountain_biking → cycling with sub_type mountain).

UPDATE activities SET sport_type = 'strength' WHERE sport_type = 'strength_training'
  AND NOT EXISTS (SELECT 1 FROM activities a2 WHERE a2.id = activities.id AND a2.parsed_sport_type = 'strength_training');
UPDATE activities SET sport_type = 'skiing' WHERE sport_type = 'alpine_skiing'
  AND parsed_sport_type IN ('skiing', 'ski');
