-- Migration 000355: gear item JSONB columns + activity_gear_assignments + RLS
--
-- Placeholder: this file keeps the 000355 number until finalize-migration.sh
-- renumbers it at merge time.
--
-- Blocks:
--   A. Reuse + Rename (sport-predicate column consolidation)
--   B. New JSONB containers + auxiliary tables
--   C. 000348 re-implementation (activity_gear_assignments, service intervals,
--      automation rules, analytics recompute)
--
-- Data loss on down-migration: see corresponding .down.sql header.

-- ============================================================
-- Block A: Reuse + Rename
-- ============================================================

-- A1: rename the column so its name matches its purpose.
ALTER TABLE gear RENAME COLUMN default_sport_predicates TO applicable_sport_predicates;

-- A2: drop old constraint and function; the new ones (A3) replace them.
ALTER TABLE gear DROP CONSTRAINT IF EXISTS gear_default_sport_predicates_valid;
DROP FUNCTION IF EXISTS fn_validate_gear_default_predicates(jsonb);

-- A3: recreate the validator under the new name (body identical to 000351,
--     only the symbol name changes).
CREATE OR REPLACE FUNCTION fn_validate_gear_applicable_sport_predicates(p JSONB)
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
    DROP CONSTRAINT IF EXISTS gear_applicable_sport_predicates_valid,
    ADD CONSTRAINT gear_applicable_sport_predicates_valid
        CHECK (fn_validate_gear_applicable_sport_predicates(applicable_sport_predicates)) NOT VALID;
ALTER TABLE gear VALIDATE CONSTRAINT gear_applicable_sport_predicates_valid;

-- A3b: scope column — free TEXT, no DB-level enum or CHECK.
--      Validation lives in geargen + API handler so new scopes need no migration.
ALTER TABLE gear ADD COLUMN IF NOT EXISTS scope TEXT;

-- Backfill scope from type. Covers all keys from config/gear-schema.yaml
-- plus the legacy 'e_bike' type (maps to 'bike' scope).
WITH scope_backfill(type_key, scope) AS (
    VALUES
        ('bike',                       'bike'),
        ('bike.road',                  'bike'),
        ('bike.mtb',                   'bike'),
        ('bike.gravel',                'bike'),
        ('bike.gravel.cyclocross',     'bike'),
        ('bike.city',                  'bike'),
        ('bike.trekking',              'bike'),
        ('bike.cargo',                 'bike'),
        ('bike.tandem',                'bike'),
        ('bike.recumbent',             'bike'),
        ('bike.kids',                  'bike'),
        ('bike.balance',               'bike'),
        ('bike.indoor',                'bike'),
        ('bike.handcycle',             'bike'),
        ('bike.consumable.chain_lube', 'bike'),
        ('bike.consumable.brake_pads', 'bike'),
        ('bike.consumable.tire_sealant','bike'),
        -- legacy type present in production before schema was formalized
        ('e_bike',                     'bike'),
        ('helmet',                     'other'),
        ('goggles',                    'other'),
        ('shoes',                      'shoes'),
        ('shoes.cycling',              'shoes'),
        ('shoes.cycling.gravel',       'shoes'),
        ('shoes.running',              'shoes'),
        ('shoes.hiking',               'shoes'),
        ('shoes.ski',                  'shoes'),
        ('shoes.climbing',             'shoes'),
        ('ski',                        'winter'),
        ('watercraft',                 'water'),
        ('dive_computer',              'diving'),
        ('climbing_harness',           'climb'),
        ('apparel',                    'apparel'),
        ('other',                      'other'),
        -- climb leaf keys
        ('climb.harness',              'climb'),
        ('climb.rope',                 'climb'),
        ('climb.crampons',             'climb'),
        ('climb.ice_axe',              'climb'),
        ('climb.belay_device',         'climb'),
        ('climb.quickdraw',            'climb'),
        ('climb.chalkbag',             'climb'),
        ('climb.crashpad',             'climb'),
        -- apparel leaf keys
        ('apparel.pants',              'apparel'),
        ('apparel.jersey',             'apparel'),
        ('apparel.gloves',             'apparel'),
        ('apparel.socks',              'apparel'),
        ('apparel.jacket',             'apparel'),
        ('apparel.shorts',             'apparel')
)
UPDATE gear g
SET scope = sb.scope
FROM scope_backfill sb
WHERE g.type = sb.type_key;

