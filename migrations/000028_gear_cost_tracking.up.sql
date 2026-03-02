ALTER TABLE gear
  ADD COLUMN IF NOT EXISTS purchase_price    NUMERIC(10,2),
  ADD COLUMN IF NOT EXISTS purchase_currency TEXT DEFAULT 'EUR',
  ADD COLUMN IF NOT EXISTS purchase_date     DATE;

ALTER TABLE gear_components
  ADD COLUMN IF NOT EXISTS purchase_price NUMERIC(10,2);

CREATE OR REPLACE VIEW gear_cost_summary AS
SELECT
  g.id AS gear_id, g.user_id, g.name,
  COALESCE(g.purchase_price, 0)
    + COALESCE((SELECT SUM(gc.purchase_price) FROM gear_components gc WHERE gc.gear_id = g.id AND gc.purchase_price IS NOT NULL), 0)
    AS total_cost,
  COALESCE(g.purchase_currency, 'EUR') AS currency,
  CASE WHEN g.total_distance_m > 0 THEN
    (COALESCE(g.purchase_price, 0)
     + COALESCE((SELECT SUM(gc.purchase_price) FROM gear_components gc WHERE gc.gear_id = g.id AND gc.purchase_price IS NOT NULL), 0))
    / (g.total_distance_m / 1000.0) ELSE NULL END AS cost_per_km,
  CASE WHEN g.total_time_s > 0 THEN
    (COALESCE(g.purchase_price, 0)
     + COALESCE((SELECT SUM(gc.purchase_price) FROM gear_components gc WHERE gc.gear_id = g.id AND gc.purchase_price IS NOT NULL), 0))
    / (g.total_time_s / 3600.0) ELSE NULL END AS cost_per_hour
FROM gear g;
