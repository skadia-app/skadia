-- 000287_geofence_consolidation.up.sql
-- Geofence-Konsolidierung: 3 Typen → 1 einheitliches Schema

CREATE OR REPLACE FUNCTION _migrate_geofence_condition(cond jsonb) RETURNS jsonb AS $$
DECLARE
  field text := cond->>'field';
  old_val jsonb;
  new_val jsonb;
BEGIN
  IF field = 'geofence' THEN
    old_val := cond->'geofence';
    IF old_val IS NULL THEN old_val := cond->'value'; END IF;
    IF old_val IS NULL OR old_val = 'null'::jsonb THEN RETURN cond; END IF;
    new_val := old_val || '{"check":"start","shape":"circle"}'::jsonb;
    RETURN jsonb_build_object('field', 'geofence', 'op', 'eq', 'value', new_val);
  ELSIF field = 'geofence_proximity' THEN
    old_val := cond->'geofence_proximity';
    IF old_val IS NULL THEN old_val := cond->'value'; END IF;
    IF old_val IS NULL OR old_val = 'null'::jsonb THEN RETURN cond; END IF;
    new_val := old_val || '{"check":"start","shape":"circle"}'::jsonb;
    RETURN jsonb_build_object('field', 'geofence', 'op', 'eq', 'value', new_val);
  ELSIF field = 'geofence_zone' THEN
    old_val := cond->'geofence_zone';
    IF old_val IS NULL THEN old_val := cond->'value'; END IF;
    IF old_val IS NULL OR old_val = 'null'::jsonb THEN RETURN cond; END IF;
    new_val := old_val || '{"check":"start"}'::jsonb;
    RETURN jsonb_build_object('field', 'geofence', 'op', 'eq', 'value', new_val);
  ELSE
    RETURN cond;
  END IF;
END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE FUNCTION _migrate_geofence_conditions(conditions jsonb) RETURNS jsonb AS $$
  SELECT COALESCE(jsonb_agg(_migrate_geofence_condition(elem)), '[]'::jsonb)
  FROM jsonb_array_elements(conditions) AS elem;
$$ LANGUAGE sql;

CREATE OR REPLACE FUNCTION _migrate_geofence_groups(groups jsonb) RETURNS jsonb AS $$
  SELECT COALESCE(jsonb_agg(
    jsonb_build_object(
      'logic', g->>'logic',
      'conditions', _migrate_geofence_conditions(g->'conditions')
    )
  ), '[]'::jsonb)
  FROM jsonb_array_elements(groups) AS g;
$$ LANGUAGE sql;

CREATE OR REPLACE FUNCTION _migrate_geofence_branches(branches jsonb) RETURNS jsonb AS $$
  SELECT COALESCE(jsonb_agg(
    jsonb_build_object(
      'type', b->>'type',
      'condition_groups', _migrate_geofence_groups(b->'condition_groups'),
      'actions', b->'actions'
    )
  ), '[]'::jsonb)
  FROM jsonb_array_elements(branches) AS b;
$$ LANGUAGE sql;

UPDATE automation_rules SET
  conditions = _migrate_geofence_conditions(conditions),
  condition_groups = CASE
    WHEN condition_groups IS NOT NULL THEN _migrate_geofence_groups(condition_groups)
    ELSE condition_groups
  END,
  branches = CASE
    WHEN branches IS NOT NULL THEN _migrate_geofence_branches(branches)
    ELSE branches
  END
WHERE conditions::text ~ 'geofence'
   OR (condition_groups IS NOT NULL AND condition_groups::text ~ 'geofence')
   OR (branches IS NOT NULL AND branches::text ~ 'geofence');

-- Validierung
DO $$
DECLARE cnt integer;
BEGIN
  SELECT COUNT(*) INTO cnt FROM automation_rules
  WHERE conditions::text ~ '"geofence_zone"'
     OR conditions::text ~ '"geofence_proximity"'
     OR (condition_groups IS NOT NULL AND (condition_groups::text ~ '"geofence_zone"' OR condition_groups::text ~ '"geofence_proximity"'))
     OR (branches IS NOT NULL AND (branches::text ~ '"geofence_zone"' OR branches::text ~ '"geofence_proximity"'));
  IF cnt > 0 THEN
    RAISE EXCEPTION 'Migration failed: % rules still contain old geofence types', cnt;
  END IF;
END $$;

DROP FUNCTION _migrate_geofence_branches(jsonb);
DROP FUNCTION _migrate_geofence_groups(jsonb);
DROP FUNCTION _migrate_geofence_conditions(jsonb);
DROP FUNCTION _migrate_geofence_condition(jsonb);