-- Any type not in the list (unknown/future) defaults to 'other'.
UPDATE gear SET scope = 'other' WHERE scope IS NULL;

ALTER TABLE gear ALTER COLUMN scope SET NOT NULL;

CREATE INDEX IF NOT EXISTS gear_user_scope_idx ON gear (user_id, scope);

-- A4: drop the legacy column (code sweep done in Task 2.0 commit 0398e8419).
ALTER TABLE gear DROP COLUMN IF EXISTS default_for_sports;

-- ============================================================
-- Block B: New JSONB containers
-- ============================================================

-- B1: per-item attribute and modifier bags on gear.
ALTER TABLE gear
    ADD COLUMN IF NOT EXISTS attributes JSONB NOT NULL DEFAULT '{}'::jsonb,
    ADD COLUMN IF NOT EXISTS modifiers  JSONB NOT NULL DEFAULT '{}'::jsonb;

-- B2: attribute bag on gear_components.
ALTER TABLE gear_components
    ADD COLUMN IF NOT EXISTS attributes JSONB NOT NULL DEFAULT '{}'::jsonb;

-- B3: GIN indexes for JSONB filter queries (jsonb_path_ops supports @> operator).
CREATE INDEX IF NOT EXISTS gear_attributes_gin
    ON gear USING GIN (attributes jsonb_path_ops);
CREATE INDEX IF NOT EXISTS gear_modifiers_gin
    ON gear USING GIN (modifiers jsonb_path_ops);
CREATE INDEX IF NOT EXISTS gear_applicable_sport_predicates_gin
    ON gear USING GIN (applicable_sport_predicates jsonb_path_ops);

-- B4: swap history for non-chain components.
CREATE TABLE IF NOT EXISTS gear_component_swap_events (
    id                  UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id             UUID        NOT NULL REFERENCES users(id)           ON DELETE CASCADE,
    gear_id             UUID        NOT NULL REFERENCES gear(id)            ON DELETE CASCADE,
    slot_key            TEXT        NOT NULL,
    retired_component_id UUID       REFERENCES gear_components(id)          ON DELETE SET NULL,
    new_component_id    UUID        REFERENCES gear_components(id)          ON DELETE SET NULL,
    swap_date           TIMESTAMPTZ NOT NULL,
    swap_reason         TEXT,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);

ALTER TABLE gear_component_swap_events ENABLE ROW LEVEL SECURITY;

GRANT SELECT ON gear_component_swap_events TO skadia_readonly;

DROP POLICY IF EXISTS user_isolation_direct ON gear_component_swap_events;
CREATE POLICY user_isolation_direct ON gear_component_swap_events
    FOR SELECT TO skadia_readonly
    USING (user_id = current_setting('app.current_user_id')::uuid);

CREATE OR REPLACE FUNCTION gear_component_swap_events_user_match()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
DECLARE
    gear_owner UUID;
BEGIN
    SELECT user_id INTO gear_owner FROM gear WHERE id = NEW.gear_id;
    IF gear_owner IS NULL THEN
        RAISE EXCEPTION 'gear_component_swap_events_user_match: gear row not found';
    END IF;
    IF NEW.user_id <> gear_owner THEN
        RAISE EXCEPTION 'gear_component_swap_events_user_match: user_id mismatch';
    END IF;
    RETURN NEW;
END;
$$;

DROP TRIGGER IF EXISTS trg_gear_component_swap_events_user_match ON gear_component_swap_events;
CREATE TRIGGER trg_gear_component_swap_events_user_match
BEFORE INSERT OR UPDATE OF user_id, gear_id
ON gear_component_swap_events
FOR EACH ROW
EXECUTE FUNCTION gear_component_swap_events_user_match();

