-- WARNUNG — HISTORISCH: Diese Migration wurde in die falsche Richtung ausgeführt.
-- Sie vereinigt auf die hyphenierte Form "Intervall-Training", aber der
-- Duden-korrekte Kompositum ist die fusionierte Form "Intervalltraining".
-- Migration 000299 korrigiert das Ergebnis. Diese Migration darf NICHT einzeln
-- zurückgerollt werden, ohne 000299 ebenfalls zurückzurollen — ein isolierter
-- Down von 000298 würde auf einer DB laufen, in der 299 bereits auf
-- "Intervalltraining" vereinheitlicht hat, und erneut eine inkonsistente
-- Mischung erzeugen.
-- Netto-Effekt 298+299 = nahezu No-Op. Neu aufgesetzte Installationen laufen
-- beide Migrations der Reihe nach durch; der Aufwand ist klein genug, um die
-- Geschichte aus Audit-Gründen intakt zu lassen, statt die Migration
-- nachträglich zu entfernen.
--
-- Unify "Intervalltraining" (without hyphen, created by enrichment code) with
-- "Intervall-Training" (with hyphen, created by seeded automation rule in
-- migration 000270). Both tags existed in parallel because the interval
-- enrichment in services/api/internal/enrichment/intervals.go upsert-ed its
-- own tag while the automation rule for `has_structured_intervals=true`
-- fired independently. Result: activities ended up carrying both tags.
--
-- Strategy per user:
--   1. If both tags exist → merge activity_tags rows onto the hyphenated tag,
--      preserving assigned_by when inserting new links, then delete the old
--      tag (activity_tags rows for it cascade).
--   2. If only the non-hyphenated tag exists → rename it in place.
--   3. Fix activities.tags array: replace or remove 'Intervalltraining' so
--      that only 'Intervall-Training' remains (avoid duplicates in array).

DO $$
DECLARE
    u_id TEXT;
    old_tag_id TEXT;
    new_tag_id TEXT;
BEGIN
    FOR u_id, old_tag_id IN
        SELECT user_id::text, id::text FROM tags WHERE name = 'Intervalltraining'
    LOOP
        SELECT id::text INTO new_tag_id
        FROM tags
        WHERE user_id::text = u_id AND name = 'Intervall-Training'
        LIMIT 1;

        IF new_tag_id IS NULL THEN
            -- No hyphenated counterpart: rename in place and we're done.
            UPDATE tags SET name = 'Intervall-Training' WHERE id::text = old_tag_id;
        ELSE
            -- Both tags exist: migrate links onto the hyphenated tag, then delete old tag.
            INSERT INTO activity_tags (activity_id, tag_id, assigned_by)
            SELECT at.activity_id, new_tag_id::uuid, at.assigned_by
            FROM activity_tags at
            WHERE at.tag_id::text = old_tag_id
            ON CONFLICT (activity_id, tag_id) DO NOTHING;

            DELETE FROM activity_tags WHERE tag_id::text = old_tag_id;
            DELETE FROM tags WHERE id::text = old_tag_id;
        END IF;

        -- Fix activities.tags array (denormalized cache) for this user's activities:
        --   A) activities that have both 'Intervalltraining' and 'Intervall-Training'
        --      → just remove the non-hyphenated one
        UPDATE activities
        SET tags = array_remove(tags, 'Intervalltraining')
        WHERE user_id::text = u_id
          AND tags @> ARRAY['Intervalltraining']
          AND tags @> ARRAY['Intervall-Training'];

        --   B) activities that have only 'Intervalltraining' (no hyphen)
        --      → replace the entry with the hyphenated form
        UPDATE activities
        SET tags = array_replace(tags, 'Intervalltraining', 'Intervall-Training')
        WHERE user_id::text = u_id
          AND tags @> ARRAY['Intervalltraining'];
    END LOOP;
END $$;
