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
BEGIN
    IF TG_OP = 'INSERT' THEN
        -- Direction A: tags TEXT[] → modifier flags (always runs).
        IF NEW.tags IS NOT NULL THEN
            NEW.is_indoor := COALESCE(NEW.is_indoor, false) OR 'Indoor' = ANY(COALESCE(NEW.tags, '{}'));
            NEW.is_commute := COALESCE(NEW.is_commute, false) OR 'Pendeln' = ANY(COALESCE(NEW.tags, '{}'));
            IF NEW.sport_type = 'cycling' THEN
                NEW.is_electric := COALESCE(NEW.is_electric, false) OR 'E-Bike' = ANY(COALESCE(NEW.tags, '{}'));
            ELSIF 'E-Bike' = ANY(COALESCE(NEW.tags, '{}')) THEN
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

    -- UPDATE Direction A: tags TEXT[] changed → flags (always runs).
    IF NEW.tags IS DISTINCT FROM OLD.tags THEN
        NEW.is_indoor := 'Indoor' = ANY(COALESCE(NEW.tags, '{}'));
        NEW.is_commute := 'Pendeln' = ANY(COALESCE(NEW.tags, '{}'));

        IF NEW.sport_type = 'cycling' THEN
            NEW.is_electric := 'E-Bike' = ANY(COALESCE(NEW.tags, '{}'));
        ELSIF 'E-Bike' = ANY(COALESCE(NEW.tags, '{}')) THEN
            -- Non-cycling row carries an 'E-Bike' tag string: strip it. The
            -- accompanying junction/count cleanup is outbound and gated.
            NEW.tags := array_remove(NEW.tags, 'E-Bike');
            IF NOT v_skip_outbound THEN
                SELECT id INTO v_tag_id FROM tags WHERE user_id = NEW.user_id AND name = 'E-Bike' AND is_system = true LIMIT 1;
                IF v_tag_id IS NOT NULL THEN
                    DELETE FROM activity_tags WHERE activity_id = NEW.id AND tag_id = v_tag_id;
                    UPDATE tags SET activity_count = (SELECT COUNT(*) FROM activity_tags WHERE tag_id = v_tag_id) WHERE id = v_tag_id;
                END IF;
            END IF;
        END IF;
    END IF;

    -- Direction B/C/D below is outbound. Skip entirely under the skip flag.
    IF v_skip_outbound THEN
        RETURN NEW;
    END IF;

    -- Direction B: is_indoor → Indoor.
    IF NEW.is_indoor IS DISTINCT FROM OLD.is_indoor THEN
        IF NEW.is_indoor AND NOT ('Indoor' = ANY(COALESCE(NEW.tags, '{}'))) THEN
            NEW.tags := array_append(COALESCE(NEW.tags, '{}'), 'Indoor');
            SELECT id INTO v_tag_id FROM tags WHERE user_id = NEW.user_id AND name = 'Indoor' AND is_system = true LIMIT 1;
            IF v_tag_id IS NOT NULL THEN
                INSERT INTO activity_tags (activity_id, tag_id, assigned_by) VALUES (NEW.id, v_tag_id, 'trigger') ON CONFLICT (activity_id, tag_id) DO NOTHING;
                UPDATE tags SET activity_count = (SELECT COUNT(*) FROM activity_tags WHERE tag_id = v_tag_id) WHERE id = v_tag_id;
            END IF;
        ELSIF NOT NEW.is_indoor AND 'Indoor' = ANY(COALESCE(NEW.tags, '{}')) THEN
            NEW.tags := array_remove(NEW.tags, 'Indoor');
            SELECT id INTO v_tag_id FROM tags WHERE user_id = NEW.user_id AND name = 'Indoor' AND is_system = true LIMIT 1;
            IF v_tag_id IS NOT NULL THEN
                DELETE FROM activity_tags WHERE activity_id = NEW.id AND tag_id = v_tag_id;
                UPDATE tags SET activity_count = (SELECT COUNT(*) FROM activity_tags WHERE tag_id = v_tag_id) WHERE id = v_tag_id;
            END IF;
        END IF;
    END IF;

    -- Direction C: is_electric / sport_type → cycling-only E-Bike.
    IF NEW.is_electric IS DISTINCT FROM OLD.is_electric OR NEW.sport_type IS DISTINCT FROM OLD.sport_type THEN
        SELECT id INTO v_tag_id FROM tags WHERE user_id = NEW.user_id AND name = 'E-Bike' AND is_system = true LIMIT 1;

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
        IF NEW.is_commute AND NOT ('Pendeln' = ANY(COALESCE(NEW.tags, '{}'))) THEN
            NEW.tags := array_append(COALESCE(NEW.tags, '{}'), 'Pendeln');
            SELECT id INTO v_tag_id FROM tags WHERE user_id = NEW.user_id AND name = 'Pendeln' AND is_system = true LIMIT 1;
            IF v_tag_id IS NOT NULL THEN
                INSERT INTO activity_tags (activity_id, tag_id, assigned_by) VALUES (NEW.id, v_tag_id, 'trigger') ON CONFLICT (activity_id, tag_id) DO NOTHING;
                UPDATE tags SET activity_count = (SELECT COUNT(*) FROM activity_tags WHERE tag_id = v_tag_id) WHERE id = v_tag_id;
            END IF;
        ELSIF NOT NEW.is_commute AND 'Pendeln' = ANY(COALESCE(NEW.tags, '{}')) THEN
            NEW.tags := array_remove(NEW.tags, 'Pendeln');
            SELECT id INTO v_tag_id FROM tags WHERE user_id = NEW.user_id AND name = 'Pendeln' AND is_system = true LIMIT 1;
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
BEGIN
    IF v_skip_outbound THEN
        RETURN NULL;
    END IF;

    IF COALESCE(NEW.is_indoor, false) THEN
        SELECT id INTO v_tag_id FROM tags WHERE user_id = NEW.user_id AND name = 'Indoor' AND is_system = true LIMIT 1;
        IF v_tag_id IS NOT NULL THEN
            INSERT INTO activity_tags (activity_id, tag_id, assigned_by) VALUES (NEW.id, v_tag_id, 'trigger') ON CONFLICT (activity_id, tag_id) DO NOTHING;
            UPDATE tags SET activity_count = (SELECT COUNT(*) FROM activity_tags WHERE tag_id = v_tag_id) WHERE id = v_tag_id;
        END IF;
    END IF;

    IF NEW.sport_type = 'cycling' AND COALESCE(NEW.is_electric, false) THEN
        SELECT id INTO v_tag_id FROM tags WHERE user_id = NEW.user_id AND name = 'E-Bike' AND is_system = true LIMIT 1;
        IF v_tag_id IS NOT NULL THEN
            INSERT INTO activity_tags (activity_id, tag_id, assigned_by) VALUES (NEW.id, v_tag_id, 'trigger') ON CONFLICT (activity_id, tag_id) DO NOTHING;
            UPDATE tags SET activity_count = (SELECT COUNT(*) FROM activity_tags WHERE tag_id = v_tag_id) WHERE id = v_tag_id;
        END IF;
    END IF;

    IF COALESCE(NEW.is_commute, false) THEN
        SELECT id INTO v_tag_id FROM tags WHERE user_id = NEW.user_id AND name = 'Pendeln' AND is_system = true LIMIT 1;
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
BEGIN
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
CREATE OR REPLACE FUNCTION skadia_resync_system_modifier_tags(p_activity_ids UUID[])
RETURNS VOID
LANGUAGE plpgsql
SET search_path = public
AS $$
DECLARE
    v_affected_tag_ids UUID[];
BEGIN
    IF p_activity_ids IS NULL OR cardinality(p_activity_ids) = 0 THEN
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
        UPDATE tags
        SET activity_count = (SELECT COUNT(*) FROM activity_tags WHERE tag_id = tags.id)
        WHERE id = ANY(v_affected_tag_ids);
    END IF;
END;
$$;