-- B5: idempotency key store for multi-replica-safe gear mutations.
CREATE TABLE IF NOT EXISTS gear_idempotency_keys (
    user_id              UUID        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    route                TEXT        NOT NULL,
    key                  TEXT        NOT NULL,
    response_jsonb       JSONB       NOT NULL,
    response_status_code SMALLINT    NOT NULL DEFAULT 200,
    created_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, route, key)
);

-- Add response_status_code to pre-existing table (idempotent for re-runs on prod DB).
ALTER TABLE gear_idempotency_keys
    ADD COLUMN IF NOT EXISTS response_status_code SMALLINT NOT NULL DEFAULT 200;

CREATE INDEX IF NOT EXISTS gear_idempotency_keys_created_at_idx
    ON gear_idempotency_keys (created_at);

ALTER TABLE gear_idempotency_keys ENABLE ROW LEVEL SECURITY;

GRANT SELECT ON gear_idempotency_keys TO skadia_readonly;

DROP POLICY IF EXISTS user_isolation_direct ON gear_idempotency_keys;
CREATE POLICY user_isolation_direct ON gear_idempotency_keys
    FOR SELECT TO skadia_readonly
    USING (user_id = current_setting('app.current_user_id')::uuid);

-- 24-hour TTL cleanup job; only registered when pg_cron extension is installed.
DO $cron_setup$
BEGIN
    IF EXISTS (SELECT 1 FROM pg_extension WHERE extname = 'pg_cron') THEN
        PERFORM cron.schedule(
            'gear_idemp_gc',
            '0 * * * *',
            'DELETE FROM gear_idempotency_keys WHERE created_at < now() - interval ''24 hours'''
        );
    ELSE
        RAISE NOTICE
            'pg_cron not installed — gear_idempotency_keys TTL cleanup must be run manually: '
            'DELETE FROM gear_idempotency_keys WHERE created_at < now() - interval ''24 hours''';
    END IF;
END;
$cron_setup$;

-- ============================================================
-- Block C: 000348 re-implementation
-- ============================================================

