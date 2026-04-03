-- 000276_fix_garmin_sensor_acronyms.up.sql
-- Fixes remaining sensor acronyms not covered by migration 000275.
-- "Bsm" → "BSM", "Rct" → "RCT" (standalone word, preceded by space).

UPDATE devices
SET product_name = replace(replace(product_name, ' Bsm', ' BSM'), 'Garmin Bsm', 'Garmin BSM')
WHERE product_name LIKE '%Bsm%';

UPDATE devices
SET product_name = replace(replace(product_name, ' Rct', ' RCT'), 'Garmin Rct', 'Garmin RCT')
WHERE product_name LIKE '%Rct%';
