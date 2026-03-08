-- Add purchase_price and currency to the gear table itself (components already
-- have these via 000020_gear_overhaul) and create the cost summary view.
ALTER TABLE gear
  ADD COLUMN IF NOT EXISTS purchase_price NUMERIC(10,2),
  ADD COLUMN IF NOT EXISTS currency       TEXT DEFAULT 'EUR',
  ADD COLUMN IF NOT EXISTS purchase_date  DATE;

-- Aggregated cost view consumed by GetGearCostSummary().
-- total_cost = gear purchase price + sum of component purchase prices + sum of
-- service event costs + sum of document amounts (receipts/invoices).
CREATE OR REPLACE VIEW gear_cost_summary AS
SELECT
    g.id                      AS gear_id,
    g.user_id,
    g.name,
    COALESCE(g.purchase_price, 0)
      + COALESCE(comp.total, 0)
      + COALESCE(svc.total, 0)
      + COALESCE(doc.total, 0)  AS total_cost,
    COALESCE(g.currency, 'EUR') AS currency,
    CASE WHEN g.total_distance_m > 0
         THEN (COALESCE(g.purchase_price, 0)
               + COALESCE(comp.total, 0)
               + COALESCE(svc.total, 0)
               + COALESCE(doc.total, 0))
              / (g.total_distance_m / 1000.0)
         ELSE NULL
    END                         AS cost_per_km,
    CASE WHEN g.total_time_s > 0
         THEN (COALESCE(g.purchase_price, 0)
               + COALESCE(comp.total, 0)
               + COALESCE(svc.total, 0)
               + COALESCE(doc.total, 0))
              / (g.total_time_s / 3600.0)
         ELSE NULL
    END                         AS cost_per_hour
FROM gear g
LEFT JOIN (
    SELECT gear_id, SUM(purchase_price) AS total
    FROM gear_components
    WHERE purchase_price IS NOT NULL
    GROUP BY gear_id
) comp ON comp.gear_id = g.id
LEFT JOIN (
    SELECT gc.gear_id, SUM(se.cost) AS total
    FROM gear_service_events se
    JOIN gear_components gc ON gc.id = se.component_id
    WHERE se.cost IS NOT NULL
    GROUP BY gc.gear_id
) svc ON svc.gear_id = g.id
LEFT JOIN (
    SELECT gear_id, SUM(document_amount) AS total
    FROM gear_documents
    WHERE document_amount IS NOT NULL
      AND doc_type IN ('receipt', 'invoice')
      AND gear_id IS NOT NULL
    GROUP BY gear_id
) doc ON doc.gear_id = g.id;