-- activity_gear_assignments table (from 000348 block 1-17, 76-87).
CREATE TABLE IF NOT EXISTS activity_gear_assignments (
    id                     UUID             PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id                UUID             NOT NULL REFERENCES users(id)       ON DELETE CASCADE,
    activity_id            UUID             NOT NULL REFERENCES activities(id)  ON DELETE CASCADE,
    gear_id                UUID             NOT NULL REFERENCES gear(id)        ON DELETE CASCADE,
    role_key               TEXT             NOT NULL DEFAULT 'primary',
    slot_key               TEXT,
    quantity               DOUBLE PRECISION NOT NULL DEFAULT 1,
    source                 TEXT             NOT NULL DEFAULT 'legacy',
    attributes             JSONB            NOT NULL DEFAULT '{}'::jsonb,
    start_offset_s         INTEGER,
    end_offset_s           INTEGER,
    distance_m_override    DOUBLE PRECISION,
    moving_time_s_override BIGINT,
    created_at             TIMESTAMPTZ      NOT NULL DEFAULT now(),
    updated_at             TIMESTAMPTZ      NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_activity_gear_assignments_one_primary
    ON activity_gear_assignments (activity_id)
    WHERE role_key = 'primary';

CREATE INDEX IF NOT EXISTS idx_activity_gear_assignments_activity_gear
    ON activity_gear_assignments (activity_id, gear_id);

CREATE INDEX IF NOT EXISTS idx_activity_gear_assignments_user
    ON activity_gear_assignments (user_id);

CREATE INDEX IF NOT EXISTS idx_activity_gear_assignments_gear
    ON activity_gear_assignments (gear_id);

ALTER TABLE activity_gear_assignments ENABLE ROW LEVEL SECURITY;

GRANT SELECT ON activity_gear_assignments TO skadia_readonly;

DROP POLICY IF EXISTS user_isolation_direct ON activity_gear_assignments;
CREATE POLICY user_isolation_direct ON activity_gear_assignments
    FOR SELECT TO skadia_readonly
    USING (user_id = current_setting('app.current_user_id')::uuid);

CREATE OR REPLACE FUNCTION activity_gear_assignments_user_match()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
DECLARE
    activity_owner_id UUID;
    gear_user UUID;
BEGIN
    SELECT user_id INTO activity_owner_id FROM activities WHERE id = NEW.activity_id;
    SELECT user_id INTO gear_user          FROM gear       WHERE id = NEW.gear_id;
    IF activity_owner_id IS NULL OR gear_user IS NULL THEN
        RAISE EXCEPTION 'activity_gear_assignments_user_match missing referenced row';
    END IF;
    IF NEW.user_id <> activity_owner_id OR NEW.user_id <> gear_user THEN
        RAISE EXCEPTION 'activity_gear_assignments_user_match user_id mismatch';
    END IF;
    RETURN NEW;
END;
$$;

DROP TRIGGER IF EXISTS trg_activity_gear_assignments_user_match ON activity_gear_assignments;
CREATE TRIGGER trg_activity_gear_assignments_user_match
BEFORE INSERT OR UPDATE OF user_id, activity_id, gear_id
ON activity_gear_assignments
FOR EACH ROW
EXECUTE FUNCTION activity_gear_assignments_user_match();

-- gear_schema_definitions: user-custom schema definitions only.
-- System defaults stay in config/gear-schema.yaml + generated schema_generated.go.
-- No is_system column (User-Decision A3).
CREATE TABLE IF NOT EXISTS gear_schema_definitions (
    id         UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id    UUID        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    type_key   TEXT        NOT NULL,
    definition JSONB       NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (user_id, type_key)
);

ALTER TABLE gear_schema_definitions ENABLE ROW LEVEL SECURITY;

GRANT SELECT ON gear_schema_definitions TO skadia_readonly;

DROP POLICY IF EXISTS user_isolation_direct ON gear_schema_definitions;
CREATE POLICY user_isolation_direct ON gear_schema_definitions
    FOR SELECT TO skadia_readonly
    USING (user_id = current_setting('app.current_user_id')::uuid);

-- gear_rule_action_state: deduplication state for automation rules.
-- 1:1 from 000348; no capability columns present → no sub-audit refinement needed.
CREATE TABLE IF NOT EXISTS gear_rule_action_state (
    user_id         UUID        NOT NULL REFERENCES users(id)          ON DELETE CASCADE,
    rule_id         UUID        NOT NULL REFERENCES automation_rules(id) ON DELETE CASCADE,
    gear_id         UUID        NOT NULL REFERENCES gear(id)           ON DELETE CASCADE,
    last_matched_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_signature  TEXT        NOT NULL,
    PRIMARY KEY (rule_id, gear_id),
    CONSTRAINT gear_rule_action_state_user_not_null
        CHECK (user_id IS NOT NULL)
);

CREATE INDEX IF NOT EXISTS idx_gear_rule_action_state_user
    ON gear_rule_action_state (user_id);

ALTER TABLE gear_rule_action_state ENABLE ROW LEVEL SECURITY;

GRANT SELECT ON gear_rule_action_state TO skadia_readonly;

DROP POLICY IF EXISTS user_isolation_direct ON gear_rule_action_state;
CREATE POLICY user_isolation_direct ON gear_rule_action_state
    FOR SELECT TO skadia_readonly
    USING (user_id = current_setting('app.current_user_id')::uuid);

CREATE OR REPLACE FUNCTION gear_rule_action_state_user_match()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
DECLARE
    rule_user UUID;
    gear_user UUID;
BEGIN
    SELECT user_id INTO rule_user FROM automation_rules WHERE id = NEW.rule_id;
    SELECT user_id INTO gear_user FROM gear             WHERE id = NEW.gear_id;
    IF rule_user IS NULL OR gear_user IS NULL THEN
        RAISE EXCEPTION 'gear_rule_action_state_user_match missing referenced row';
    END IF;
    IF NEW.user_id <> rule_user OR NEW.user_id <> gear_user THEN
        RAISE EXCEPTION 'gear_rule_action_state_user_match user_id mismatch';
    END IF;
    RETURN NEW;
END;
$$;

DROP TRIGGER IF EXISTS trg_gear_rule_action_state_user_match ON gear_rule_action_state;
CREATE TRIGGER trg_gear_rule_action_state_user_match
BEFORE INSERT OR UPDATE OF user_id, rule_id, gear_id
ON gear_rule_action_state
FOR EACH ROW
EXECUTE FUNCTION gear_rule_action_state_user_match();

-- 000348 gear analytics columns.
ALTER TABLE gear
    ADD COLUMN IF NOT EXISTS total_elevation_loss_m DOUBLE PRECISION NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS total_moving_time_s    BIGINT           NOT NULL DEFAULT 0;

-- gear_components analytics + classification columns.
-- slot_key links a component to a slot defined in the gear-type schema
-- (gear_types[*].slots in config/gear-schema.yaml). NULL means a free
-- (unslotted) component. The schema handles validation; no DB CHECK so
-- new slots can be added without a migration.
-- preset_gear_type_key optionally locks a component to a specific gear_type_key
-- (or any descendant via gear_type ParentKey chain). When set, it takes
-- precedence over the implicit TypeKey/SourceGearTypeKey transfer logic in
-- gearcompat.CanTransferComponent. Validation lives in Go (schema-aware).
ALTER TABLE gear_components
    ADD COLUMN IF NOT EXISTS total_elevation_loss_m DOUBLE PRECISION NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS total_moving_time_s    BIGINT           NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS total_activities       INTEGER          NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS type_key               TEXT,
    ADD COLUMN IF NOT EXISTS slot_key               TEXT,
    ADD COLUMN IF NOT EXISTS preset_gear_type_key   TEXT;

-- component_assignments per-assignment analytics.
ALTER TABLE component_assignments
    ADD COLUMN IF NOT EXISTS elevation_loss_m  DOUBLE PRECISION NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS moving_time_s     BIGINT           NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS activities_count  INTEGER          NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS attributes        JSONB            NOT NULL DEFAULT '{}'::jsonb;

-- chain_rotation_members per-member analytics.
ALTER TABLE chain_rotation_members
    ADD COLUMN IF NOT EXISTS total_elevation_loss_m DOUBLE PRECISION NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS total_moving_time_s    BIGINT           NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS total_time_s           BIGINT           NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS total_activities       INTEGER          NOT NULL DEFAULT 0;

-- chain_rotation_groups swap-interval refactor.
-- Adds metric_key + value columns; migrates data from the legacy km/hm columns.
-- Sub-audit decision: 1:1 from 000348 (no Modifier-Audience integration needed —
-- the metric_key is a generic measurement axis, not an electric/indoor modifier).
ALTER TABLE chain_rotation_groups
    ADD COLUMN IF NOT EXISTS swap_interval_metric_key TEXT,
    ADD COLUMN IF NOT EXISTS swap_interval_value      DOUBLE PRECISION;

UPDATE chain_rotation_groups
SET swap_interval_metric_key = 'distance_m',
    swap_interval_value      = swap_interval_km * 1000
WHERE swap_interval_metric_key IS NULL
  AND swap_interval_km IS NOT NULL;

UPDATE chain_rotation_groups
SET swap_interval_metric_key = 'elevation_gain_m',
    swap_interval_value      = swap_interval_hm
WHERE swap_interval_metric_key IS NULL
  AND swap_interval_hm IS NOT NULL;

ALTER TABLE chain_rotation_groups
    DROP CONSTRAINT IF EXISTS chain_rotation_groups_swap_interval_metric_key_check;

ALTER TABLE chain_rotation_groups
    ADD CONSTRAINT chain_rotation_groups_swap_interval_metric_key_check
    CHECK (
        swap_interval_metric_key IS NULL
        OR swap_interval_metric_key IN (
            'distance_m', 'moving_time_s', 'elapsed_time_s',
            'elevation_gain_m', 'elevation_loss_m'
        )
    );

-- gear_service_intervals: add metric_key column.
-- Sub-audit decision: 1:1 from 000348 (metric_key values are measurement axes,
-- not related to GearApplicableModifierKeys; no formalization needed yet).
ALTER TABLE gear_service_intervals
    ADD COLUMN IF NOT EXISTS metric_key                 TEXT,
    ADD COLUMN IF NOT EXISTS last_service_metric_value  DOUBLE PRECISION;

ALTER TABLE gear_service_events
    ADD COLUMN IF NOT EXISTS metric_snapshot JSONB NOT NULL DEFAULT '{}'::jsonb;

-- Migrate interval_type values to metric_key.
UPDATE gear_service_intervals
SET metric_key = interval_type
WHERE metric_key IS NULL;

-- Normalize legacy 'time_s' alias.
UPDATE gear_service_intervals
SET metric_key   = 'elapsed_time_s'
WHERE metric_key = 'time_s';

UPDATE gear_service_intervals
SET interval_type = 'elapsed_time_s'
WHERE interval_type = 'time_s';

-- Guard: reject rows with unsupported metric_key before adding the constraint.
DO $$
DECLARE
    bad_count INTEGER;
BEGIN
    SELECT COUNT(*) INTO bad_count
    FROM gear_service_intervals
    WHERE metric_key NOT IN (
        'distance_m', 'moving_time_s', 'elapsed_time_s',
        'activity_count', 'elapsed_days',
        'elevation_gain_m', 'elevation_loss_m'
    );
    IF bad_count > 0 THEN
        RAISE EXCEPTION
            'gear_service_intervals contains % unsupported metric_key values before constraint creation',
            bad_count;
    END IF;
END;
$$;

-- Backfill last_service_metric_value from legacy column where applicable.
UPDATE gear_service_intervals
SET last_service_metric_value = last_service_distance_m
WHERE metric_key = 'distance_m'
  AND last_service_metric_value IS NULL;

ALTER TABLE gear_service_intervals
    ALTER COLUMN metric_key SET NOT NULL;

ALTER TABLE gear_service_intervals
    DROP CONSTRAINT IF EXISTS gear_service_intervals_metric_key_check;

ALTER TABLE gear_service_intervals
    ADD CONSTRAINT gear_service_intervals_metric_key_check
    CHECK (metric_key IN (
        'distance_m', 'moving_time_s', 'elapsed_time_s',
        'activity_count', 'elapsed_days',
        'elevation_gain_m', 'elevation_loss_m'
    ));

-- automation_rules: add on_gear_usage_update trigger event + warning columns.
ALTER TABLE automation_rules
    DROP CONSTRAINT IF EXISTS chk_trigger_event;

ALTER TABLE automation_rules
    ADD CONSTRAINT chk_trigger_event CHECK (
        trigger_event IN (
            'on_import', 'on_enrichment_complete',
            'on_manual_apply', 'on_gear_usage_update'
        )
    );

ALTER TABLE automation_rules
    ADD COLUMN IF NOT EXISTS last_warning_at      TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS last_warning_code    TEXT,
    ADD COLUMN IF NOT EXISTS last_warning_message TEXT;

-- Final analytics recompute: populate gear and gear_components totals from
-- activity_gear_assignments (which may be empty at this point; Phase 10
-- restores from backup, so this is a no-op on first apply and correct after
-- restore).
UPDATE gear g
SET total_distance_m      = src.total_distance_m,
    total_moving_time_s   = src.total_moving_time_s,
    total_time_s          = src.total_time_s,
    total_elevation_gain_m = src.total_elevation_gain_m,
    total_elevation_loss_m = src.total_elevation_loss_m,
    total_activities      = src.total_activities
FROM (
    SELECT
        aga.gear_id,
        COALESCE(SUM(COALESCE(aga.distance_m_override, a.distance_m, 0)), 0)         AS total_distance_m,
        COALESCE(SUM(COALESCE(aga.moving_time_s_override, a.moving_time_s, 0)), 0)   AS total_moving_time_s,
        COALESCE(SUM(COALESCE(a.elapsed_time_s, 0)), 0)                              AS total_time_s,
        COALESCE(SUM(COALESCE(a.elevation_gain_m, 0)), 0)                            AS total_elevation_gain_m,
        COALESCE(SUM(COALESCE(a.elevation_loss_m, 0)), 0)                            AS total_elevation_loss_m,
        COUNT(*)                                                                      AS total_activities
    FROM activity_gear_assignments aga
    JOIN activities a ON a.id = aga.activity_id
    WHERE NOT a.is_group_secondary
    GROUP BY aga.gear_id
) src
WHERE g.id = src.gear_id;

-- ============================================================
-- Final-block: terminate the legacy activities.gear_id column.
-- ============================================================
-- All Go write paths now write activity_gear_assignments directly
-- (Wave-5/Cluster I). The transitional mirror trigger + function
-- created in earlier iterations of this migration are no longer
-- needed; we drop them here together with the column itself.
--
-- Safety net: insert a primary aga row for any activity that still
-- has gear_id set but no primary aga entry. Production already ran
-- the prior block's backfill, so this catches only stragglers
-- introduced between backfill and this block (in development) or
-- legacy environments where the prior block did not run. ON CONFLICT
-- DO NOTHING preserves any existing primary row.
INSERT INTO activity_gear_assignments
    (id, user_id, activity_id, gear_id, role_key, source, quantity)
SELECT
    gen_random_uuid(),
    a.user_id,
    a.id,
    a.gear_id,
    'primary',
    'legacy',
    1
FROM activities a
WHERE a.gear_id IS NOT NULL
  AND NOT EXISTS (
      SELECT 1
      FROM activity_gear_assignments aga
      WHERE aga.activity_id = a.id
        AND aga.role_key    = 'primary'
  );

-- Drop the mirror trigger pair + function; they reference NEW.gear_id
-- which would be invalid once the column is dropped.
DROP TRIGGER IF EXISTS trg_mirror_activity_gear_to_aga        ON activities;
DROP TRIGGER IF EXISTS trg_mirror_activity_gear_to_aga_insert ON activities;
DROP TRIGGER IF EXISTS trg_mirror_activity_gear_to_aga_update ON activities;
DROP FUNCTION IF EXISTS public.fn_mirror_activity_gear_to_aga();

-- Drop the column.
ALTER TABLE activities DROP COLUMN IF EXISTS gear_id;

-- Note on the (former) standalone 000355 migration:
-- An earlier branch revision had a separate 000355_gear_assignment_mirror_trigger
-- migration that was later inlined into this file. There is no reachable
-- "auto-cleanup" we can do for dev DBs in that intermediate state — once a
-- DB has v=355 in schema_migrations, the runner sees v=354 as already
-- applied and does not re-execute this file. Affected developers must
-- rebuild via `./scripts/test_db.sh drop && ./scripts/test_db.sh ensure`.
-- Production was never exposed to that intermediate state.

-- ============================================================
-- Block D: bike_setups component snapshot
-- ============================================================

ALTER TABLE bike_setups
    ADD COLUMN IF NOT EXISTS component_snapshot JSONB;

COMMENT ON COLUMN bike_setups.component_snapshot IS
    'Optional passive snapshot of gear_components at the time the setup was saved.
     Versioned JSONB envelope: {version, captured_at, components: [...]}.
     Reading: Setup activation does NOT mutate live gear_components — snapshot is read-only history.
     Future versions may add service_intervals_snapshot / chain_rotations_snapshot fields.';
