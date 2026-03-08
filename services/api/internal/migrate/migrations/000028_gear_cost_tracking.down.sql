DROP VIEW IF EXISTS gear_cost_summary;

ALTER TABLE gear
  DROP COLUMN IF EXISTS purchase_price,
  DROP COLUMN IF EXISTS currency,
  DROP COLUMN IF EXISTS purchase_date;
