-- 000288_remove_ml_commute.down.sql
-- WARNUNG: Lossy Rollback — Löscht alle commute_auto-Regeln unwiderruflich.

CREATE TABLE IF NOT EXISTS commute_clusters (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    cluster_label INTEGER NOT NULL,
    center      GEOMETRY(Point, 4326),
    radius_m    DOUBLE PRECISION,
    point_count INTEGER,
    created_at  TIMESTAMPTZ DEFAULT NOW()
);

ALTER TABLE user_enrichment_settings
  ADD COLUMN IF NOT EXISTS enable_commute_detection BOOLEAN NOT NULL DEFAULT true;

DELETE FROM automation_rules WHERE system_type = 'commute_auto';

ALTER TABLE automation_rules DROP CONSTRAINT IF EXISTS automation_rules_system_type_check;
ALTER TABLE automation_rules ADD CONSTRAINT automation_rules_system_type_check
  CHECK (system_type IN (
    'electric_sync', 'round_trip', 'frost', 'nacht',
    'virtual_sync', 'indoor_sync', 'commute_sync'
  ));
