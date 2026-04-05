-- 000287_geofence_consolidation.down.sql
-- WARNUNG: Lossy Rollback — Conditions mit check:"end"/check:"track" werden zu check:"start" degradiert.

CREATE OR REPLACE FUNCTION _rollback_geofence_condition(cond jsonb) RETURNS jsonb AS $$
DECLARE
  val jsonb := cond->'value';
  shape text;
BEGIN
  IF cond->>'field' != 'geofence' THEN RETURN cond; END IF;
  shape := COALESCE(val->>'shape', 'circle');
  val := val - 'check';
  IF shape = 'polygon' THEN
    -- geofence_zone hatte shape-Feld — beibehalten, nur check entfernen
    RETURN jsonb_build_object('field', 'geofence_zone', 'op', 'eq', 'value', val, 'geofence_zone', val);
  ELSE
    val := val - 'shape'; -- Legacy geofence hatte kein shape-Feld
    RETURN jsonb_build_object('field', 'geofence', 'op', 'eq', 'value', val, 'geofence', val);
  END IF;
END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE FUNCTION _rollback_geofence_conditions(conditions jsonb) RETURNS jsonb AS $$
  SELECT COALESCE(jsonb_agg(_rollback_geofence_condition(elem)), '[]'::jsonb)
  FROM jsonb_array_elements(conditions) AS elem;
$$ LANGUAGE sql;

CREATE OR REPLACE FUNCTION _rollback_geofence_groups(groups jsonb) RETURNS jsonb AS $$
  SELECT COALESCE(jsonb_agg(
    jsonb_build_object('logic', g->>'logic', 'conditions', _rollback_geofence_conditions(g->'conditions'))
  ), '[]'::jsonb)
  FROM jsonb_array_elements(groups) AS g;
$$ LANGUAGE sql;

CREATE OR REPLACE FUNCTION _rollback_geofence_branches(branches jsonb) RETURNS jsonb AS $$
  SELECT COALESCE(jsonb_agg(
    jsonb_build_object('type', b->>'type', 'condition_groups', _rollback_geofence_groups(b->'condition_groups'), 'actions', b->'actions')
  ), '[]'::jsonb)
  FROM jsonb_array_elements(branches) AS b;
$$ LANGUAGE sql;

UPDATE automation_rules SET
  conditions = _rollback_geofence_conditions(conditions),
  condition_groups = CASE WHEN condition_groups IS NOT NULL THEN _rollback_geofence_groups(condition_groups) ELSE condition_groups END,
  branches = CASE WHEN branches IS NOT NULL THEN _rollback_geofence_branches(branches) ELSE branches END
WHERE conditions::text ~ '"geofence"'
   OR (condition_groups IS NOT NULL AND condition_groups::text ~ '"geofence"')
   OR (branches IS NOT NULL AND branches::text ~ '"geofence"');

DROP FUNCTION _rollback_geofence_branches(jsonb);
DROP FUNCTION _rollback_geofence_groups(jsonb);
DROP FUNCTION _rollback_geofence_conditions(jsonb);
DROP FUNCTION _rollback_geofence_condition(jsonb);
