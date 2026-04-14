-- Correct the tag spelling back to "Intervalltraining" (no hyphen).
-- Migration 000298 renamed in the wrong direction; the Duden-correct compound
-- is the fused form "Intervalltraining" (Intervall + Training), matching what
-- the enrichment code originally used before the accidental unification.
-- The seeded tag from migration 000270 was the odd one out.
--
-- Same merge logic as 000298 but reversed:
--   1. Both tags exist → migrate activity_tags onto "Intervalltraining", drop other
--   2. Only "Intervall-Training" exists → rename in place
--   3. Fix activities.tags denormalized array

DO $$
DECLARE
    u_id TEXT;
    hyphen_tag_id TEXT;
    correct_tag_id TEXT;
BEGIN
    FOR u_id, hyphen_tag_id IN
        SELECT user_id::text, id::text FROM tags WHERE name = 'Intervall-Training'
    LOOP
        SELECT id::text INTO correct_tag_id
        FROM tags
        WHERE user_id::text = u_id AND name = 'Intervalltraining'
        LIMIT 1;

        IF correct_tag_id IS NULL THEN
            UPDATE tags SET name = 'Intervalltraining' WHERE id::text = hyphen_tag_id;
        ELSE
            INSERT INTO activity_tags (activity_id, tag_id, assigned_by)
            SELECT at.activity_id, correct_tag_id::uuid, at.assigned_by
            FROM activity_tags at
            WHERE at.tag_id::text = hyphen_tag_id
            ON CONFLICT (activity_id, tag_id) DO NOTHING;

            DELETE FROM activity_tags WHERE tag_id::text = hyphen_tag_id;
            DELETE FROM tags WHERE id::text = hyphen_tag_id;
        END IF;

        UPDATE activities
        SET tags = array_remove(tags, 'Intervall-Training')
        WHERE user_id::text = u_id
          AND tags @> ARRAY['Intervall-Training']
          AND tags @> ARRAY['Intervalltraining'];

        UPDATE activities
        SET tags = array_replace(tags, 'Intervall-Training', 'Intervalltraining')
        WHERE user_id::text = u_id
          AND tags @> ARRAY['Intervall-Training'];
    END LOOP;

    -- Also fix any automation_rules rows that still name the hyphenated tag.
    UPDATE automation_rules
    SET name = 'Intervalltraining'
    WHERE name = 'Intervall-Training';
END $$;
