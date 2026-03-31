UPDATE activities
SET sub_type = 'subsportinvalid(63)'
WHERE sport_type = 'diving'
  AND sub_type = 'apnea_pool'
  AND fit_sub_sport_id = 63;
