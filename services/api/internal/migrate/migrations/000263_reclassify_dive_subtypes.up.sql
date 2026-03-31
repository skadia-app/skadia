-- Reclassify CCR dives: activities with closed_circuit_diluent gas → ccr_diving
UPDATE activities
SET sub_type = 'ccr_diving'
WHERE sport_type = 'diving'
  AND id IN (
    SELECT DISTINCT activity_id
    FROM activity_dive_gases
    WHERE mode = 'closed_circuit_diluent'
  );

-- Reclassify pool apnea: activities imported with sub_sport 63 (previously stored as apnea_diving)
UPDATE activities
SET sub_type = 'apnea_pool'
WHERE sport_type = 'diving'
  AND sub_type = 'apnea_diving'
  AND fit_sub_sport_id = 63;
