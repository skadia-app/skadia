-- Neue Spalten
ALTER TABLE activities ADD COLUMN is_virtual BOOLEAN NOT NULL DEFAULT false;
ALTER TABLE activities ADD COLUMN exclude_from_heatmap BOOLEAN NOT NULL DEFAULT false;
ALTER TABLE activities ADD COLUMN exclude_from_stats BOOLEAN NOT NULL DEFAULT false;

-- Backfill Phase 1: Strava sport_type Normalisierung
UPDATE activities SET sport_type = 'cycling', is_virtual = true, exclude_from_heatmap = true
WHERE sport_type = 'virtual_ride';

UPDATE activities SET sport_type = 'running', is_virtual = true, exclude_from_heatmap = true
WHERE sport_type = 'virtual_run';

UPDATE activities SET sport_type = 'rowing', is_virtual = true, exclude_from_heatmap = true
WHERE sport_type = 'virtual_row';

-- Backfill Phase 2: FIT sub_type Normalisierung
UPDATE activities SET sub_type = 'generic', is_virtual = true, exclude_from_heatmap = true
WHERE sub_type = 'virtual_activity';

-- Backfill Phase 3: Heatmap bereinigen
DELETE FROM heatmap_lines WHERE activity_id IN (
  SELECT id FROM activities WHERE is_virtual = true
);
DELETE FROM heatmap_points WHERE activity_id IN (
  SELECT id FROM activities WHERE is_virtual = true
);

-- CHECK Constraint erweitern für virtual_sync
ALTER TABLE automation_rules DROP CONSTRAINT IF EXISTS automation_rules_system_type_check;
ALTER TABLE automation_rules ADD CONSTRAINT automation_rules_system_type_check
  CHECK (system_type IN ('ebike_sync', 'round_trip', 'frost', 'nacht', 'virtual_sync'));
