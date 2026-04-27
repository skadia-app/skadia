-- 000337_map_layers_seed_sort_order_fix.up.sql
-- Pin sort_order on the seven seeded system base layers so GET /map_layers
-- returns a deterministic order across deploys. Idempotent reaffirmation —
-- values match the 000333 seed plus the implicit cyclosm slot.

UPDATE map_layers SET sort_order = 1 WHERE key = 'osm'       AND is_system;
UPDATE map_layers SET sort_order = 2 WHERE key = 'topo'      AND is_system;
UPDATE map_layers SET sort_order = 3 WHERE key = 'cyclosm'   AND is_system;
UPDATE map_layers SET sort_order = 4 WHERE key = 'satellite' AND is_system;
UPDATE map_layers SET sort_order = 5 WHERE key = 'dark'      AND is_system;
UPDATE map_layers SET sort_order = 6 WHERE key = 'local'     AND is_system;
UPDATE map_layers SET sort_order = 7 WHERE key = 'offline'   AND is_system;
