-- Down migration for 000355: gear item JSONB + activity_gear_assignments.
--
-- DATA LOSS WARNING:
--   - gear.scope, gear.attributes, gear.modifiers are dropped (no recovery).
--   - gear_components.type_key, .slot_key, .attributes, .total_* are dropped.
--   - All rows in activity_gear_assignments are dropped (151 rows in production
--     as of 2026-05-08; restore from
--     ~/skadia-backups/20260508-pre-down-348/activity_gear_assignments_data.sql
--     after re-applying the up migration in Phase 10).
--   - gear_schema_definitions, gear_rule_action_state, gear_component_swap_events,
--     gear_idempotency_keys are dropped with all their data.
--   - chain_rotation_groups.swap_interval_metric_key / _value are dropped;
--     where possible the legacy km/hm columns are repopulated from those values
--     before they are dropped (distance_m → km, elevation_gain_m → hm); other
--     metric_keys cannot round-trip into km/hm and are lost.
--   - gear_service_intervals.metric_key / last_service_metric_value are dropped;
--     interval_type is reverted from 'elapsed_time_s' to 'time_s' to match the
--     pre-up state.
--   - automation_rules.last_warning_* columns are dropped.
--   - automation_rules.chk_trigger_event is narrowed back (on_gear_usage_update
--     removed); any existing rows with that trigger_event are deleted first.
--
-- Destructive rollback: intended only for development branch cleanup.

-- ============================================================
-- Block D (LIFO step 0): remove bike_setups component snapshot.
-- ============================================================
ALTER TABLE bike_setups DROP COLUMN IF EXISTS component_snapshot;

-- ============================================================
-- LIFO step 1: undo the final-block of the up migration.
-- Recreates activities.gear_id, repopulates from the primary aga
-- row (so on-disk state matches what the column would have held),
-- then drops the gear_id column again — this restores the
-- pre-final-block schema before the rest of the down proceeds.
-- The mirror trigger + function are NOT recreated; legacy state
-- with the trigger is no longer reproducible since both are
-- removed in the new up flow.
-- ============================================================
ALTER TABLE activities ADD COLUMN IF NOT EXISTS gear_id UUID
    REFERENCES gear(id) ON DELETE SET NULL;

-- Backfill activities.gear_id from the primary aga row. Any role_key='primary'
-- row qualifies; this is the inverse of the up.sql safety-net backfill.
UPDATE activities a
   SET gear_id = aga.gear_id
  FROM activity_gear_assignments aga
 WHERE aga.activity_id = a.id
   AND aga.role_key    = 'primary'
   AND a.gear_id IS NULL;

-- ============================================================
-- Mirror trigger teardown (no-op on fresh down: trigger was dropped in up).
-- aga rows created by the trigger or backfill are dropped later when
-- the activity_gear_assignments table itself is dropped in Block C.
-- ============================================================
DROP TRIGGER IF EXISTS trg_mirror_activity_gear_to_aga        ON activities;
DROP TRIGGER IF EXISTS trg_mirror_activity_gear_to_aga_insert ON activities;
DROP TRIGGER IF EXISTS trg_mirror_activity_gear_to_aga_update ON activities;
DROP FUNCTION IF EXISTS public.fn_mirror_activity_gear_to_aga();

-- ============================================================
-- Unregister pg_cron job if present
-- ============================================================
DO $cron_teardown$
BEGIN
    IF EXISTS (SELECT 1 FROM pg_extension WHERE extname = 'pg_cron') THEN
        PERFORM cron.unschedule('gear_idemp_gc');
    END IF;
END;
$cron_teardown$;

-- ============================================================
-- Block C: remove 000348 re-implementation
-- ============================================================

-- Remove rows with on_gear_usage_update before narrowing the constraint.
DELETE FROM automation_rules WHERE trigger_event = 'on_gear_usage_update';

ALTER TABLE automation_rules
    DROP CONSTRAINT IF EXISTS chk_trigger_event;

