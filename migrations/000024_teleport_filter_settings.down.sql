ALTER TABLE users
  DROP COLUMN IF EXISTS teleport_threshold_m,
  DROP COLUMN IF EXISTS teleport_filter_map,
  DROP COLUMN IF EXISTS teleport_filter_activity;
-- Funktionen bleiben (wurden bereits in 000023 angelegt, down von 000023 loescht sie)
