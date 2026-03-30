-- ══════════════════════════════════════════════════════════════════════
-- Step 1: Create system tag "Indoor" for all users
-- ══════════════════════════════════════════════════════════════════════
INSERT INTO tags (id, user_id, name, color, text_color, is_system, is_auto, description)
SELECT gen_random_uuid(), u.id, 'Indoor', '#3B82F6', '#FFFFFF', true, false,
       'Automatisch bei Indoor-Aktivitäten (Sync mit is_indoor)'
FROM users u
WHERE NOT EXISTS (
    SELECT 1 FROM tags t
    WHERE t.user_id = u.id AND t.name = 'Indoor' AND t.is_system = true
);

-- ══════════════════════════════════════════════════════════════════════
-- Step 2: Recompute is_indoor from taxonomy (fix false positives from !HasGPS)
-- ══════════════════════════════════════════════════════════════════════
UPDATE activities SET is_indoor = false;

UPDATE activities SET is_indoor = true
WHERE sport_type IN (
    'training', 'fitness_equipment', 'strength_training', 'indoor_cycling',
    'lap_swimming', 'yoga', 'pilates', 'treadmill_running', 'hiit', 'dance',
    'jump_rope', 'meditation', 'boxing', 'mixed_martial_arts', 'floor_climbing'
)
   OR sub_type LIKE 'indoor_%'
   OR sub_type IN ('treadmill', 'spin', 'lap_swimming');

-- ══════════════════════════════════════════════════════════════════════
-- Step 3: Backfill — assign Indoor tag to corrected is_indoor=true activities
-- ══════════════════════════════════════════════════════════════════════
INSERT INTO activity_tags (activity_id, tag_id, assigned_by)
SELECT a.id, t.id, 'migration'
FROM activities a
JOIN tags t ON t.user_id = a.user_id AND t.name = 'Indoor' AND t.is_system = true
WHERE a.is_indoor = true
ON CONFLICT DO NOTHING;

-- Step 3b: Update activity_count on Indoor tags
UPDATE tags SET activity_count = (
    SELECT COUNT(*) FROM activity_tags at2 WHERE at2.tag_id = tags.id
)
WHERE name = 'Indoor' AND is_system = true;

-- Step 3c: Add 'Indoor' to tags TEXT[] array
UPDATE activities SET tags = array_append(tags, 'Indoor')
WHERE is_indoor = true AND NOT ('Indoor' = ANY(COALESCE(tags, '{}')));

-- ══════════════════════════════════════════════════════════════════════
-- Step 4: Replace simple Electric trigger with combined bidirectional trigger
--         Handles: tags↔is_indoor AND tags↔is_electric (both directions)
-- ══════════════════════════════════════════════════════════════════════
DROP TRIGGER IF EXISTS trg_sync_electric_from_tags ON activities;
DROP FUNCTION IF EXISTS sync_electric_from_tags();

CREATE OR REPLACE FUNCTION sync_system_flags_and_tags() RETURNS TRIGGER AS $$
DECLARE
    v_tag_id UUID;
BEGIN
    -- ── Direction A: tags TEXT[] changed → derive boolean flags ──
    -- NOTE: When tags and is_indoor/is_electric are set simultaneously in one UPDATE,
    -- Direction A runs first and may override the direct flag value. This is intentional:
    -- explicit tag changes are authoritative. Application code should not set both
    -- in a single statement with conflicting intent.
    IF NEW.tags IS DISTINCT FROM OLD.tags THEN
        NEW.is_indoor := 'Indoor' = ANY(COALESCE(NEW.tags, '{}'));
        NEW.is_electric := 'E-Bike' = ANY(COALESCE(NEW.tags, '{}'));
    END IF;

    -- ── Direction B: is_indoor changed directly → sync tag + junction ──
    IF NEW.is_indoor IS DISTINCT FROM OLD.is_indoor THEN
        IF NEW.is_indoor AND NOT ('Indoor' = ANY(COALESCE(NEW.tags, '{}'))) THEN
            NEW.tags := array_append(COALESCE(NEW.tags, '{}'), 'Indoor');
            SELECT id INTO v_tag_id FROM tags
                WHERE user_id = NEW.user_id AND name = 'Indoor' AND is_system = true LIMIT 1;
            IF v_tag_id IS NOT NULL THEN
                INSERT INTO activity_tags (activity_id, tag_id, assigned_by)
                    VALUES (NEW.id, v_tag_id, 'trigger') ON CONFLICT DO NOTHING;
                UPDATE tags SET activity_count = activity_count + 1 WHERE id = v_tag_id;
            END IF;
        ELSIF NOT NEW.is_indoor AND 'Indoor' = ANY(COALESCE(NEW.tags, '{}')) THEN
            NEW.tags := array_remove(NEW.tags, 'Indoor');
            SELECT id INTO v_tag_id FROM tags
                WHERE user_id = NEW.user_id AND name = 'Indoor' AND is_system = true LIMIT 1;
            IF v_tag_id IS NOT NULL THEN
                DELETE FROM activity_tags WHERE activity_id = NEW.id AND tag_id = v_tag_id;
                UPDATE tags SET activity_count = GREATEST(activity_count - 1, 0) WHERE id = v_tag_id;
            END IF;
        END IF;
    END IF;

    -- ── Direction C: is_electric changed directly → sync tag + junction ──
    IF NEW.is_electric IS DISTINCT FROM OLD.is_electric THEN
        IF NEW.is_electric AND NOT ('E-Bike' = ANY(COALESCE(NEW.tags, '{}'))) THEN
            NEW.tags := array_append(COALESCE(NEW.tags, '{}'), 'E-Bike');
            SELECT id INTO v_tag_id FROM tags
                WHERE user_id = NEW.user_id AND name = 'E-Bike' AND is_system = true LIMIT 1;
            IF v_tag_id IS NOT NULL THEN
                INSERT INTO activity_tags (activity_id, tag_id, assigned_by)
                    VALUES (NEW.id, v_tag_id, 'trigger') ON CONFLICT DO NOTHING;
                UPDATE tags SET activity_count = activity_count + 1 WHERE id = v_tag_id;
            END IF;
        ELSIF NOT NEW.is_electric AND 'E-Bike' = ANY(COALESCE(NEW.tags, '{}')) THEN
            NEW.tags := array_remove(NEW.tags, 'E-Bike');
            SELECT id INTO v_tag_id FROM tags
                WHERE user_id = NEW.user_id AND name = 'E-Bike' AND is_system = true LIMIT 1;
            IF v_tag_id IS NOT NULL THEN
                DELETE FROM activity_tags WHERE activity_id = NEW.id AND tag_id = v_tag_id;
                UPDATE tags SET activity_count = GREATEST(activity_count - 1, 0) WHERE id = v_tag_id;
            END IF;
        END IF;
    END IF;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_sync_system_flags_and_tags
    BEFORE UPDATE OF tags, is_indoor, is_electric ON activities
    FOR EACH ROW
    EXECUTE FUNCTION sync_system_flags_and_tags();

-- ══════════════════════════════════════════════════════════════════════
-- Step 5: Create system rule (skipped by rules engine)
-- ══════════════════════════════════════════════════════════════════════
INSERT INTO automation_rules (id, user_id, name, is_active, system_type, priority, conditions, conditions_logic, actions)
SELECT gen_random_uuid(), u.id, 'Indoor-Sync', true, 'indoor_sync', -100, '[]'::jsonb, 'and', '[]'::jsonb
FROM users u
WHERE NOT EXISTS (
    SELECT 1 FROM automation_rules r
    WHERE r.user_id = u.id AND r.system_type = 'indoor_sync'
);