ALTER TABLE automation_rules
    ADD CONSTRAINT chk_trigger_event CHECK (
        trigger_event IN ('on_import', 'on_enrichment_complete', 'on_manual_apply')
    );

ALTER TABLE automation_rules
    DROP COLUMN IF EXISTS last_warning_at,
    DROP COLUMN IF EXISTS last_warning_code,
    DROP COLUMN IF EXISTS last_warning_message;

-- Inverse of the up.sql normalization: revert 'elapsed_time_s' rows to the
-- legacy 'time_s' alias before dropping metric_key.
UPDATE gear_service_intervals
SET interval_type = 'time_s'
WHERE interval_type = 'elapsed_time_s';

ALTER TABLE gear_service_intervals
    DROP CONSTRAINT IF EXISTS gear_service_intervals_metric_key_check;

ALTER TABLE gear_service_intervals
    DROP COLUMN IF EXISTS metric_key,
    DROP COLUMN IF EXISTS last_service_metric_value;

ALTER TABLE gear_service_events
    DROP COLUMN IF EXISTS metric_snapshot;

-- Inverse of the up.sql swap_interval refactor: restore legacy km/hm columns
-- from metric_key/value where possible. distance_m → km, elevation_gain_m → hm.
-- Other metric_keys (moving_time_s, elapsed_time_s, elevation_loss_m) cannot
-- round-trip into km/hm and are lost on rollback.
UPDATE chain_rotation_groups
SET swap_interval_km = swap_interval_value / 1000.0
WHERE swap_interval_metric_key = 'distance_m'
  AND swap_interval_value      IS NOT NULL
  AND swap_interval_km         IS NULL;

UPDATE chain_rotation_groups
SET swap_interval_hm = swap_interval_value
WHERE swap_interval_metric_key = 'elevation_gain_m'
  AND swap_interval_value      IS NOT NULL
  AND swap_interval_hm         IS NULL;

ALTER TABLE chain_rotation_groups
    DROP CONSTRAINT IF EXISTS chain_rotation_groups_swap_interval_metric_key_check;

ALTER TABLE chain_rotation_groups
    DROP COLUMN IF EXISTS swap_interval_metric_key,
    DROP COLUMN IF EXISTS swap_interval_value;

ALTER TABLE chain_rotation_members
    DROP COLUMN IF EXISTS total_elevation_loss_m,
    DROP COLUMN IF EXISTS total_moving_time_s,
    DROP COLUMN IF EXISTS total_time_s,
    DROP COLUMN IF EXISTS total_activities;

ALTER TABLE component_assignments
    DROP COLUMN IF EXISTS elevation_loss_m,
    DROP COLUMN IF EXISTS moving_time_s,
    DROP COLUMN IF EXISTS activities_count,
    DROP COLUMN IF EXISTS attributes;

ALTER TABLE gear_components
    DROP COLUMN IF EXISTS preset_gear_type_key,
    DROP COLUMN IF EXISTS total_elevation_loss_m,
    DROP COLUMN IF EXISTS total_moving_time_s,
    DROP COLUMN IF EXISTS total_activities,
    DROP COLUMN IF EXISTS type_key,
    DROP COLUMN IF EXISTS slot_key;

ALTER TABLE gear
    DROP COLUMN IF EXISTS total_elevation_loss_m,
    DROP COLUMN IF EXISTS total_moving_time_s;

DROP TABLE IF EXISTS gear_rule_action_state;
DROP FUNCTION IF EXISTS gear_rule_action_state_user_match();

DROP TABLE IF EXISTS gear_schema_definitions;

DROP TABLE IF EXISTS activity_gear_assignments;
DROP FUNCTION IF EXISTS activity_gear_assignments_user_match();

-- ============================================================
-- Block B: remove new JSONB containers and auxiliary tables
-- ============================================================

ALTER TABLE gear_idempotency_keys
    DROP COLUMN IF EXISTS response_status_code;

DROP TABLE IF EXISTS gear_idempotency_keys;

