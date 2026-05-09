-- 000353_system_modifier_tag_sync_hardening.up.sql
--
-- System modifier tag sync hardening:
--
-- 1. Adds a btree index on activity_tags(tag_id, activity_id) so the resync
--    function's per-tag count recomputes and the trigger junctions/deletes
--    in the cycling-only E-Bike branch run on an indexed plan.
--
-- 2. Replaces sync_system_flags_and_tags(), sync_system_flags_and_tags_insert_
--    junction(), sync_virtual_tag(), sync_virtual_tag_insert() and
--    sync_virtual_tag_insert_junction() with skip-gated bodies. The skip flag
--    is skadia.skip_system_modifier_tag_sync, scoped per-transaction. When
--    set, only OUTBOUND modifier-flag → denormalized-tag work is suppressed:
--    junction inserts/deletes, activities.tags adjustments driven by flag
--    changes, and activity_count writes. INBOUND tags-array → modifier-flag
--    derivation still runs, so a tags-array-driven write keeps deriving
--    flags consistently inside a caller-managed transaction.
--
-- 3. Adds skadia_resync_system_modifier_tags(uuid[]) — a set-based,
--    idempotent repair function for the four registry-backed system tags
--    (Indoor / Pendeln / virtuell / cycling-only E-Bike). It promotes
--    same-user manual tags that already use one of the reserved names,
--    inserts missing system tag rows for users with affected activities,
--    inserts missing junctions BEFORE any deletes (so the virtuell
--    delete-protection trigger never trips), reconciles the
--    activities.tags array, drops stale activity_tag_removals for tags
--    the modifier columns now say should be present, and recomputes
--    activity_count for the four system tags reachable from the affected
--    user set.
--
-- The skip gate exists only so caller-managed transactions can suppress
-- per-row trigger work for known bulk paths AND guarantee a single resync
-- call before commit. Do NOT set the skip flag without calling
-- skadia_resync_system_modifier_tags() afterwards in the same transaction.

SELECT pg_advisory_xact_lock(hashtext('system_modifier_tag_sync_hardening'), 1);

-- 1. Index
CREATE INDEX IF NOT EXISTS idx_activity_tags_tag_id_activity_id
    ON activity_tags (tag_id, activity_id);

