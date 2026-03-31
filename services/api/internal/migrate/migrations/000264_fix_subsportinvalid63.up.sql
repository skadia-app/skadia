-- Fix activities where SubSport 63 was stored as raw 'subsportinvalid(63)' string
-- before the apnea_pool fix was deployed.
UPDATE activities
SET sub_type = 'apnea_pool'
WHERE sport_type = 'diving'
  AND sub_type = 'subsportinvalid(63)';