DROP TABLE IF EXISTS gear_component_swap_events;
DROP FUNCTION IF EXISTS gear_component_swap_events_user_match();

DROP INDEX IF EXISTS gear_applicable_sport_predicates_gin;
DROP INDEX IF EXISTS gear_modifiers_gin;
DROP INDEX IF EXISTS gear_attributes_gin;

ALTER TABLE gear_components
    DROP COLUMN IF EXISTS attributes;

ALTER TABLE gear
    DROP COLUMN IF EXISTS modifiers,
    DROP COLUMN IF EXISTS attributes;

-- ============================================================
-- Block A: reverse scope + predicate rename
-- ============================================================

DROP INDEX IF EXISTS gear_user_scope_idx;

ALTER TABLE gear
    DROP COLUMN IF EXISTS scope;

-- Restore legacy default_for_sports column (empty — data was never stored here
-- after the 000351 backfill; new rows since 000351 used default_sport_predicates).
ALTER TABLE gear
    ADD COLUMN IF NOT EXISTS default_for_sports TEXT[];

-- Recreate old validator function (body from migration 000351).
CREATE OR REPLACE FUNCTION fn_validate_gear_default_predicates(p JSONB)
RETURNS BOOLEAN
LANGUAGE sql
IMMUTABLE
SET search_path = public
AS $$
WITH items AS (
  SELECT value AS predicate
  FROM jsonb_array_elements(CASE WHEN jsonb_typeof(p) = 'array' THEN p ELSE '[]'::jsonb END)
),
modifier_items AS (
  SELECT predicate, modifier.key, modifier.value
  FROM items
  CROSS JOIN LATERAL jsonb_each(COALESCE(predicate->'modifiers', '{}'::jsonb)) AS modifier(key, value)
  WHERE jsonb_typeof(predicate) = 'object'
)
SELECT
  COALESCE(jsonb_typeof(p) = 'array', false)
  AND NOT EXISTS (
    SELECT 1
    FROM items
    WHERE jsonb_typeof(predicate) <> 'object'
       OR EXISTS (
            SELECT 1
            FROM jsonb_object_keys(predicate) AS pred_keys(key)
            WHERE pred_keys.key NOT IN ('sport_type', 'sub_type', 'modifiers')
       )
       OR NOT (predicate ? 'sport_type')
       OR jsonb_typeof(predicate->'sport_type') <> 'string'
       OR predicate->>'sport_type' = 'unknown'
       OR (
            predicate ? 'sub_type'
            AND jsonb_typeof(predicate->'sub_type') <> 'string'
       )
       OR (
            predicate ? 'modifiers'
            AND jsonb_typeof(predicate->'modifiers') <> 'object'
       )
       OR NOT fn_validate_predicates(jsonb_build_array(jsonb_strip_nulls(jsonb_build_object(
            'sport',
            predicate->>'sport_type',
            'sub_type',
            predicate->>'sub_type'
       ))))
  )
  AND NOT EXISTS (
    SELECT 1
    FROM modifier_items
    WHERE key NOT IN ('is_electric', 'is_indoor', 'is_virtual')
       OR jsonb_typeof(value) <> 'boolean'
  );
$$;

ALTER TABLE gear
    DROP CONSTRAINT IF EXISTS gear_applicable_sport_predicates_valid;

DROP FUNCTION IF EXISTS fn_validate_gear_applicable_sport_predicates(jsonb);

-- Rename column back.
ALTER TABLE gear RENAME COLUMN applicable_sport_predicates TO default_sport_predicates;

-- Restore original CHECK constraint.
ALTER TABLE gear
    DROP CONSTRAINT IF EXISTS gear_default_sport_predicates_valid,
    ADD CONSTRAINT gear_default_sport_predicates_valid
        CHECK (fn_validate_gear_default_predicates(default_sport_predicates)) NOT VALID;
ALTER TABLE gear VALIDATE CONSTRAINT gear_default_sport_predicates_valid;
