-- NOTE: This DOWN migration is NOT lossless. The original sub_type before CCR detection
-- (single_gas_diving, multi_gas_diving, etc.) is not preserved. We fall back to
-- multi_gas_diving as the closest approximation for CCR dives.
UPDATE activities
SET sub_type = 'multi_gas_diving'
WHERE sport_type = 'diving'
  AND sub_type = 'ccr_diving';

-- apnea_pool → apnea_diving (restores previous incorrect mapping)
UPDATE activities
SET sub_type = 'apnea_diving'
WHERE sport_type = 'diving'
  AND sub_type = 'apnea_pool';
