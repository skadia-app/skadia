-- Manual CCR reclassification for b237d04d-79c7-4826-abc4-e6eaf4954af9.
-- This activity is a confirmed CCR dive (Garmin reports CCR, 62m depth, deco obligations)
-- but activity_dive_gases has no records for it, preventing automatic CCR detection.
UPDATE activities
SET sub_type = 'ccr_diving'
WHERE id = 'b237d04d-79c7-4826-abc4-e6eaf4954af9';
