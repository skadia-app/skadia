DROP VIEW IF EXISTS gear_cost_summary;

ALTER TABLE gear_components
  DROP COLUMN IF EXISTS purchase_price;

ALTER TABLE gear
  DROP COLUMN IF EXISTS purchase_price,
  DROP COLUMN IF EXISTS purchase_currency,
  DROP COLUMN IF EXISTS purchase_date;