-- 2a. Skip-gated sync_system_flags_and_tags()
CREATE OR REPLACE FUNCTION sync_system_flags_and_tags() RETURNS TRIGGER
LANGUAGE plpgsql
SET search_path = public
AS $$
DECLARE
    v_tag_id UUID;
    v_skip_outbound BOOLEAN := COALESCE(current_setting('skadia.skip_system_modifier_tag_sync', true), 'false') = 'true';
    v_rename_in_progress BOOLEAN := COALESCE(current_setting('skadia.tag_rename_in_progress', true), 'false') = 'true';
    -- reserved-name slot guard (Critic #3): compare OLD/NEW membership per slot
    v_indoor_was   BOOLEAN;
    v_indoor_now   BOOLEAN;
    v_commute_was  BOOLEAN;
    v_commute_now  BOOLEAN;
    v_electric_was BOOLEAN;
    v_electric_now BOOLEAN;
    -- Critic #5: deterministic lock ordering — collected before any outbound write
    v_reserved_tag_ids UUID[];
BEGIN
    -- Tag-rename propagation path: a rename only swaps a literal in the
    -- tags array. Modifier semantics are unchanged (the row still means
    -- the same thing, only the label differs), so every branch of this
    -- trigger must short-circuit. Otherwise the slot-guard would mis-fire
    -- on the OLD-literal disappearing.
    IF v_rename_in_progress THEN
        RETURN NEW;
    END IF;

    IF TG_OP = 'INSERT' THEN
        -- Direction A: tags TEXT[] → modifier flags (always runs).
        IF NEW.tags IS NOT NULL THEN
            NEW.is_indoor := COALESCE(NEW.is_indoor, false) OR 'Indoor' = ANY(COALESCE(NEW.tags, '{}'));
            NEW.is_commute := COALESCE(NEW.is_commute, false) OR 'Pendeln' = ANY(COALESCE(NEW.tags, '{}'));
            IF NEW.sport_type = 'cycling' THEN
                NEW.is_electric := COALESCE(NEW.is_electric, false) OR 'E-Bike' = ANY(COALESCE(NEW.tags, '{}'));
            ELSIF 'E-Bike' = ANY(COALESCE(NEW.tags, '{}')) THEN
                -- Non-cycling insert: strip E-Bike from the array unconditionally.
                -- This is a consistency correction on the row being inserted, not
                -- outbound junction/count work, so it is not gated on skip flag.
                NEW.tags := array_remove(NEW.tags, 'E-Bike');
            END IF;
        END IF;

        -- Outbound: flags → tags TEXT[]. Skipped under the skip flag.
        IF NOT v_skip_outbound THEN
            IF COALESCE(NEW.is_indoor, false) AND NOT ('Indoor' = ANY(COALESCE(NEW.tags, '{}'))) THEN
                NEW.tags := array_append(COALESCE(NEW.tags, '{}'), 'Indoor');
            END IF;
            IF COALESCE(NEW.is_commute, false) AND NOT ('Pendeln' = ANY(COALESCE(NEW.tags, '{}'))) THEN
                NEW.tags := array_append(COALESCE(NEW.tags, '{}'), 'Pendeln');
            END IF;
            IF NEW.sport_type = 'cycling' AND COALESCE(NEW.is_electric, false) AND NOT ('E-Bike' = ANY(COALESCE(NEW.tags, '{}'))) THEN
                NEW.tags := array_append(COALESCE(NEW.tags, '{}'), 'E-Bike');
            ELSIF NEW.sport_type <> 'cycling' AND 'E-Bike' = ANY(COALESCE(NEW.tags, '{}')) THEN
                NEW.tags := array_remove(NEW.tags, 'E-Bike');
            END IF;
        END IF;

        RETURN NEW;
    END IF;

    -- UPDATE Direction A (inbound): tags TEXT[] changed → flags (always runs, even
    -- under skip flag). Only re-derives flags when the relevant slot membership
    -- actually changed between OLD and NEW (Critic #3 slot-guard).
    IF NEW.tags IS DISTINCT FROM OLD.tags THEN
        v_indoor_was   := 'Indoor'  = ANY(COALESCE(OLD.tags, '{}'));
        v_indoor_now   := 'Indoor'  = ANY(COALESCE(NEW.tags, '{}'));
        v_commute_was  := 'Pendeln' = ANY(COALESCE(OLD.tags, '{}'));
        v_commute_now  := 'Pendeln' = ANY(COALESCE(NEW.tags, '{}'));
        v_electric_was := 'E-Bike'  = ANY(COALESCE(OLD.tags, '{}'));
        v_electric_now := 'E-Bike'  = ANY(COALESCE(NEW.tags, '{}'));

        IF v_indoor_was IS DISTINCT FROM v_indoor_now THEN
            NEW.is_indoor := v_indoor_now;
        END IF;
        IF v_commute_was IS DISTINCT FROM v_commute_now THEN
            NEW.is_commute := v_commute_now;
        END IF;

        IF NEW.sport_type = 'cycling' AND v_electric_was IS DISTINCT FROM v_electric_now THEN
            NEW.is_electric := v_electric_now;
        END IF;
    END IF;

    -- Outbound work (Directions A-outbound + B + C + D): skip entirely when the
    -- skip flag is set. Callers that set the flag must call
    -- skadia_resync_system_modifier_tags() before commit to reconcile state.
    IF v_skip_outbound THEN
        RETURN NEW;
    END IF;

    -- Critic #5: acquire row locks on all reserved system tag rows for this user
    -- in primary-key order before any outbound write. Two concurrent paths that
    -- touch different subsets of these tags (e.g. one flips Indoor then E-Bike,
    -- another does the inverse) will both acquire locks in the same order (sorted
    -- by UUID), preventing lock-cycle deadlocks.
    SELECT array_agg(id ORDER BY id) INTO v_reserved_tag_ids
    FROM tags
    WHERE user_id = NEW.user_id
      AND name IN ('Indoor', 'Pendeln', 'virtuell', 'E-Bike')
      AND is_system = true;

    IF v_reserved_tag_ids IS NOT NULL AND cardinality(v_reserved_tag_ids) > 0 THEN
        PERFORM 1 FROM tags WHERE id = ANY(v_reserved_tag_ids) ORDER BY id FOR UPDATE;
    END IF;

    -- Direction A (outbound portion): non-cycling E-Bike strip. When a tags-array
    -- UPDATE carries 'E-Bike' on a non-cycling activity, strip both the array entry
    -- and the junction row. Gating both on NOT v_skip_outbound (the skip-return
    -- above) ensures mid-tx reads stay self-consistent (Critic #13).
    IF NEW.tags IS DISTINCT FROM OLD.tags
       AND NEW.sport_type <> 'cycling'
       AND 'E-Bike' = ANY(COALESCE(NEW.tags, '{}'))
    THEN
        NEW.tags := array_remove(NEW.tags, 'E-Bike');
        SELECT id INTO v_tag_id FROM tags WHERE id = ANY(v_reserved_tag_ids) AND name = 'E-Bike';
        IF v_tag_id IS NOT NULL THEN
            DELETE FROM activity_tags WHERE activity_id = NEW.id AND tag_id = v_tag_id;
            UPDATE tags SET activity_count = (SELECT COUNT(*) FROM activity_tags WHERE tag_id = v_tag_id) WHERE id = v_tag_id;
        END IF;
    END IF;

    -- Direction B: is_indoor → Indoor.
    IF NEW.is_indoor IS DISTINCT FROM OLD.is_indoor THEN
        SELECT id INTO v_tag_id FROM tags WHERE id = ANY(v_reserved_tag_ids) AND name = 'Indoor';
        IF NEW.is_indoor AND NOT ('Indoor' = ANY(COALESCE(NEW.tags, '{}'))) THEN
            NEW.tags := array_append(COALESCE(NEW.tags, '{}'), 'Indoor');
            IF v_tag_id IS NOT NULL THEN
                INSERT INTO activity_tags (activity_id, tag_id, assigned_by) VALUES (NEW.id, v_tag_id, 'trigger') ON CONFLICT (activity_id, tag_id) DO NOTHING;
                UPDATE tags SET activity_count = (SELECT COUNT(*) FROM activity_tags WHERE tag_id = v_tag_id) WHERE id = v_tag_id;
            END IF;
        ELSIF NOT NEW.is_indoor AND 'Indoor' = ANY(COALESCE(NEW.tags, '{}')) THEN
            NEW.tags := array_remove(NEW.tags, 'Indoor');
            IF v_tag_id IS NOT NULL THEN
                DELETE FROM activity_tags WHERE activity_id = NEW.id AND tag_id = v_tag_id;
                UPDATE tags SET activity_count = (SELECT COUNT(*) FROM activity_tags WHERE tag_id = v_tag_id) WHERE id = v_tag_id;
            END IF;
        END IF;
    END IF;

    -- Direction C: is_electric / sport_type → cycling-only E-Bike.
    IF NEW.is_electric IS DISTINCT FROM OLD.is_electric OR NEW.sport_type IS DISTINCT FROM OLD.sport_type THEN
        SELECT id INTO v_tag_id FROM tags WHERE id = ANY(v_reserved_tag_ids) AND name = 'E-Bike';

        IF v_tag_id IS NOT NULL AND NEW.sport_type = 'cycling' AND NEW.is_electric AND NOT ('E-Bike' = ANY(COALESCE(NEW.tags, '{}'))) THEN
            NEW.tags := array_append(COALESCE(NEW.tags, '{}'), 'E-Bike');
            INSERT INTO activity_tags (activity_id, tag_id, assigned_by) VALUES (NEW.id, v_tag_id, 'trigger') ON CONFLICT (activity_id, tag_id) DO NOTHING;
            UPDATE tags SET activity_count = (SELECT COUNT(*) FROM activity_tags WHERE tag_id = v_tag_id) WHERE id = v_tag_id;
        ELSIF (NEW.sport_type <> 'cycling' OR NOT NEW.is_electric) AND 'E-Bike' = ANY(COALESCE(NEW.tags, '{}')) THEN
            NEW.tags := array_remove(NEW.tags, 'E-Bike');
            IF v_tag_id IS NOT NULL THEN
                DELETE FROM activity_tags WHERE activity_id = NEW.id AND tag_id = v_tag_id;
                UPDATE tags SET activity_count = (SELECT COUNT(*) FROM activity_tags WHERE tag_id = v_tag_id) WHERE id = v_tag_id;
            END IF;
        ELSIF v_tag_id IS NOT NULL AND NEW.sport_type <> 'cycling' AND OLD.sport_type = 'cycling' THEN
            DELETE FROM activity_tags WHERE activity_id = NEW.id AND tag_id = v_tag_id;
            UPDATE tags SET activity_count = (SELECT COUNT(*) FROM activity_tags WHERE tag_id = v_tag_id) WHERE id = v_tag_id;
        END IF;
    END IF;

    -- Direction D: is_commute → Pendeln.
    IF NEW.is_commute IS DISTINCT FROM OLD.is_commute THEN
        SELECT id INTO v_tag_id FROM tags WHERE id = ANY(v_reserved_tag_ids) AND name = 'Pendeln';
        IF NEW.is_commute AND NOT ('Pendeln' = ANY(COALESCE(NEW.tags, '{}'))) THEN
            NEW.tags := array_append(COALESCE(NEW.tags, '{}'), 'Pendeln');
            IF v_tag_id IS NOT NULL THEN
                INSERT INTO activity_tags (activity_id, tag_id, assigned_by) VALUES (NEW.id, v_tag_id, 'trigger') ON CONFLICT (activity_id, tag_id) DO NOTHING;
                UPDATE tags SET activity_count = (SELECT COUNT(*) FROM activity_tags WHERE tag_id = v_tag_id) WHERE id = v_tag_id;
            END IF;
        ELSIF NOT NEW.is_commute AND 'Pendeln' = ANY(COALESCE(NEW.tags, '{}')) THEN
            NEW.tags := array_remove(NEW.tags, 'Pendeln');
            IF v_tag_id IS NOT NULL THEN
                DELETE FROM activity_tags WHERE activity_id = NEW.id AND tag_id = v_tag_id;
                UPDATE tags SET activity_count = (SELECT COUNT(*) FROM activity_tags WHERE tag_id = v_tag_id) WHERE id = v_tag_id;
            END IF;
        END IF;
    END IF;

    RETURN NEW;
END;
$$;

-- 2b. Skip-gated sync_system_flags_and_tags_insert_junction()
CREATE OR REPLACE FUNCTION sync_system_flags_and_tags_insert_junction() RETURNS TRIGGER
LANGUAGE plpgsql
SET search_path = public
AS $$
DECLARE
    v_tag_id UUID;
    v_skip_outbound BOOLEAN := COALESCE(current_setting('skadia.skip_system_modifier_tag_sync', true), 'false') = 'true';
    -- Critic #5: deterministic lock ordering
    v_reserved_tag_ids UUID[];
BEGIN
    IF v_skip_outbound THEN
        RETURN NULL;
    END IF;

    -- Critic #5: lock all reserved system tag rows for this user in PK order
    -- before any write so concurrent INSERT paths cannot deadlock.
    SELECT array_agg(id ORDER BY id) INTO v_reserved_tag_ids
    FROM tags
    WHERE user_id = NEW.user_id
      AND name IN ('Indoor', 'Pendeln', 'virtuell', 'E-Bike')
      AND is_system = true;

    IF v_reserved_tag_ids IS NOT NULL AND cardinality(v_reserved_tag_ids) > 0 THEN
        PERFORM 1 FROM tags WHERE id = ANY(v_reserved_tag_ids) ORDER BY id FOR UPDATE;
    END IF;

    IF COALESCE(NEW.is_indoor, false) THEN
        SELECT id INTO v_tag_id FROM tags WHERE id = ANY(v_reserved_tag_ids) AND name = 'Indoor';
        IF v_tag_id IS NOT NULL THEN
            INSERT INTO activity_tags (activity_id, tag_id, assigned_by) VALUES (NEW.id, v_tag_id, 'trigger') ON CONFLICT (activity_id, tag_id) DO NOTHING;
            UPDATE tags SET activity_count = (SELECT COUNT(*) FROM activity_tags WHERE tag_id = v_tag_id) WHERE id = v_tag_id;
        END IF;
    END IF;

    IF NEW.sport_type = 'cycling' AND COALESCE(NEW.is_electric, false) THEN
        SELECT id INTO v_tag_id FROM tags WHERE id = ANY(v_reserved_tag_ids) AND name = 'E-Bike';
        IF v_tag_id IS NOT NULL THEN
            INSERT INTO activity_tags (activity_id, tag_id, assigned_by) VALUES (NEW.id, v_tag_id, 'trigger') ON CONFLICT (activity_id, tag_id) DO NOTHING;
            UPDATE tags SET activity_count = (SELECT COUNT(*) FROM activity_tags WHERE tag_id = v_tag_id) WHERE id = v_tag_id;
        END IF;
    END IF;

    IF COALESCE(NEW.is_commute, false) THEN
        SELECT id INTO v_tag_id FROM tags WHERE id = ANY(v_reserved_tag_ids) AND name = 'Pendeln';
        IF v_tag_id IS NOT NULL THEN
            INSERT INTO activity_tags (activity_id, tag_id, assigned_by) VALUES (NEW.id, v_tag_id, 'trigger') ON CONFLICT (activity_id, tag_id) DO NOTHING;
            UPDATE tags SET activity_count = (SELECT COUNT(*) FROM activity_tags WHERE tag_id = v_tag_id) WHERE id = v_tag_id;
        END IF;
    END IF;

    RETURN NULL;
END;
$$;

-- 2c. Skip-gated sync_virtual_tag()
CREATE OR REPLACE FUNCTION sync_virtual_tag() RETURNS TRIGGER
LANGUAGE plpgsql
SET search_path = public
AS $$
DECLARE
    v_tag_id UUID;
    v_skip_outbound BOOLEAN := COALESCE(current_setting('skadia.skip_system_modifier_tag_sync', true), 'false') = 'true';
    v_rename_in_progress BOOLEAN := COALESCE(current_setting('skadia.tag_rename_in_progress', true), 'false') = 'true';
BEGIN
    IF v_rename_in_progress THEN
        RETURN NEW;
    END IF;

    IF NEW.is_virtual IS DISTINCT FROM OLD.is_virtual THEN
        IF NEW.is_virtual THEN
            IF NOT v_skip_outbound THEN
                IF NOT ('virtuell' = ANY(COALESCE(NEW.tags, '{}'))) THEN
                    NEW.tags := array_append(COALESCE(NEW.tags, '{}'), 'virtuell');
                END IF;
                SELECT id INTO v_tag_id FROM tags WHERE user_id = NEW.user_id AND name = 'virtuell';
                IF v_tag_id IS NOT NULL THEN
                    INSERT INTO activity_tags (activity_id, tag_id, assigned_by)
                    VALUES (NEW.id, v_tag_id, 'system')
                    ON CONFLICT (activity_id, tag_id) DO NOTHING;
                END IF;
            END IF;
        ELSE
            IF NOT v_skip_outbound THEN
                NEW.tags := array_remove(NEW.tags, 'virtuell');
                SELECT id INTO v_tag_id FROM tags WHERE user_id = NEW.user_id AND name = 'virtuell';
                IF v_tag_id IS NOT NULL THEN
                    DELETE FROM activity_tags WHERE activity_id = NEW.id AND tag_id = v_tag_id;
                END IF;
            END IF;
        END IF;
    END IF;
    RETURN NEW;
END;
$$;

-- 2d. Skip-gated sync_virtual_tag_insert()
CREATE OR REPLACE FUNCTION sync_virtual_tag_insert() RETURNS TRIGGER
LANGUAGE plpgsql
SET search_path = public
AS $$
DECLARE
    v_skip_outbound BOOLEAN := COALESCE(current_setting('skadia.skip_system_modifier_tag_sync', true), 'false') = 'true';
BEGIN
    IF NEW.is_virtual AND NOT v_skip_outbound THEN
        IF NOT ('virtuell' = ANY(COALESCE(NEW.tags, '{}'))) THEN
            NEW.tags := array_append(COALESCE(NEW.tags, '{}'), 'virtuell');
        END IF;
    END IF;
    RETURN NEW;
END;
$$;

-- 2e. Skip-gated sync_virtual_tag_insert_junction()
CREATE OR REPLACE FUNCTION sync_virtual_tag_insert_junction() RETURNS TRIGGER
LANGUAGE plpgsql
SET search_path = public
AS $$
DECLARE
    v_tag_id UUID;
    v_skip_outbound BOOLEAN := COALESCE(current_setting('skadia.skip_system_modifier_tag_sync', true), 'false') = 'true';
BEGIN
    IF v_skip_outbound THEN
        RETURN NULL;
    END IF;
    IF NEW.is_virtual THEN
        SELECT id INTO v_tag_id FROM tags WHERE user_id = NEW.user_id AND name = 'virtuell';
        IF v_tag_id IS NOT NULL THEN
            INSERT INTO activity_tags (activity_id, tag_id, assigned_by)
            VALUES (NEW.id, v_tag_id, 'system')
            ON CONFLICT (activity_id, tag_id) DO NOTHING;
        END IF;
    END IF;
    RETURN NULL;
END;
$$;

-- 3. Resync function: set-based repair for the four registry-backed system
--    tags. Operations are bounded by p_activity_ids and scoped to each
--    activity's user_id; rows from other users are never touched.
--
--    Returns a single row with repair counts for observability:
--      junctions_inserted  — activity_tags rows added
--      junctions_deleted   — activity_tags rows removed
--      tags_promoted       — manual tag rows promoted to system shape
--      removals_dropped    — activity_tag_removals rows dropped
--      system_tags_seeded  — system tag rows created for users that lacked them
CREATE OR REPLACE FUNCTION skadia_resync_system_modifier_tags(p_activity_ids UUID[])
RETURNS TABLE(junctions_inserted INT, junctions_deleted INT, tags_promoted INT, removals_dropped INT, system_tags_seeded INT)
LANGUAGE plpgsql
SET search_path = public
AS $$
DECLARE
    v_affected_tag_ids UUID[];
    v_row_count        INT;
    v_junctions_inserted  INT := 0;
    v_junctions_deleted   INT := 0;
    v_tags_promoted       INT := 0;
    v_removals_dropped    INT := 0;
    v_system_tags_seeded  INT := 0;
BEGIN
    IF p_activity_ids IS NULL OR cardinality(p_activity_ids) = 0 THEN
        RETURN QUERY SELECT 0, 0, 0, 0, 0;
        RETURN;
    END IF;

    -- 1. Promote pre-existing same-user manual tags with reserved system
    --    names to the system shape. tags is unique on (user_id, name), so
    --    same-user duplicates cannot exist.
    UPDATE tags t
    SET is_system = true,
        category = 'system',
        is_auto = false
    FROM activities a
    WHERE a.id = ANY(p_activity_ids)
      AND t.user_id = a.user_id
      AND t.name IN ('Indoor', 'Pendeln', 'virtuell', 'E-Bike')
      AND t.is_system = false;
    GET DIAGNOSTICS v_row_count = ROW_COUNT;
    v_tags_promoted := v_row_count;

    -- 1b. Insert missing system tags for every user owning an affected
    --     activity. ON CONFLICT skips users whose tag was just promoted or
    --     was already system-shaped.
    INSERT INTO tags (user_id, name, color, text_color, category, is_system, is_auto, description)
    SELECT DISTINCT a.user_id, n.name, n.color, n.text_color, 'system', true, false, n.description
    FROM activities a
    CROSS JOIN (VALUES
        ('Indoor',   '#3B82F6', '#FFFFFF', 'Automatisch bei Indoor-Aktivitäten'),
        ('Pendeln',  '#6B7280', '#FFFFFF', 'Automatisch bei Pendel-Aktivitäten'),
        ('virtuell', '#8B5CF6', '#FFFFFF', 'Automatisch bei virtuellen Aktivitäten'),
        ('E-Bike',   '#16A34A', '#FFFFFF', 'Automatisch bei elektrischen Radfahrten')
    ) AS n(name, color, text_color, description)
    WHERE a.id = ANY(p_activity_ids)
    ON CONFLICT (user_id, name) DO NOTHING;
    GET DIAGNOSTICS v_row_count = ROW_COUNT;
    v_system_tags_seeded := v_row_count;

    -- 2a. Insert missing junctions FIRST (before any deletes) so the
    --     virtuell delete-protection trigger never sees a missing row
    --     mid-resync.
    INSERT INTO activity_tags (activity_id, tag_id, assigned_by)
    SELECT a.id, t.id, 'system'
    FROM activities a
    JOIN tags t ON t.user_id = a.user_id AND t.is_system = true
    WHERE a.id = ANY(p_activity_ids)
      AND (
          (t.name = 'Indoor'   AND COALESCE(a.is_indoor,   false))
          OR (t.name = 'Pendeln'  AND COALESCE(a.is_commute,  false))
          OR (t.name = 'virtuell' AND COALESCE(a.is_virtual,  false))
          OR (t.name = 'E-Bike'   AND a.sport_type = 'cycling' AND COALESCE(a.is_electric, false))
      )
    ON CONFLICT (activity_id, tag_id) DO NOTHING;
    GET DIAGNOSTICS v_row_count = ROW_COUNT;
    v_junctions_inserted := v_row_count;

    -- 2b. Reconcile activities.tags TEXT[] with modifier flags. Reserved
    --     system names are stripped from the array, then re-added based on
    --     flag truth.
    UPDATE activities a
    SET tags = (
        SELECT COALESCE(array_agg(DISTINCT x ORDER BY x), ARRAY[]::text[])
        FROM unnest(
            COALESCE(
                array_remove(
                    array_remove(
                        array_remove(
                            array_remove(COALESCE(a.tags, ARRAY[]::text[]), 'Indoor'),
                            'Pendeln'
                        ),
                        'virtuell'
                    ),
                    'E-Bike'
                ),
                ARRAY[]::text[]
            )
            ||
            CASE WHEN COALESCE(a.is_indoor,  false) THEN ARRAY['Indoor']::text[]   ELSE ARRAY[]::text[] END
            ||
            CASE WHEN COALESCE(a.is_commute, false) THEN ARRAY['Pendeln']::text[]  ELSE ARRAY[]::text[] END
            ||
            CASE WHEN COALESCE(a.is_virtual, false) THEN ARRAY['virtuell']::text[] ELSE ARRAY[]::text[] END
            ||
            CASE WHEN a.sport_type = 'cycling' AND COALESCE(a.is_electric, false)
                 THEN ARRAY['E-Bike']::text[] ELSE ARRAY[]::text[] END
        ) AS x
    )
    WHERE a.id = ANY(p_activity_ids);

    -- 2c. Delete junctions whose modifier flag now says the tag must NOT be
    --     attached. virtuell is only deleted when is_virtual=false, which
    --     never trips the prevent_virtual_tag_removal trigger.
    DELETE FROM activity_tags at
    USING activities a, tags t
    WHERE at.activity_id = a.id
      AND at.tag_id = t.id
      AND a.id = ANY(p_activity_ids)
      AND t.is_system = true
      AND (
          (t.name = 'Indoor'   AND NOT COALESCE(a.is_indoor,   false))
          OR (t.name = 'Pendeln'  AND NOT COALESCE(a.is_commute,  false))
          OR (t.name = 'virtuell' AND NOT COALESCE(a.is_virtual,  false))
          OR (t.name = 'E-Bike'   AND (a.sport_type <> 'cycling' OR NOT COALESCE(a.is_electric, false)))
      );
    GET DIAGNOSTICS v_row_count = ROW_COUNT;
    v_junctions_deleted := v_row_count;

    -- 3. Drop stale activity_tag_removals for system tags whose modifier
    --    flag now says the tag should be present. Otherwise the removal row
    --    silently blocks future trigger inserts.
    DELETE FROM activity_tag_removals atr
    USING activities a, tags t
    WHERE atr.activity_id = a.id
      AND atr.tag_id = t.id
      AND a.id = ANY(p_activity_ids)
      AND t.is_system = true
      AND (
          (t.name = 'Indoor'   AND COALESCE(a.is_indoor,   false))
          OR (t.name = 'Pendeln'  AND COALESCE(a.is_commute,  false))
          OR (t.name = 'virtuell' AND COALESCE(a.is_virtual,  false))
          OR (t.name = 'E-Bike'   AND a.sport_type = 'cycling' AND COALESCE(a.is_electric, false))
      );
    GET DIAGNOSTICS v_row_count = ROW_COUNT;
    v_removals_dropped := v_row_count;

    -- 4. Recompute activity_count for the four reserved system tags
    --    reachable from the affected user set. Tags belonging to other
    --    users, and non-reserved tags belonging to the same user, are not
    --    touched.
    SELECT array_agg(DISTINCT t.id) INTO v_affected_tag_ids
    FROM activities a
    JOIN tags t ON t.user_id = a.user_id
                 AND t.is_system = true
                 AND t.name IN ('Indoor', 'Pendeln', 'virtuell', 'E-Bike')
    WHERE a.id = ANY(p_activity_ids);

    IF v_affected_tag_ids IS NOT NULL AND cardinality(v_affected_tag_ids) > 0 THEN
        -- deterministic lock fence (Critic #12): ORDER BY id ensures two concurrent
        -- helper transactions acquire row locks in the same order, preventing deadlocks
        -- when both touch the same tag IDs from different activity subsets.
        PERFORM 1 FROM tags WHERE id = ANY(v_affected_tag_ids) ORDER BY id FOR UPDATE;

        UPDATE tags
        SET activity_count = (SELECT COUNT(*) FROM activity_tags WHERE tag_id = tags.id)
        WHERE id = ANY(v_affected_tag_ids);
    END IF;

    RETURN QUERY SELECT v_junctions_inserted, v_junctions_deleted, v_tags_promoted, v_removals_dropped, v_system_tags_seeded;
END;
$$;

-- 4. Skip-gated AFTER DELETE trigger on activity_tags. A direct DELETE
--    against a registry-backed reserved-name junction (admin SQL, or any
--    code path that forgets the paired UPDATE on activities) would leave
--    the modifier column stale. The trigger flips the modifier column to
--    false, strips the literal from activities.tags, and recomputes the
--    tag's activity_count.
--
--    The activities UPDATE re-fires sync_system_flags_and_tags BEFORE
--    UPDATE; that path's outbound work is a no-op because the tags array
--    no longer contains the reserved name (we already removed it) and
--    the junction is already gone (idempotent DELETE).
--
--    Skip-flag (skadia.skip_system_modifier_tag_sync) suppresses the
--    trigger for caller-managed bulk paths that own reconciliation via
--    skadia_resync_system_modifier_tags.
CREATE OR REPLACE FUNCTION sync_system_modifier_tags_on_junction_delete() RETURNS TRIGGER
LANGUAGE plpgsql
SET search_path = public
AS $$
DECLARE
    v_skip_outbound BOOLEAN := COALESCE(current_setting('skadia.skip_system_modifier_tag_sync', true), 'false') = 'true';
    v_tag_name TEXT;
    v_is_system BOOLEAN;
    v_user_id UUID;
BEGIN
    IF v_skip_outbound THEN
        RETURN NULL;
    END IF;

    -- Resolve the tag the deleted junction pointed at. AFTER DELETE on
    -- activity_tags fires before any cascaded DELETE on tags commits, so
    -- the tag row is still readable for direct DELETEs. For CASCADE-from-
    -- tags-delete, the tag row is gone or is_system=false; either way the
    -- gate below skips reconciliation, which is the contract.
    SELECT t.name, t.is_system, t.user_id
      INTO v_tag_name, v_is_system, v_user_id
      FROM tags t WHERE t.id = OLD.tag_id;

    IF v_tag_name IS NULL OR NOT COALESCE(v_is_system, false) THEN
        RETURN NULL;
    END IF;

    IF v_tag_name = 'Indoor' THEN
        UPDATE activities
        SET is_indoor = false,
            tags = array_remove(COALESCE(tags, ARRAY[]::text[]), 'Indoor')
        WHERE id = OLD.activity_id
          AND user_id = v_user_id;
    ELSIF v_tag_name = 'Pendeln' THEN
        UPDATE activities
        SET is_commute = false,
            tags = array_remove(COALESCE(tags, ARRAY[]::text[]), 'Pendeln')
        WHERE id = OLD.activity_id
          AND user_id = v_user_id;
    ELSIF v_tag_name = 'virtuell' THEN
        UPDATE activities
        SET is_virtual = false,
            tags = array_remove(COALESCE(tags, ARRAY[]::text[]), 'virtuell')
        WHERE id = OLD.activity_id
          AND user_id = v_user_id;
    ELSIF v_tag_name = 'E-Bike' THEN
        -- Cycling-only contract: is_electric is a generic flag preserved on
        -- non-cycling activities even when an orphan E-Bike junction is
        -- removed (per 000350 design). On cycling activities, flip the flag
        -- to false to keep tag and flag in sync.
        UPDATE activities
        SET is_electric = CASE WHEN sport_type = 'cycling' THEN false ELSE is_electric END,
            tags = array_remove(COALESCE(tags, ARRAY[]::text[]), 'E-Bike')
        WHERE id = OLD.activity_id
          AND user_id = v_user_id;
    ELSE
        -- Non-reserved system tag (none exist today, but defensive) — no
        -- modifier column to flip.
        RETURN NULL;
    END IF;

    -- activity_count is recomputed by the BEFORE UPDATE trigger
    -- (sync_system_flags_and_tags Direction B) that fires on the activities
    -- UPDATE above. A second UPDATE here would be redundant.

    RETURN NULL;
END;
$$;

DROP TRIGGER IF EXISTS trg_sync_system_modifier_tags_on_junction_delete ON activity_tags;
CREATE TRIGGER trg_sync_system_modifier_tags_on_junction_delete
    AFTER DELETE ON activity_tags
    FOR EACH ROW
    EXECUTE FUNCTION sync_system_modifier_tags_on_junction_delete();

-- 5. Tag-rename propagation. UPDATE tags SET name = 'NewName' WHERE id = ...
--    does not propagate into activities.tags TEXT[] arrays — junctions track
--    tag_id (correct), but the denormalised array carries the OLD literal
--    that no longer matches anything. Add an AFTER UPDATE OF (name) trigger
--    that array_replaces the OLD name with the NEW name across every
--    activity owned by the same user.
--
--    Narrow scope: system tags only (OLD.is_system = true OR NEW.is_system =
--    true). The non-system tag rename gap is documented as a follow-up — it
--    is a UX-driven decision (does the user expect the array to follow the
--    rename, or is the array a snapshot?) that belongs in a separate phase.
--
--    The trigger does NOT touch modifier columns. Modifier sync is bound to
--    tag_id semantics, not to the literal in the tags array, so a rename is
--    a no-op for modifier flags.
--
--    Skip-flag (skadia.skip_system_modifier_tag_sync) suppresses the
--    propagation for caller-managed bulk paths that own reconciliation via
--    skadia_resync_system_modifier_tags.
--
--    FOR EACH ROW (not statement-level) so a multi-row UPDATE renaming
--    several tags in one statement still propagates per-row correctly.
CREATE OR REPLACE FUNCTION sync_system_tag_rename_into_activities_tags() RETURNS TRIGGER
LANGUAGE plpgsql
SET search_path = public
AS $$
DECLARE
    v_skip_outbound BOOLEAN := COALESCE(current_setting('skadia.skip_system_modifier_tag_sync', true), 'false') = 'true';
BEGIN
    IF v_skip_outbound THEN
        RETURN NULL;
    END IF;

    -- Narrow scope: only fire when at least one side of the rename is a
    -- system tag. Non-system tag renames are intentionally out of scope.
    IF NOT (COALESCE(OLD.is_system, false) OR COALESCE(NEW.is_system, false)) THEN
        RETURN NULL;
    END IF;

    -- No-op if the name did not actually change (defensive — the trigger is
    -- already gated on UPDATE OF name, but PG fires that on any UPDATE that
    -- targets the column even if the value is unchanged).
    IF OLD.name IS NOT DISTINCT FROM NEW.name THEN
        RETURN NULL;
    END IF;

    -- The activities UPDATE re-fires sync_system_flags_and_tags BEFORE
    -- UPDATE. Direction A (inbound tags-array → modifier flag slot guard)
    -- would see the OLD reserved literal disappear from the array and flip
    -- the modifier column to false; the outbound branches would also fire
    -- spuriously. A rename is a no-op for modifier semantics — the row's
    -- meaning has not changed, only the label — so suppress all trigger
    -- branches for the duration of the propagation UPDATE via a dedicated
    -- in-progress gate. The dedicated gate is separate from the skip flag
    -- because the skip flag intentionally keeps Direction A inbound active
    -- (a tags-array-driven write under that flag must still derive flags).
    PERFORM set_config('skadia.tag_rename_in_progress', 'true', true);
    UPDATE activities
    SET tags = array_replace(tags, OLD.name, NEW.name)
    WHERE user_id = NEW.user_id
      AND OLD.name = ANY(tags);
    PERFORM set_config('skadia.tag_rename_in_progress', 'false', true);

    RETURN NULL;
END;
$$;

DROP TRIGGER IF EXISTS trg_sync_system_tag_rename_into_activities_tags ON tags;
CREATE TRIGGER trg_sync_system_tag_rename_into_activities_tags
    AFTER UPDATE OF name ON tags
    FOR EACH ROW
    EXECUTE FUNCTION sync_system_tag_rename_into_activities_tags();
